package handler

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/weiawesome/wes-io-live/presence-service/internal/domain"
	"github.com/weiawesome/wes-io-live/presence-service/internal/hub"
	"github.com/weiawesome/wes-io-live/presence-service/internal/service"
)

// WSHandler handles WebSocket connections for presence.
type WSHandler struct {
	hub      *hub.Hub
	service  service.PresenceService
	upgrader websocket.Upgrader
}

// NewWSHandler creates a new WebSocket handler.
func NewWSHandler(h *hub.Hub, svc service.PresenceService) *WSHandler {
	return &WSHandler{
		hub:     h,
		service: svc,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				return true // Allow all origins for now
			},
		},
	}
}

// HandleWebSocket handles WebSocket upgrade and connection.
func (h *WSHandler) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}

	clientID := uuid.New().String()
	client := &hub.Client{
		ID:   clientID,
		Hub:  h.hub,
		Conn: conn,
		Send: make(chan []byte, 256),
	}

	h.hub.Register(client)

	// Start goroutines for reading and writing
	go client.WritePump()
	go client.ReadPump(func(c *hub.Client, message []byte) {
		h.handleMessage(c, message)
	})
}

func (h *WSHandler) handleMessage(c *hub.Client, message []byte) {
	ctx := context.Background()

	// Parse base message to determine type
	var base domain.BaseMessage
	if err := json.Unmarshal(message, &base); err != nil {
		log.Printf("Failed to parse message: %v", err)
		c.SendMessage(domain.NewErrorMessage("invalid message format"))
		return
	}

	switch base.Type {
	case domain.MsgTypeJoin:
		var msg domain.JoinMessage
		if err := json.Unmarshal(message, &msg); err != nil {
			c.SendMessage(domain.NewErrorMessage("invalid join message"))
			return
		}
		if msg.RoomID == "" {
			c.SendMessage(domain.NewErrorMessage("room_id is required"))
			return
		}
		if err := h.service.HandleJoin(ctx, c, msg.RoomID, msg.Token, msg.DeviceHash); err != nil {
			log.Printf("HandleJoin error: %v", err)
		}

	case domain.MsgTypeLeave:
		var msg domain.LeaveMessage
		if err := json.Unmarshal(message, &msg); err != nil {
			c.SendMessage(domain.NewErrorMessage("invalid leave message"))
			return
		}
		if msg.RoomID == "" {
			c.SendMessage(domain.NewErrorMessage("room_id is required"))
			return
		}
		if err := h.service.HandleLeave(ctx, c, msg.RoomID); err != nil {
			log.Printf("HandleLeave error: %v", err)
		}

	case domain.MsgTypePing:
		if err := h.service.HandleHeartbeat(ctx, c); err != nil {
			log.Printf("HandleHeartbeat error: %v", err)
		}

	default:
		c.SendMessage(domain.NewErrorMessage("unknown message type: " + base.Type))
	}
}

// OnDisconnect is called when a client disconnects.
func (h *WSHandler) OnDisconnect(c *hub.Client) {
	ctx := context.Background()
	if err := h.service.HandleDisconnect(ctx, c); err != nil {
		log.Printf("HandleDisconnect error: %v", err)
	}
}
