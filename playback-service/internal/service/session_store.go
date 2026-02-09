package service

import (
	"context"
	"time"
)

// SessionState represents the state of a VOD session.
type SessionState int

const (
	SessionStarting SessionState = iota
	SessionLive
	SessionFinalizing
	SessionCompleted
)

// VODSession represents a single broadcast session.
type VODSession struct {
	RoomID    string       `json:"room_id"`
	SessionID string       `json:"session_id"`
	StartTime time.Time    `json:"start_time"`
	State     SessionState `json:"state"`
	LocalDir  string       `json:"local_dir"`
	S3Prefix  string       `json:"s3_prefix"`
}

// IsActive returns true if the session is in an active state.
func (s *VODSession) IsActive() bool {
	return s.State == SessionStarting || s.State == SessionLive || s.State == SessionFinalizing
}

// SessionStore defines the read-only interface for session storage.
// This is a simplified version for playback-service that only needs read access.
type SessionStore interface {
	// Get retrieves the active session for a room.
	// Returns nil if no active session exists.
	Get(ctx context.Context, roomID string) (*VODSession, error)
}

// NoOpSessionStore is a no-op implementation when session awareness is disabled.
type NoOpSessionStore struct{}

// NewNoOpSessionStore creates a new no-op session store.
func NewNoOpSessionStore() *NoOpSessionStore {
	return &NoOpSessionStore{}
}

// Get always returns nil (no session awareness).
func (s *NoOpSessionStore) Get(ctx context.Context, roomID string) (*VODSession, error) {
	return nil, nil
}
