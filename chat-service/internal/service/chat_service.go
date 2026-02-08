package service

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/weiawesome/wes-io-live/chat-service/internal/client"
	"github.com/weiawesome/wes-io-live/chat-service/internal/domain"
	"github.com/weiawesome/wes-io-live/chat-service/internal/hub"
	"github.com/weiawesome/wes-io-live/chat-service/internal/kafka"
	"github.com/weiawesome/wes-io-live/chat-service/internal/registry"
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
		c.SendMessage(&domain.AuthResultMessage{
			Type:    domain.MsgTypeAuthResult,
			Success: false,
			Message: result.Error,
		})
		return fmt.Errorf("invalid token: %s", result.Error)
	}

	c.Session.Authenticate(result.UserID, result.Username, result.Email, result.Roles)

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
		log.Printf("Failed to register room-session in registry: %v", err)
	}

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
		log.Printf("Failed to generate message ID: %v", err)
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
		log.Printf("Failed to produce chat message: %v", err)
		return c.SendMessage(domain.NewErrorMessage(domain.ErrCodeInternalError, "Failed to send message"))
	}

	return nil
}

func (s *chatService) HandleLeaveRoom(ctx context.Context, c *hub.Client) error {
	if !c.Session.IsInRoom() {
		return nil
	}
	return s.handleLeaveInternal(ctx, c)
}

func (s *chatService) HandleDisconnect(ctx context.Context, c *hub.Client) error {
	if !c.Session.IsInRoom() {
		return nil
	}
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
			log.Printf("Failed to deregister room-session: %v", err)
		}
	}

	return nil
}

func (s *chatService) Start(ctx context.Context) error {
	if err := s.registry.StartHeartbeat(ctx); err != nil {
		return fmt.Errorf("failed to start registry heartbeat: %w", err)
	}
	log.Println("Chat service started")
	return nil
}

func (s *chatService) Stop() error {
	s.registry.StopHeartbeat()
	if err := s.producer.Close(); err != nil {
		log.Printf("Failed to close kafka producer: %v", err)
	}
	return nil
}
