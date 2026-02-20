package service

import (
	"context"
	"errors"
	"time"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/sync/singleflight"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/weiawesome/wes-io-live/pkg/log"
	pb "github.com/weiawesome/wes-io-live/proto/auth"
	"github.com/weiawesome/wes-io-live/user-service/internal/audit"
	"github.com/weiawesome/wes-io-live/user-service/internal/cache"
	"github.com/weiawesome/wes-io-live/user-service/internal/domain"
	"github.com/weiawesome/wes-io-live/user-service/internal/repository"
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
	cache      cache.UserCache
	cacheTTL   time.Duration
	sf         singleflight.Group
}

// NewUserService creates a new user service.
func NewUserService(repo repository.UserRepository, authServiceAddr string, userCache cache.UserCache, cacheTTL time.Duration) (UserService, error) {
	conn, err := grpc.Dial(authServiceAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}

	return &userServiceImpl{
		repo:       repo,
		authClient: pb.NewAuthServiceClient(conn),
		cache:      userCache,
		cacheTTL:   cacheTTL,
	}, nil
}

// Register registers a new user.
func (s *userServiceImpl) Register(ctx context.Context, req *domain.RegisterRequest) (*domain.AuthResponse, error) {
	l := log.Ctx(ctx)

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		l.Error().Err(err).Msg("failed to hash password")
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
		l.Error().Err(err).Msg("failed to create user")
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
		l.Error().Err(err).Str(log.FieldUserID, user.ID).Msg("failed to generate tokens after register")
		return nil, err
	}

	// Async write cache (by ID + by Email)
	s.asyncCacheSet(user)

	audit.Log(ctx, audit.ActionRegister, user.ID, "user registered")

	return &domain.AuthResponse{
		User:         user.ToResponse(),
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresAt:    tokenResp.AccessExpiresAt,
	}, nil
}

// Login authenticates a user.
func (s *userServiceImpl) Login(ctx context.Context, req *domain.LoginRequest) (*domain.AuthResponse, error) {
	l := log.Ctx(ctx)

	// Find user via cache-aside + singleflight (by email)
	user, err := s.getUserByEmail(ctx, req.Email)
	if err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			audit.LogWithDetail(ctx, audit.ActionLoginFailed, "", req.Email, "login failed: user not found")
			return nil, ErrInvalidCredentials
		}
		l.Error().Err(err).Msg("failed to get user by email")
		return nil, err
	}

	// Verify password
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		audit.LogWithDetail(ctx, audit.ActionLoginFailed, user.ID, req.Email, "login failed: wrong password")
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
		l.Error().Err(err).Str(log.FieldUserID, user.ID).Msg("failed to generate tokens after login")
		return nil, err
	}

	audit.Log(ctx, audit.ActionLogin, user.ID, "user logged in")

	return &domain.AuthResponse{
		User:         user.ToResponse(),
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresAt:    tokenResp.AccessExpiresAt,
	}, nil
}

// RefreshToken refreshes a user's access token.
func (s *userServiceImpl) RefreshToken(ctx context.Context, req *domain.RefreshTokenRequest) (*domain.AuthResponse, error) {
	l := log.Ctx(ctx)

	// Refresh token
	tokenResp, err := s.authClient.RefreshToken(ctx, &pb.RefreshTokenRequest{
		RefreshToken: req.RefreshToken,
	})
	if err != nil {
		l.Warn().Err(err).Msg("failed to refresh token")
		return nil, ErrInvalidCredentials
	}

	// Validate token
	validateResp, err := s.authClient.ValidateToken(ctx, &pb.ValidateTokenRequest{
		AccessToken: tokenResp.AccessToken,
	})
	if err != nil || !validateResp.Valid {
		l.Warn().Err(err).Msg("refreshed token validation failed")
		return nil, ErrInvalidCredentials
	}

	// Get user (uses cache-aside + singleflight via GetUser path)
	userResp, err := s.GetUser(ctx, validateResp.UserId)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return nil, ErrInvalidCredentials
		}
		l.Error().Err(err).Str(log.FieldUserID, validateResp.UserId).Msg("failed to get user after token refresh")
		return nil, err
	}

	audit.Log(ctx, audit.ActionRefreshToken, userResp.ID, "token refreshed")

	// Return new tokens
	return &domain.AuthResponse{
		User:         *userResp,
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresAt:    tokenResp.AccessExpiresAt,
	}, nil
}

