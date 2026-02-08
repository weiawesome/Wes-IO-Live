package handler

import (
	"net/http"
	"strconv"

	"github.com/weiawesome/wes-io-live/chat-history-service/internal/domain"
	"github.com/weiawesome/wes-io-live/chat-history-service/internal/service"
	"github.com/gin-gonic/gin"
)

const (
	defaultLimit = 50
	maxLimit     = 100
)

type HTTPHandler struct {
	chatHistoryService service.ChatHistoryService
}

func NewHTTPHandler(chatHistoryService service.ChatHistoryService) *HTTPHandler {
	return &HTTPHandler{
		chatHistoryService: chatHistoryService,
	}
}

func (h *HTTPHandler) RegisterRoutes(r *gin.Engine) {
	api := r.Group("/api/v1")
	{
		api.GET("/rooms/:room_id/sessions/:session_id/messages", h.GetMessages)
	}

	r.GET("/health", h.HealthCheck)
}

func (h *HTTPHandler) GetMessages(c *gin.Context) {
	roomID := c.Param("room_id")
	sessionID := c.Param("session_id")

	if roomID == "" || sessionID == "" {
		c.JSON(http.StatusBadRequest, domain.APIResponse{
			Success: false,
			Error:   "room_id and session_id are required",
		})
		return
	}

	cursor := c.Query("cursor")
	direction := c.DefaultQuery("direction", "backward")

	if direction != "backward" && direction != "forward" {
		c.JSON(http.StatusBadRequest, domain.APIResponse{
			Success: false,
			Error:   "direction must be 'backward' or 'forward'",
		})
		return
	}

	limit := defaultLimit
	if limitStr := c.Query("limit"); limitStr != "" {
		parsedLimit, err := strconv.Atoi(limitStr)
		if err != nil || parsedLimit < 1 {
			c.JSON(http.StatusBadRequest, domain.APIResponse{
				Success: false,
				Error:   "limit must be a positive integer",
			})
			return
		}
		limit = parsedLimit
		if limit > maxLimit {
			limit = maxLimit
		}
	}

	result, err := h.chatHistoryService.GetChatHistory(c.Request.Context(), roomID, sessionID, cursor, limit, direction)
	if err != nil {
		c.JSON(http.StatusInternalServerError, domain.APIResponse{
			Success: false,
			Error:   "failed to get chat history",
		})
		return
	}

	c.JSON(http.StatusOK, domain.APIResponse{
		Success: true,
		Data:    result,
	})
}

func (h *HTTPHandler) HealthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
	})
}
