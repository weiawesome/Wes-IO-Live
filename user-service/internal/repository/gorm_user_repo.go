package repository

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/weiawesome/wes-io-live/user-service/internal/domain"
)

// GormUserRepository implements UserRepository using GORM.
type GormUserRepository struct {
	db *gorm.DB
}

// NewGormUserRepository creates a new GORM-based user repository.
func NewGormUserRepository(db *gorm.DB) *GormUserRepository {
	return &GormUserRepository{db: db}
}

// Create creates a new user.
func (r *GormUserRepository) Create(ctx context.Context, user *domain.User) error {
	user.ID = uuid.New().String()
	if user.Roles == nil {
		user.Roles = []string{"user"}
	}

	model := domain.UserToModel(user)
	result := r.db.WithContext(ctx).Create(model)
	if result.Error != nil {
		return r.handleError(result.Error)
	}

	// Update the domain object with generated timestamps
	user.CreatedAt = model.CreatedAt
	user.UpdatedAt = model.UpdatedAt
	return nil
}

// GetByID retrieves a user by ID.
func (r *GormUserRepository) GetByID(ctx context.Context, id string) (*domain.User, error) {
	var model domain.UserModel
	result := r.db.WithContext(ctx).First(&model, "id = ?", id)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, result.Error
	}
	return model.ToDomain(), nil
}

// GetByEmail retrieves a user by email.
func (r *GormUserRepository) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	var model domain.UserModel
	result := r.db.WithContext(ctx).First(&model, "email = ?", email)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, result.Error
	}
	return model.ToDomain(), nil
}

// Update updates a user.
func (r *GormUserRepository) Update(ctx context.Context, user *domain.User) error {
	model := domain.UserToModel(user)
	result := r.db.WithContext(ctx).Model(&domain.UserModel{}).
		Where("id = ?", user.ID).
		Updates(map[string]interface{}{
			"display_name":  model.DisplayName,
			"password_hash": model.PasswordHash,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrUserNotFound
	}

	// Get updated timestamp
	var updated domain.UserModel
	r.db.WithContext(ctx).First(&updated, "id = ?", user.ID)
	user.UpdatedAt = updated.UpdatedAt
	return nil
}

// Delete soft-deletes a user and obfuscates unique fields to allow re-registration.
func (r *GormUserRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// First check if user exists
		var model domain.UserModel
		if err := tx.First(&model, "id = ?", id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrUserNotFound
			}
			return err
		}

		// Obfuscate unique fields to allow re-registration with same email/username
		suffix := "_deleted_" + id
		if err := tx.Model(&domain.UserModel{}).Where("id = ?", id).Updates(map[string]interface{}{
			"email":    model.Email + suffix,
			"username": model.Username + suffix,
		}).Error; err != nil {
			return err
		}

		// Soft delete the user
		return tx.Delete(&domain.UserModel{}, "id = ?", id).Error
	})
}

// handleError converts database-specific errors to domain errors.
func (r *GormUserRepository) handleError(err error) error {
	errStr := err.Error()

	// PostgreSQL unique constraint violation
	if strings.Contains(errStr, "duplicate key") || strings.Contains(errStr, "UNIQUE constraint") {
		if strings.Contains(errStr, "email") {
			return ErrEmailExists
		}
		if strings.Contains(errStr, "username") {
			return ErrUsernameExists
		}
	}

	// MySQL unique constraint violation
	if strings.Contains(errStr, "Duplicate entry") {
		if strings.Contains(errStr, "email") {
			return ErrEmailExists
		}
		if strings.Contains(errStr, "username") {
			return ErrUsernameExists
		}
	}

	return err
}
