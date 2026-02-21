package consumer

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	pkglog "github.com/weiawesome/wes-io-live/pkg/log"
)

// ConfluentConsumer implements CDCEventConsumer using confluent-kafka-go.
type ConfluentConsumer struct {
	consumer *kafka.Consumer
	topic    string
	handler  CDCEventHandler
	doneCh   chan struct{}
}

// NewConfluentConsumer creates a new Kafka consumer for CDC events.
func NewConfluentConsumer(brokers, topic, groupID string, handler CDCEventHandler) (*ConfluentConsumer, error) {
	c, err := kafka.NewConsumer(&kafka.ConfigMap{
		"bootstrap.servers":  brokers,
		"group.id":           groupID,
		"auto.offset.reset":  "latest",
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

// Start begins consuming CDC messages from Kafka.
func (cc *ConfluentConsumer) Start(ctx context.Context) error {
	if err := cc.consumer.Subscribe(cc.topic, nil); err != nil {
		return fmt.Errorf("failed to subscribe to topic %s: %w", cc.topic, err)
	}

	l := pkglog.L()
	l.Info().Str("topic", cc.topic).Msg("kafka CDC consumer started")

	go cc.consumeLoop(ctx)

	return nil
}

func (cc *ConfluentConsumer) consumeLoop(ctx context.Context) {
	l := pkglog.L()
	defer close(cc.doneCh)

	for {
		select {
		case <-ctx.Done():
			l.Info().Msg("kafka CDC consumer shutting down")
			return
		default:
			msg, err := cc.consumer.ReadMessage(100 * time.Millisecond)
			if err != nil {
				if err.(kafka.Error).Code() == kafka.ErrTimedOut {
					continue
				}
				l.Error().Err(err).Msg("kafka CDC consumer error")
				continue
			}

			cc.processMessage(context.WithoutCancel(ctx), msg)
		}
	}
}

func (cc *ConfluentConsumer) processMessage(ctx context.Context, msg *kafka.Message) {
	l := pkglog.L()

	var event DebeziumMessage
	if err := json.Unmarshal(msg.Value, &event); err != nil {
		l.Error().Err(err).Msg("failed to unmarshal debezium CDC event")
		return
	}

	l.Info().
		Str("op", event.Payload.Op).
		Int64("ts_ms", event.Payload.TsMs).
		Msg("received CDC event")

	if err := cc.handler.HandleCDCEvent(ctx, &event); err != nil {
		l.Error().Err(err).Str("op", event.Payload.Op).Msg("failed to handle CDC event")
	}
}

// Close stops the consumer and releases resources.
// It waits for any in-flight processMessage call to complete before closing.
func (cc *ConfluentConsumer) Close() error {
	<-cc.doneCh // wait for in-flight processMessage to complete
	if err := cc.consumer.Close(); err != nil {
		return fmt.Errorf("failed to close kafka consumer: %w", err)
	}
	return nil
}
