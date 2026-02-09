package service

import (
	"context"
	"time"
)

// SessionState represents the state of a VOD session.
type SessionState int

const (
	// SessionStarting indicates the session is being initialized.
	SessionStarting SessionState = iota
	// SessionLive indicates the session is actively streaming.
	SessionLive
	// SessionFinalizing indicates the session is being finalized (uploading final assets).
	SessionFinalizing
	// SessionCompleted indicates the session has been finalized.
	SessionCompleted
)

// String returns the string representation of SessionState.
func (s SessionState) String() string {
	switch s {
	case SessionStarting:
		return "starting"
	case SessionLive:
		return "live"
	case SessionFinalizing:
		return "finalizing"
	case SessionCompleted:
		return "completed"
	default:
		return "unknown"
	}
}

// VODSession represents a single broadcast session for VOD recording.
// Each broadcast creates a new session with a unique SessionID (timestamp-based).
type VODSession struct {
	// RoomID is the room identifier.
	RoomID string `json:"room_id"`

	// SessionID is a unique identifier for this session (timestamp format: 2006-01-02T15-04-05Z).
	SessionID string `json:"session_id"`

	// StartTime is when the session started.
	StartTime time.Time `json:"start_time"`

	// State is the current state of the session.
	State SessionState `json:"state"`

	// LocalDir is the local HLS output directory for this session.
	// Format: {hlsOutputDir}/room_{roomID}/{sessionID}/
	LocalDir string `json:"local_dir"`

	// S3Prefix is the S3 key prefix for this session's VOD files.
	// Format: vod/room_{roomID}/{sessionID}/
	S3Prefix string `json:"s3_prefix"`
}

// IsActive returns true if the session is in an active state (not completed).
func (s *VODSession) IsActive() bool {
	return s.State == SessionStarting || s.State == SessionLive || s.State == SessionFinalizing
}

// CanFinalize returns true if the session can be finalized.
func (s *VODSession) CanFinalize() bool {
	return s.State == SessionLive
}

// SessionStore manages VOD session state storage.
// This interface abstracts the storage backend, allowing for different implementations:
// - MemorySessionStore: Single-instance deployment (in-memory map)
// - RedisSessionStore: Multi-instance deployment (Redis for distributed state)
type SessionStore interface {
	// Save stores or updates a session.
	Save(ctx context.Context, session *VODSession) error

	// Get retrieves the active session for a room.
	// Returns nil if no active session exists.
	Get(ctx context.Context, roomID string) (*VODSession, error)

	// Delete removes a session from the store.
	Delete(ctx context.Context, roomID string) error

	// List returns all active sessions.
	List(ctx context.Context) ([]*VODSession, error)

	// GetByState returns sessions filtered by state.
	GetByState(ctx context.Context, state SessionState) ([]*VODSession, error)
}

// VODInfo represents information about a VOD recording.
type VODInfo struct {
	SessionID string    `json:"session_id"`
	StartTime time.Time `json:"start_time"`
	URL       string    `json:"url,omitempty"`
}
