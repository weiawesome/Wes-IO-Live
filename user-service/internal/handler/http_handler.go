package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/weiawesome/wes-io-live/pkg/log"
	"github.com/weiawesome/wes-io-live/pkg/middleware"
	"github.com/weiawesome/wes-io-live/pkg/response"
	"github.com/weiawesome/wes-io-live/user-service/internal/domain"
	"github.com/weiawesome/wes-io-live/user-service/internal/repository"
	"github.com/weiawesome/wes-io-live/user-service/internal/service"
)

// Handler handles HTTP requests for user service.
type Handler struct {
	userService    service.UserService
	authMiddleware *middleware.AuthMiddleware
}

// NewHandler creates a new HTTP handler.
func NewHandler(userService service.UserService, authMiddleware *middleware.AuthMiddleware) *Handler {
	return &Handler{
		userService:    userService,
		authMiddleware: authMiddleware,
	}
}

// RegisterRoutes registers all routes.
func (h *Handler) RegisterRoutes(r *gin.Engine) {
	api := r.Group("/api/v1")
	{
		// Public routes
		auth := api.Group("/auth")
		{
			auth.POST("/register", h.Register)
			auth.POST("/login", h.Login)
			auth.POST("/refresh", h.RefreshToken)
			auth.POST("/logout", h.authMiddleware.RequireAuth(), h.Logout)
		}

		// Protected routes
		users := api.Group("/users")
		users.Use(h.authMiddleware.RequireAuth())
		{
			users.GET("/me", h.GetMe)
			users.PUT("/me", h.UpdateMe)
			users.PUT("/me/password", h.ChangePassword)
			users.DELETE("/me", h.DeleteMe)
			users.POST("/me/avatar/presign", h.PresignAvatar)
			users.DELETE("/me/avatar", h.DeleteAvatar)
		}
	}
}

// Register handles user registration.
func (h *Handler) Register(c *gin.Context) {
	ctx := c.Request.Context()
	l := log.Ctx(ctx)
	var req domain.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		l.Warn().Err(err).Msg("invalid register request")
		response.BadRequest(c, err.Error())
		return
	}

	result, err := h.userService.Register(ctx, &req)
	if err != nil {
		if errors.Is(err, repository.ErrEmailExists) {
			response.Conflict(c, "email already exists")
			return
		}
		if errors.Is(err, repository.ErrUsernameExists) {
			response.Conflict(c, "username already exists")
			return
		}
		l.Error().Err(err).Msg("register failed")
		response.InternalError(c, "failed to register user")
		return
	}

	response.Created(c, result)
}

// Login handles user login.
func (h *Handler) Login(c *gin.Context) {
	ctx := c.Request.Context()
	l := log.Ctx(ctx)
	var req domain.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		l.Warn().Err(err).Msg("invalid login request")
		response.BadRequest(c, err.Error())
		return
	}

	result, err := h.userService.Login(ctx, &req)
	if err != nil {
		if errors.Is(err, service.ErrInvalidCredentials) {
			response.Unauthorized(c, "invalid email or password")
			return
		}
		l.Error().Err(err).Msg("login failed")
		response.InternalError(c, "failed to login")
		return
	}

	response.Success(c, result)
}

// RefreshToken handles token refresh.
func (h *Handler) RefreshToken(c *gin.Context) {
	ctx := c.Request.Context()
	l := log.Ctx(ctx)
	var req domain.RefreshTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		l.Warn().Err(err).Msg("invalid refresh token request")
		response.BadRequest(c, err.Error())
		return
	}

	result, err := h.userService.RefreshToken(ctx, &req)
	if err != nil {
		l.Warn().Err(err).Msg("refresh token failed")
		response.Unauthorized(c, "invalid or expired refresh token")
		return
	}

	response.Success(c, result)
}

// Logout handles user logout.
func (h *Handler) Logout(c *gin.Context) {
	ctx := c.Request.Context()
	l := log.Ctx(ctx)
	userID := middleware.GetUserID(c)
	if userID == "" {
		response.Unauthorized(c, "unauthorized")
		return
	}

	if err := h.userService.Logout(ctx, userID); err != nil {
		l.Error().Err(err).Str(log.FieldUserID, userID).Msg("logout failed")
		response.InternalError(c, "failed to logout")
		return
	}

	response.Success(c, gin.H{"message": "logged out successfully"})
}

