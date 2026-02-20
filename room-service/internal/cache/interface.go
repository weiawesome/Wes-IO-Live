package cache

import (
	"context"
	"time"

	"github.com/weiawesome/wes-io-live/room-service/internal/domain"
)

type RoomCacheResult struct {
	Room domain.Room `json:"room"`
}

type RoomCache interface {
	Get(ctx context.Context, key string) (*RoomCacheResult, error)
	Set(ctx context.Context, key string, result *RoomCacheResult, ttl time.Duration) error
	Delete(ctx context.Context, keys ...string) error
	BuildKeyByID(roomID string) string
	Close() error
}
