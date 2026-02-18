package dispatcher

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/weiawesome/wes-io-live/chat-dispatcher/internal/delivery"
	"github.com/weiawesome/wes-io-live/chat-dispatcher/internal/registry"
	"github.com/weiawesome/wes-io-live/pkg/log"
	pb "github.com/weiawesome/wes-io-live/proto/chat"
)

// ChatMessage aligns with chat-service/internal/domain.ChatMessage
type ChatMessage struct {
	MessageID string    `json:"message_id"`
	UserID    string    `json:"user_id"`
	Username  string    `json:"username"`
	RoomID    string    `json:"room_id"`
	SessionID string    `json:"session_id"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

type Dispatcher struct {
	registry registry.RegistryLookup
	delivery delivery.DeliveryClient
}

func NewDispatcher(reg registry.RegistryLookup, del delivery.DeliveryClient) *Dispatcher {
	return &Dispatcher{
		registry: reg,
		delivery: del,
	}
}

func (d *Dispatcher) HandleMessage(ctx context.Context, key, value []byte) error {
	l := log.Ctx(ctx)

	var msg ChatMessage
	if err := json.Unmarshal(value, &msg); err != nil {
		l.Warn().Err(err).Msg("failed to unmarshal message")
		return nil // skip malformed messages
	}

	// Lookup which chat-service instance owns this room-session
	addr, err := d.registry.Lookup(ctx, msg.RoomID, msg.SessionID)
	if err != nil {
		if strings.Contains(err.Error(), "not registered") {
			l.Debug().Str("room_id", msg.RoomID).Str("session_id", msg.SessionID).Msg("room-session not registered, skipping")
		} else {
			l.Error().Err(err).Str("room_id", msg.RoomID).Str("session_id", msg.SessionID).Msg("registry lookup failed")
		}
		return nil // skip, don't block consumer
	}

	// Build gRPC request
	req := &pb.DeliverMessageRequest{
		RoomId:    msg.RoomID,
		SessionId: msg.SessionID,
		Message: &pb.ChatMessagePayload{
			MessageId:       msg.MessageID,
			UserId:          msg.UserID,
			Username:        msg.Username,
			RoomId:          msg.RoomID,
			SessionId:       msg.SessionID,
			TimestampUnixMs: msg.Timestamp.UnixMilli(),
			Content:         msg.Content,
		},
	}

	resp, err := d.delivery.DeliverMessage(ctx, addr, req)
	if err != nil {
		l.Error().Err(err).Str("message_id", msg.MessageID).Str("addr", addr).Msg("grpc delivery failed")
		return nil // skip, don't block consumer
	}

	l.Info().
		Str("message_id", msg.MessageID).
		Str("room_id", msg.RoomID).
		Str("session_id", msg.SessionID).
		Str("addr", addr).
		Int32("delivered_count", resp.DeliveredCount).
		Msg("message delivered")

	return nil
}

func (d *Dispatcher) Close() error {
	var errs []string
	if err := d.registry.Close(); err != nil {
		errs = append(errs, fmt.Sprintf("registry: %v", err))
	}
	if err := d.delivery.Close(); err != nil {
		errs = append(errs, fmt.Sprintf("delivery: %v", err))
	}
	if len(errs) > 0 {
		return fmt.Errorf("dispatcher close errors: %s", strings.Join(errs, "; "))
	}
	return nil
}
