package domain

// Event types for internal communication.
// These are used with the pubsub system.

// RoomState represents the current state of a room.
type RoomState struct {
	RoomID      string `json:"room_id"`
	IsLive      bool   `json:"is_live"`
	HLSUrl      string `json:"hls_url,omitempty"`
	ViewerCount int    `json:"viewer_count"`
}

// BroadcasterInfo represents information about the broadcaster.
type BroadcasterInfo struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	RoomID   string `json:"room_id"`
}
