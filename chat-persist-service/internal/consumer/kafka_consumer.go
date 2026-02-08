package consumer

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/weiawesome/wes-io-live/chat-persist-service/internal/cassandra"
	"github.com/weiawesome/wes-io-live/chat-persist-service/internal/config"
	"github.com/weiawesome/wes-io-live/chat-persist-service/internal/domain"
)

// Consumer consumes messages from Kafka and persists them to Cassandra.
type Consumer struct {
	consumer   *kafka.Consumer
	topic      string
	groupID    string
	repository *cassandra.MessageRepository
}

// NewConsumer creates a new Kafka consumer.
func NewConsumer(cfg config.KafkaConfig, repo *cassandra.MessageRepository) (*Consumer, error) {
	c, err := kafka.NewConsumer(&kafka.ConfigMap{
		"bootstrap.servers":       cfg.Brokers,
		"group.id":                cfg.GroupID,
		"auto.offset.reset":       cfg.AutoOffsetReset,
		"enable.auto.commit":      true,
		"auto.commit.interval.ms": 5000,
		"max.poll.interval.ms":    cfg.MaxPollIntervalMs,
		"session.timeout.ms":      cfg.SessionTimeoutMs,
		"heartbeat.interval.ms":   cfg.HeartbeatIntervalMs,
		"fetch.min.bytes":         cfg.FetchMinBytes,
		"fetch.wait.max.ms":       cfg.FetchMaxWaitMs,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create kafka consumer: %w", err)
	}

	return &Consumer{
		consumer:   c,
		topic:      cfg.Topic,
		groupID:    cfg.GroupID,
		repository: repo,
	}, nil
}

// Run starts consuming messages from Kafka.
func (c *Consumer) Run(ctx context.Context) error {
	if err := c.consumer.Subscribe(c.topic, nil); err != nil {
		return fmt.Errorf("failed to subscribe to topic %s: %w", c.topic, err)
	}

	log.Printf("Kafka consumer started (topic: %s, group: %s)", c.topic, c.groupID)

	for {
		select {
		case <-ctx.Done():
			log.Println("Kafka consumer stopping...")
			return nil
		default:
		}

		ev := c.consumer.Poll(500)
		if ev == nil {
			continue
		}

		switch e := ev.(type) {
		case *kafka.Message:
			if err := c.handleMessage(ctx, e.Value); err != nil {
				log.Printf("HandleMessage error (partition=%d offset=%v): %v",
					e.TopicPartition.Partition, e.TopicPartition.Offset, err)
			}
		case kafka.Error:
			log.Printf("Kafka error: %v (code=%d fatal=%v)", e, e.Code(), e.IsFatal())
			if e.IsFatal() {
				return fmt.Errorf("fatal kafka error: %w", e)
			}
		case kafka.OffsetsCommitted:
			// Normal auto-commit acknowledgement
		default:
			// Ignore other events (rebalance, etc.)
		}
	}
}

// handleMessage processes a single Kafka message.
func (c *Consumer) handleMessage(ctx context.Context, value []byte) error {
	var msg domain.ChatMessage
	if err := json.Unmarshal(value, &msg); err != nil {
		return fmt.Errorf("failed to unmarshal message: %w", err)
	}

	if err := c.repository.SaveMessage(ctx, &msg); err != nil {
		return fmt.Errorf("failed to persist message: %w", err)
	}

	log.Printf("Persisted message: room=%s session=%s message_id=%s",
		msg.RoomID, msg.SessionID, msg.MessageID)

	return nil
}

// Close closes the Kafka consumer.
func (c *Consumer) Close() error {
	log.Println("Closing Kafka consumer...")
	return c.consumer.Close()
}
