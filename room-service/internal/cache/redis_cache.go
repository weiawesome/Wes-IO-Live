package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/weiawesome/wes-io-live/room-service/internal/config"
)

var ErrCacheMiss = errors.New("cache miss")

type RedisRoomCache struct {
	client *redis.Client
	prefix string
}

func NewRedisRoomCache(cfg config.RedisConfig, prefix string) (*RedisRoomCache, error) {
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

	return &RedisRoomCache{
		client: client,
		prefix: prefix,
	}, nil
}

func (c *RedisRoomCache) BuildKeyByID(roomID string) string {
	return fmt.Sprintf("%s:id:%s", c.prefix, roomID)
}

func (c *RedisRoomCache) Get(ctx context.Context, key string) (*RoomCacheResult, error) {
	data, err := c.client.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, ErrCacheMiss
		}
		return nil, fmt.Errorf("failed to get from redis: %w", err)
	}

	var result RoomCacheResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal cache data: %w", err)
	}

	return &result, nil
}

func (c *RedisRoomCache) Set(ctx context.Context, key string, result *RoomCacheResult, ttl time.Duration) error {
	data, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal cache data: %w", err)
	}

	if err := c.client.Set(ctx, key, data, ttl).Err(); err != nil {
		return fmt.Errorf("failed to set in redis: %w", err)
	}

	return nil
}

func (c *RedisRoomCache) Delete(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}

	if err := c.client.Del(ctx, keys...).Err(); err != nil {
		return fmt.Errorf("failed to delete from redis: %w", err)
	}

	return nil
}

func (c *RedisRoomCache) Close() error {
	return c.client.Close()
}
