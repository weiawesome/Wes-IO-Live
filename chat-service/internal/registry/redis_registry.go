package registry

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/weiawesome/wes-io-live/chat-service/internal/config"
	"github.com/weiawesome/wes-io-live/pkg/log"
)

type RedisRegistry struct {
	client            *redis.Client
	advertiseAddress  string
	prefix            string
	keyTTL            time.Duration
	heartbeatInterval time.Duration
	managedKeys       map[string]struct{} // keys managed by this instance
	mu                sync.RWMutex
	cancel            context.CancelFunc
}

func NewRedisRegistry(cfg config.RedisConfig, advertiseAddress string) (*RedisRegistry, error) {
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

	return &RedisRegistry{
		client:            client,
		advertiseAddress:  advertiseAddress,
		prefix:            cfg.RegistryPrefix,
		keyTTL:            cfg.KeyTTL,
		heartbeatInterval: cfg.HeartbeatInterval,
		managedKeys:       make(map[string]struct{}),
	}, nil
}

func (r *RedisRegistry) keyFor(roomID, sessionID string) string {
	return fmt.Sprintf("%s:room:%s:session:%s", r.prefix, roomID, sessionID)
}

func (r *RedisRegistry) Register(ctx context.Context, roomID, sessionID string) error {
	key := r.keyFor(roomID, sessionID)

	if err := r.client.Set(ctx, key, r.advertiseAddress, r.keyTTL).Err(); err != nil {
		return fmt.Errorf("failed to register room-session: %w", err)
	}

	r.mu.Lock()
	r.managedKeys[key] = struct{}{}
	r.mu.Unlock()

	l := log.L()
	l.Info().Str("room_id", roomID).Str("session_id", sessionID).Str("address", r.advertiseAddress).Msg("registered room-session")
	return nil
}

func (r *RedisRegistry) Deregister(ctx context.Context, roomID, sessionID string) error {
	key := r.keyFor(roomID, sessionID)

	if err := r.client.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("failed to deregister room-session: %w", err)
	}

	r.mu.Lock()
	delete(r.managedKeys, key)
	r.mu.Unlock()

	l := log.L()
	l.Info().Str("room_id", roomID).Str("session_id", sessionID).Msg("deregistered room-session")
	return nil
}

func (r *RedisRegistry) Lookup(ctx context.Context, roomID, sessionID string) (string, error) {
	key := r.keyFor(roomID, sessionID)

	addr, err := r.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", fmt.Errorf("room-session %s:%s not found", roomID, sessionID)
	}
	if err != nil {
		return "", fmt.Errorf("failed to lookup room-session: %w", err)
	}

	return addr, nil
}

func (r *RedisRegistry) StartHeartbeat(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	r.cancel = cancel

	go r.heartbeatLoop(ctx)
	l := log.L()
	l.Info().Dur("interval", r.heartbeatInterval).Dur("ttl", r.keyTTL).Msg("registry heartbeat started")
	return nil
}

func (r *RedisRegistry) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(r.heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.refreshKeys(ctx)
		}
	}
}

func (r *RedisRegistry) refreshKeys(ctx context.Context) {
	r.mu.RLock()
	keys := make([]string, 0, len(r.managedKeys))
	for k := range r.managedKeys {
		keys = append(keys, k)
	}
	r.mu.RUnlock()

	for _, key := range keys {
		if err := r.client.Set(ctx, key, r.advertiseAddress, r.keyTTL).Err(); err != nil {
			l := log.L()
			l.Error().Str("key", key).Err(err).Msg("failed to refresh key")
		}
	}
}

func (r *RedisRegistry) StopHeartbeat() {
	if r.cancel != nil {
		r.cancel()
	}
}

func (r *RedisRegistry) Close() error {
	r.StopHeartbeat()
	return r.client.Close()
}
