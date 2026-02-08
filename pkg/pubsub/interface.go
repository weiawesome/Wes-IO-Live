package pubsub

import (
	"context"
	"encoding/json"
	"time"
)

// Event represents a message published to the event bus.
type Event struct {
	Type      string          `json:"type"`
	RoomID    string          `json:"room_id"`
	Payload   json.RawMessage `json:"payload"`
	Timestamp time.Time       `json:"timestamp"`
}

// NewEvent creates a new event with the current timestamp.
func NewEvent(eventType, roomID string, payload interface{}) (*Event, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return &Event{
		Type:      eventType,
		RoomID:    roomID,
		Payload:   data,
		Timestamp: time.Now(),
	}, nil
}

// UnmarshalPayload unmarshals the event payload into the given struct.
func (e *Event) UnmarshalPayload(v interface{}) error {
	return json.Unmarshal(e.Payload, v)
}

// Publisher publishes events to the event bus.
type Publisher interface {
	Publish(ctx context.Context, channel string, event *Event) error
}

// Subscriber subscribes to events from the event bus.
type Subscriber interface {
	Subscribe(ctx context.Context, channel string) (<-chan *Event, error)
	SubscribePattern(ctx context.Context, pattern string) (<-chan *Event, error)
	Unsubscribe(ctx context.Context, channel string) error
}

// PubSub combines Publisher and Subscriber interfaces.
type PubSub interface {
	Publisher
	Subscriber
	Close() error
}