// Logout revokes user tokens.
func (s *userServiceImpl) Logout(ctx context.Context, userID string) error {
	l := log.Ctx(ctx)

	_, err := s.authClient.RevokeTokens(ctx, &pb.RevokeTokensRequest{
		UserId: userID,
	})
	if err != nil {
		l.Error().Err(err).Str(log.FieldUserID, userID).Msg("failed to revoke tokens")
		return err
	}

	audit.Log(ctx, audit.ActionLogout, userID, "user logged out")
	return nil
}

// GetUser retrieves a user by ID with cache-aside + singleflight.
func (s *userServiceImpl) GetUser(ctx context.Context, userID string) (*domain.UserResponse, error) {
	l := log.Ctx(ctx)

	cacheKey := s.cache.BuildKeyByID(userID)

	result, err, _ := s.sf.Do(cacheKey, func() (interface{}, error) {
		return s.fetchUserByIDWithCache(ctx, userID, cacheKey)
	})
	if err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			return nil, ErrUserNotFound
		}
		l.Error().Err(err).Str(log.FieldUserID, userID).Msg("failed to get user")
		return nil, err
	}

	user := result.(*domain.User)
	resp := user.ToResponse()
	return &resp, nil
}

// UpdateUser updates a user.
func (s *userServiceImpl) UpdateUser(ctx context.Context, userID string, req *domain.UpdateUserRequest) (*domain.UserResponse, error) {
	l := log.Ctx(ctx)

	user, err := s.repo.GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			return nil, ErrUserNotFound
		}
		l.Error().Err(err).Str(log.FieldUserID, userID).Msg("failed to get user for update")
		return nil, err
	}

	if req.DisplayName != nil {
		user.DisplayName = *req.DisplayName
	}

	if err := s.repo.Update(ctx, user); err != nil {
		l.Error().Err(err).Str(log.FieldUserID, userID).Msg("failed to update user")
		return nil, err
	}

	// Invalidate cache (by ID + by Email)
	s.invalidateCache(ctx, userID, user.Email)

	audit.Log(ctx, audit.ActionUpdateProfile, userID, "profile updated")

	resp := user.ToResponse()
	return &resp, nil
}

// ChangePassword changes user password after verifying current password.
func (s *userServiceImpl) ChangePassword(ctx context.Context, userID string, req *domain.ChangePasswordRequest) error {
	l := log.Ctx(ctx)

	user, err := s.repo.GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			return ErrUserNotFound
		}
		l.Error().Err(err).Str(log.FieldUserID, userID).Msg("failed to get user for password change")
		return err
	}

	// Verify current password
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.CurrentPassword)); err != nil {
		return ErrWrongPassword
	}

	// Hash new password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		l.Error().Err(err).Msg("failed to hash new password")
		return err
	}
	user.PasswordHash = string(hashedPassword)

	if err := s.repo.Update(ctx, user); err != nil {
		l.Error().Err(err).Str(log.FieldUserID, userID).Msg("failed to update password")
		return err
	}

	// Invalidate cache (by ID + by Email)
	s.invalidateCache(ctx, userID, user.Email)

	audit.Log(ctx, audit.ActionChangePassword, userID, "password changed")
	return nil
}

