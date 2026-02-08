package domain

// WebSocket message types from client.
const (
	MsgTypeJoin  = "join"
	MsgTypeLeave = "leave"
	MsgTypePing  = "ping" // heartbeat, same as signal/chat
)

// WebSocket message types to client.
const (
	MsgTypeJoined = "joined"
	MsgTypeCount  = "count"
	MsgTypeError  = "error"
	MsgTypePong   = "pong"
)

// Client -> Server messages

// JoinMessage is sent by client to join a room.
type JoinMessage struct {
	Type       string `json:"type"`
	RoomID     string `json:"room_id"`
	Token      string `json:"token,omitempty"`       // For authenticated users
	DeviceHash string `json:"device_hash,omitempty"` // For anonymous users
}

// LeaveMessage is sent by client to leave a room.
type LeaveMessage struct {
	Type   string `json:"type"`
	RoomID string `json:"room_id"`
}

// HeartbeatMessage is sent by client to maintain connection.
type HeartbeatMessage struct {
	Type string `json:"type"`
}

// BaseMessage is the base structure for all WebSocket messages.
type BaseMessage struct {
	Type string `json:"type"`
}

// Server -> Client messages

// PresenceCount represents the count breakdown.
type PresenceCount struct {
	Authenticated int `json:"authenticated"`
	Anonymous     int `json:"anonymous"`
	Total         int `json:"total"`
}

// RoomUpdatePayload is the message published to Redis Pub/Sub for multi-instance count sync.
type RoomUpdatePayload struct {
	RoomID           string        `json:"room_id"`
	Count            PresenceCount `json:"count"`
	OriginInstanceID string        `json:"origin_instance_id,omitempty"`
}

// JoinedMessage is sent when client successfully joins a room.
type JoinedMessage struct {
	Type   string        `json:"type"`
	RoomID string        `json:"room_id"`
	Count  PresenceCount `json:"count"`
}

// CountMessage is sent when viewer count changes.
type CountMessage struct {
	Type   string        `json:"type"`
	RoomID string        `json:"room_id"`
	Count  PresenceCount `json:"count"`
}

// ErrorMessage is sent when an error occurs.
type ErrorMessage struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// NewErrorMessage creates a new error message.
func NewErrorMessage(message string) *ErrorMessage {
	return &ErrorMessage{
		Type:    MsgTypeError,
		Message: message,
	}
}

// UserIdentity represents user identity for presence tracking.
type UserIdentity struct {
	UserID     string // For authenticated users
	DeviceHash string // For anonymous users
	IsAuth     bool   // Whether user is authenticated
}

// GetIdentifier returns the unique identifier for the user.
func (u *UserIdentity) GetIdentifier() string {
	if u.IsAuth {
		return u.UserID
	}
	return u.DeviceHash
}

// LiveStatus represents the live streaming status of a room.
type LiveStatus struct {
	IsLive        bool   `json:"is_live"`
	BroadcasterID string `json:"broadcaster_id,omitempty"`
	StartedAt     int64  `json:"started_at,omitempty"`
}

// RoomInfo combines presence count and live status for a room.
type RoomInfo struct {
	RoomID     string        `json:"room_id"`
	Count      PresenceCount `json:"count"`
	LiveStatus LiveStatus    `json:"live_status"`
}

// LiveRoomsResponse is the response for listing live rooms.
type LiveRoomsResponse struct {
	Rooms []string `json:"rooms"`
	Total int      `json:"total"`
}
