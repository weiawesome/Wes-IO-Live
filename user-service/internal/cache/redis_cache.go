package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/weiawesome/wes-io-live/user-service/internal/config"
)

var ErrCacheMiss = errors.New("cache miss")

type RedisUserCache struct {
	client *redis.Client
	prefix string
}

func NewRedisUserCache(cfg config.RedisConfig, prefix string) (*RedisUserCache, error) {
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

	return &RedisUserCache{
		client: client,
		prefix: prefix,
	}, nil
}

func (c *RedisUserCache) BuildKeyByID(userID string) string {
	return fmt.Sprintf("%s:id:%s", c.prefix, userID)
}

func (c *RedisUserCache) BuildKeyByEmail(email string) string {
	return fmt.Sprintf("%s:email:%s", c.prefix, email)
}

func (c *RedisUserCache) Get(ctx context.Context, key string) (*UserCacheResult, error) {
	data, err := c.client.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, ErrCacheMiss
		}
		return nil, fmt.Errorf("failed to get from redis: %w", err)
	}

	var result UserCacheResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal cache data: %w", err)
	}

	return &result, nil
}

func (c *RedisUserCache) Set(ctx context.Context, key string, result *UserCacheResult, ttl time.Duration) error {
	data, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal cache data: %w", err)
	}

	if err := c.client.Set(ctx, key, data, ttl).Err(); err != nil {
		return fmt.Errorf("failed to set in redis: %w", err)
	}

	return nil
}

func (c *RedisUserCache) Delete(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}

	if err := c.client.Del(ctx, keys...).Err(); err != nil {
		return fmt.Errorf("failed to delete from redis: %w", err)
	}

	return nil
}

func (c *RedisUserCache) Close() error {
	return c.client.Close()
}
