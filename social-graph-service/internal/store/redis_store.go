package store

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/go-redis/redis/v8"
)

const (
	followersCountKeyPrefix = "social:followers:"
	hotKeyScoresKey         = "social:hotkey:scores"
)

// FollowStore defines Redis operations for followers count caching and hot key tracking.
type FollowStore interface {
	GetFollowersCount(ctx context.Context, userID string) (int64, bool, error)
	SetFollowersCount(ctx context.Context, userID string, count int64) error
	CondIncrFollowersCount(ctx context.Context, userID string) error
	CondDecrFollowersCount(ctx context.Context, userID string) error
	RecordAccess(ctx context.Context, userID string) error
	GetTopHotKeys(ctx context.Context, n int64) ([]string, error)
	ResetHotKeyScores(ctx context.Context) error
	Close() error
}

// RedisFollowStore implements FollowStore backed by Redis.
type RedisFollowStore struct {
	client *redis.Client
}

// NewRedisFollowStore creates a new Redis-backed follow store.
func NewRedisFollowStore(address, password string, db int) (*RedisFollowStore, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     address,
		Password: password,
		DB:       db,
	})

	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to redis: %w", err)
	}

	return &RedisFollowStore{client: client}, nil
}

func followersCountKey(userID string) string {
	return followersCountKeyPrefix + userID
}

// GetFollowersCount returns the cached followers count for a user.
// Returns (count, true, nil) on hit, (0, false, nil) on miss, (0, false, err) on error.
func (s *RedisFollowStore) GetFollowersCount(ctx context.Context, userID string) (int64, bool, error) {
	val, err := s.client.Get(ctx, followersCountKey(userID)).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return 0, false, nil
		}
		return 0, false, fmt.Errorf("redis get followers count: %w", err)
	}

	count, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return 0, false, fmt.Errorf("parse followers count: %w", err)
	}
	return count, true, nil
}

// SetFollowersCount sets the followers count for a user in Redis.
func (s *RedisFollowStore) SetFollowersCount(ctx context.Context, userID string, count int64) error {
	err := s.client.Set(ctx, followersCountKey(userID), count, 0).Err()
	if err != nil {
		return fmt.Errorf("redis set followers count: %w", err)
	}
	return nil
}

// condIncrScript atomically increments the key only if it exists.
// Returns 1 if incremented, 0 if key did not exist.
var condIncrScript = redis.NewScript(`
local key = KEYS[1]
if redis.call("EXISTS", key) == 1 then
  return redis.call("INCR", key)
end
return 0
`)

// condDecrScript atomically decrements the key only if it exists and result >= 0.
// Returns the new value if decremented, 0 if key did not exist.
var condDecrScript = redis.NewScript(`
local key = KEYS[1]
if redis.call("EXISTS", key) == 1 then
  local val = tonumber(redis.call("GET", key))
  if val and val > 0 then
    return redis.call("DECR", key)
  end
end
return 0
`)

// CondIncrFollowersCount atomically increments the followers count only if the key exists.
// This prevents stale cache from being initialized via CDC alone.
func (s *RedisFollowStore) CondIncrFollowersCount(ctx context.Context, userID string) error {
	err := condIncrScript.Run(ctx, s.client, []string{followersCountKey(userID)}).Err()
	if err != nil && !errors.Is(err, redis.Nil) {
		return fmt.Errorf("redis cond incr followers count: %w", err)
	}
	return nil
}

// CondDecrFollowersCount atomically decrements the followers count only if the key exists.
func (s *RedisFollowStore) CondDecrFollowersCount(ctx context.Context, userID string) error {
	err := condDecrScript.Run(ctx, s.client, []string{followersCountKey(userID)}).Err()
	if err != nil && !errors.Is(err, redis.Nil) {
		return fmt.Errorf("redis cond decr followers count: %w", err)
	}
	return nil
}

// RecordAccess increments the access score for a user in the hot key sorted set.
func (s *RedisFollowStore) RecordAccess(ctx context.Context, userID string) error {
	err := s.client.ZIncrBy(ctx, hotKeyScoresKey, 1, userID).Err()
	if err != nil {
		return fmt.Errorf("redis record access: %w", err)
	}
	return nil
}

// GetTopHotKeys returns the top-n most accessed user IDs.
func (s *RedisFollowStore) GetTopHotKeys(ctx context.Context, n int64) ([]string, error) {
	keys, err := s.client.ZRevRange(ctx, hotKeyScoresKey, 0, n-1).Result()
	if err != nil {
		return nil, fmt.Errorf("redis get top hot keys: %w", err)
	}
	return keys, nil
}

// ResetHotKeyScores deletes the hot key scores sorted set.
func (s *RedisFollowStore) ResetHotKeyScores(ctx context.Context) error {
	err := s.client.Del(ctx, hotKeyScoresKey).Err()
	if err != nil {
		return fmt.Errorf("redis reset hot key scores: %w", err)
	}
	return nil
}

// Close closes the Redis client.
func (s *RedisFollowStore) Close() error {
	return s.client.Close()
}

// Ensure interface is satisfied at compile time.
var _ FollowStore = (*RedisFollowStore)(nil)
