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
}
