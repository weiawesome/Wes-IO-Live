package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/weiawesome/wes-io-live/chat-service/internal/domain"
)

type ConfluentProducer struct {
	producer *kafka.Producer
	topic    string
	doneCh   chan struct{}
}

func NewConfluentProducer(brokers, topic string, partitions int) (*ConfluentProducer, error) {
	// Ensure topic exists with desired partition count
	if err := ensureTopic(brokers, topic, partitions); err != nil {
		log.Printf("Warning: failed to ensure topic %s: %v (may already exist)", topic, err)
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
	for e := range cp.producer.Events() {
		switch ev := e.(type) {
		case *kafka.Message:
			if ev.TopicPartition.Error != nil {
				log.Printf("Kafka delivery failed: %v", ev.TopicPartition.Error)
			}
		}
	}
	close(cp.doneCh)
}

func (cp *ConfluentProducer) ProduceMessage(ctx context.Context, msg *domain.ChatMessage) error {
	value, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal chat message: %w", err)
	}

	// Use room_id as key for consistent partition assignment
	err = cp.producer.Produce(&kafka.Message{
		TopicPartition: kafka.TopicPartition{
			Topic:     &cp.topic,
			Partition: kafka.PartitionAny,
		},
		Key:   []byte(msg.RoomID),
		Value: value,
	}, nil)
	if err != nil {
		return fmt.Errorf("failed to produce message: %w", err)
	}

	return nil
}

func (cp *ConfluentProducer) Close() error {
	cp.producer.Flush(5000)
	cp.producer.Close()
	<-cp.doneCh
	return nil
}
