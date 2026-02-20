package cache

import (
	"context"
	"time"

	"github.com/weiawesome/wes-io-live/search-service/internal/domain"
)

// SearchCache defines the interface for caching search results.
type SearchCache interface {
	Get(ctx context.Context, key string) (*domain.SearchResponse, error)
	Set(ctx context.Context, key string, result *domain.SearchResponse, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
	Close() error
}
