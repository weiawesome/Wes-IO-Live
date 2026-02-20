package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/weiawesome/wes-io-live/pkg/log"
	"github.com/weiawesome/wes-io-live/pkg/response"
	"github.com/weiawesome/wes-io-live/search-service/internal/domain"
	"github.com/weiawesome/wes-io-live/search-service/internal/service"
)

// Handler handles HTTP requests for search service.
type Handler struct {
	searchService service.SearchService
}

// NewHandler creates a new HTTP handler.
func NewHandler(searchService service.SearchService) *Handler {
	return &Handler{
		searchService: searchService,
	}
}

// RegisterRoutes registers all routes.
func (h *Handler) RegisterRoutes(r *gin.Engine) {
	api := r.Group("/api/v1")
	{
		api.GET("/search", h.Search)
		api.GET("/search/users", h.SearchUsers)
		api.GET("/search/rooms", h.SearchRooms)
	}
}

// Search handles unified search across users and rooms.
func (h *Handler) Search(c *gin.Context) {
	ctx := c.Request.Context()
	l := log.Ctx(ctx)

	var req domain.SearchRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		l.Warn().Err(err).Msg("invalid search request")
		response.BadRequest(c, err.Error())
		return
	}

	result, err := h.searchService.Search(ctx, &req)
	if err != nil {
		l.Error().Err(err).Str("query", req.Query).Msg("search failed")
		response.InternalError(c, "search failed")
		return
	}

	response.Success(c, result)
}

// SearchUsers handles user-only search.
func (h *Handler) SearchUsers(c *gin.Context) {
	ctx := c.Request.Context()
	l := log.Ctx(ctx)

	var req domain.SearchRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		l.Warn().Err(err).Msg("invalid search request")
		response.BadRequest(c, err.Error())
		return
	}

	result, err := h.searchService.SearchUsers(ctx, &req)
	if err != nil {
		l.Error().Err(err).Str("query", req.Query).Msg("search users failed")
		response.InternalError(c, "search failed")
		return
	}

	response.Success(c, result)
}

// SearchRooms handles room-only search.
func (h *Handler) SearchRooms(c *gin.Context) {
	ctx := c.Request.Context()
	l := log.Ctx(ctx)

	var req domain.SearchRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		l.Warn().Err(err).Msg("invalid search request")
		response.BadRequest(c, err.Error())
		return
	}

	result, err := h.searchService.SearchRooms(ctx, &req)
	if err != nil {
		l.Error().Err(err).Str("query", req.Query).Msg("search rooms failed")
		response.InternalError(c, "search failed")
		return
	}

	response.Success(c, result)
}
