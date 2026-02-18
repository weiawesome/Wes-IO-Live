package hub

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/weiawesome/wes-io-live/chat-service/internal/config"
	"github.com/weiawesome/wes-io-live/pkg/log"
)

type Hub struct {
	clients      map[string]*Client            // clientID -> client
	roomSessions map[string]map[string]*Client  // "roomID:sessionID" -> clientID -> client
	register     chan *Client
	unregister   chan *Client
	broadcast    chan *RoomSessionMessage
	mu           sync.RWMutex
	config       config.WebSocketConfig
}

type RoomSessionMessage struct {
	RoomID    string
	SessionID string
	Message   []byte
	Exclude   string // Client ID to exclude
}

func NewHub(cfg config.WebSocketConfig) *Hub {
	return &Hub{
		clients:      make(map[string]*Client),
		roomSessions: make(map[string]map[string]*Client),
		register:     make(chan *Client),
		unregister:   make(chan *Client),
		broadcast:    make(chan *RoomSessionMessage, 256),
		config:       cfg,
	}
}

func roomSessionKey(roomID, sessionID string) string {
	return fmt.Sprintf("%s:%s", roomID, sessionID)
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client.ID] = client
			h.mu.Unlock()
			l := log.L()
			l.Debug().Str("client_id", client.ID).Msg("client registered")

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client.ID]; ok {
				for key, rsClients := range h.roomSessions {
					delete(rsClients, client.ID)
					if len(rsClients) == 0 {
						delete(h.roomSessions, key)
					}
				}
				delete(h.clients, client.ID)
				close(client.Send)
			}
			h.mu.Unlock()
			l := log.L()
			l.Debug().Str("client_id", client.ID).Msg("client unregistered")

		case msg := <-h.broadcast:
			key := roomSessionKey(msg.RoomID, msg.SessionID)
			h.mu.RLock()
			if rsClients, ok := h.roomSessions[key]; ok {
				for clientID, client := range rsClients {
					if clientID == msg.Exclude {
						continue
					}
					select {
					case client.Send <- msg.Message:
					default:
						go h.removeClient(client)
					}
				}
			}
			h.mu.RUnlock()
		}
	}
}

func (h *Hub) Register(client *Client) {
	h.register <- client
}

func (h *Hub) Unregister(client *Client) {
	h.unregister <- client
}

func (h *Hub) JoinRoomSession(client *Client, roomID, sessionID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	key := roomSessionKey(roomID, sessionID)
	if _, ok := h.roomSessions[key]; !ok {
		h.roomSessions[key] = make(map[string]*Client)
	}
	h.roomSessions[key][client.ID] = client
	l := log.L()
	l.Info().Str("client_id", client.ID).Str("room_session", key).Msg("client joined room-session")
}

func (h *Hub) LeaveRoomSession(client *Client, roomID, sessionID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	key := roomSessionKey(roomID, sessionID)
	if rsClients, ok := h.roomSessions[key]; ok {
		delete(rsClients, client.ID)
		if len(rsClients) == 0 {
			delete(h.roomSessions, key)
		}
	}
	l := log.L()
	l.Info().Str("client_id", client.ID).Str("room_id", roomID).Str("session_id", sessionID).Msg("client left room-session")
}

func (h *Hub) BroadcastToRoomSession(roomID, sessionID string, message interface{}, exclude string) error {
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}

	h.broadcast <- &RoomSessionMessage{
		RoomID:    roomID,
		SessionID: sessionID,
		Message:   data,
		Exclude:   exclude,
	}
	return nil
}

func (h *Hub) GetRoomSessionClientCount(roomID, sessionID string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()

	key := roomSessionKey(roomID, sessionID)
	if rsClients, ok := h.roomSessions[key]; ok {
		return len(rsClients)
	}
	return 0
}

// BroadcastRawToRoomSession sends raw bytes to all clients in a room-session.
func (h *Hub) BroadcastRawToRoomSession(roomID, sessionID string, data []byte, exclude string) {
	h.broadcast <- &RoomSessionMessage{
		RoomID:    roomID,
		SessionID: sessionID,
		Message:   data,
		Exclude:   exclude,
	}
}

func (h *Hub) removeClient(client *Client) {
	h.unregister <- client
}
