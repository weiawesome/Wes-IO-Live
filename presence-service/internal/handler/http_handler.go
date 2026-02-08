package handler

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/weiawesome/wes-io-live/presence-service/internal/domain"
	"github.com/weiawesome/wes-io-live/presence-service/internal/service"
)

// HTTPHandler handles HTTP API requests for presence.
type HTTPHandler struct {
	service service.PresenceService
}

// NewHTTPHandler creates a new HTTP handler.
func NewHTTPHandler(svc service.PresenceService) *HTTPHandler {
	return &HTTPHandler{
		service: svc,
	}
}

// PresenceResponse is the API response for presence queries.
type PresenceResponse struct {
	Authenticated int `json:"authenticated"`
	Anonymous     int `json:"anonymous"`
	Total         int `json:"total"`
}

// GetPresence handles GET /api/v1/rooms/{room_id}/presence
func (h *HTTPHandler) GetPresence(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	roomID := vars["room_id"]

	if roomID == "" {
		http.Error(w, "room_id is required", http.StatusBadRequest)
		return
	}

	count, err := h.service.GetRoomCount(r.Context(), roomID)
	if err != nil {
		http.Error(w, "failed to get presence count", http.StatusInternalServerError)
		return
	}

	response := PresenceResponse{
		Authenticated: count.Authenticated,
		Anonymous:     count.Anonymous,
		Total:         count.Total,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// GetRoomInfo handles GET /api/v1/rooms/{room_id}
// Returns complete room info including presence count and live status.
func (h *HTTPHandler) GetRoomInfo(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	roomID := vars["room_id"]

	if roomID == "" {
		http.Error(w, "room_id is required", http.StatusBadRequest)
		return
	}

	roomInfo, err := h.service.GetRoomInfo(r.Context(), roomID)
	if err != nil {
		http.Error(w, "failed to get room info", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(roomInfo)
}

// GetLiveRooms handles GET /api/v1/live-rooms
// Returns all room IDs that are currently live.
func (h *HTTPHandler) GetLiveRooms(w http.ResponseWriter, r *http.Request) {
	rooms, err := h.service.GetAllLiveRooms(r.Context())
	if err != nil {
		http.Error(w, "failed to get live rooms", http.StatusInternalServerError)
		return
	}

	response := domain.LiveRoomsResponse{
		Rooms: rooms,
		Total: len(rooms),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// HealthCheck handles GET /health
func (h *HTTPHandler) HealthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
