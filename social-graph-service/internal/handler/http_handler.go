package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	pkglog "github.com/weiawesome/wes-io-live/pkg/log"
	"github.com/weiawesome/wes-io-live/pkg/middleware"
	"github.com/weiawesome/wes-io-live/pkg/response"
	"github.com/weiawesome/wes-io-live/social-graph-service/internal/service"
)

// Handler handles HTTP requests for the social graph service.
type Handler struct {
	svc            service.SocialGraphService
	authMiddleware *middleware.AuthMiddleware
}

// NewHandler creates a new HTTP handler.
func NewHandler(svc service.SocialGraphService, authMiddleware *middleware.AuthMiddleware) *Handler {
	return &Handler{
		svc:            svc,
		authMiddleware: authMiddleware,
	}
}

// RegisterRoutes registers all routes onto the Gin engine.
func (h *Handler) RegisterRoutes(r *gin.Engine) {
	api := r.Group("/api/v1")
	{
		users := api.Group("/users")
		{
			// POST /api/v1/users/:user_id/follow — auth required
			users.POST("/:user_id/follow", h.authMiddleware.RequireAuth(), h.Follow)
			// DELETE /api/v1/users/:user_id/follow — auth required
			users.DELETE("/:user_id/follow", h.authMiddleware.RequireAuth(), h.Unfollow)
			// GET /api/v1/users/:user_id/followers/count — no auth
			users.GET("/:user_id/followers/count", h.GetFollowersCount)
			// POST /api/v1/users/:user_id/following/status — no auth
			users.POST("/:user_id/following/status", h.BatchIsFollowing)
		}
	}
}

// Follow handles POST /api/v1/users/:user_id/follow.
// The authenticated user follows the target user.
func (h *Handler) Follow(c *gin.Context) {
	ctx := c.Request.Context()
	l := pkglog.Ctx(ctx)

	followerID := middleware.GetUserID(c)
	if followerID == "" {
		response.Unauthorized(c, "unauthorized")
		return
	}

	targetID := c.Param("user_id")
	if targetID == "" {
		response.BadRequest(c, "user_id is required")
		return
	}

	if err := h.svc.Follow(ctx, followerID, targetID); err != nil {
		switch {
		case errors.Is(err, service.ErrSelfFollow):
			response.BadRequest(c, "cannot follow yourself")
		case errors.Is(err, service.ErrAlreadyFollowing):
			response.Conflict(c, "already following")
		default:
			l.Error().Err(err).
				Str("follower_id", followerID).
				Str("target_id", targetID).
				Msg("follow failed")
			response.InternalError(c, "failed to follow user")
		}
		return
	}

	response.Created(c, gin.H{"message": "followed successfully"})
}

// Unfollow handles DELETE /api/v1/users/:user_id/follow.
// The authenticated user unfollows the target user.
func (h *Handler) Unfollow(c *gin.Context) {
	ctx := c.Request.Context()
	l := pkglog.Ctx(ctx)

	followerID := middleware.GetUserID(c)
	if followerID == "" {
		response.Unauthorized(c, "unauthorized")
		return
	}

	targetID := c.Param("user_id")
	if targetID == "" {
		response.BadRequest(c, "user_id is required")
		return
	}

	if err := h.svc.Unfollow(ctx, followerID, targetID); err != nil {
		switch {
		case errors.Is(err, service.ErrNotFollowing):
			response.Conflict(c, "not following")
		default:
			l.Error().Err(err).
				Str("follower_id", followerID).
				Str("target_id", targetID).
				Msg("unfollow failed")
			response.InternalError(c, "failed to unfollow user")
		}
		return
	}

	c.Status(http.StatusNoContent)
}

// GetFollowersCount handles GET /api/v1/users/:user_id/followers/count.
func (h *Handler) GetFollowersCount(c *gin.Context) {
	ctx := c.Request.Context()
	l := pkglog.Ctx(ctx)

	userID := c.Param("user_id")
	if userID == "" {
		response.BadRequest(c, "user_id is required")
		return
	}

	count, err := h.svc.GetFollowersCount(ctx, userID)
	if err != nil {
		l.Error().Err(err).Str("user_id", userID).Msg("get followers count failed")
		response.InternalError(c, "failed to get followers count")
		return
	}

	response.Success(c, gin.H{"count": count})
}

// followingStatusRequest is the request body for POST /users/:follower_id/following/status.
type followingStatusRequest struct {
	TargetIDs []string `json:"target_ids" binding:"required"`
}

// BatchIsFollowing handles POST /api/v1/users/:user_id/following/status.
func (h *Handler) BatchIsFollowing(c *gin.Context) {
	ctx := c.Request.Context()
	l := pkglog.Ctx(ctx)

	followerID := c.Param("user_id")
	if followerID == "" {
		response.BadRequest(c, "user_id is required")
		return
	}

	var req followingStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		l.Warn().Err(err).Msg("invalid following status request")
		response.BadRequest(c, err.Error())
		return
	}

	results, err := h.svc.BatchIsFollowing(ctx, followerID, req.TargetIDs)
	if err != nil {
		l.Error().Err(err).Str("follower_id", followerID).Msg("batch is-following failed")
		response.InternalError(c, "failed to check following status")
		return
	}

	response.Success(c, gin.H{"results": results})
}
