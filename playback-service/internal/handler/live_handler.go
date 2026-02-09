package handler

import (
	"log"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/weiawesome/wes-io-live/playback-service/internal/service"
)

// LiveHandler handles live stream playback requests.
type LiveHandler struct {
	playbackSvc *service.PlaybackService
}

// NewLiveHandler creates a new live handler.
func NewLiveHandler(playbackSvc *service.PlaybackService) *LiveHandler {
	return &LiveHandler{
		playbackSvc: playbackSvc,
	}
}

// RegisterRoutes registers the live playback routes.
func (h *LiveHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/live/", h.handleLive)
}

// handleLive handles live stream requests.
// Supports:
// - GET /live/{roomID}/stream.m3u8 - Live stream (auto-detect sessionID from session store)
// - GET /live/{roomID}/{sessionID}/{file} - Live stream with explicit sessionID
func (h *LiveHandler) handleLive(w http.ResponseWriter, r *http.Request) {
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

	// Parse path: /live/{roomID}/... or /live/{roomID}/{sessionID}/...
	path := strings.TrimPrefix(r.URL.Path, "/live/")
	if path == "" || path == r.URL.Path {
		http.Error(w, "Room ID required", http.StatusBadRequest)
		return
	}

	// Security: prevent directory traversal
	cleanPath := filepath.Clean(path)
	if strings.Contains(cleanPath, "..") {
		http.NotFound(w, r)
		return
	}

	parts := strings.SplitN(cleanPath, "/", 3)
	roomID := parts[0]

	// Handle simplified live stream request: /live/{roomID}/stream.m3u8
	if len(parts) == 2 && parts[1] == "stream.m3u8" {
		h.handleSimplifiedLive(w, r, roomID)
		return
	}

	// Handle explicit session request: /live/{roomID}/{sessionID}/{file}
	if len(parts) >= 3 {
		sessionID := parts[1]
		filename := parts[2]
		h.handleSessionLive(w, r, roomID, sessionID, filename)
		return
	}

	http.Error(w, "Invalid path format", http.StatusBadRequest)
}

// handleSimplifiedLive handles requests without explicit sessionID.
// It looks up the active session from the session store.
func (h *LiveHandler) handleSimplifiedLive(w http.ResponseWriter, r *http.Request, roomID string) {
	// Try to get active session from session store
	sessionID, err := h.playbackSvc.GetActiveSessionID(r.Context(), roomID)
	if err != nil {
		log.Printf("Error getting active session for room %s: %v", roomID, err)
		http.Error(w, "Failed to get active session", http.StatusInternalServerError)
		return
	}

	if sessionID == "" {
		http.Error(w, "No active live stream for room "+roomID, http.StatusNotFound)
		return
	}

	// Redirect to the session-specific URL
	if h.playbackSvc.IsRedirectMode() {
		// In redirect mode, serve content directly (will redirect to S3)
		h.handleSessionLive(w, r, roomID, sessionID, "stream.m3u8")
	} else {
		// In proxy mode, redirect to session-specific URL
		redirectURL := "/live/" + roomID + "/" + sessionID + "/stream.m3u8"
		http.Redirect(w, r, redirectURL, http.StatusTemporaryRedirect)
	}
}

// handleSessionLive handles live stream requests with explicit sessionID.
func (h *LiveHandler) handleSessionLive(w http.ResponseWriter, r *http.Request, roomID, sessionID, filename string) {
	// Only allow .m3u8 and .ts files
	ext := filepath.Ext(filename)
	if ext != ".m3u8" && ext != ".ts" {
		http.NotFound(w, r)
		return
	}

	err := h.playbackSvc.ServeLiveContent(r.Context(), w, r, roomID, sessionID, filename)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			http.NotFound(w, r)
			return
		}
		log.Printf("Error serving live content: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}
