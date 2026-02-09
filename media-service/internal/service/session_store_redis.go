package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/weiawesome/wes-io-live/media-service/internal/config"
)

// RedisSessionStore is a Redis-backed implementation of SessionStore.
// Suitable for multi-instance deployments where session state needs to be shared.
type RedisSessionStore struct {
	client    *redis.Client
	keyPrefix string
	ttl       time.Duration
}

// NewRedisSessionStore creates a new Redis-backed session store.
func NewRedisSessionStore(cfg config.SessionRedisConfig) (*RedisSessionStore, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Address,
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to redis: %w", err)
	}

	return &RedisSessionStore{
		client:    client,
		keyPrefix: cfg.KeyPrefix,
		ttl:       time.Duration(cfg.TTL) * time.Second,
	}, nil
}

// key returns the Redis key for a room's session.
func (s *RedisSessionStore) key(roomID string) string {
	return s.keyPrefix + roomID
}

// Save stores or updates a session.
func (s *RedisSessionStore) Save(ctx context.Context, session *VODSession) error {
	data, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("failed to marshal session: %w", err)
	}

	key := s.key(session.RoomID)
	if err := s.client.Set(ctx, key, data, s.ttl).Err(); err != nil {
		return fmt.Errorf("failed to save session to redis: %w", err)
	}

	return nil
}

// Get retrieves the active session for a room.
// Returns nil if no active session exists.
func (s *RedisSessionStore) Get(ctx context.Context, roomID string) (*VODSession, error) {
	key := s.key(roomID)
	data, err := s.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get session from redis: %w", err)
	}

	var session VODSession
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("failed to unmarshal session: %w", err)
	}

	return &session, nil
}

// Delete removes a session from the store.
func (s *RedisSessionStore) Delete(ctx context.Context, roomID string) error {
	key := s.key(roomID)
	if err := s.client.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("failed to delete session from redis: %w", err)
	}
	return nil
}

// List returns all active sessions.
func (s *RedisSessionStore) List(ctx context.Context) ([]*VODSession, error) {
	pattern := s.keyPrefix + "*"
	keys, err := s.client.Keys(ctx, pattern).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to list session keys: %w", err)
	}

	if len(keys) == 0 {
		return []*VODSession{}, nil
	}

	// Use MGET to fetch all sessions at once
	values, err := s.client.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get sessions: %w", err)
	}

	result := make([]*VODSession, 0, len(values))
	for _, val := range values {
		if val == nil {
			continue
		}

		data, ok := val.(string)
		if !ok {
			continue
		}

		var session VODSession
		if err := json.Unmarshal([]byte(data), &session); err != nil {
			continue
		}
		result = append(result, &session)
	}

	return result, nil
}

// GetByState returns sessions filtered by state.
func (s *RedisSessionStore) GetByState(ctx context.Context, state SessionState) ([]*VODSession, error) {
	sessions, err := s.List(ctx)
	if err != nil {
		return nil, err
	}

	var result []*VODSession
	for _, session := range sessions {
		if session.State == state {
			result = append(result, session)
		}
	}

	return result, nil
}

// Close closes the Redis client connection.
func (s *RedisSessionStore) Close() error {
	return s.client.Close()
}

// Ensure RedisSessionStore implements SessionStore interface
var _ SessionStore = (*RedisSessionStore)(nil)
