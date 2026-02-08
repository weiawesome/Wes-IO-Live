package kafka

import "context"

// BroadcastEvent represents a broadcast state change event from signal-service.
type BroadcastEvent struct {
	Type          string `json:"type"`            // "broadcast_started" | "broadcast_stopped"
	RoomID        string `json:"room_id"`
	BroadcasterID string `json:"broadcaster_id"`
	Reason        string `json:"reason,omitempty"` // "explicit" | "disconnect" | "timeout"
	Timestamp     int64  `json:"timestamp"`
}

// Event types
const (
	EventBroadcastStarted = "broadcast_started"
	EventBroadcastStopped = "broadcast_stopped"
)

// Stop reasons
const (
	ReasonExplicit   = "explicit"
	ReasonDisconnect = "disconnect"
	ReasonTimeout    = "timeout"
)

// BroadcastEventHandler handles incoming broadcast events.
type BroadcastEventHandler interface {
	HandleBroadcastEvent(ctx context.Context, event *BroadcastEvent) error
}

// BroadcastEventConsumer defines the interface for consuming broadcast events.
type BroadcastEventConsumer interface {
	Start(ctx context.Context) error
	Close() error
}
