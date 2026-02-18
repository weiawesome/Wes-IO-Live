package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/weiawesome/wes-io-live/chat-history-service/internal/cache"
	"github.com/weiawesome/wes-io-live/chat-history-service/internal/domain"
	"github.com/weiawesome/wes-io-live/chat-history-service/internal/repository"
	"github.com/weiawesome/wes-io-live/pkg/log"
	"golang.org/x/sync/singleflight"
)

type chatHistoryServiceImpl struct {
	repo     repository.MessageRepository
	cache    cache.MessageCache
	cacheTTL time.Duration
	sf       singleflight.Group
}

func NewChatHistoryService(
	repo repository.MessageRepository,
	msgCache cache.MessageCache,
	cacheTTL time.Duration,
) ChatHistoryService {
	return &chatHistoryServiceImpl{
		repo:     repo,
		cache:    msgCache,
		cacheTTL: cacheTTL,
	}
}

func (s *chatHistoryServiceImpl) GetChatHistory(
	ctx context.Context,
	roomID, sessionID string,
	cursor string,
	limit int,
	direction string,
) (*domain.ChatHistoryResponse, error) {
	// Always fetch latest page directly to avoid caching empty results
	if cursor == "" && direction == "backward" {
		dir := repository.ParseDirection(direction)
		messages, nextCursor, hasMore, err := s.repo.GetMessages(ctx, roomID, sessionID, cursor, limit, dir)
		if err != nil {
			return nil, fmt.Errorf("failed to get messages from repository: %w", err)
		}
		return &domain.ChatHistoryResponse{
			Messages:   messages,
			NextCursor: nextCursor,
			HasMore:    hasMore,
		}, nil
	}

	// Build cache key
	cacheKey := s.cache.BuildKey(roomID, sessionID, cursor, direction, limit)

	// Use singleflight to prevent duplicate requests for the same key
	result, err, _ := s.sf.Do(cacheKey, func() (interface{}, error) {
		return s.fetchWithCache(ctx, roomID, sessionID, cursor, limit, direction, cacheKey)
	})

	if err != nil {
		return nil, err
	}

	cacheResult, ok := result.(*cache.MessageCacheResult)
	if !ok {
		return nil, fmt.Errorf("unexpected result type from singleflight")
	}

	return &domain.ChatHistoryResponse{
		Messages:   cacheResult.Messages,
		NextCursor: cacheResult.NextCursor,
		HasMore:    cacheResult.HasMore,
	}, nil
}

func (s *chatHistoryServiceImpl) fetchWithCache(
	ctx context.Context,
	roomID, sessionID string,
	cursor string,
	limit int,
	direction string,
	cacheKey string,
) (*cache.MessageCacheResult, error) {
	// Try to get from cache
	cached, err := s.cache.Get(ctx, cacheKey)
	if err == nil {
		return cached, nil
	}

	if !errors.Is(err, cache.ErrCacheMiss) {
		// Log error but continue to fetch from DB
		l := log.Ctx(ctx)
		l.Warn().Err(err).Msg("cache get error")
	}

	// Fetch from database
	dir := repository.ParseDirection(direction)
	messages, nextCursor, hasMore, err := s.repo.GetMessages(ctx, roomID, sessionID, cursor, limit, dir)
	if err != nil {
		return nil, fmt.Errorf("failed to get messages from repository: %w", err)
	}

	// Build result
	result := &cache.MessageCacheResult{
		Messages:   messages,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}

	// Store in cache (async to avoid blocking response)
	go func() {
		cacheCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := s.cache.Set(cacheCtx, cacheKey, result, s.cacheTTL); err != nil {
			l := log.L()
			l.Warn().Err(err).Msg("cache set error")
		}
	}()

	return result, nil
}
