package pubsub

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
)

// channelToTopicAndKey converts a Redis-style channel to a Kafka topic and message key.
//
//	"signal:room:ROOM123:to_media" → topic: "signal-to-media", key: "ROOM123"
//	"media:room:ROOM456:to_signal" → topic: "media-to-signal", key: "ROOM456"
func channelToTopicAndKey(channel string) (topic, key string, err error) {
	// Expected format: {prefix}:room:{roomID}:to_{target}
	parts := strings.Split(channel, ":")
	if len(parts) != 4 || parts[1] != "room" {
		return "", "", fmt.Errorf("invalid channel format: %s", channel)
	}
	prefix := parts[0]  // "signal" or "media"
	roomID := parts[2]  // the room ID
	suffix := parts[3]  // "to_media" or "to_signal"

	topic = prefix + "-" + strings.ReplaceAll(suffix, "_", "-")
	return topic, roomID, nil
}

// patternToTopic converts a Redis-style subscribe pattern to a Kafka topic.
//
//	"signal:room:*:to_media" → "signal-to-media"
//	"media:room:*:to_signal" → "media-to-signal"
func patternToTopic(pattern string) (string, error) {
	// Replace wildcard with a placeholder, reuse channelToTopicAndKey
	channel := strings.ReplaceAll(pattern, "*", "_placeholder_")
	topic, _, err := channelToTopicAndKey(channel)
	return topic, err
}

// channelToTopicAndFilter converts a specific channel to topic + a roomID filter for Subscribe.
func channelToTopicAndFilter(channel string) (topic, roomID string, err error) {
	return channelToTopicAndKey(channel)
}

// kafkaSubscription tracks a single consumer subscription.
type kafkaSubscription struct {
	consumer *kafka.Consumer
	cancel   context.CancelFunc
}

// KafkaPubSub implements PubSub interface using Apache Kafka.
type KafkaPubSub struct {
	producer      *kafka.Producer
	subscriptions map[string]*kafkaSubscription // key (channel or pattern) → subscription
	config        KafkaConfig
	mu            sync.Mutex
	doneCh        chan struct{}
}

