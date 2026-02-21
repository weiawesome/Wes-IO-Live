package mq

import (
	"context"
	"time"
)

// MinIOUploadEvent holds parsed fields from a MinIO s3:ObjectCreated Kafka notification.
type MinIOUploadEvent struct {
	Bucket      string
	Key         string // URL-decoded, e.g. "avatars/raw/{userID}/{uuid}.jpg"
	Size        int64
	ContentType string
	EventTime   time.Time
}

// AvatarObjectRef identifies a stored object by its bucket and key.
type AvatarObjectRef struct {
	Bucket string `json:"bucket"`
	Key    string `json:"key"`
}

// AvatarProcessedObjects holds bucket+key refs for each processed size variant.
type AvatarProcessedObjects struct {
	Sm AvatarObjectRef `json:"sm"` // 48×48
	Md AvatarObjectRef `json:"md"` // 128×128
	Lg AvatarObjectRef `json:"lg"` // 512×512
}

// AvatarProcessedEvent is published to Kafka after resize completes.
// Each consumer defines its own matching struct — the contract is the JSON schema.
type AvatarProcessedEvent struct {
	UserID    string                 `json:"user_id"`
	Raw       AvatarObjectRef        `json:"raw"`
	Processed AvatarProcessedObjects `json:"processed"`
	Timestamp int64                  `json:"timestamp"`
}

// MinIOEventHandler is the business-logic callback injected into the consumer.
type MinIOEventHandler interface {
	HandleUploadEvent(ctx context.Context, event *MinIOUploadEvent) error
}

// MinIOEventConsumer abstracts the Kafka consumer for MinIO events.
type MinIOEventConsumer interface {
	Start(ctx context.Context) error
	Close() error
}

// AvatarEventPublisher abstracts the Kafka producer for avatar-processed events.
type AvatarEventPublisher interface {
	PublishAvatarProcessed(ctx context.Context, event *AvatarProcessedEvent) error
	Close() error
}
