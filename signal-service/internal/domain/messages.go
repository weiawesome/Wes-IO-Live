package domain

import "encoding/json"

// WebSocket message types from client.
const (
	MsgTypeAuth           = "auth"
	MsgTypeJoinRoom       = "join_room"
	MsgTypeStartBroadcast = "start_broadcast"
	MsgTypeICECandidate   = "ice_candidate"
	MsgTypeStopBroadcast  = "stop_broadcast"
	MsgTypeLeaveRoom      = "leave_room"
	MsgTypePing           = "ping"
)

// WebSocket message types to client.
const (
	MsgTypeAuthResult       = "auth_result"
	MsgTypeRoomJoined       = "room_joined"
	MsgTypeBroadcastStarted = "broadcast_started"
	MsgTypeStreamAvailable  = "stream_available"
	MsgTypeViewerCount      = "viewer_count"
	MsgTypeError            = "error"
	MsgTypePong             = "pong"
)

// BaseMessage is the base structure for all WebSocket messages.
type BaseMessage struct {
	Type string `json:"type"`
}

// Client -> Server messages

// AuthMessage is sent by client to authenticate.
type AuthMessage struct {
	Type  string `json:"type"`
	Token string `json:"token"`
}

// JoinRoomMessage is sent by client to join a room as viewer.
type JoinRoomMessage struct {
	Type   string `json:"type"`
	RoomID string `json:"room_id"`
}

// StartBroadcastMessage is sent by broadcaster to start streaming.
type StartBroadcastMessage struct {
	Type   string          `json:"type"`
	RoomID string          `json:"room_id"`
	Offer  json.RawMessage `json:"offer"` // SDP offer
}

// ICECandidateMessage is sent when an ICE candidate is available.
type ICECandidateMessage struct {
	Type      string          `json:"type"`
	RoomID    string          `json:"room_id"`
	Candidate json.RawMessage `json:"candidate"`
}

// StopBroadcastMessage is sent by broadcaster to stop streaming.
type StopBroadcastMessage struct {
	Type   string `json:"type"`
	RoomID string `json:"room_id"`
}

// LeaveRoomMessage is sent by client to leave a room.
type LeaveRoomMessage struct {
	Type   string `json:"type"`
	RoomID string `json:"room_id"`
}

// Server -> Client messages

// AuthResultMessage is sent to client after authentication.
type AuthResultMessage struct {
	Type     string `json:"type"`
	Success  bool   `json:"success"`
	UserID   string `json:"user_id,omitempty"`
	Username string `json:"username,omitempty"`
	Message  string `json:"message,omitempty"`
}

// RoomJoinedMessage is sent when client successfully joins a room.
type RoomJoinedMessage struct {
	Type        string `json:"type"`
	RoomID      string `json:"room_id"`
	IsOwner     bool   `json:"is_owner"`
	ViewerCount int    `json:"viewer_count"`
	IsLive      bool   `json:"is_live"`
	HLSUrl      string `json:"hls_url,omitempty"`
}

// BroadcastStartedMessage is sent when broadcast is established.
type BroadcastStartedMessage struct {
	Type   string          `json:"type"`
	RoomID string          `json:"room_id"`
	Answer json.RawMessage `json:"answer"` // SDP answer
}

// StreamAvailableMessage is sent to viewers when HLS is ready.
type StreamAvailableMessage struct {
	Type   string `json:"type"`
	RoomID string `json:"room_id"`
	HLSUrl string `json:"hls_url"`
}

// ViewerCountMessage is sent when viewer count changes.
type ViewerCountMessage struct {
	Type   string `json:"type"`
	RoomID string `json:"room_id"`
	Count  int    `json:"count"`
}

// ErrorMessage is sent when an error occurs.
type ErrorMessage struct {
	Type    string `json:"type"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Error codes
const (
	ErrCodeUnauthorized     = "UNAUTHORIZED"
	ErrCodeForbidden        = "FORBIDDEN"
	ErrCodeNotFound         = "NOT_FOUND"
	ErrCodeBadRequest       = "BAD_REQUEST"
	ErrCodeInternalError    = "INTERNAL_ERROR"
	ErrCodeRoomNotLive      = "ROOM_NOT_LIVE"
	ErrCodeAlreadyStreaming = "ALREADY_STREAMING"
)

// NewErrorMessage creates a new error message.
func NewErrorMessage(code, message string) *ErrorMessage {
	return &ErrorMessage{
		Type:    MsgTypeError,
		Code:    code,
		Message: message,
	}
}