// NewKafkaPubSub creates a new Kafka-based PubSub instance.
func NewKafkaPubSub(cfg KafkaConfig) (*KafkaPubSub, error) {
	p, err := kafka.NewProducer(&kafka.ConfigMap{
		"bootstrap.servers": cfg.Brokers,
		"acks":              "1",
		"linger.ms":         5,
		"compression.type":  "snappy",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create kafka producer: %w", err)
	}

	kps := &KafkaPubSub{
		producer:      p,
		subscriptions: make(map[string]*kafkaSubscription),
		config:        cfg,
		doneCh:        make(chan struct{}),
	}

	go kps.deliveryReportHandler()

	// Ensure the two fixed topics exist
	if err := kps.ensureTopics(); err != nil {
		log.Printf("Warning: failed to ensure Kafka topics: %v (may already exist)", err)
	}

	return kps, nil
}

// ensureTopics creates the fixed topics if they don't exist.
func (k *KafkaPubSub) ensureTopics() error {
	admin, err := kafka.NewAdminClient(&kafka.ConfigMap{
		"bootstrap.servers": k.config.Brokers,
	})
	if err != nil {
		return fmt.Errorf("failed to create admin client: %w", err)
	}
	defer admin.Close()

	partitions := k.config.Partitions
	if partitions <= 0 {
		partitions = 4
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	topics := []kafka.TopicSpecification{
		{
			Topic:             "signal-to-media",
			NumPartitions:     partitions,
			ReplicationFactor: 1,
		},
		{
			Topic:             "media-to-signal",
			NumPartitions:     partitions,
			ReplicationFactor: 1,
		},
	}

	results, err := admin.CreateTopics(ctx, topics)
	if err != nil {
		return fmt.Errorf("failed to create topics: %w", err)
	}

	for _, r := range results {
		if r.Error.Code() != kafka.ErrNoError && r.Error.Code() != kafka.ErrTopicAlreadyExists {
			log.Printf("Warning: failed to create topic %s: %v", r.Topic, r.Error)
		}
	}

	return nil
}

// deliveryReportHandler processes delivery reports from the producer.
func (k *KafkaPubSub) deliveryReportHandler() {
	for e := range k.producer.Events() {
		switch ev := e.(type) {
		case *kafka.Message:
			if ev.TopicPartition.Error != nil {
				log.Printf("Kafka pubsub delivery failed: %v", ev.TopicPartition.Error)
			}
		}
	}
	close(k.doneCh)
}

// Publish publishes an event to the specified channel (converted to Kafka topic + key).
func (k *KafkaPubSub) Publish(ctx context.Context, channel string, event *Event) error {
	topic, key, err := channelToTopicAndKey(channel)
	if err != nil {
		return fmt.Errorf("failed to parse channel: %w", err)
	}

	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	err = k.producer.Produce(&kafka.Message{
		TopicPartition: kafka.TopicPartition{
			Topic:     &topic,
			Partition: kafka.PartitionAny,
		},
		Key:   []byte(key),
		Value: data,
	}, nil)
	if err != nil {
		return fmt.Errorf("failed to produce message: %w", err)
	}

	return nil
}

// Subscribe subscribes to a specific channel, filtering messages by roomID.
func (k *KafkaPubSub) Subscribe(ctx context.Context, channel string) (<-chan *Event, error) {
	topic, roomID, err := channelToTopicAndFilter(channel)
	if err != nil {
		return nil, fmt.Errorf("failed to parse channel: %w", err)
	}

	return k.subscribeToTopic(ctx, channel, topic, roomID)
}

// SubscribePattern subscribes to channels matching a pattern (consumes all messages on the topic).
func (k *KafkaPubSub) SubscribePattern(ctx context.Context, pattern string) (<-chan *Event, error) {
	topic, err := patternToTopic(pattern)
	if err != nil {
		return nil, fmt.Errorf("failed to parse pattern: %w", err)
	}

	return k.subscribeToTopic(ctx, pattern, topic, "")
}

// subscribeToTopic creates a consumer for a topic, optionally filtering by roomID.
func (k *KafkaPubSub) subscribeToTopic(ctx context.Context, subKey, topic, filterRoomID string) (<-chan *Event, error) {
	k.mu.Lock()
	defer k.mu.Unlock()

	// Close existing subscription for this key if any
	if existing, ok := k.subscriptions[subKey]; ok {
		existing.cancel()
		existing.consumer.Close()
		delete(k.subscriptions, subKey)
	}

	groupID := k.config.GroupID
	if groupID == "" {
		groupID = "pubsub-default"
	}

	// For specific channel subscriptions, use a unique group ID per channel
	// to avoid competing with pattern subscriptions
	consumerGroupID := groupID
	if filterRoomID != "" {
		consumerGroupID = fmt.Sprintf("%s-%s", groupID, sanitizeGroupID(subKey))
	}

	c, err := kafka.NewConsumer(&kafka.ConfigMap{
		"bootstrap.servers":       k.config.Brokers,
		"group.id":                consumerGroupID,
		"auto.offset.reset":       "latest",
		"enable.auto.commit":      true,
		"auto.commit.interval.ms": 5000,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create kafka consumer: %w", err)
	}

	if err := c.Subscribe(topic, nil); err != nil {
		c.Close()
		return nil, fmt.Errorf("failed to subscribe to topic %s: %w", topic, err)
	}

	subCtx, cancel := context.WithCancel(ctx)
	eventCh := make(chan *Event, 100)

	k.subscriptions[subKey] = &kafkaSubscription{
		consumer: c,
		cancel:   cancel,
	}

	go k.consumeMessages(subCtx, c, eventCh, filterRoomID)

	return eventCh, nil
}

// consumeMessages polls Kafka and forwards events to the channel.
func (k *KafkaPubSub) consumeMessages(ctx context.Context, c *kafka.Consumer, eventCh chan<- *Event, filterRoomID string) {
	defer close(eventCh)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		ev := c.Poll(500)
		if ev == nil {
			continue
		}

		switch e := ev.(type) {
		case *kafka.Message:
			// If filtering by roomID, check the message key
			if filterRoomID != "" && string(e.Key) != filterRoomID {
				continue
			}

			var event Event
			if err := json.Unmarshal(e.Value, &event); err != nil {
				log.Printf("Kafka pubsub: failed to unmarshal event: %v", err)
				continue
			}

			select {
			case eventCh <- &event:
			case <-ctx.Done():
				return
			default:
				// Channel full, skip message
			}

		case kafka.Error:
			log.Printf("Kafka pubsub error: %v (code=%d fatal=%v)", e, e.Code(), e.IsFatal())
			if e.IsFatal() {
				return
			}

		case kafka.OffsetsCommitted:
			// Normal auto-commit
		default:
			// Ignore other events
		}
	}
}

// Unsubscribe unsubscribes from a channel or pattern.
func (k *KafkaPubSub) Unsubscribe(ctx context.Context, channel string) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	if sub, ok := k.subscriptions[channel]; ok {
		sub.cancel()
		if err := sub.consumer.Close(); err != nil {
			return fmt.Errorf("failed to close consumer: %w", err)
		}
		delete(k.subscriptions, channel)
	}

	return nil
}

// Close closes all subscriptions and the producer.
func (k *KafkaPubSub) Close() error {
	k.mu.Lock()
	defer k.mu.Unlock()

	for key, sub := range k.subscriptions {
		sub.cancel()
		sub.consumer.Close()
		delete(k.subscriptions, key)
	}

	k.producer.Flush(5000)
	k.producer.Close()
	<-k.doneCh

	return nil
}

// sanitizeGroupID replaces characters not suitable for Kafka group IDs.
var groupIDRegexp = regexp.MustCompile(`[^a-zA-Z0-9._-]`)

func sanitizeGroupID(s string) string {
	return groupIDRegexp.ReplaceAllString(s, "-")
}
