package repository

import (
	"context"
	"errors"

	"github.com/weiawesome/wes-io-live/user-service/internal/domain"
)

var (
	ErrUserNotFound   = errors.New("user not found")
	ErrEmailExists    = errors.New("email already exists")
	ErrUsernameExists = errors.New("username already exists")
)

// UserRepository defines the interface for user data persistence.
type UserRepository interface {
	Create(ctx context.Context, user *domain.User) error
	GetByID(ctx context.Context, id string) (*domain.User, error)
	GetByEmail(ctx context.Context, email string) (*domain.User, error)
	Update(ctx context.Context, user *domain.User) error
	Delete(ctx context.Context, id string) error
	// UpdateAvatar persists the avatar_objects JSON for a user.
	// Pass nil to clear all avatar data.
	UpdateAvatar(ctx context.Context, userID string, objects *string) error
}
