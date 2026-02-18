package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// HealthHandler handles health check requests.
type HealthHandler struct {
	version string
}

// NewHealthHandler creates a new health handler.
func NewHealthHandler(version string) *HealthHandler {
	return &HealthHandler{
		version: version,
	}
}

// RegisterRoutes registers the health check routes.
func (h *HealthHandler) RegisterRoutes(r *gin.Engine) {
	r.Any("/health", h.handleHealth)
	r.Any("/healthz", h.handleHealth)
}

// handleHealth returns the service health status.
func (h *HealthHandler) handleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"service": "playback-service",
		"version": h.version,
	})
}

// setCORSHeaders sets CORS headers for cross-origin requests.
func setCORSHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Range")
	w.Header().Set("Access-Control-Expose-Headers", "Content-Length, Content-Range")
}
