package service

import (
	"context"
	"errors"
	"time"

	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/singleflight"

	"github.com/weiawesome/wes-io-live/pkg/log"
	"github.com/weiawesome/wes-io-live/search-service/internal/cache"
	"github.com/weiawesome/wes-io-live/search-service/internal/domain"
	"github.com/weiawesome/wes-io-live/search-service/internal/repository"
)

const (
	defaultLimit = 20
	maxLimit     = 100
)

type searchServiceImpl struct {
	repo     repository.SearchRepository
	cache    cache.SearchCache
	cacheTTL time.Duration
	sf       singleflight.Group
}

// NewSearchService creates a new search service.
func NewSearchService(repo repository.SearchRepository, searchCache cache.SearchCache, cacheTTL time.Duration) SearchService {
	return &searchServiceImpl{
		repo:     repo,
		cache:    searchCache,
		cacheTTL: cacheTTL,
	}
}

func (s *searchServiceImpl) Search(ctx context.Context, req *domain.SearchRequest) (*domain.SearchResponse, error) {
	s.normalizeRequest(req)

	cacheKey := s.cache.(*cache.RedisSearchCache).BuildKey("all", req.Query, req.Offset, req.Limit)

	result, err, _ := s.sf.Do(cacheKey, func() (interface{}, error) {
		// Try cache
		cached, err := s.cache.Get(ctx, cacheKey)
		if err == nil {
			return cached, nil
		}
		if !errors.Is(err, cache.ErrCacheMiss) {
			l := log.Ctx(ctx)
			l.Warn().Err(err).Msg("cache get error")
		}

		// Query both indexes in parallel
		var users []domain.UserResult
		var rooms []domain.RoomResult
		var userTotal, roomTotal int

		g, gCtx := errgroup.WithContext(ctx)

		g.Go(func() error {
			var err error
			users, userTotal, err = s.repo.SearchUsers(gCtx, req.Query, req.Offset, req.Limit)
			return err
		})

		g.Go(func() error {
			var err error
			rooms, roomTotal, err = s.repo.SearchRooms(gCtx, req.Query, req.Offset, req.Limit)
			return err
		})

		if err := g.Wait(); err != nil {
			return nil, err
		}

		resp := &domain.SearchResponse{
			Users: users,
			Rooms: rooms,
			Total: userTotal + roomTotal,
		}

		// Async write cache
		s.asyncCacheSet(cacheKey, resp)

		return resp, nil
	})

	if err != nil {
		return nil, err
	}

	return result.(*domain.SearchResponse), nil
}

func (s *searchServiceImpl) SearchUsers(ctx context.Context, req *domain.SearchRequest) (*domain.SearchResponse, error) {
	s.normalizeRequest(req)

	cacheKey := s.cache.(*cache.RedisSearchCache).BuildKey("user", req.Query, req.Offset, req.Limit)

	result, err, _ := s.sf.Do(cacheKey, func() (interface{}, error) {
		// Try cache
		cached, err := s.cache.Get(ctx, cacheKey)
		if err == nil {
			return cached, nil
		}
		if !errors.Is(err, cache.ErrCacheMiss) {
			l := log.Ctx(ctx)
			l.Warn().Err(err).Msg("cache get error")
		}

		users, total, err := s.repo.SearchUsers(ctx, req.Query, req.Offset, req.Limit)
		if err != nil {
			return nil, err
		}

		resp := &domain.SearchResponse{
			Users: users,
			Total: total,
		}

		s.asyncCacheSet(cacheKey, resp)

		return resp, nil
	})

	if err != nil {
		return nil, err
	}

	return result.(*domain.SearchResponse), nil
}

func (s *searchServiceImpl) SearchRooms(ctx context.Context, req *domain.SearchRequest) (*domain.SearchResponse, error) {
	s.normalizeRequest(req)

	cacheKey := s.cache.(*cache.RedisSearchCache).BuildKey("room", req.Query, req.Offset, req.Limit)

	result, err, _ := s.sf.Do(cacheKey, func() (interface{}, error) {
		// Try cache
		cached, err := s.cache.Get(ctx, cacheKey)
		if err == nil {
			return cached, nil
		}
		if !errors.Is(err, cache.ErrCacheMiss) {
			l := log.Ctx(ctx)
			l.Warn().Err(err).Msg("cache get error")
		}

		rooms, total, err := s.repo.SearchRooms(ctx, req.Query, req.Offset, req.Limit)
		if err != nil {
			return nil, err
		}

		resp := &domain.SearchResponse{
			Rooms: rooms,
			Total: total,
		}

		s.asyncCacheSet(cacheKey, resp)

		return resp, nil
	})

	if err != nil {
		return nil, err
	}

	return result.(*domain.SearchResponse), nil
}

func (s *searchServiceImpl) normalizeRequest(req *domain.SearchRequest) {
	if req.Limit <= 0 {
		req.Limit = defaultLimit
	}
	if req.Limit > maxLimit {
		req.Limit = maxLimit
	}
	if req.Offset < 0 {
		req.Offset = 0
	}
}

func (s *searchServiceImpl) asyncCacheSet(key string, resp *domain.SearchResponse) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		if err := s.cache.Set(ctx, key, resp, s.cacheTTL); err != nil {
			l := log.L()
			l.Warn().Err(err).Str("key", key).Msg("cache set error")
		}
	}()
}
