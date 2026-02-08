package cache

import (
	"context"
	"time"

	"github.com/weiawesome/wes-io-live/chat-history-service/internal/domain"
)

type MessageCacheResult struct {
	Messages   []domain.ChatMessage `json:"messages"`
	NextCursor string               `json:"next_cursor"`
	HasMore    bool                 `json:"has_more"`
}

type MessageCache interface {
	Get(ctx context.Context, key string) (*MessageCacheResult, error)
	Set(ctx context.Context, key string, result *MessageCacheResult, ttl time.Duration) error
	BuildKey(roomID, sessionID, cursor, direction string, limit int) string
	Close() error
}
