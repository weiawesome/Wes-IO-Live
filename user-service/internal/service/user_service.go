package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/sync/singleflight"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/weiawesome/wes-io-live/pkg/log"
	"github.com/weiawesome/wes-io-live/pkg/storage"
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

const (
	avatarPresignExpiry    = 5 * time.Minute
	avatarPresignGetExpiry = 1 * time.Hour
)

// lifecycleCfg holds the tag key/value used to mark objects for background deletion.
type lifecycleCfg struct {
	TagKey   string
	TagValue string
}

// NewLifecycleCfg constructs a lifecycleCfg for use in main.
func NewLifecycleCfg(tagKey, tagValue string) lifecycleCfg {
	return lifecycleCfg{TagKey: tagKey, TagValue: tagValue}
}

// userServiceImpl implements UserService interface.
type userServiceImpl struct {
	repo       repository.UserRepository
	authClient pb.AuthServiceClient
	cache      cache.UserCache // used only for email-keyed Login cache
	cacheTTL   time.Duration
	sfEmail    singleflight.Group // deduplicates concurrent Login lookups by email
	// storages maps bucket name → storage client for tagging and URL generation.
	storages  map[string]storage.Storage
	rawBucket string // bucket where raw uploads land (used in GenerateAvatarUploadURL)
	publicURL string // base URL for generating download URLs: {publicURL}/{bucket}/{key}
	lifecycle lifecycleCfg
}

// NewUserService creates a new user service.
func NewUserService(
	repo repository.UserRepository,
	authServiceAddr string,
	userCache cache.UserCache,
	cacheTTL time.Duration,
	storages map[string]storage.Storage,
	rawBucket string,
	publicURL string,
	lc lifecycleCfg,
) (UserService, error) {
	conn, err := grpc.Dial(authServiceAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}

	return &userServiceImpl{
		repo:       repo,
		authClient: pb.NewAuthServiceClient(conn),
		cache:      userCache,
		cacheTTL:   cacheTTL,
		storages:   storages,
		rawBucket:  rawBucket,
		publicURL:  strings.TrimSuffix(publicURL, "/"),
		lifecycle:  lc,
	}, nil
}

// Register registers a new user.
func (s *userServiceImpl) Register(ctx context.Context, req *domain.RegisterRequest) (*domain.AuthResponse, error) {
	l := log.Ctx(ctx)

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		l.Error().Err(err).Msg("failed to hash password")
		return nil, err
	}

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

	audit.Log(ctx, audit.ActionRegister, user.ID, "user registered")

	return &domain.AuthResponse{
		User:         s.userToResponse(ctx, user),
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresAt:    tokenResp.AccessExpiresAt,
	}, nil
}

// Login authenticates a user.
func (s *userServiceImpl) Login(ctx context.Context, req *domain.LoginRequest) (*domain.AuthResponse, error) {
	l := log.Ctx(ctx)

	user, err := s.getUserByEmail(ctx, req.Email)
	if err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			audit.LogWithDetail(ctx, audit.ActionLoginFailed, "", req.Email, "login failed: user not found")
			return nil, ErrInvalidCredentials
		}
		l.Error().Err(err).Msg("failed to get user by email")
		return nil, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		audit.LogWithDetail(ctx, audit.ActionLoginFailed, user.ID, req.Email, "login failed: wrong password")
		return nil, ErrInvalidCredentials
	}

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
		User:         s.userToResponse(ctx, user),
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresAt:    tokenResp.AccessExpiresAt,
	}, nil
}

// RefreshToken refreshes a user's access token.
func (s *userServiceImpl) RefreshToken(ctx context.Context, req *domain.RefreshTokenRequest) (*domain.AuthResponse, error) {
	l := log.Ctx(ctx)

	tokenResp, err := s.authClient.RefreshToken(ctx, &pb.RefreshTokenRequest{
		RefreshToken: req.RefreshToken,
	})
	if err != nil {
		l.Warn().Err(err).Msg("failed to refresh token")
		return nil, ErrInvalidCredentials
	}

	validateResp, err := s.authClient.ValidateToken(ctx, &pb.ValidateTokenRequest{
		AccessToken: tokenResp.AccessToken,
	})
	if err != nil || !validateResp.Valid {
		l.Warn().Err(err).Msg("refreshed token validation failed")
		return nil, ErrInvalidCredentials
	}

	userResp, err := s.GetUser(ctx, validateResp.UserId)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return nil, ErrInvalidCredentials
		}
		l.Error().Err(err).Str(log.FieldUserID, validateResp.UserId).Msg("failed to get user after token refresh")
		return nil, err
	}

	audit.Log(ctx, audit.ActionRefreshToken, userResp.ID, "token refreshed")

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

