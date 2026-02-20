package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/weiawesome/wes-io-live/search-service/internal/config"
	"github.com/weiawesome/wes-io-live/search-service/internal/domain"
)

var ErrCacheMiss = errors.New("cache miss")

type RedisSearchCache struct {
	client *redis.Client
	prefix string
}

// NewRedisSearchCache creates a new Redis-based search cache.
func NewRedisSearchCache(cfg config.RedisConfig, prefix string) (*RedisSearchCache, error) {
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

	return &RedisSearchCache{
		client: client,
		prefix: prefix,
	}, nil
}

// BuildKey creates a cache key from search parameters.
func (c *RedisSearchCache) BuildKey(typ, query string, offset, limit int) string {
	return fmt.Sprintf("%s:%s:%s:%d:%d", c.prefix, typ, query, offset, limit)
}

func (c *RedisSearchCache) Get(ctx context.Context, key string) (*domain.SearchResponse, error) {
	data, err := c.client.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, ErrCacheMiss
		}
		return nil, fmt.Errorf("failed to get from redis: %w", err)
	}

	var result domain.SearchResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal cache data: %w", err)
	}

	return &result, nil
}

func (c *RedisSearchCache) Set(ctx context.Context, key string, result *domain.SearchResponse, ttl time.Duration) error {
	data, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal cache data: %w", err)
	}

	if err := c.client.Set(ctx, key, data, ttl).Err(); err != nil {
		return fmt.Errorf("failed to set in redis: %w", err)
	}

	return nil
}

func (c *RedisSearchCache) Delete(ctx context.Context, key string) error {
	if err := c.client.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("failed to delete from redis: %w", err)
	}

	return nil
}

func (c *RedisSearchCache) Close() error {
	return c.client.Close()
}
