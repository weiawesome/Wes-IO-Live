package consumer

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/weiawesome/wes-io-live/chat-persist-service/internal/cassandra"
	"github.com/weiawesome/wes-io-live/chat-persist-service/internal/config"
	"github.com/weiawesome/wes-io-live/chat-persist-service/internal/domain"
	"github.com/weiawesome/wes-io-live/pkg/log"
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

	l := log.L()
	l.Info().Str("topic", c.topic).Str("group", c.groupID).Msg("kafka consumer started")

	for {
		select {
		case <-ctx.Done():
			l.Info().Msg("kafka consumer stopping")
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
				cl := log.Ctx(ctx)
				cl.Error().
					Int32("partition", e.TopicPartition.Partition).
					Str("offset", e.TopicPartition.Offset.String()).
					Err(err).
					Msg("handle message error")
			}
		case kafka.Error:
			l.Error().
				Int("code", int(e.Code())).
				Bool("fatal", e.IsFatal()).
				Err(e).
				Msg("kafka error")
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

	cl := log.Ctx(ctx)
	cl.Info().
		Str("room_id", msg.RoomID).
		Str("session_id", msg.SessionID).
		Str("message_id", msg.MessageID).
		Msg("message persisted")

	return nil
}

// Close closes the Kafka consumer.
func (c *Consumer) Close() error {
	l := log.L()
	l.Info().Msg("closing kafka consumer")
	return c.consumer.Close()
}
