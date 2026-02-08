package handler

import (
	"encoding/json"
	"net/http"
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
func (h *HealthHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/health", h.handleHealth)
	mux.HandleFunc("/healthz", h.handleHealth)
}

// handleHealth returns the service health status.
func (h *HealthHandler) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
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
