package domain

import (
	"sync"
	"time"

	"github.com/pion/webrtc/v4"
)

// StreamState represents the state of a stream for a room.
type StreamState string

const (
	StreamStateIdle       StreamState = "idle"
	StreamStateConnecting StreamState = "connecting"
	StreamStateLive       StreamState = "live"
	StreamStateStopping   StreamState = "stopping"
)

// Stream represents an active stream for a room.
type Stream struct {
	RoomID         string
	UserID         string
	State          StreamState
	PeerConnection *webrtc.PeerConnection
	VideoTrack     *webrtc.TrackRemote
	AudioTrack     *webrtc.TrackRemote
	HLSUrl         string
	CreatedAt      time.Time
	StartedAt      *time.Time
	mu             sync.RWMutex
}

// NewStream creates a new stream instance.
func NewStream(roomID, userID string) *Stream {
	return &Stream{
		RoomID:    roomID,
		UserID:    userID,
		State:     StreamStateIdle,
		CreatedAt: time.Now().UTC(),
	}
}

// SetState updates the stream state.
func (s *Stream) SetState(state StreamState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.State = state
	if state == StreamStateLive && s.StartedAt == nil {
		now := time.Now().UTC()
		s.StartedAt = &now
	}
}

// GetState returns the current stream state.
func (s *Stream) GetState() StreamState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.State
}

// SetVideoTrack sets the video track.
func (s *Stream) SetVideoTrack(track *webrtc.TrackRemote) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.VideoTrack = track
}

// GetVideoTrack returns the video track.
func (s *Stream) GetVideoTrack() *webrtc.TrackRemote {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.VideoTrack
}

// SetAudioTrack sets the audio track.
func (s *Stream) SetAudioTrack(track *webrtc.TrackRemote) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.AudioTrack = track
}

// GetAudioTrack returns the audio track.
func (s *Stream) GetAudioTrack() *webrtc.TrackRemote {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.AudioTrack
}

// SetHLSUrl sets the HLS URL.
func (s *Stream) SetHLSUrl(url string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.HLSUrl = url
}

// GetHLSUrl returns the HLS URL.
func (s *Stream) GetHLSUrl() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.HLSUrl
}

// Close cleans up the stream resources.
func (s *Stream) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.PeerConnection != nil {
		s.PeerConnection.Close()
		s.PeerConnection = nil
	}

	s.VideoTrack = nil
	s.AudioTrack = nil
	s.State = StreamStateIdle

	return nil
}
