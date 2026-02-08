package registry

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/weiawesome/wes-io-live/chat-dispatcher/internal/config"
)

type RedisLookupClient struct {
	client        *redis.Client
	prefix        string
	lookupTimeout time.Duration
}

func NewRedisLookupClient(cfg config.RedisConfig) (*RedisLookupClient, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Address,
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to redis: %w", err)
	}

	return &RedisLookupClient{
		client:        client,
		prefix:        cfg.RegistryPrefix,
		lookupTimeout: cfg.LookupTimeout,
	}, nil
}

// keyFor builds the registry key matching chat-service format:
// {prefix}:room:{roomID}:session:{sessionID}
func (r *RedisLookupClient) keyFor(roomID, sessionID string) string {
	return fmt.Sprintf("%s:room:%s:session:%s", r.prefix, roomID, sessionID)
}

func (r *RedisLookupClient) Lookup(ctx context.Context, roomID, sessionID string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, r.lookupTimeout)
	defer cancel()

	key := r.keyFor(roomID, sessionID)
	addr, err := r.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", fmt.Errorf("room-session not registered: %s:%s", roomID, sessionID)
	}
	if err != nil {
		return "", fmt.Errorf("failed to lookup room-session: %w", err)
	}

	return addr, nil
}

func (r *RedisLookupClient) Close() error {
	return r.client.Close()
}
