package service

import (
	"context"
	"errors"

	pkglog "github.com/weiawesome/wes-io-live/pkg/log"
	"github.com/weiawesome/wes-io-live/social-graph-service/internal/consumer"
	"github.com/weiawesome/wes-io-live/social-graph-service/internal/repository"
	"github.com/weiawesome/wes-io-live/social-graph-service/internal/store"
)

// socialGraphService implements SocialGraphService.
type socialGraphService struct {
	repo  repository.FollowRepository
	store store.FollowStore
}

// NewSocialGraphService creates a new SocialGraphService instance.
func NewSocialGraphService(repo repository.FollowRepository, store store.FollowStore) SocialGraphService {
	return &socialGraphService{
		repo:  repo,
		store: store,
	}
}

// Follow creates a follow relationship from followerID to followingID.
func (s *socialGraphService) Follow(ctx context.Context, followerID, followingID string) error {
	l := pkglog.Ctx(ctx)

	if followerID == followingID {
		return ErrSelfFollow
	}

	if err := s.repo.Follow(ctx, followerID, followingID); err != nil {
		if errors.Is(err, repository.ErrAlreadyFollowing) {
			return ErrAlreadyFollowing
		}
		l.Error().Err(err).
			Str("follower_id", followerID).
			Str("following_id", followingID).
			Msg("failed to follow user")
		return err
	}

	return nil
}

// Unfollow removes the follow relationship from followerID to followingID.
func (s *socialGraphService) Unfollow(ctx context.Context, followerID, followingID string) error {
	l := pkglog.Ctx(ctx)

	if err := s.repo.Unfollow(ctx, followerID, followingID); err != nil {
		if errors.Is(err, repository.ErrFollowNotFound) {
			return ErrNotFollowing
		}
		l.Error().Err(err).
			Str("follower_id", followerID).
			Str("following_id", followingID).
			Msg("failed to unfollow user")
		return err
	}

	return nil
}

// GetFollowersCount returns the number of followers for userID.
// It checks Redis first; on miss it queries the DB, populates Redis, and records a hot key access.
func (s *socialGraphService) GetFollowersCount(ctx context.Context, userID string) (int64, error) {
	l := pkglog.Ctx(ctx)

	// Always record access for hot key tracking (best-effort)
	if err := s.store.RecordAccess(ctx, userID); err != nil {
		l.Warn().Err(err).Str("user_id", userID).Msg("failed to record hot key access")
	}

	// Try Redis cache first
	count, found, err := s.store.GetFollowersCount(ctx, userID)
	if err != nil {
		l.Warn().Err(err).Str("user_id", userID).Msg("redis get followers count failed, falling back to db")
	}
	if found {
		return count, nil
	}

	// Cache miss: query DB
	count, err = s.repo.GetFollowersCount(ctx, userID)
	if err != nil {
		l.Error().Err(err).Str("user_id", userID).Msg("failed to get followers count from db")
		return 0, err
	}

	// Populate Redis
	if err := s.store.SetFollowersCount(ctx, userID, count); err != nil {
		l.Warn().Err(err).Str("user_id", userID).Msg("failed to set followers count in redis")
	}

	return count, nil
}

// BatchIsFollowing checks whether followerID follows each of the given targetIDs.
func (s *socialGraphService) BatchIsFollowing(ctx context.Context, followerID string, targetIDs []string) (map[string]bool, error) {
	return s.repo.BatchIsFollowing(ctx, followerID, targetIDs)
}

// HandleCDCEvent processes a Debezium CDC event and updates Redis accordingly.
func (s *socialGraphService) HandleCDCEvent(ctx context.Context, event *consumer.DebeziumMessage) error {
	l := pkglog.Ctx(ctx)
	op := event.Payload.Op

	switch op {
	case "r":
		// Snapshot read — skip
		return nil

	case "c":
		// New follow: increment followers count for followingID
		if event.Payload.After == nil {
			l.Warn().Msg("CDC create event missing 'after' field")
			return nil
		}
		if err := s.store.CondIncrFollowersCount(ctx, event.Payload.After.FollowingID); err != nil {
			l.Error().Err(err).Str("following_id", event.Payload.After.FollowingID).Msg("failed to cond incr followers count")
			return err
		}

	case "u":
		// Soft delete (unfollow) and soft restore (re-follow) land here.
		// after.deleted_at != nil  → unfollow → decrement
		// after.deleted_at == nil  → restore  → increment
		if event.Payload.After == nil {
			l.Warn().Msg("CDC update event missing 'after' field")
			return nil
		}
		after := event.Payload.After
		if after.DeletedAt != nil {
			// Row was soft-deleted (unfollow)
			if err := s.store.CondDecrFollowersCount(ctx, after.FollowingID); err != nil {
				l.Error().Err(err).Str("following_id", after.FollowingID).Msg("failed to cond decr followers count (soft delete)")
				return err
			}
		} else {
			// Row was restored (re-follow without a new INSERT)
			if err := s.store.CondIncrFollowersCount(ctx, after.FollowingID); err != nil {
				l.Error().Err(err).Str("following_id", after.FollowingID).Msg("failed to cond incr followers count (soft restore)")
				return err
			}
		}

	case "d":
		// Hard delete — reliable because REPLICA IDENTITY FULL is set on the table.
		if event.Payload.Before == nil {
			l.Warn().Msg("CDC hard-delete event missing 'before' field")
			return nil
		}
		if err := s.store.CondDecrFollowersCount(ctx, event.Payload.Before.FollowingID); err != nil {
			l.Error().Err(err).Str("following_id", event.Payload.Before.FollowingID).Msg("failed to cond decr followers count (hard delete)")
			return err
		}

	default:
		l.Warn().Str("op", op).Msg("unknown CDC operation, skipping")
	}

	return nil
}

// Ensure interface is satisfied at compile time.
var _ SocialGraphService = (*socialGraphService)(nil)