// GetUser retrieves a user by ID directly from DB (no cache).
func (s *userServiceImpl) GetUser(ctx context.Context, userID string) (*domain.UserResponse, error) {
	l := log.Ctx(ctx)

	user, err := s.repo.GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			return nil, ErrUserNotFound
		}
		l.Error().Err(err).Str(log.FieldUserID, userID).Msg("failed to get user")
		return nil, err
	}

	resp := s.userToResponse(ctx, user)
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

	s.invalidateCache(ctx, userID, user.Email)

	audit.Log(ctx, audit.ActionUpdateProfile, userID, "profile updated")

	resp := s.userToResponse(ctx, user)
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

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.CurrentPassword)); err != nil {
		return ErrWrongPassword
	}

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

	s.invalidateCache(ctx, userID, user.Email)

	audit.Log(ctx, audit.ActionChangePassword, userID, "password changed")
	return nil
}

// DeleteUser deletes a user.
func (s *userServiceImpl) DeleteUser(ctx context.Context, userID string) error {
	l := log.Ctx(ctx)

	user, err := s.repo.GetByID(ctx, userID)
	if err != nil && !errors.Is(err, repository.ErrUserNotFound) {
		l.Warn().Err(err).Str(log.FieldUserID, userID).Msg("failed to get user before delete for cache invalidation")
	}

	if _, err := s.authClient.RevokeTokens(ctx, &pb.RevokeTokensRequest{
		UserId: userID,
	}); err != nil {
		l.Warn().Err(err).Str(log.FieldUserID, userID).Msg("failed to revoke tokens before delete")
	}

	if err := s.repo.Delete(ctx, userID); err != nil {
		l.Error().Err(err).Str(log.FieldUserID, userID).Msg("failed to delete user")
		return err
	}

	if user != nil {
		s.invalidateCache(ctx, userID, user.Email)
	} else {
		s.invalidateCache(ctx, userID, "")
	}

	audit.Log(ctx, audit.ActionDeleteAccount, userID, "account deleted")
	return nil
}


// getUserByEmail fetches user by email with cache-aside + singleflight.
// Only the email-keyed cache is used (Login path); GetUser/me goes to DB directly.
func (s *userServiceImpl) getUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	cacheKey := s.cache.BuildKeyByEmail(email)

	result, err, _ := s.sfEmail.Do(cacheKey, func() (interface{}, error) {
		cached, err := s.cache.Get(ctx, cacheKey)
		if err == nil {
			return &cached.User, nil
		}
		if !errors.Is(err, cache.ErrCacheMiss) {
			l := log.Ctx(ctx)
			l.Warn().Err(err).Msg("cache get error")
		}

		return s.repo.GetByEmail(ctx, email)
	})

	if err != nil {
		return nil, err
	}

	return result.(*domain.User), nil
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

// storageForBucket returns the storage client for the given bucket, or nil if not found.
func (s *userServiceImpl) storageForBucket(bucket string) storage.Storage {
	return s.storages[bucket]
}

// avatarRefURL generates a download URL for an AvatarObjectRef.
// Uses publicURL/{bucket}/{key} if publicURL is configured, otherwise falls back to a presigned GET.
func (s *userServiceImpl) avatarRefURL(ctx context.Context, ref *domain.AvatarObjectRef) string {
	if ref == nil {
		return ""
	}
	if s.publicURL != "" {
		return fmt.Sprintf("%s/%s/%s", s.publicURL, ref.Bucket, ref.Key)
	}
	st := s.storageForBucket(ref.Bucket)
	if st == nil {
		return ""
	}
	u, err := st.GetURL(ctx, ref.Key, avatarPresignGetExpiry)
	if err != nil {
		l := log.Ctx(ctx)
		l.Warn().Err(err).Str("bucket", ref.Bucket).Str("key", ref.Key).Msg("failed to generate presigned get URL")
		return ""
	}
	return u
}

