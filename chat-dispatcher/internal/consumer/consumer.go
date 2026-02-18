package consumer

import (
	"context"
	"fmt"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/weiawesome/wes-io-live/chat-dispatcher/internal/config"
	"github.com/weiawesome/wes-io-live/chat-dispatcher/internal/dispatcher"
	"github.com/weiawesome/wes-io-live/pkg/log"
)

type Consumer struct {
	consumer   *kafka.Consumer
	topic      string
	groupID    string
	dispatcher *dispatcher.Dispatcher
}

func NewConsumer(cfg config.KafkaConfig, d *dispatcher.Dispatcher) (*Consumer, error) {
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
		dispatcher: d,
	}, nil
}

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
			if err := c.dispatcher.HandleMessage(ctx, e.Key, e.Value); err != nil {
				cl := log.Ctx(ctx)
				cl.Error().
					Int32("partition", e.TopicPartition.Partition).
					Str("offset", e.TopicPartition.Offset.String()).
					Err(err).
					Msg("handle message error")
			}
		case kafka.Error:
			l.Error().Int("code", int(e.Code())).Bool("fatal", e.IsFatal()).Err(e).Msg("kafka error")
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

func (c *Consumer) Close() error {
	l := log.L()
	l.Info().Msg("closing kafka consumer")
	return c.consumer.Close()
}
