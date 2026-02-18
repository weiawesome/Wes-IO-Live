package service

import (
	"context"
	"fmt"
	"time"

	"github.com/weiawesome/wes-io-live/chat-service/internal/audit"
	"github.com/weiawesome/wes-io-live/chat-service/internal/client"
	"github.com/weiawesome/wes-io-live/chat-service/internal/domain"
	"github.com/weiawesome/wes-io-live/chat-service/internal/hub"
	"github.com/weiawesome/wes-io-live/chat-service/internal/kafka"
	"github.com/weiawesome/wes-io-live/chat-service/internal/registry"
	"github.com/weiawesome/wes-io-live/pkg/log"
)

type chatService struct {
	hub        *hub.Hub
	authClient *client.AuthClient
	idClient   *client.IDClient
	producer   kafka.MessageProducer
	registry   registry.Registry
}

func NewChatService(
	h *hub.Hub,
	authClient *client.AuthClient,
	idClient *client.IDClient,
	producer kafka.MessageProducer,
	reg registry.Registry,
) ChatService {
	return &chatService{
		hub:        h,
		authClient: authClient,
		idClient:   idClient,
		producer:   producer,
		registry:   reg,
	}
}

func (s *chatService) HandleAuth(ctx context.Context, c *hub.Client, token string) error {
	result, err := s.authClient.ValidateToken(ctx, token)
	if err != nil {
		c.SendMessage(&domain.AuthResultMessage{
			Type:    domain.MsgTypeAuthResult,
			Success: false,
			Message: "Authentication service unavailable",
		})
		return err
	}

	if !result.Valid {
		audit.LogWithDetail(ctx, audit.ActionAuthFailed, "", result.Error, "auth failed: invalid token")
		c.SendMessage(&domain.AuthResultMessage{
			Type:    domain.MsgTypeAuthResult,
			Success: false,
			Message: result.Error,
		})
		return fmt.Errorf("invalid token: %s", result.Error)
	}

	c.Session.Authenticate(result.UserID, result.Username, result.Email, result.Roles)
	audit.Log(ctx, audit.ActionAuth, result.UserID, "user authenticated")

	return c.SendMessage(&domain.AuthResultMessage{
		Type:     domain.MsgTypeAuthResult,
		Success:  true,
		UserID:   result.UserID,
		Username: result.Username,
	})
}

func (s *chatService) HandleJoinRoom(ctx context.Context, c *hub.Client, roomID, sessionID string) error {
	if !c.Session.IsAuthenticated() {
		return c.SendMessage(domain.NewErrorMessage(domain.ErrCodeUnauthorized, "Not authenticated"))
	}

	// Leave current room if any
	if c.Session.IsInRoom() {
		s.handleLeaveInternal(ctx, c)
	}

	// Join the room-session
	s.hub.JoinRoomSession(c, roomID, sessionID)
	c.Session.JoinRoom(roomID, sessionID)

	// Register in Redis
	if err := s.registry.Register(ctx, roomID, sessionID); err != nil {
		l := log.Ctx(ctx)
		l.Error().Err(err).Msg("failed to register room-session in registry")
	}

	audit.LogWithDetail(ctx, audit.ActionJoinRoom, c.Session.GetUserID(), fmt.Sprintf("%s:%s", roomID, sessionID), "joined room")

	return c.SendMessage(&domain.RoomJoinedMessage{
		Type:      domain.MsgTypeRoomJoined,
		RoomID:    roomID,
		SessionID: sessionID,
	})
}

func (s *chatService) HandleChatMessage(ctx context.Context, c *hub.Client, content string) error {
	if !c.Session.IsAuthenticated() {
		return c.SendMessage(domain.NewErrorMessage(domain.ErrCodeUnauthorized, "Not authenticated"))
	}

	if !c.Session.IsInRoom() {
		return c.SendMessage(domain.NewErrorMessage(domain.ErrCodeNotInRoom, "Not in a room"))
	}

	roomID, sessionID := c.Session.GetCurrentRoom()

	// Generate Snowflake ID via id-service
	msgID, err := s.idClient.GenerateID(ctx)
	if err != nil {
		l := log.Ctx(ctx)
		l.Error().Err(err).Msg("failed to generate message id")
		return c.SendMessage(domain.NewErrorMessage(domain.ErrCodeInternalError, "Failed to generate message ID"))
	}

	msg := &domain.ChatMessage{
		MessageID: msgID,
		UserID:    c.Session.GetUserID(),
		Username:  c.Session.GetUsername(),
		RoomID:    roomID,
		SessionID: sessionID,
		Content:   content,
		Timestamp: time.Now().UTC(),
	}

	// Produce to Kafka only; delivery back to WS clients happens via downstream gRPC
	if err := s.producer.ProduceMessage(ctx, msg); err != nil {
		l := log.Ctx(ctx)
		l.Error().Err(err).Msg("failed to produce chat message")
		return c.SendMessage(domain.NewErrorMessage(domain.ErrCodeInternalError, "Failed to send message"))
	}

	audit.Log(ctx, audit.ActionSendMessage, c.Session.GetUserID(), "message sent")

	return nil
}

func (s *chatService) HandleLeaveRoom(ctx context.Context, c *hub.Client) error {
	if !c.Session.IsInRoom() {
		return nil
	}
	if err := s.handleLeaveInternal(ctx, c); err != nil {
		return err
	}
	audit.Log(ctx, audit.ActionLeaveRoom, c.Session.GetUserID(), "left room")
	return nil
}

func (s *chatService) HandleDisconnect(ctx context.Context, c *hub.Client) error {
	if !c.Session.IsInRoom() {
		return nil
	}
	audit.Log(ctx, audit.ActionDisconnect, c.Session.GetUserID(), "disconnected")
	return s.handleLeaveInternal(ctx, c)
}

func (s *chatService) handleLeaveInternal(ctx context.Context, c *hub.Client) error {
	roomID, sessionID := c.Session.GetCurrentRoom()
	if roomID == "" {
		return nil
	}

	s.hub.LeaveRoomSession(c, roomID, sessionID)
	c.Session.LeaveRoom()

	// Deregister from Redis if no more clients in this room-session
	count := s.hub.GetRoomSessionClientCount(roomID, sessionID)
	if count == 0 {
		if err := s.registry.Deregister(ctx, roomID, sessionID); err != nil {
			l := log.Ctx(ctx)
			l.Error().Err(err).Msg("failed to deregister room-session")
		}
	}

	return nil
}

func (s *chatService) Start(ctx context.Context) error {
	if err := s.registry.StartHeartbeat(ctx); err != nil {
		return fmt.Errorf("failed to start registry heartbeat: %w", err)
	}
	l := log.L()
	l.Info().Msg("chat service started")
	return nil
}

func (s *chatService) Stop() error {
	s.registry.StopHeartbeat()
	if err := s.producer.Close(); err != nil {
		l := log.L()
		l.Error().Err(err).Msg("failed to close kafka producer")
	}
	return nil
}
