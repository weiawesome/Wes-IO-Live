package consumer

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	pkglog "github.com/weiawesome/wes-io-live/pkg/log"
)

// ConfluentConsumer implements AvatarProcessedConsumer using confluent-kafka-go.
type ConfluentConsumer struct {
	consumer *kafka.Consumer
	topic    string
	handler  AvatarProcessedHandler
	doneCh   chan struct{}
}

// NewConfluentConsumer creates a new Kafka consumer for avatar-processed events.
func NewConfluentConsumer(brokers, topic, groupID string, handler AvatarProcessedHandler) (*ConfluentConsumer, error) {
	c, err := kafka.NewConsumer(&kafka.ConfigMap{
		"bootstrap.servers":  brokers,
		"group.id":           groupID,
		"auto.offset.reset":  "earliest",
		"enable.auto.commit": true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create kafka consumer: %w", err)
	}

	return &ConfluentConsumer{
		consumer: c,
		topic:    topic,
		handler:  handler,
		doneCh:   make(chan struct{}),
	}, nil
}

// Start begins consuming messages from Kafka.
func (cc *ConfluentConsumer) Start(ctx context.Context) error {
	if err := cc.consumer.Subscribe(cc.topic, nil); err != nil {
		return fmt.Errorf("failed to subscribe to topic %s: %w", cc.topic, err)
	}

	l := pkglog.L()
	l.Info().Str("topic", cc.topic).Msg("avatar-processed consumer started")

	go cc.consumeLoop(ctx)

	return nil
}

func (cc *ConfluentConsumer) consumeLoop(ctx context.Context) {
	l := pkglog.L()
	defer close(cc.doneCh)

	for {
		select {
		case <-ctx.Done():
			l.Info().Msg("avatar-processed consumer shutting down")
			return
		default:
			msg, err := cc.consumer.ReadMessage(100 * time.Millisecond)
			if err != nil {
				if err.(kafka.Error).Code() == kafka.ErrTimedOut {
					continue
				}
				l.Error().Err(err).Msg("avatar-processed consumer error")
				continue
			}

			cc.processMessage(ctx, msg)
		}
	}
}

func (cc *ConfluentConsumer) processMessage(ctx context.Context, msg *kafka.Message) {
	l := pkglog.L()

	var event AvatarProcessedEvent
	if err := json.Unmarshal(msg.Value, &event); err != nil {
		l.Error().Err(err).Msg("failed to unmarshal avatar-processed event")
		return
	}

	l.Info().
		Str("user_id", event.UserID).
		Msg("received avatar-processed event")

	if err := cc.handler.HandleAvatarProcessed(ctx, &event); err != nil {
		l.Error().Err(err).Str("user_id", event.UserID).Msg("failed to handle avatar-processed event")
	}
}

// Close stops the consumer and releases resources.
func (cc *ConfluentConsumer) Close() error {
	if err := cc.consumer.Close(); err != nil {
		return fmt.Errorf("failed to close kafka consumer: %w", err)
	}
	<-cc.doneCh
	return nil
}
