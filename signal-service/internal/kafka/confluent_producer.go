package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	pkglog "github.com/weiawesome/wes-io-live/pkg/log"
)

// ConfluentProducer implements BroadcastEventProducer using confluent-kafka-go.
type ConfluentProducer struct {
	producer *kafka.Producer
	topic    string
	doneCh   chan struct{}
}

// NewConfluentProducer creates a new Kafka producer for broadcast events.
func NewConfluentProducer(brokers, topic string, partitions int) (*ConfluentProducer, error) {
	// Ensure topic exists with desired partition count
	if err := ensureTopic(brokers, topic, partitions); err != nil {
		l := pkglog.L()
		l.Warn().Err(err).Str("topic", topic).Msg("failed to ensure topic, may already exist")
	}

	p, err := kafka.NewProducer(&kafka.ConfigMap{
		"bootstrap.servers": brokers,
		"acks":              "1",
		"linger.ms":         5,
		"compression.type":  "snappy",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create kafka producer: %w", err)
	}

	cp := &ConfluentProducer{
		producer: p,
		topic:    topic,
		doneCh:   make(chan struct{}),
	}

	go cp.deliveryReportHandler()

	return cp, nil
}

func ensureTopic(brokers, topic string, partitions int) error {
	admin, err := kafka.NewAdminClient(&kafka.ConfigMap{
		"bootstrap.servers": brokers,
	})
	if err != nil {
		return fmt.Errorf("failed to create admin client: %w", err)
	}
	defer admin.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	results, err := admin.CreateTopics(ctx, []kafka.TopicSpecification{
		{
			Topic:             topic,
			NumPartitions:     partitions,
			ReplicationFactor: 1,
		},
	})
	if err != nil {
		return err
	}

	for _, result := range results {
		if result.Error.Code() != kafka.ErrNoError && result.Error.Code() != kafka.ErrTopicAlreadyExists {
			return fmt.Errorf("failed to create topic %s: %v", result.Topic, result.Error)
		}
	}

	return nil
}

func (cp *ConfluentProducer) deliveryReportHandler() {
	l := pkglog.L()
	for e := range cp.producer.Events() {
		switch ev := e.(type) {
		case *kafka.Message:
			if ev.TopicPartition.Error != nil {
				l.Error().Err(ev.TopicPartition.Error).Msg("kafka delivery failed")
			}
		}
	}
	close(cp.doneCh)
}

func (cp *ConfluentProducer) produceEvent(ctx context.Context, event *BroadcastEvent) error {
	value, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal broadcast event: %w", err)
	}

	// Use room_id as key for consistent partition assignment
	err = cp.producer.Produce(&kafka.Message{
		TopicPartition: kafka.TopicPartition{
			Topic:     &cp.topic,
			Partition: kafka.PartitionAny,
		},
		Key:   []byte(event.RoomID),
		Value: value,
	}, nil)
	if err != nil {
		return fmt.Errorf("failed to produce message: %w", err)
	}

	return nil
}

// ProduceBroadcastStarted sends a broadcast_started event to Kafka.
func (cp *ConfluentProducer) ProduceBroadcastStarted(ctx context.Context, roomID, broadcasterID string) error {
	event := &BroadcastEvent{
		Type:          EventBroadcastStarted,
		RoomID:        roomID,
		BroadcasterID: broadcasterID,
		Timestamp:     time.Now().Unix(),
	}
	return cp.produceEvent(ctx, event)
}

// ProduceBroadcastStopped sends a broadcast_stopped event to Kafka.
func (cp *ConfluentProducer) ProduceBroadcastStopped(ctx context.Context, roomID, broadcasterID, reason string) error {
	event := &BroadcastEvent{
		Type:          EventBroadcastStopped,
		RoomID:        roomID,
		BroadcasterID: broadcasterID,
		Reason:        reason,
		Timestamp:     time.Now().Unix(),
	}
	return cp.produceEvent(ctx, event)
}

// Close flushes pending messages and closes the producer.
func (cp *ConfluentProducer) Close() error {
	cp.producer.Flush(5000)
	cp.producer.Close()
	<-cp.doneCh
	return nil
}
