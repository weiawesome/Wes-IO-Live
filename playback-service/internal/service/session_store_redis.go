package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/weiawesome/wes-io-live/playback-service/internal/config"
)

// RedisSessionStore is a read-only Redis-backed implementation of SessionStore.
// This is a simplified version for playback-service that only needs read access.
type RedisSessionStore struct {
	client    *redis.Client
	keyPrefix string
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
	}, nil
}

// key returns the Redis key for a room's session.
func (s *RedisSessionStore) key(roomID string) string {
	return s.keyPrefix + roomID
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

// Close closes the Redis client connection.
func (s *RedisSessionStore) Close() error {
	return s.client.Close()
}

// Ensure RedisSessionStore implements SessionStore interface
var _ SessionStore = (*RedisSessionStore)(nil)
