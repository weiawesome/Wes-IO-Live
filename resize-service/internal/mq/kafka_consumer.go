package mq

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	pkglog "github.com/weiawesome/wes-io-live/pkg/log"
)

// minioEventRaw is the raw MinIO Kafka notification structure.
type minioEventRaw struct {
	EventName string `json:"EventName"`
	Records   []struct {
		EventName string    `json:"eventName"`
		EventTime time.Time `json:"eventTime"`
		S3        struct {
			Bucket struct {
				Name string `json:"name"`
			} `json:"bucket"`
			Object struct {
				Key         string `json:"key"`
				Size        int64  `json:"size"`
				ContentType string `json:"contentType"`
			} `json:"object"`
		} `json:"s3"`
	} `json:"Records"`
}

// KafkaConsumer implements MinIOEventConsumer using confluent-kafka-go.
type KafkaConsumer struct {
	consumer         *kafka.Consumer
	topic            string
	handler          MinIOEventHandler
	bucketFilter     string
	prefixFilter     string
	eventNameFilters []string
	doneCh           chan struct{}
}

// NewKafkaConsumer creates a new Kafka consumer for MinIO events.
func NewKafkaConsumer(brokers, topic, groupID, bucketFilter, prefixFilter string, eventNameFilters []string, handler MinIOEventHandler) (*KafkaConsumer, error) {
	c, err := kafka.NewConsumer(&kafka.ConfigMap{
		"bootstrap.servers":  brokers,
		"group.id":           groupID,
		"auto.offset.reset":  "latest",
		"enable.auto.commit": true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create kafka consumer: %w", err)
	}

	return &KafkaConsumer{
		consumer:         c,
		topic:            topic,
		handler:          handler,
		bucketFilter:     bucketFilter,
		prefixFilter:     prefixFilter,
		eventNameFilters: eventNameFilters,
		doneCh:           make(chan struct{}),
	}, nil
}

// Start begins consuming messages from Kafka in a background goroutine.
func (kc *KafkaConsumer) Start(ctx context.Context) error {
	if err := kc.consumer.Subscribe(kc.topic, nil); err != nil {
		return fmt.Errorf("failed to subscribe to topic %s: %w", kc.topic, err)
	}

	l := pkglog.L()
	l.Info().Str("topic", kc.topic).Str("group", kc.consumer.String()).Msg("minio event consumer started")

	go kc.consumeLoop(ctx)

	return nil
}

func (kc *KafkaConsumer) consumeLoop(ctx context.Context) {
	l := pkglog.L()
	defer close(kc.doneCh)

	for {
		select {
		case <-ctx.Done():
			l.Info().Msg("minio event consumer shutting down")
			return
		default:
			msg, err := kc.consumer.ReadMessage(100 * time.Millisecond)
			if err != nil {
				if err.(kafka.Error).Code() == kafka.ErrTimedOut {
					continue
				}
				l.Error().Err(err).Msg("kafka consumer error")
				continue
			}
			// Use a detached context so in-flight processing completes even after shutdown signal.
			kc.processMessage(context.WithoutCancel(ctx), msg)
		}
	}
}

func (kc *KafkaConsumer) processMessage(ctx context.Context, msg *kafka.Message) {
	l := pkglog.L()

	var raw minioEventRaw
	if err := json.Unmarshal(msg.Value, &raw); err != nil {
		l.Error().Err(err).Msg("failed to unmarshal minio event")
		return
	}

	if len(raw.Records) == 0 {
		return
	}

	rec := raw.Records[0]
	bucket := rec.S3.Bucket.Name

	key, err := url.QueryUnescape(rec.S3.Object.Key)
	if err != nil {
		l.Error().Err(err).Str("raw_key", rec.S3.Object.Key).Msg("failed to url-decode key")
		return
	}

	// Filter: only process allowed event types.
	allowed := false
	for _, f := range kc.eventNameFilters {
		if rec.EventName == f {
			allowed = true
			break
		}
	}
	if !allowed {
		return
	}

	// Filter: only process events from the target bucket and key prefix.
	if bucket != kc.bucketFilter || !strings.HasPrefix(key, kc.prefixFilter) {
		return
	}

	event := &MinIOUploadEvent{
		Bucket:      bucket,
		Key:         key,
		Size:        rec.S3.Object.Size,
		ContentType: rec.S3.Object.ContentType,
		EventTime:   rec.EventTime,
	}

	l.Info().
		Str("bucket", bucket).
		Str("key", key).
		Int64("size", event.Size).
		Msg("received minio upload event")

	if err := kc.handler.HandleUploadEvent(ctx, event); err != nil {
		l.Error().Err(err).Str("key", key).Msg("failed to handle upload event")
	}
}

// Close waits for the consume loop to drain, then closes the Kafka client.
// ctx must already be cancelled before calling Close.
func (kc *KafkaConsumer) Close() error {
	<-kc.doneCh // wait for in-flight processMessage to complete
	if err := kc.consumer.Close(); err != nil {
		return fmt.Errorf("failed to close kafka consumer: %w", err)
	}
	return nil
}
