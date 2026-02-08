package pubsub

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/weiawesome/wes-io-live/presence-service/internal/domain"
	"github.com/weiawesome/wes-io-live/presence-service/internal/hub"
)

// Subscriber subscribes to Redis Pub/Sub for room count updates and broadcasts to local hub.
type Subscriber struct {
	client     *redis.Client
	channel    string
	hub        *hub.Hub
	instanceID string
}

// NewSubscriber creates a new Redis Pub/Sub subscriber for room updates.
func NewSubscriber(client *redis.Client, channel string, h *hub.Hub, instanceID string) *Subscriber {
	if channel == "" {
		channel = "presence:room_updates"
	}
	return &Subscriber{
		client:     client,
		channel:    channel,
		hub:        h,
		instanceID: instanceID,
	}
}

// Run subscribes to the channel and broadcasts count updates to local hub until ctx is done.
// Reconnects on receive errors.
func (s *Subscriber) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			if err := s.runSubscription(ctx); err != nil && ctx.Err() == nil {
				log.Printf("Presence pubsub subscription error: %v; reconnecting in 2s", err)
				select {
				case <-ctx.Done():
					return
				case <-time.After(2 * time.Second):
					continue
				}
			}
			return
		}
	}
}

func (s *Subscriber) runSubscription(ctx context.Context) error {
	pubsub := s.client.Subscribe(ctx, s.channel)
	defer pubsub.Close()

	// Wait for subscription to be active
	if _, err := pubsub.Receive(ctx); err != nil {
		return err
	}

	ch := pubsub.Channel()

	for {
		select {
		case <-ctx.Done():
			return nil
		case msg, ok := <-ch:
			if !ok {
				return nil
			}
			s.handleMessage(ctx, msg.Payload)
		}
	}
}

func (s *Subscriber) handleMessage(ctx context.Context, payload string) {
	var update domain.RoomUpdatePayload
	if err := json.Unmarshal([]byte(payload), &update); err != nil {
		log.Printf("Presence pubsub: invalid payload: %v", err)
		return
	}
	if update.RoomID == "" {
		return
	}

	msg := &domain.CountMessage{
		Type:   domain.MsgTypeCount,
		RoomID: update.RoomID,
		Count:  update.Count,
	}
	if err := s.hub.BroadcastToRoom(update.RoomID, msg, ""); err != nil {
		log.Printf("Presence pubsub: broadcast error for room %s: %v", update.RoomID, err)
	}
}