// DeleteUser deletes a user.
func (s *userServiceImpl) DeleteUser(ctx context.Context, userID string) error {
	l := log.Ctx(ctx)

	// Get user email before deletion for cache invalidation
	user, err := s.repo.GetByID(ctx, userID)
	if err != nil && !errors.Is(err, repository.ErrUserNotFound) {
		l.Warn().Err(err).Str(log.FieldUserID, userID).Msg("failed to get user before delete for cache invalidation")
	}

	// Revoke tokens first
	if _, err := s.authClient.RevokeTokens(ctx, &pb.RevokeTokensRequest{
		UserId: userID,
	}); err != nil {
		l.Warn().Err(err).Str(log.FieldUserID, userID).Msg("failed to revoke tokens before delete")
	}

	if err := s.repo.Delete(ctx, userID); err != nil {
		l.Error().Err(err).Str(log.FieldUserID, userID).Msg("failed to delete user")
		return err
	}

	// Invalidate cache
	if user != nil {
		s.invalidateCache(ctx, userID, user.Email)
	} else {
		s.invalidateCache(ctx, userID, "")
	}

	audit.Log(ctx, audit.ActionDeleteAccount, userID, "account deleted")
	return nil
}

// fetchUserByIDWithCache tries cache first, falls back to DB.
func (s *userServiceImpl) fetchUserByIDWithCache(ctx context.Context, userID, cacheKey string) (*domain.User, error) {
	// Try cache
	cached, err := s.cache.Get(ctx, cacheKey)
	if err == nil {
		return &cached.User, nil
	}
	if !errors.Is(err, cache.ErrCacheMiss) {
		l := log.Ctx(ctx)
		l.Warn().Err(err).Msg("cache get error")
	}

	// Fetch from DB
	user, err := s.repo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Async write cache
	s.asyncCacheSet(user)

	return user, nil
}

// getUserByEmail fetches user by email with cache-aside + singleflight.
func (s *userServiceImpl) getUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	cacheKey := s.cache.BuildKeyByEmail(email)

	result, err, _ := s.sf.Do(cacheKey, func() (interface{}, error) {
		// Try cache
		cached, err := s.cache.Get(ctx, cacheKey)
		if err == nil {
			return &cached.User, nil
		}
		if !errors.Is(err, cache.ErrCacheMiss) {
			l := log.Ctx(ctx)
			l.Warn().Err(err).Msg("cache get error")
		}

		// Fetch from DB
		user, err := s.repo.GetByEmail(ctx, email)
		if err != nil {
			return nil, err
		}

		// Async write cache (by ID + by Email)
		s.asyncCacheSet(user)

		return user, nil
	})

	if err != nil {
		return nil, err
	}

	return result.(*domain.User), nil
}

// asyncCacheSet writes user to cache asynchronously (by ID + by Email).
func (s *userServiceImpl) asyncCacheSet(user *domain.User) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		result := &cache.UserCacheResult{User: *user}

		if err := s.cache.Set(ctx, s.cache.BuildKeyByID(user.ID), result, s.cacheTTL); err != nil {
			l := log.L()
			l.Warn().Err(err).Str(log.FieldUserID, user.ID).Msg("cache set error (by ID)")
		}

		if err := s.cache.Set(ctx, s.cache.BuildKeyByEmail(user.Email), result, s.cacheTTL); err != nil {
			l := log.L()
			l.Warn().Err(err).Str(log.FieldUserID, user.ID).Msg("cache set error (by email)")
		}
	}()
}

// invalidateCache deletes cache entries for a user.
func (s *userServiceImpl) invalidateCache(ctx context.Context, userID, email string) {
	keys := []string{s.cache.BuildKeyByID(userID)}
	if email != "" {
		keys = append(keys, s.cache.BuildKeyByEmail(email))
	}

	if err := s.cache.Delete(ctx, keys...); err != nil {
		l := log.Ctx(ctx)
		l.Warn().Err(err).Str(log.FieldUserID, userID).Msg("cache invalidation error")
	}
}
