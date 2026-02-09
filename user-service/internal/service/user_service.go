package service

import (
	"context"
	"errors"

	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/weiawesome/wes-io-live/user-service/internal/domain"
	"github.com/weiawesome/wes-io-live/user-service/internal/repository"
	pb "github.com/weiawesome/wes-io-live/proto/auth"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUserNotFound       = errors.New("user not found")
	ErrWrongPassword      = errors.New("current password is incorrect")
)

// userServiceImpl implements UserService interface.
type userServiceImpl struct {
	repo       repository.UserRepository
	authClient pb.AuthServiceClient
}

// NewUserService creates a new user service.
func NewUserService(repo repository.UserRepository, authServiceAddr string) (UserService, error) {
	conn, err := grpc.Dial(authServiceAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}

	return &userServiceImpl{
		repo:       repo,
		authClient: pb.NewAuthServiceClient(conn),
	}, nil
}

// Register registers a new user.
func (s *userServiceImpl) Register(ctx context.Context, req *domain.RegisterRequest) (*domain.AuthResponse, error) {
	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	// Create user
	user := &domain.User{
		Email:        req.Email,
		Username:     req.Username,
		DisplayName:  req.DisplayName,
		PasswordHash: string(hashedPassword),
		Roles:        []string{"user"},
	}

	if err := s.repo.Create(ctx, user); err != nil {
		return nil, err
	}

	// Generate tokens
	tokenResp, err := s.authClient.GenerateTokens(ctx, &pb.GenerateTokensRequest{
		UserId:   user.ID,
		Email:    user.Email,
		Username: user.Username,
		Roles:    user.Roles,
	})
	if err != nil {
		return nil, err
	}

	return &domain.AuthResponse{
		User:         user.ToResponse(),
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresAt:    tokenResp.AccessExpiresAt,
	}, nil
}

// Login authenticates a user.
func (s *userServiceImpl) Login(ctx context.Context, req *domain.LoginRequest) (*domain.AuthResponse, error) {
	// Find user
	user, err := s.repo.GetByEmail(ctx, req.Email)
	if err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}

	// Verify password
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		return nil, ErrInvalidCredentials
	}

	// Generate tokens
	tokenResp, err := s.authClient.GenerateTokens(ctx, &pb.GenerateTokensRequest{
		UserId:   user.ID,
		Email:    user.Email,
		Username: user.Username,
		Roles:    user.Roles,
	})
	if err != nil {
		return nil, err
	}

	return &domain.AuthResponse{
		User:         user.ToResponse(),
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresAt:    tokenResp.AccessExpiresAt,
	}, nil
}

// RefreshToken refreshes a user's access token.
func (s *userServiceImpl) RefreshToken(ctx context.Context, req *domain.RefreshTokenRequest) (*domain.AuthResponse, error) {
	// Refresh token
	tokenResp, err := s.authClient.RefreshToken(ctx, &pb.RefreshTokenRequest{
		RefreshToken: req.RefreshToken,
	})
	if err != nil {
		return nil, ErrInvalidCredentials
	}

	// Validate token
	validateResp, err := s.authClient.ValidateToken(ctx, &pb.ValidateTokenRequest{
		AccessToken: tokenResp.AccessToken,
	})
	if err != nil || !validateResp.Valid {
		return nil, ErrInvalidCredentials
	}

	// Get user
	user, err := s.repo.GetByID(ctx, validateResp.UserId)
	if err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}

	// Return new tokens
	return &domain.AuthResponse{
		User: user.ToResponse(),
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresAt:    tokenResp.AccessExpiresAt,
	}, nil
}

// Logout revokes user tokens.
func (s *userServiceImpl) Logout(ctx context.Context, userID string) error {
	_, err := s.authClient.RevokeTokens(ctx, &pb.RevokeTokensRequest{
		UserId: userID,
	})
	return err
}

// GetUser retrieves a user by ID.
func (s *userServiceImpl) GetUser(ctx context.Context, userID string) (*domain.UserResponse, error) {
	user, err := s.repo.GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}

	resp := user.ToResponse()
	return &resp, nil
}

// UpdateUser updates a user.
func (s *userServiceImpl) UpdateUser(ctx context.Context, userID string, req *domain.UpdateUserRequest) (*domain.UserResponse, error) {
	user, err := s.repo.GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}

	if req.DisplayName != nil {
		user.DisplayName = *req.DisplayName
	}

	if err := s.repo.Update(ctx, user); err != nil {
		return nil, err
	}

	resp := user.ToResponse()
	return &resp, nil
}

// ChangePassword changes user password after verifying current password.
func (s *userServiceImpl) ChangePassword(ctx context.Context, userID string, req *domain.ChangePasswordRequest) error {
	user, err := s.repo.GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			return ErrUserNotFound
		}
		return err
	}

	// Verify current password
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.CurrentPassword)); err != nil {
		return ErrWrongPassword
	}

	// Hash new password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	user.PasswordHash = string(hashedPassword)

	return s.repo.Update(ctx, user)
}

// DeleteUser deletes a user.
func (s *userServiceImpl) DeleteUser(ctx context.Context, userID string) error {
	// Revoke tokens first
	s.authClient.RevokeTokens(ctx, &pb.RevokeTokensRequest{
		UserId: userID,
	})

	return s.repo.Delete(ctx, userID)
}