// avatarURLsFromObjects derives AvatarURLs from stored AvatarObjects for API responses.
func (s *userServiceImpl) avatarURLsFromObjects(ctx context.Context, objs *domain.AvatarObjects) *domain.AvatarURLs {
	if objs == nil {
		return nil
	}
	sm := s.avatarRefURL(ctx, objs.Sm)
	md := s.avatarRefURL(ctx, objs.Md)
	lg := s.avatarRefURL(ctx, objs.Lg)
	if sm == "" && md == "" && lg == "" {
		return nil
	}
	return &domain.AvatarURLs{Sm: sm, Md: md, Lg: lg}
}

// userToResponse converts a User to UserResponse, generating avatar URLs on the fly.
func (s *userServiceImpl) userToResponse(ctx context.Context, user *domain.User) domain.UserResponse {
	resp := user.ToResponse()
	resp.AvatarURLs = s.avatarURLsFromObjects(ctx, user.AvatarObjects)
	return resp
}

// marshalAvatarObjects serializes AvatarObjects to a JSON string pointer.
func marshalAvatarObjects(objs *domain.AvatarObjects) (*string, error) {
	if objs == nil {
		return nil, nil
	}
	b, err := json.Marshal(objs)
	if err != nil {
		return nil, err
	}
	s := string(b)
	return &s, nil
}

// GenerateAvatarUploadURL generates a presigned PUT URL for direct avatar upload.
func (s *userServiceImpl) GenerateAvatarUploadURL(ctx context.Context, userID, contentType string) (*domain.AvatarPresignResponse, error) {
	l := log.Ctx(ctx)

	rawSt := s.storageForBucket(s.rawBucket)
	if rawSt == nil {
		return nil, fmt.Errorf("storage not configured")
	}

	switch contentType {
	case "image/jpeg", "image/png", "image/webp":
	default:
		return nil, fmt.Errorf("unsupported content type: %s", contentType)
	}

	ext := map[string]string{
		"image/jpeg": "jpg",
		"image/png":  "png",
		"image/webp": "webp",
	}[contentType]

	id, _ := uuid.NewV7()
	key := fmt.Sprintf("avatars/raw/%s/%s.%s", userID, id.String(), ext)

	uploadURL, err := rawSt.GetUploadURL(ctx, key, contentType, avatarPresignExpiry)
	if err != nil {
		l.Error().Err(err).Str(log.FieldUserID, userID).Msg("failed to generate presigned upload URL")
		return nil, err
	}

	// Persist the raw bucket+key; processed sizes will be added by HandleAvatarProcessed.
	objs := &domain.AvatarObjects{
		Raw: &domain.AvatarObjectRef{Bucket: s.rawBucket, Key: key},
	}
	objsJSON, err := marshalAvatarObjects(objs)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal avatar objects: %w", err)
	}
	if err := s.repo.UpdateAvatar(ctx, userID, objsJSON); err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			return nil, ErrUserNotFound
		}
		l.Error().Err(err).Str(log.FieldUserID, userID).Msg("failed to persist avatar objects")
		return nil, err
	}

	user, _ := s.repo.GetByID(ctx, userID)
	if user != nil {
		s.invalidateCache(ctx, userID, user.Email)
	} else {
		s.invalidateCache(ctx, userID, "")
	}

	audit.Log(ctx, audit.ActionAvatarPresign, userID, "avatar presigned URL generated")

	return &domain.AvatarPresignResponse{
		UploadURL: uploadURL,
		Key:       key,
		ExpiresIn: int(avatarPresignExpiry.Seconds()),
	}, nil
}