// GetMe returns current user info.
func (h *Handler) GetMe(c *gin.Context) {
	ctx := c.Request.Context()
	l := log.Ctx(ctx)
	userID := middleware.GetUserID(c)
	if userID == "" {
		response.Unauthorized(c, "unauthorized")
		return
	}

	user, err := h.userService.GetUser(ctx, userID)
	if err != nil {
		if errors.Is(err, service.ErrUserNotFound) {
			response.NotFound(c, "user not found")
			return
		}
		l.Error().Err(err).Str(log.FieldUserID, userID).Msg("get user failed")
		response.InternalError(c, "failed to get user")
		return
	}

	response.Success(c, user)
}

// UpdateMe updates current user.
func (h *Handler) UpdateMe(c *gin.Context) {
	ctx := c.Request.Context()
	l := log.Ctx(ctx)
	userID := middleware.GetUserID(c)
	if userID == "" {
		response.Unauthorized(c, "unauthorized")
		return
	}

	var req domain.UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		l.Warn().Err(err).Msg("invalid update request")
		response.BadRequest(c, err.Error())
		return
	}

	user, err := h.userService.UpdateUser(ctx, userID, &req)
	if err != nil {
		if errors.Is(err, service.ErrUserNotFound) {
			response.NotFound(c, "user not found")
			return
		}
		l.Error().Err(err).Str(log.FieldUserID, userID).Msg("update user failed")
		response.InternalError(c, "failed to update user")
		return
	}

	response.Success(c, user)
}

// ChangePassword changes current user's password.
func (h *Handler) ChangePassword(c *gin.Context) {
	ctx := c.Request.Context()
	l := log.Ctx(ctx)
	userID := middleware.GetUserID(c)
	if userID == "" {
		response.Unauthorized(c, "unauthorized")
		return
	}

	var req domain.ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		l.Warn().Err(err).Msg("invalid change password request")
		response.BadRequest(c, err.Error())
		return
	}

	if err := h.userService.ChangePassword(ctx, userID, &req); err != nil {
		if errors.Is(err, service.ErrUserNotFound) {
			response.NotFound(c, "user not found")
			return
		}
		if errors.Is(err, service.ErrWrongPassword) {
			response.BadRequest(c, "current password is incorrect")
			return
		}
		l.Error().Err(err).Str(log.FieldUserID, userID).Msg("change password failed")
		response.InternalError(c, "failed to change password")
		return
	}

	response.Success(c, gin.H{"message": "password changed successfully"})
}

// DeleteMe deletes current user.
func (h *Handler) DeleteMe(c *gin.Context) {
	ctx := c.Request.Context()
	l := log.Ctx(ctx)
	userID := middleware.GetUserID(c)
	if userID == "" {
		response.Unauthorized(c, "unauthorized")
		return
	}

	if err := h.userService.DeleteUser(ctx, userID); err != nil {
		if errors.Is(err, service.ErrUserNotFound) {
			response.NotFound(c, "user not found")
			return
		}
		l.Error().Err(err).Str(log.FieldUserID, userID).Msg("delete user failed")
		response.InternalError(c, "failed to delete user")
		return
	}

	c.Status(http.StatusNoContent)
}

// PresignAvatar generates a presigned PUT URL for avatar upload.
func (h *Handler) PresignAvatar(c *gin.Context) {
	ctx := c.Request.Context()
	l := log.Ctx(ctx)
	userID := middleware.GetUserID(c)
	if userID == "" {
		response.Unauthorized(c, "unauthorized")
		return
	}

	var req domain.AvatarPresignRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		l.Warn().Err(err).Msg("invalid avatar presign request")
		response.BadRequest(c, err.Error())
		return
	}

	result, err := h.userService.GenerateAvatarUploadURL(ctx, userID, req.ContentType)
	if err != nil {
		if errors.Is(err, service.ErrUserNotFound) {
			response.NotFound(c, "user not found")
			return
		}
		l.Error().Err(err).Str(log.FieldUserID, userID).Msg("avatar presign failed")
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, result)
}

// DeleteAvatar removes the current user's avatar.
func (h *Handler) DeleteAvatar(c *gin.Context) {
	ctx := c.Request.Context()
	l := log.Ctx(ctx)
	userID := middleware.GetUserID(c)
	if userID == "" {
		response.Unauthorized(c, "unauthorized")
		return
	}

	if err := h.userService.DeleteAvatar(ctx, userID); err != nil {
		if errors.Is(err, service.ErrUserNotFound) {
			response.NotFound(c, "user not found")
			return
		}
		l.Error().Err(err).Str(log.FieldUserID, userID).Msg("delete avatar failed")
		response.InternalError(c, "failed to delete avatar")
		return
	}

	c.Status(http.StatusNoContent)
}
