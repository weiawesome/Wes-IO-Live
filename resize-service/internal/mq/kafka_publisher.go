package mq

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	pkglog "github.com/weiawesome/wes-io-live/pkg/log"
)

// KafkaPublisher implements AvatarEventPublisher using confluent-kafka-go.
type KafkaPublisher struct {
	producer *kafka.Producer
	topic    string
	doneCh   chan struct{}
}

// NewKafkaPublisher creates a new Kafka producer for avatar-processed events.
func NewKafkaPublisher(brokers, topic string) (*KafkaPublisher, error) {
	if err := ensureTopic(brokers, topic, 1); err != nil {
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

	kp := &KafkaPublisher{
		producer: p,
		topic:    topic,
		doneCh:   make(chan struct{}),
	}

	go kp.deliveryReportHandler()

	return kp, nil
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

func (kp *KafkaPublisher) deliveryReportHandler() {
	l := pkglog.L()
	for e := range kp.producer.Events() {
		switch ev := e.(type) {
		case *kafka.Message:
			if ev.TopicPartition.Error != nil {
				l.Error().Err(ev.TopicPartition.Error).Msg("kafka delivery failed")
			}
		}
	}
	close(kp.doneCh)
}

// PublishAvatarProcessed sends an avatar-processed event to Kafka.
// The userID is used as the message key for consistent partition assignment.
func (kp *KafkaPublisher) PublishAvatarProcessed(ctx context.Context, event *AvatarProcessedEvent) error {
	value, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal avatar processed event: %w", err)
	}

	err = kp.producer.Produce(&kafka.Message{
		TopicPartition: kafka.TopicPartition{
			Topic:     &kp.topic,
			Partition: kafka.PartitionAny,
		},
		Key:   []byte(event.UserID),
		Value: value,
	}, nil)
	if err != nil {
		return fmt.Errorf("failed to produce message: %w", err)
	}

	return nil
}

// Close flushes pending messages and releases producer resources.
func (kp *KafkaPublisher) Close() error {
	kp.producer.Flush(5000)
	kp.producer.Close()
	<-kp.doneCh
	return nil
}
