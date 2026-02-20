package service

import (
	"context"

	"github.com/weiawesome/wes-io-live/search-service/internal/domain"
)

// SearchService defines the interface for search business logic.
type SearchService interface {
	Search(ctx context.Context, req *domain.SearchRequest) (*domain.SearchResponse, error)
	SearchUsers(ctx context.Context, req *domain.SearchRequest) (*domain.SearchResponse, error)
	SearchRooms(ctx context.Context, req *domain.SearchRequest) (*domain.SearchResponse, error)
}
