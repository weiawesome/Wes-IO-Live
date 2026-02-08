package domain

import (
	"sync"
	"time"
)

// Session represents a client's WebSocket session.
type Session struct {
	ID            string
	UserID        string
	Username      string
	Email         string
	Roles         []string
	Authenticated bool
	CurrentRoomID string
	IsBroadcaster bool
	CreatedAt     time.Time
	LastActiveAt  time.Time
	mu            sync.RWMutex
}

// NewSession creates a new session with a unique ID.
func NewSession(id string) *Session {
	now := time.Now()
	return &Session{
		ID:           id,
		CreatedAt:    now,
		LastActiveAt: now,
	}
}

// Authenticate sets the user information after successful authentication.
func (s *Session) Authenticate(userID, username, email string, roles []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.UserID = userID
	s.Username = username
	s.Email = email
	s.Roles = roles
	s.Authenticated = true
	s.LastActiveAt = time.Now()
}

// IsAuthenticated returns whether the session is authenticated.
func (s *Session) IsAuthenticated() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Authenticated
}

// JoinRoom sets the current room for the session.
func (s *Session) JoinRoom(roomID string, isBroadcaster bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.CurrentRoomID = roomID
	s.IsBroadcaster = isBroadcaster
	s.LastActiveAt = time.Now()
}

// LeaveRoom clears the current room from the session.
func (s *Session) LeaveRoom() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.CurrentRoomID = ""
	s.IsBroadcaster = false
	s.LastActiveAt = time.Now()
}

// GetCurrentRoom returns the current room ID.
func (s *Session) GetCurrentRoom() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.CurrentRoomID
}

// GetUserID returns the user ID.
func (s *Session) GetUserID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.UserID
}

// IsBroadcasting returns whether the session is currently broadcasting.
func (s *Session) IsBroadcasting() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.IsBroadcaster
}

// UpdateActivity updates the last active timestamp.
func (s *Session) UpdateActivity() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.LastActiveAt = time.Now()
}
