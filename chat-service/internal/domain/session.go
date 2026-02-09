package domain

import (
	"sync"
	"time"
)

type Session struct {
	ID            string
	UserID        string
	Username      string
	Email         string
	Roles         []string
	Authenticated bool
	CurrentRoomID string
	CurrentSessionID string
	CreatedAt     time.Time
	LastActiveAt  time.Time
	mu            sync.RWMutex
}

func NewSession(id string) *Session {
	now := time.Now()
	return &Session{
		ID:           id,
		CreatedAt:    now,
		LastActiveAt: now,
	}
}

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

func (s *Session) IsAuthenticated() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Authenticated
}

func (s *Session) JoinRoom(roomID, sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.CurrentRoomID = roomID
	s.CurrentSessionID = sessionID
	s.LastActiveAt = time.Now()
}

func (s *Session) LeaveRoom() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.CurrentRoomID = ""
	s.CurrentSessionID = ""
	s.LastActiveAt = time.Now()
}

func (s *Session) GetCurrentRoom() (string, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.CurrentRoomID, s.CurrentSessionID
}

func (s *Session) GetUserID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.UserID
}

func (s *Session) GetUsername() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Username
}

func (s *Session) IsInRoom() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.CurrentRoomID != ""
}

func (s *Session) UpdateActivity() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.LastActiveAt = time.Now()
}
