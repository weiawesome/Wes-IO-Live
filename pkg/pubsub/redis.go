package pubsub

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/redis/go-redis/v9"
)

// RedisPubSub implements PubSub interface using Redis.
type RedisPubSub struct {
	client        *redis.Client
	subscriptions map[string]*redis.PubSub
	mu            sync.RWMutex
}

// NewRedisPubSub creates a new Redis-based PubSub instance.
func NewRedisPubSub(cfg RedisConfig) (*RedisPubSub, error) {
	client := redis.NewClient(&redis.Options{
		Addr:         cfg.Address,
		Password:     cfg.Password,
		DB:           cfg.DB,
		PoolSize:     cfg.PoolSize,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	})

	// Test connection
	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to redis: %w", err)
	}

	return &RedisPubSub{
		client:        client,
		subscriptions: make(map[string]*redis.PubSub),
	}, nil
}

// Publish publishes an event to the specified channel.
func (r *RedisPubSub) Publish(ctx context.Context, channel string, event *Event) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	return r.client.Publish(ctx, channel, data).Err()
}

// Subscribe subscribes to a specific channel.
func (r *RedisPubSub) Subscribe(ctx context.Context, channel string) (<-chan *Event, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	pubsub := r.client.Subscribe(ctx, channel)
	r.subscriptions[channel] = pubsub

	eventCh := make(chan *Event, 100)

	go r.processMessages(ctx, pubsub, eventCh)

	return eventCh, nil
}

// SubscribePattern subscribes to channels matching a pattern.
func (r *RedisPubSub) SubscribePattern(ctx context.Context, pattern string) (<-chan *Event, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	pubsub := r.client.PSubscribe(ctx, pattern)
	r.subscriptions[pattern] = pubsub

	eventCh := make(chan *Event, 100)

	go r.processMessages(ctx, pubsub, eventCh)

	return eventCh, nil
}

// Unsubscribe unsubscribes from a channel.
func (r *RedisPubSub) Unsubscribe(ctx context.Context, channel string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if pubsub, ok := r.subscriptions[channel]; ok {
		if err := pubsub.Close(); err != nil {
			return err
		}
		delete(r.subscriptions, channel)
	}

	return nil
}

// Close closes all subscriptions and the Redis client.
func (r *RedisPubSub) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, pubsub := range r.subscriptions {
		pubsub.Close()
	}
	r.subscriptions = make(map[string]*redis.PubSub)

	return r.client.Close()
}

// processMessages reads messages from the Redis pubsub and sends them to the event channel.
func (r *RedisPubSub) processMessages(ctx context.Context, pubsub *redis.PubSub, eventCh chan<- *Event) {
	defer close(eventCh)

	ch := pubsub.Channel()

	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}

			var event Event
			if err := json.Unmarshal([]byte(msg.Payload), &event); err != nil {
				continue
			}

			select {
			case eventCh <- &event:
			case <-ctx.Done():
				return
			default:
				// Channel full, skip message
			}
		}
	}
}

// GetClient returns the underlying Redis client for advanced operations.
func (r *RedisPubSub) GetClient() *redis.Client {
	return r.client
}
