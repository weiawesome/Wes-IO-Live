package service

import (
	"context"

	"github.com/weiawesome/wes-io-live/user-service/internal/domain"
)

// UserService defines the interface for user business logic.
type UserService interface {
	Register(ctx context.Context, req *domain.RegisterRequest) (*domain.AuthResponse, error)
	Login(ctx context.Context, req *domain.LoginRequest) (*domain.AuthResponse, error)
	RefreshToken(ctx context.Context, req *domain.RefreshTokenRequest) (*domain.AuthResponse, error)
	Logout(ctx context.Context, userID string) error
	GetUser(ctx context.Context, userID string) (*domain.UserResponse, error)
	UpdateUser(ctx context.Context, userID string, req *domain.UpdateUserRequest) (*domain.UserResponse, error)
	ChangePassword(ctx context.Context, userID string, req *domain.ChangePasswordRequest) error
	DeleteUser(ctx context.Context, userID string) error
	// GenerateAvatarUploadURL generates a presigned PUT URL for direct avatar upload.
	GenerateAvatarUploadURL(ctx context.Context, userID, contentType string) (*domain.AvatarPresignResponse, error)
	// DeleteAvatar removes avatar data for a user.
	DeleteAvatar(ctx context.Context, userID string) error
	// HandleAvatarProcessed tags old avatar objects for lifecycle deletion and updates the DB
	// with new bucket+key refs for raw and all processed sizes.
	HandleAvatarProcessed(ctx context.Context, userID string, raw domain.AvatarObjectRef, processed domain.AvatarObjects) error
}
