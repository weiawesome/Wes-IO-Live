package kafka

import (
	"context"

	"github.com/weiawesome/wes-io-live/chat-service/internal/domain"
)

type MessageProducer interface {
	ProduceMessage(ctx context.Context, msg *domain.ChatMessage) error
	Close() error
}
