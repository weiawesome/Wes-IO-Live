package domain

// WebSocket message types from client.
const (
	MsgTypeAuth        = "auth"
	MsgTypeJoinRoom    = "join_room"
	MsgTypeChatMessage = "chat_message"
	MsgTypeLeaveRoom   = "leave_room"
	MsgTypePing        = "ping"
)

// WebSocket message types to client.
const (
	MsgTypeAuthResult  = "auth_result"
	MsgTypeRoomJoined  = "room_joined"
	MsgTypeError       = "error"
	MsgTypePong        = "pong"
)

// Error codes
const (
	ErrCodeUnauthorized  = "UNAUTHORIZED"
	ErrCodeBadRequest    = "BAD_REQUEST"
	ErrCodeInternalError = "INTERNAL_ERROR"
	ErrCodeNotInRoom     = "NOT_IN_ROOM"
)

// BaseMessage is the base structure for all WebSocket messages.
type BaseMessage struct {
	Type string `json:"type"`
}

// Client -> Server messages

type AuthMessage struct {
	Type  string `json:"type"`
	Token string `json:"token"`
}

type JoinRoomMessage struct {
	Type      string `json:"type"`
	RoomID    string `json:"room_id"`
	SessionID string `json:"session_id"`
}

type ChatMessageWS struct {
	Type    string `json:"type"`
	Content string `json:"content"`
}

type LeaveRoomMessage struct {
	Type string `json:"type"`
}

// Server -> Client messages

type AuthResultMessage struct {
	Type     string `json:"type"`
	Success  bool   `json:"success"`
	UserID   string `json:"user_id,omitempty"`
	Username string `json:"username,omitempty"`
	Message  string `json:"message,omitempty"`
}

type RoomJoinedMessage struct {
	Type      string `json:"type"`
	RoomID    string `json:"room_id"`
	SessionID string `json:"session_id"`
}

type ChatMessageOut struct {
	Type      string `json:"type"`
	MessageID string `json:"message_id"`
	UserID    string `json:"user_id"`
	Username  string `json:"username"`
	RoomID    string `json:"room_id"`
	SessionID string `json:"session_id"`
	Content   string `json:"content"`
	Timestamp int64  `json:"timestamp"`
}

type SystemMessageOut struct {
	Type            string `json:"type"`
	Content         string `json:"content"`
	SystemEventType string `json:"system_event_type"`
	RoomID          string `json:"room_id"`
	SessionID       string `json:"session_id"`
	Timestamp       int64  `json:"timestamp"`
}

type ErrorMessage struct {
	Type    string `json:"type"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func NewErrorMessage(code, message string) *ErrorMessage {
	return &ErrorMessage{
		Type:    MsgTypeError,
		Code:    code,
		Message: message,
	}
}
