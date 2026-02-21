package repository

import (
	"context"
	"errors"
)

var (
	ErrFollowNotFound  = errors.New("follow relationship not found")
	ErrAlreadyFollowing = errors.New("already following")
)

// FollowRepository defines persistence operations for follow relationships.
type FollowRepository interface {
	Follow(ctx context.Context, followerID, followingID string) error
	Unfollow(ctx context.Context, followerID, followingID string) error
	IsFollowing(ctx context.Context, followerID, followingID string) (bool, error)
	BatchIsFollowing(ctx context.Context, followerID string, targetIDs []string) (map[string]bool, error)
	GetFollowersCount(ctx context.Context, userID string) (int64, error)
}
