package service

import (
	"context"
	"errors"

	"github.com/weiawesome/wes-io-live/social-graph-service/internal/consumer"
)

var (
	ErrAlreadyFollowing = errors.New("already following")
	ErrNotFollowing     = errors.New("not following")
	ErrSelfFollow       = errors.New("cannot follow yourself")
)

// SocialGraphService defines the business logic for the social graph.
type SocialGraphService interface {
	Follow(ctx context.Context, followerID, followingID string) error
	Unfollow(ctx context.Context, followerID, followingID string) error
	GetFollowersCount(ctx context.Context, userID string) (int64, error)
	BatchIsFollowing(ctx context.Context, followerID string, targetIDs []string) (map[string]bool, error)
	HandleCDCEvent(ctx context.Context, event *consumer.DebeziumMessage) error
}
