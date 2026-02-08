package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
)

// ConfluentConsumer implements BroadcastEventConsumer using confluent-kafka-go.
type ConfluentConsumer struct {
	consumer *kafka.Consumer
	topic    string
	handler  BroadcastEventHandler
	doneCh   chan struct{}
}

// NewConfluentConsumer creates a new Kafka consumer for broadcast events.
func NewConfluentConsumer(brokers, topic, groupID string, handler BroadcastEventHandler) (*ConfluentConsumer, error) {
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

// Start begins consuming messages from Kafka.
func (cc *ConfluentConsumer) Start(ctx context.Context) error {
	if err := cc.consumer.Subscribe(cc.topic, nil); err != nil {
		return fmt.Errorf("failed to subscribe to topic %s: %w", cc.topic, err)
	}

	log.Printf("Kafka consumer started, subscribed to topic: %s", cc.topic)

	go cc.consumeLoop(ctx)

	return nil
}

func (cc *ConfluentConsumer) consumeLoop(ctx context.Context) {
	defer close(cc.doneCh)

	for {
		select {
		case <-ctx.Done():
			log.Println("Kafka consumer shutting down...")
			return
		default:
			msg, err := cc.consumer.ReadMessage(100 * time.Millisecond)
			if err != nil {
				// Timeout is expected, continue
				if err.(kafka.Error).Code() == kafka.ErrTimedOut {
					continue
				}
				log.Printf("Kafka consumer error: %v", err)
				continue
			}

			cc.processMessage(ctx, msg)
		}
	}
}

func (cc *ConfluentConsumer) processMessage(ctx context.Context, msg *kafka.Message) {
	var event BroadcastEvent
	if err := json.Unmarshal(msg.Value, &event); err != nil {
		log.Printf("Failed to unmarshal broadcast event: %v", err)
		return
	}

	log.Printf("Received broadcast event: type=%s room=%s broadcaster=%s reason=%s",
		event.Type, event.RoomID, event.BroadcasterID, event.Reason)

	if err := cc.handler.HandleBroadcastEvent(ctx, &event); err != nil {
		log.Printf("Failed to handle broadcast event: %v", err)
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
