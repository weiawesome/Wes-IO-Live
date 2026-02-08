package dispatcher

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/weiawesome/wes-io-live/chat-dispatcher/internal/delivery"
	"github.com/weiawesome/wes-io-live/chat-dispatcher/internal/registry"
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
	var msg ChatMessage
	if err := json.Unmarshal(value, &msg); err != nil {
		log.Printf("Failed to unmarshal message: %v", err)
		return nil // skip malformed messages
	}

	// Lookup which chat-service instance owns this room-session
	addr, err := d.registry.Lookup(ctx, msg.RoomID, msg.SessionID)
	if err != nil {
		if strings.Contains(err.Error(), "not registered") {
			log.Printf("Room-session %s:%s not registered, skipping", msg.RoomID, msg.SessionID)
		} else {
			log.Printf("Registry lookup failed for %s:%s: %v", msg.RoomID, msg.SessionID, err)
		}
		return nil // skip, don't block consumer
	}

	// Build gRPC request
	req := &pb.DeliverMessageRequest{
		RoomId:    msg.RoomID,
		SessionId: msg.SessionID,
		Message: &pb.ChatMessagePayload{
			MessageId:     msg.MessageID,
			UserId:        msg.UserID,
			Username:      msg.Username,
			RoomId:        msg.RoomID,
			SessionId:     msg.SessionID,
			TimestampUnixMs: msg.Timestamp.UnixMilli(),
			Content:       msg.Content,
		},
	}

	resp, err := d.delivery.DeliverMessage(ctx, addr, req)
	if err != nil {
		log.Printf("gRPC delivery failed for msg=%s to %s: %v", msg.MessageID, addr, err)
		return nil // skip, don't block consumer
	}

	log.Printf("Delivered msg=%s to room=%s session=%s via %s (%d clients)",
		msg.MessageID, msg.RoomID, msg.SessionID, addr, resp.DeliveredCount)

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
