package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	pkglog "github.com/weiawesome/wes-io-live/pkg/log"
	"github.com/weiawesome/wes-io-live/signal-service/internal/domain"
	"github.com/weiawesome/wes-io-live/signal-service/internal/hub"
	"github.com/weiawesome/wes-io-live/signal-service/internal/service"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for development
	},
}

// WSHandler handles WebSocket connections.
type WSHandler struct {
	hub     *hub.Hub
	service service.SignalService
}

// NewWSHandler creates a new WebSocket handler.
func NewWSHandler(h *hub.Hub, svc service.SignalService) *WSHandler {
	return &WSHandler{
		hub:     h,
		service: svc,
	}
}

// HandleWebSocket handles WebSocket upgrade and message routing.
func (h *WSHandler) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	l := pkglog.L()

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		l.Error().Err(err).Msg("websocket upgrade failed")
		return
	}

	clientID := uuid.New().String()
	client := &hub.Client{
		ID:      clientID,
		Hub:     h.hub,
		Conn:    conn,
		Send:    make(chan []byte, 256),
		Session: domain.NewSession(clientID),
	}

	// Set disconnect handler to clean up broadcast state
	client.SetDisconnectHandler(func(c *hub.Client) {
		ctx := context.Background()
		if err := h.service.HandleDisconnect(ctx, c); err != nil {
			l.Error().Err(err).Str("client_id", c.ID).Msg("disconnect handler error")
		}
	})

	h.hub.Register(client)

	go client.WritePump()
	go client.ReadPump(h.handleMessage)
}

func (h *WSHandler) handleMessage(client *hub.Client, message []byte) {
	l := pkglog.L()

	var base domain.BaseMessage
	if err := json.Unmarshal(message, &base); err != nil {
		client.SendMessage(domain.NewErrorMessage(domain.ErrCodeBadRequest, "Invalid message format"))
		return
	}

	ctx := context.Background()

	switch base.Type {
	case domain.MsgTypeAuth:
		var msg domain.AuthMessage
		if err := json.Unmarshal(message, &msg); err != nil {
			client.SendMessage(domain.NewErrorMessage(domain.ErrCodeBadRequest, "Invalid auth message"))
			return
		}
		if err := h.service.HandleAuth(ctx, client, msg.Token); err != nil {
			l.Error().Err(err).Str("client_id", client.ID).Msg("auth failed")
		}

	case domain.MsgTypeJoinRoom:
		var msg domain.JoinRoomMessage
		if err := json.Unmarshal(message, &msg); err != nil {
			client.SendMessage(domain.NewErrorMessage(domain.ErrCodeBadRequest, "Invalid join_room message"))
			return
		}
		if err := h.service.HandleJoinRoom(ctx, client, msg.RoomID); err != nil {
			l.Error().Err(err).Str("client_id", client.ID).Msg("join room failed")
		}

	case domain.MsgTypeStartBroadcast:
		var msg domain.StartBroadcastMessage
		if err := json.Unmarshal(message, &msg); err != nil {
			client.SendMessage(domain.NewErrorMessage(domain.ErrCodeBadRequest, "Invalid start_broadcast message"))
			return
		}
		if err := h.service.HandleStartBroadcast(ctx, client, msg.RoomID, msg.Offer); err != nil {
			l.Error().Err(err).Str("client_id", client.ID).Msg("start broadcast failed")
		}

	case domain.MsgTypeICECandidate:
		var msg domain.ICECandidateMessage
		if err := json.Unmarshal(message, &msg); err != nil {
			client.SendMessage(domain.NewErrorMessage(domain.ErrCodeBadRequest, "Invalid ice_candidate message"))
			return
		}
		if err := h.service.HandleICECandidate(ctx, client, msg.RoomID, msg.Candidate); err != nil {
			l.Error().Err(err).Str("client_id", client.ID).Msg("ice candidate failed")
		}

	case domain.MsgTypeStopBroadcast:
		var msg domain.StopBroadcastMessage
		if err := json.Unmarshal(message, &msg); err != nil {
			client.SendMessage(domain.NewErrorMessage(domain.ErrCodeBadRequest, "Invalid stop_broadcast message"))
			return
		}
		if err := h.service.HandleStopBroadcast(ctx, client, msg.RoomID); err != nil {
			l.Error().Err(err).Str("client_id", client.ID).Msg("stop broadcast failed")
		}

	case domain.MsgTypeLeaveRoom:
		var msg domain.LeaveRoomMessage
		if err := json.Unmarshal(message, &msg); err != nil {
			client.SendMessage(domain.NewErrorMessage(domain.ErrCodeBadRequest, "Invalid leave_room message"))
			return
		}
		if err := h.service.HandleLeaveRoom(ctx, client, msg.RoomID); err != nil {
			l.Error().Err(err).Str("client_id", client.ID).Msg("leave room failed")
		}

	case domain.MsgTypePing:
		client.SendMessage(map[string]string{"type": domain.MsgTypePong})

	default:
		client.SendMessage(domain.NewErrorMessage(domain.ErrCodeBadRequest, "Unknown message type"))
	}
}

// RegisterRoutes registers the WebSocket route.
func (h *WSHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/ws", h.HandleWebSocket)
}
