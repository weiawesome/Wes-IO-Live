package kafka

import "context"

// BroadcastEvent represents a broadcast state change event.
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

// BroadcastEventProducer defines the interface for producing broadcast events.
type BroadcastEventProducer interface {
	ProduceBroadcastStarted(ctx context.Context, roomID, broadcasterID string) error
	ProduceBroadcastStopped(ctx context.Context, roomID, broadcasterID, reason string) error
	Close() error
}
