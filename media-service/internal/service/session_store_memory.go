package service

import (
	"context"
	"sync"
)

// MemorySessionStore is an in-memory implementation of SessionStore.
// Suitable for single-instance deployments.
type MemorySessionStore struct {
	sessions map[string]*VODSession // roomID -> session
	mu       sync.RWMutex
}

// NewMemorySessionStore creates a new in-memory session store.
func NewMemorySessionStore() *MemorySessionStore {
	return &MemorySessionStore{
		sessions: make(map[string]*VODSession),
	}
}

// Save stores or updates a session.
func (s *MemorySessionStore) Save(ctx context.Context, session *VODSession) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Make a copy to prevent external modifications
	sessionCopy := *session
	s.sessions[session.RoomID] = &sessionCopy
	return nil
}

// Get retrieves the active session for a room.
// Returns nil if no active session exists.
func (s *MemorySessionStore) Get(ctx context.Context, roomID string) (*VODSession, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	session, exists := s.sessions[roomID]
	if !exists {
		return nil, nil
	}

	// Return a copy to prevent external modifications
	sessionCopy := *session
	return &sessionCopy, nil
}

// Delete removes a session from the store.
func (s *MemorySessionStore) Delete(ctx context.Context, roomID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.sessions, roomID)
	return nil
}

// List returns all active sessions.
func (s *MemorySessionStore) List(ctx context.Context) ([]*VODSession, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*VODSession, 0, len(s.sessions))
	for _, session := range s.sessions {
		sessionCopy := *session
		result = append(result, &sessionCopy)
	}
	return result, nil
}

// GetByState returns sessions filtered by state.
func (s *MemorySessionStore) GetByState(ctx context.Context, state SessionState) ([]*VODSession, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*VODSession
	for _, session := range s.sessions {
		if session.State == state {
			sessionCopy := *session
			result = append(result, &sessionCopy)
		}
	}
	return result, nil
}

// Ensure MemorySessionStore implements SessionStore interface
var _ SessionStore = (*MemorySessionStore)(nil)
