package pubsub

import "fmt"

// Channel naming conventions for the live streaming system.
const (
	// Signal -> Media channels
	ChannelSignalToMedia = "signal:room:%s:to_media"

	// Media -> Signal channels
	ChannelMediaToSignal = "media:room:%s:to_signal"
)

// Event types for Signal -> Media communication.
const (
	EventStartBroadcast = "start_broadcast"
	EventICECandidate   = "ice_candidate"
	EventStopBroadcast  = "stop_broadcast"
)

// Event types for Media -> Signal communication.
const (
	EventBroadcastAnswer    = "broadcast_answer"
	EventServerICECandidate = "server_ice_candidate"
	EventStreamReady        = "stream_ready"
	EventStreamEnded        = "stream_ended"
)

// SignalToMediaChannel returns the channel name for Signal -> Media events.
func SignalToMediaChannel(roomID string) string {
	return fmt.Sprintf(ChannelSignalToMedia, roomID)
}

// MediaToSignalChannel returns the channel name for Media -> Signal events.
func MediaToSignalChannel(roomID string) string {
	return fmt.Sprintf(ChannelMediaToSignal, roomID)
}

// Event payloads for Signal -> Media.

// StartBroadcastPayload is sent when a broadcaster starts streaming.
type StartBroadcastPayload struct {
	RoomID string `json:"room_id"`
	UserID string `json:"user_id"`
	Offer  string `json:"offer"` // SDP JSON
}

// ICECandidatePayload is sent when an ICE candidate is received.
type ICECandidatePayload struct {
	RoomID    string `json:"room_id"`
	Candidate string `json:"candidate"` // JSON
}

// StopBroadcastPayload is sent when a broadcast should stop.
type StopBroadcastPayload struct {
	RoomID string `json:"room_id"`
	Reason string `json:"reason"` // "manual", "disconnect"
}

// Event payloads for Media -> Signal.

// BroadcastAnswerPayload is sent when the media service creates an SDP answer.
type BroadcastAnswerPayload struct {
	RoomID string `json:"room_id"`
	Answer string `json:"answer"` // SDP JSON
}

// ServerICECandidatePayload is sent when the server has an ICE candidate.
type ServerICECandidatePayload struct {
	RoomID    string `json:"room_id"`
	Candidate string `json:"candidate"`
}

// StreamReadyPayload is sent when HLS streaming is ready.
type StreamReadyPayload struct {
	RoomID string `json:"room_id"`
	HLSUrl string `json:"hls_url"`
}

// StreamEndedPayload is sent when the stream has ended.
type StreamEndedPayload struct {
	RoomID string `json:"room_id"`
}
