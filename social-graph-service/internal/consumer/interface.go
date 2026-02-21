package consumer

import "context"

// DebeziumFollowRecord represents a row from the follows table in a Debezium CDC event.
type DebeziumFollowRecord struct {
	ID          int64   `json:"id"`
	FollowerID  string  `json:"follower_id"`
	FollowingID string  `json:"following_id"`
	CreatedAt   *string `json:"created_at"`
	DeletedAt   *string `json:"deleted_at"` // nil = active, non-nil = soft-deleted
}

// DebeziumPayload is the payload field of a Debezium CDC message.
type DebeziumPayload struct {
	Before *DebeziumFollowRecord `json:"before"`
	After  *DebeziumFollowRecord `json:"after"`
	Op     string                `json:"op"` // "c"=create, "u"=update, "d"=delete, "r"=snapshot
	TsMs   int64                 `json:"ts_ms"`
}

// DebeziumMessage is the top-level Debezium CDC message envelope.
type DebeziumMessage struct {
	Payload DebeziumPayload `json:"payload"`
}

// CDCEventHandler processes a decoded Debezium CDC message.
type CDCEventHandler interface {
	HandleCDCEvent(ctx context.Context, event *DebeziumMessage) error
}

// CDCEventConsumer manages the Kafka consumer lifecycle.
type CDCEventConsumer interface {
	Start(ctx context.Context) error
	Close() error
}
