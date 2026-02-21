package consumer

import "context"

// AvatarObjectRef identifies a stored object by its bucket and key.
type AvatarObjectRef struct {
	Bucket string `json:"bucket"`
	Key    string `json:"key"`
}

// AvatarProcessedObjects holds bucket+key refs for each processed size variant.
type AvatarProcessedObjects struct {
	Sm AvatarObjectRef `json:"sm"`
	Md AvatarObjectRef `json:"md"`
	Lg AvatarObjectRef `json:"lg"`
}

// AvatarProcessedEvent is consumed after resize-service completes.
// The JSON schema matches what resize-service publishes.
type AvatarProcessedEvent struct {
	UserID    string                 `json:"user_id"`
	Raw       AvatarObjectRef        `json:"raw"`
	Processed AvatarProcessedObjects `json:"processed"`
	Timestamp int64                  `json:"timestamp"`
}

// AvatarProcessedHandler handles incoming avatar-processed events.
type AvatarProcessedHandler interface {
	HandleAvatarProcessed(ctx context.Context, event *AvatarProcessedEvent) error
}

// AvatarProcessedConsumer defines the interface for consuming avatar-processed events.
type AvatarProcessedConsumer interface {
	Start(ctx context.Context) error
	Close() error
}
