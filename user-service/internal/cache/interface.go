package cache

import (
	"context"
	"time"

	"github.com/weiawesome/wes-io-live/user-service/internal/domain"
)

type UserCacheResult struct {
	User domain.User `json:"user"`
}

type UserCache interface {
	Get(ctx context.Context, key string) (*UserCacheResult, error)
	Set(ctx context.Context, key string, result *UserCacheResult, ttl time.Duration) error
	Delete(ctx context.Context, keys ...string) error
	BuildKeyByID(userID string) string
	BuildKeyByEmail(email string) string
	Close() error
}
