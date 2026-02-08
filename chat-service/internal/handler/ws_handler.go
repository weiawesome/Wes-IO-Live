package handler

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/weiawesome/wes-io-live/chat-service/internal/config"
	"github.com/weiawesome/wes-io-live/chat-service/internal/domain"
	"github.com/weiawesome/wes-io-live/chat-service/internal/hub"
	"github.com/weiawesome/wes-io-live/chat-service/internal/service"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type WSHandler struct {
	hub     *hub.Hub
	service service.ChatService
	wsCfg   config.WebSocketConfig
}

func NewWSHandler(h *hub.Hub, svc service.ChatService, wsCfg config.WebSocketConfig) *WSHandler {
	return &WSHandler{
		hub:     h,
		service: svc,
		wsCfg:   wsCfg,
	}
}

func (h *WSHandler) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}

	client := hub.NewClient(uuid.New().String(), h.hub, conn, h.wsCfg)

	h.hub.Register(client)

	go client.WritePump()
	go client.ReadPump(h.handleMessage)
}

func (h *WSHandler) handleMessage(client *hub.Client, message []byte) {
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
			log.Printf("Auth failed for client %s: %v", client.ID, err)
		}

	case domain.MsgTypeJoinRoom:
		var msg domain.JoinRoomMessage
		if err := json.Unmarshal(message, &msg); err != nil {
			client.SendMessage(domain.NewErrorMessage(domain.ErrCodeBadRequest, "Invalid join_room message"))
			return
		}
		if err := h.service.HandleJoinRoom(ctx, client, msg.RoomID, msg.SessionID); err != nil {
			log.Printf("Join room failed for client %s: %v", client.ID, err)
		}

	case domain.MsgTypeChatMessage:
		var msg domain.ChatMessageWS
		if err := json.Unmarshal(message, &msg); err != nil {
			client.SendMessage(domain.NewErrorMessage(domain.ErrCodeBadRequest, "Invalid chat_message"))
			return
		}
		if err := h.service.HandleChatMessage(ctx, client, msg.Content); err != nil {
			log.Printf("Chat message failed for client %s: %v", client.ID, err)
		}

	case domain.MsgTypeLeaveRoom:
		if err := h.service.HandleLeaveRoom(ctx, client); err != nil {
			log.Printf("Leave room failed for client %s: %v", client.ID, err)
		}

	case domain.MsgTypePing:
		client.SendMessage(map[string]string{"type": domain.MsgTypePong})

	default:
		client.SendMessage(domain.NewErrorMessage(domain.ErrCodeBadRequest, "Unknown message type"))
	}
}

func (h *WSHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/chat/ws", h.HandleWebSocket)
}
