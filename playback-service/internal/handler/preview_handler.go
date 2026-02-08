package handler

import (
	"log"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/weiawesome/wes-io-live/playback-service/internal/service"
)

// PreviewHandler handles preview image requests.
type PreviewHandler struct {
	playbackSvc *service.PlaybackService
}

// NewPreviewHandler creates a new preview handler.
func NewPreviewHandler(playbackSvc *service.PlaybackService) *PreviewHandler {
	return &PreviewHandler{
		playbackSvc: playbackSvc,
	}
}

// RegisterRoutes registers the preview routes.
func (h *PreviewHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/preview/", h.handlePreview)
}

// handlePreview handles preview requests.
// Supports:
// - GET /preview/{roomID}/latest/thumbnail.jpg - Get latest session thumbnail
// - GET /preview/{roomID}/{sessionID}/thumbnail.jpg - Get specific session thumbnail
func (h *PreviewHandler) handlePreview(w http.ResponseWriter, r *http.Request) {
	// Set CORS headers
	setCORSHeaders(w)

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse path: /preview/{roomID}/...
	path := strings.TrimPrefix(r.URL.Path, "/preview/")
	if path == "" || path == r.URL.Path {
		http.Error(w, "Room ID required", http.StatusBadRequest)
		return
	}

	// Remove trailing slash
	path = strings.TrimSuffix(path, "/")

	// Security: prevent directory traversal
	cleanPath := filepath.Clean(path)
	if strings.Contains(cleanPath, "..") {
		http.NotFound(w, r)
		return
	}

	parts := strings.SplitN(cleanPath, "/", 3)
	if len(parts) < 2 {
		http.Error(w, "Invalid path format", http.StatusBadRequest)
		return
	}

	roomID := parts[0]

	// Handle latest: /preview/{roomID}/latest/thumbnail.jpg
	if parts[1] == "latest" {
		filename := "thumbnail.jpg"
		if len(parts) >= 3 {
			filename = parts[2]
		}
		h.handleLatestPreview(w, r, roomID, filename)
		return
	}

	// Handle specific session: /preview/{roomID}/{sessionID}/thumbnail.jpg
	sessionID := parts[1]
	filename := "thumbnail.jpg"
	if len(parts) >= 3 {
		filename = parts[2]
	}
	h.handleSessionPreview(w, r, roomID, sessionID, filename)
}

// handleLatestPreview serves the latest session's preview image.
func (h *PreviewHandler) handleLatestPreview(w http.ResponseWriter, r *http.Request, roomID, filename string) {
	// Only allow image files
	ext := filepath.Ext(filename)
	if ext != ".jpg" && ext != ".jpeg" && ext != ".png" && ext != ".webp" {
		http.NotFound(w, r)
		return
	}

	err := h.playbackSvc.ServeLatestPreview(r.Context(), w, r, roomID, filename)
	if err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "no active") || strings.Contains(err.Error(), "no preview") {
			http.NotFound(w, r)
			return
		}
		log.Printf("Error serving latest preview for room %s: %v", roomID, err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// handleSessionPreview serves a specific session's preview image.
func (h *PreviewHandler) handleSessionPreview(w http.ResponseWriter, r *http.Request, roomID, sessionID, filename string) {
	// Only allow image files
	ext := filepath.Ext(filename)
	if ext != ".jpg" && ext != ".jpeg" && ext != ".png" && ext != ".webp" {
		http.NotFound(w, r)
		return
	}

	err := h.playbackSvc.ServePreviewContent(r.Context(), w, r, roomID, sessionID, filename)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			http.NotFound(w, r)
			return
		}
		log.Printf("Error serving preview for room %s session %s: %v", roomID, sessionID, err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}