// DeleteAvatar removes avatar data for the user.
// DeleteAvatar tags all avatar objects for lifecycle deletion and clears avatar_objects in DB.
// Storage is cleaned up asynchronously by the bucket lifecycle rule; no immediate Delete.
func (s *userServiceImpl) DeleteAvatar(ctx context.Context, userID string) error {
	l := log.Ctx(ctx)

	user, err := s.repo.GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			return ErrUserNotFound
		}
		l.Error().Err(err).Str(log.FieldUserID, userID).Msg("failed to get user before avatar delete")
		return err
	}

	if user.AvatarObjects != nil {
		objs := user.AvatarObjects
		for _, ref := range []*domain.AvatarObjectRef{objs.Raw, objs.Sm, objs.Md, objs.Lg} {
			if ref == nil {
				continue
			}
			st := s.storageForBucket(ref.Bucket)
			if st == nil {
				continue
			}
			if err := st.TagObject(ctx, ref.Key, s.lifecycle.TagKey, s.lifecycle.TagValue); err != nil {
				l.Warn().Err(err).Str("bucket", ref.Bucket).Str("key", ref.Key).Msg("failed to tag avatar object for deletion")
			}
		}
	}

	if err := s.repo.UpdateAvatar(ctx, userID, nil); err != nil {
		l.Error().Err(err).Str(log.FieldUserID, userID).Msg("failed to clear avatar in db")
		return err
	}

	s.invalidateCache(ctx, userID, user.Email)

	audit.Log(ctx, audit.ActionAvatarDelete, userID, "avatar deleted")
	return nil
}

// HandleAvatarProcessed tags old avatar objects for lifecycle deletion and updates the DB
// with the new bucket+key refs for raw and all processed sizes.
func (s *userServiceImpl) HandleAvatarProcessed(ctx context.Context, userID string, raw domain.AvatarObjectRef, processed domain.AvatarObjects) error {
	l := log.Ctx(ctx)

	user, err := s.repo.GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			return ErrUserNotFound
		}
		l.Error().Err(err).Str(log.FieldUserID, userID).Msg("failed to get user for avatar-processed")
		return err
	}

	// Tag old processed images for lifecycle deletion using stored bucket+key directly.
	if user.AvatarObjects != nil {
		old := user.AvatarObjects
		for _, ref := range []*domain.AvatarObjectRef{old.Sm, old.Md, old.Lg} {
			if ref == nil {
				continue
			}
			st := s.storageForBucket(ref.Bucket)
			if st == nil {
				continue
			}
			if err := st.TagObject(ctx, ref.Key, s.lifecycle.TagKey, s.lifecycle.TagValue); err != nil {
				l.Warn().Err(err).Str("bucket", ref.Bucket).Str("key", ref.Key).Msg("failed to tag old processed avatar for deletion")
			}
		}

		// Tag old raw key (belt-and-suspenders; resize-service already tags the current raw).
		if old.Raw != nil {
			st := s.storageForBucket(old.Raw.Bucket)
			if st != nil {
				if err := st.TagObject(ctx, old.Raw.Key, s.lifecycle.TagKey, s.lifecycle.TagValue); err != nil {
					l.Warn().Err(err).Str("bucket", old.Raw.Bucket).Str("key", old.Raw.Key).Msg("failed to tag old raw avatar for deletion")
				}
			}
		}
	}

	// Defensively remove lifecycle tags from new processed images.
	for _, ref := range []*domain.AvatarObjectRef{processed.Sm, processed.Md, processed.Lg} {
		if ref == nil {
			continue
		}
		st := s.storageForBucket(ref.Bucket)
		if st == nil {
			continue
		}
		if err := st.RemoveObjectTagging(ctx, ref.Key); err != nil {
			l.Warn().Err(err).Str("bucket", ref.Bucket).Str("key", ref.Key).Msg("failed to remove tag from new processed avatar")
		}
	}

	// Build the new AvatarObjects including raw and all processed sizes.
	newObjs := &domain.AvatarObjects{
		Raw: &raw,
		Sm:  processed.Sm,
		Md:  processed.Md,
		Lg:  processed.Lg,
	}

	objsJSON, err := marshalAvatarObjects(newObjs)
	if err != nil {
		return fmt.Errorf("failed to marshal avatar objects: %w", err)
	}

	if err := s.repo.UpdateAvatar(ctx, userID, objsJSON); err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			return ErrUserNotFound
		}
		l.Error().Err(err).Str(log.FieldUserID, userID).Msg("failed to update avatar objects in db")
		return err
	}

	l.Info().Str(log.FieldUserID, userID).Msg("avatar-processed: db updated")
	return nil
}
