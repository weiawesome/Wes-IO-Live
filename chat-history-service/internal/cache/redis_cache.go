package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/weiawesome/wes-io-live/chat-history-service/internal/config"
	"github.com/redis/go-redis/v9"
)

var ErrCacheMiss = errors.New("cache miss")

type RedisMessageCache struct {
	client *redis.Client
	prefix string
}

func NewRedisMessageCache(cfg config.RedisConfig, prefix string) (*RedisMessageCache, error) {
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

	return &RedisMessageCache{
		client: client,
		prefix: prefix,
	}, nil
}

func (c *RedisMessageCache) BuildKey(roomID, sessionID, cursor, direction string, limit int) string {
	if cursor == "" {
		cursor = "start"
	}
	return fmt.Sprintf("%s:%s:%s:%s:%s:%d", c.prefix, roomID, sessionID, cursor, direction, limit)
}

func (c *RedisMessageCache) Get(ctx context.Context, key string) (*MessageCacheResult, error) {
	data, err := c.client.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, ErrCacheMiss
		}
		return nil, fmt.Errorf("failed to get from redis: %w", err)
	}

	var result MessageCacheResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal cache data: %w", err)
	}

	return &result, nil
}

func (c *RedisMessageCache) Set(ctx context.Context, key string, result *MessageCacheResult, ttl time.Duration) error {
	data, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal cache data: %w", err)
	}

	if err := c.client.Set(ctx, key, data, ttl).Err(); err != nil {
		return fmt.Errorf("failed to set in redis: %w", err)
	}

	return nil
}

func (c *RedisMessageCache) Close() error {
	return c.client.Close()
}
