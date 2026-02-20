package repository

import (
	"context"

	"github.com/weiawesome/wes-io-live/search-service/internal/domain"
)

// SearchRepository defines the interface for search operations against Elasticsearch.
type SearchRepository interface {
	SearchUsers(ctx context.Context, query string, offset, limit int) ([]domain.UserResult, int, error)
	SearchRooms(ctx context.Context, query string, offset, limit int) ([]domain.RoomResult, int, error)
}
