package repository

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/weiawesome/wes-io-live/social-graph-service/internal/domain"
)

// isUniqueViolation reports whether err is a unique-constraint violation.
// GORM v1.25+ wraps these as gorm.ErrDuplicatedKey.
func isUniqueViolation(err error) bool {
	return errors.Is(err, gorm.ErrDuplicatedKey)
}

// GormFollowRepository implements FollowRepository using GORM.
type GormFollowRepository struct {
	db *gorm.DB
}

// NewGormFollowRepository creates a new GORM-backed follow repository.
func NewGormFollowRepository(db *gorm.DB) *GormFollowRepository {
	return &GormFollowRepository{db: db}
}

// Follow creates a follow relationship between two users.
// If a soft-deleted record already exists for the (follower, following) pair,
// it is restored (deleted_at set back to NULL) rather than inserting a new row,
// avoiding duplicate history entries.
func (r *GormFollowRepository) Follow(ctx context.Context, followerID, followingID string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Step 1: attempt to restore any existing soft-deleted record.
		result := tx.Unscoped().
			Model(&domain.FollowModel{}).
			Where("follower_id = ? AND following_id = ? AND deleted_at IS NOT NULL", followerID, followingID).
			Update("deleted_at", nil)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected > 0 {
			// Restored an existing row; CDC will fire a "u" event.
			return nil
		}

		// Step 2: no soft-deleted record found — insert a fresh row.
		model := domain.FollowModel{
			FollowerID:  followerID,
			FollowingID: followingID,
		}
		if err := tx.Create(&model).Error; err != nil {
			if isUniqueViolation(err) {
				return ErrAlreadyFollowing
			}
			return err
		}
		return nil
	})
}

// Unfollow removes a follow relationship between two users.
func (r *GormFollowRepository) Unfollow(ctx context.Context, followerID, followingID string) error {
	result := r.db.WithContext(ctx).
		Where("follower_id = ? AND following_id = ?", followerID, followingID).
		Delete(&domain.FollowModel{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrFollowNotFound
	}
	return nil
}

// IsFollowing checks if followerID follows followingID.
func (r *GormFollowRepository) IsFollowing(ctx context.Context, followerID, followingID string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&domain.FollowModel{}).
		Where("follower_id = ? AND following_id = ?", followerID, followingID).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// BatchIsFollowing checks if followerID follows each of the targetIDs.
func (r *GormFollowRepository) BatchIsFollowing(ctx context.Context, followerID string, targetIDs []string) (map[string]bool, error) {
	result := make(map[string]bool, len(targetIDs))
	for _, id := range targetIDs {
		result[id] = false
	}

	if len(targetIDs) == 0 {
		return result, nil
	}

	var models []domain.FollowModel
	err := r.db.WithContext(ctx).
		Where("follower_id = ? AND following_id IN ?", followerID, targetIDs).
		Find(&models).Error
	if err != nil {
		return nil, err
	}

	for _, m := range models {
		result[m.FollowingID] = true
	}
	return result, nil
}

// GetFollowersCount returns the total number of followers for a given user.
func (r *GormFollowRepository) GetFollowersCount(ctx context.Context, userID string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&domain.FollowModel{}).
		Where("following_id = ?", userID).
		Count(&count).Error
	if err != nil {
		return 0, err
	}
	return count, nil
}

// Ensure interface is satisfied at compile time.
var _ FollowRepository = (*GormFollowRepository)(nil)

// isNotFound checks if the error is a "record not found" error.
func isNotFound(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}
