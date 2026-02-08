package handler

import (
	"encoding/json"
	"net/http"

	"github.com/weiawesome/wes-io-live/ice-service/internal/config"
)

// ICEHandler serves ICE server configuration.
type ICEHandler struct {
	iceServers []config.ICEServer
}

// NewICEHandler creates a new ICE handler.
func NewICEHandler(iceServers []config.ICEServer) *ICEHandler {
	return &ICEHandler{
		iceServers: iceServers,
	}
}

// ServeHTTP handles ICE server requests.
func (h *ICEHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Set CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	w.Header().Set("Content-Type", "application/json")

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	servers := h.iceServers

	// Always include a STUN server as fallback
	hasSTUN := false
	for _, s := range servers {
		for _, url := range s.URLs {
			if len(url) > 4 && url[:4] == "stun" {
				hasSTUN = true
				break
			}
		}
	}
	if !hasSTUN {
		servers = append([]config.ICEServer{{
			URLs: []string{"stun:stun.l.google.com:19302"},
		}}, servers...)
	}

	response := map[string]interface{}{
		"iceServers": servers,
	}

	json.NewEncoder(w).Encode(response)
}

// RegisterRoutes registers the ICE routes.
func (h *ICEHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("/api/ice-servers", h)
}
