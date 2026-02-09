package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/weiawesome/wes-io-live/playback-service/internal/service"
)

// VODHandler handles VOD playback requests.
type VODHandler struct {
	playbackSvc *service.PlaybackService
}

// NewVODHandler creates a new VOD handler.
func NewVODHandler(playbackSvc *service.PlaybackService) *VODHandler {
	return &VODHandler{
		playbackSvc: playbackSvc,
	}
}

// RegisterRoutes registers the VOD playback routes.
func (h *VODHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/vod/", h.handleVOD)
}

// handleVOD handles VOD requests.
// Supports:
// - GET /vod/{roomID} - List all VOD sessions for a room
// - GET /vod/{roomID}/latest - Get the latest VOD URL
// - GET /vod/{roomID}/{sessionID}/{file} - Stream VOD content
func (h *VODHandler) handleVOD(w http.ResponseWriter, r *http.Request) {
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

	// Parse path: /vod/{roomID}/...
	path := strings.TrimPrefix(r.URL.Path, "/vod/")
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
	roomID := parts[0]

	// Handle list request: /vod/{roomID}
	if len(parts) == 1 {
		h.handleListVODs(w, r, roomID)
		return
	}

	// Handle latest request: /vod/{roomID}/latest
	if len(parts) == 2 && parts[1] == "latest" {
		h.handleLatestVOD(w, r, roomID)
		return
	}

	// Handle file request: /vod/{roomID}/{sessionID}/{file}
	if len(parts) >= 2 {
		sessionID := parts[1]

		// If only sessionID without file, return session info
		if len(parts) == 2 {
			h.handleSessionInfo(w, r, roomID, sessionID)
			return
		}

		filename := parts[2]
		h.handleVODContent(w, r, roomID, sessionID, filename)
		return
	}

	http.Error(w, "Invalid path format", http.StatusBadRequest)
}

// handleListVODs returns a list of all VOD sessions for a room.
func (h *VODHandler) handleListVODs(w http.ResponseWriter, r *http.Request, roomID string) {
	vods, err := h.playbackSvc.ListRoomVODs(r.Context(), roomID)
	if err != nil {
		log.Printf("Error listing VODs for room %s: %v", roomID, err)
		http.Error(w, "Failed to list VODs", http.StatusInternalServerError)
		return
	}

	if len(vods) == 0 {
		http.Error(w, "No VOD recordings found for room "+roomID, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"room_id":  roomID,
		"sessions": vods,
	})
}

// handleLatestVOD returns the URL for the latest VOD session.
func (h *VODHandler) handleLatestVOD(w http.ResponseWriter, r *http.Request, roomID string) {
	sessionID, url, err := h.playbackSvc.GetLatestVODURL(r.Context(), roomID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "no VOD") {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		log.Printf("Error getting latest VOD for room %s: %v", roomID, err)
		http.Error(w, "Failed to get latest VOD", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"room_id":    roomID,
		"session_id": sessionID,
		"url":        url,
	})
}

// handleSessionInfo returns information about a specific VOD session.
func (h *VODHandler) handleSessionInfo(w http.ResponseWriter, r *http.Request, roomID, sessionID string) {
	url, err := h.playbackSvc.GetSessionVODURL(r.Context(), roomID, sessionID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		log.Printf("Error getting VOD URL for room %s session %s: %v", roomID, sessionID, err)
		http.Error(w, "Failed to get VOD URL", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"room_id":    roomID,
		"session_id": sessionID,
		"url":        url,
	})
}

// handleVODContent serves VOD content files.
func (h *VODHandler) handleVODContent(w http.ResponseWriter, r *http.Request, roomID, sessionID, filename string) {
	// Only allow .m3u8 and .ts files
	ext := filepath.Ext(filename)
	if ext != ".m3u8" && ext != ".ts" {
		http.NotFound(w, r)
		return
	}

	err := h.playbackSvc.ServeVODContent(r.Context(), w, r, roomID, sessionID, filename)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			http.NotFound(w, r)
			return
		}
		log.Printf("Error serving VOD content: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}
