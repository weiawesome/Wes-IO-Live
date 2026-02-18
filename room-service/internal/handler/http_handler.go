package handler

import (
	"errors"

	"github.com/gin-gonic/gin"

	"github.com/weiawesome/wes-io-live/pkg/log"
	"github.com/weiawesome/wes-io-live/pkg/middleware"
	"github.com/weiawesome/wes-io-live/pkg/response"
	"github.com/weiawesome/wes-io-live/room-service/internal/domain"
	"github.com/weiawesome/wes-io-live/room-service/internal/service"
)

// Handler handles HTTP requests for room service.
type Handler struct {
	roomService    service.RoomService
	authMiddleware *middleware.AuthMiddleware
}

// NewHandler creates a new HTTP handler.
func NewHandler(roomService service.RoomService, authMiddleware *middleware.AuthMiddleware) *Handler {
	return &Handler{
		roomService:    roomService,
		authMiddleware: authMiddleware,
	}
}

// RegisterRoutes registers all routes.
func (h *Handler) RegisterRoutes(r *gin.Engine) {
	api := r.Group("/api/v1")
	{
		rooms := api.Group("/rooms")
		{
			// Public routes
			rooms.GET("", h.ListRooms)
			rooms.GET("/search", h.SearchRooms)
			rooms.GET("/:id", h.GetRoom)

			// Protected routes
			rooms.POST("", h.authMiddleware.RequireAuth(), h.CreateRoom)
			rooms.DELETE("/:id", h.authMiddleware.RequireAuth(), h.CloseRoom)
			rooms.GET("/my", h.authMiddleware.RequireAuth(), h.GetMyRooms)
		}
	}
}

// CreateRoom creates a new room.
func (h *Handler) CreateRoom(c *gin.Context) {
	ctx := c.Request.Context()
	l := log.Ctx(ctx)

	userID := middleware.GetUserID(c)
	username := middleware.GetUsername(c)
	if userID == "" {
		response.Unauthorized(c, "unauthorized")
		return
	}

	var req domain.CreateRoomRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		l.Warn().Err(err).Msg("failed to bind create room request")
		response.BadRequest(c, err.Error())
		return
	}

	room, err := h.roomService.CreateRoom(ctx, userID, username, &req)
	if err != nil {
		if errors.Is(err, service.ErrMaxRoomsReached) {
			response.Error(c, 429, "MAX_ROOMS_REACHED", "you have reached the maximum number of active rooms")
			return
		}
		l.Error().Err(err).Msg("failed to create room")
		response.InternalError(c, "failed to create room")
		return
	}

	response.Created(c, room)
}

// GetRoom retrieves a room by ID.
func (h *Handler) GetRoom(c *gin.Context) {
	ctx := c.Request.Context()
	l := log.Ctx(ctx)

	roomID := c.Param("id")

	room, err := h.roomService.GetRoom(ctx, roomID)
	if err != nil {
		if errors.Is(err, service.ErrRoomNotFound) {
			response.NotFound(c, "room not found")
			return
		}
		l.Error().Err(err).Str("room_id", roomID).Msg("failed to get room")
		response.InternalError(c, "failed to get room")
		return
	}

	response.Success(c, room)
}

// ListRooms lists rooms with pagination.
func (h *Handler) ListRooms(c *gin.Context) {
	ctx := c.Request.Context()
	l := log.Ctx(ctx)

	var req domain.ListRoomsRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	if req.Page < 1 {
		req.Page = 1
	}
	if req.PageSize < 1 || req.PageSize > 100 {
		req.PageSize = 20
	}

	result, err := h.roomService.ListRooms(ctx, req.Page, req.PageSize, req.Status)
	if err != nil {
		l.Error().Err(err).Msg("failed to list rooms")
		response.InternalError(c, "failed to list rooms")
		return
	}

	response.Success(c, result)
}

// SearchRooms searches rooms.
func (h *Handler) SearchRooms(c *gin.Context) {
	ctx := c.Request.Context()
	l := log.Ctx(ctx)

	var req domain.SearchRoomsRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		l.Warn().Err(err).Msg("failed to bind search rooms request")
		response.BadRequest(c, err.Error())
		return
	}

	if req.Page < 1 {
		req.Page = 1
	}
	if req.PageSize < 1 || req.PageSize > 100 {
		req.PageSize = 20
	}

	result, err := h.roomService.SearchRooms(ctx, req.Query, req.Page, req.PageSize)
	if err != nil {
		l.Error().Err(err).Msg("failed to search rooms")
		response.InternalError(c, "failed to search rooms")
		return
	}

	response.Success(c, result)
}

// GetMyRooms retrieves current user's rooms.
func (h *Handler) GetMyRooms(c *gin.Context) {
	ctx := c.Request.Context()
	l := log.Ctx(ctx)

	userID := middleware.GetUserID(c)
	if userID == "" {
		response.Unauthorized(c, "unauthorized")
		return
	}

	rooms, err := h.roomService.GetMyRooms(ctx, userID)
	if err != nil {
		l.Error().Err(err).Msg("failed to get my rooms")
		response.InternalError(c, "failed to get rooms")
		return
	}

	// Get stats
	activeCount, maxAllowed, _ := h.roomService.GetRoomStats(ctx, userID)

	response.Success(c, gin.H{
		"rooms":        rooms,
		"active_count": activeCount,
		"max_allowed":  maxAllowed,
	})
}

// CloseRoom closes a room.
func (h *Handler) CloseRoom(c *gin.Context) {
	ctx := c.Request.Context()
	l := log.Ctx(ctx)

	userID := middleware.GetUserID(c)
	if userID == "" {
		response.Unauthorized(c, "unauthorized")
		return
	}

	roomID := c.Param("id")

	err := h.roomService.CloseRoom(ctx, userID, roomID)
	if err != nil {
		if errors.Is(err, service.ErrRoomNotFound) {
			response.NotFound(c, "room not found")
			return
		}
		if errors.Is(err, service.ErrNotRoomOwner) {
			response.Forbidden(c, "you are not the owner of this room")
			return
		}
		l.Error().Err(err).Str("room_id", roomID).Msg("failed to close room")
		response.InternalError(c, "failed to close room")
		return
	}

	response.Success(c, gin.H{"message": "room closed successfully"})
}
