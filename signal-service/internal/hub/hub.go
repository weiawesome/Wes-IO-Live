package hub

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	pkglog "github.com/weiawesome/wes-io-live/pkg/log"
	"github.com/weiawesome/wes-io-live/signal-service/internal/config"
	"github.com/weiawesome/wes-io-live/signal-service/internal/domain"
)

// DisconnectHandler is called when a client disconnects.
type DisconnectHandler func(*Client)

// Client represents a connected WebSocket client.
type Client struct {
	ID                string
	Hub               *Hub
	Conn              *websocket.Conn
	Send              chan []byte
	Session           *domain.Session
	disconnectHandler DisconnectHandler
}

// SetDisconnectHandler sets the handler to be called on disconnect.
func (c *Client) SetDisconnectHandler(handler DisconnectHandler) {
	c.disconnectHandler = handler
}

// Hub manages all WebSocket connections.
type Hub struct {
	clients    map[string]*Client
	rooms      map[string]map[string]*Client // roomID -> clientID -> client
	register   chan *Client
	unregister chan *Client
	broadcast  chan *RoomMessage
	mu         sync.RWMutex
	config     config.WebSocketConfig
}

// RoomMessage is a message to be broadcast to a room.
type RoomMessage struct {
	RoomID  string
	Message []byte
	Exclude string // Client ID to exclude from broadcast
}

// NewHub creates a new Hub.
func NewHub(cfg config.WebSocketConfig) *Hub {
	return &Hub{
		clients:    make(map[string]*Client),
		rooms:      make(map[string]map[string]*Client),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		broadcast:  make(chan *RoomMessage, 256),
		config:     cfg,
	}
}

// Run starts the hub's main loop.
func (h *Hub) Run() {
	l := pkglog.L()
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client.ID] = client
			h.mu.Unlock()
			l.Info().Str("client_id", client.ID).Msg("client registered")

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client.ID]; ok {
				// Remove from all rooms
				for roomID, roomClients := range h.rooms {
					delete(roomClients, client.ID)
					if len(roomClients) == 0 {
						delete(h.rooms, roomID)
					}
				}
				delete(h.clients, client.ID)
				close(client.Send)
			}
			h.mu.Unlock()
			l.Info().Str("client_id", client.ID).Msg("client unregistered")

		case msg := <-h.broadcast:
			h.mu.RLock()
			if roomClients, ok := h.rooms[msg.RoomID]; ok {
				for clientID, client := range roomClients {
					if clientID == msg.Exclude {
						continue
					}
					select {
					case client.Send <- msg.Message:
					default:
						// Client's send buffer is full
						go h.removeClient(client)
					}
				}
			}
			h.mu.RUnlock()
		}
	}
}

// Register adds a client to the hub.
func (h *Hub) Register(client *Client) {
	h.register <- client
}

// Unregister removes a client from the hub.
func (h *Hub) Unregister(client *Client) {
	h.unregister <- client
}

// JoinRoom adds a client to a room.
func (h *Hub) JoinRoom(client *Client, roomID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, ok := h.rooms[roomID]; !ok {
		h.rooms[roomID] = make(map[string]*Client)
	}
	h.rooms[roomID][client.ID] = client
	l := pkglog.L()
	l.Info().Str("client_id", client.ID).Str("room_id", roomID).Msg("client joined room")
}

// LeaveRoom removes a client from a room.
func (h *Hub) LeaveRoom(client *Client, roomID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if roomClients, ok := h.rooms[roomID]; ok {
		delete(roomClients, client.ID)
		if len(roomClients) == 0 {
			delete(h.rooms, roomID)
		}
	}
	l := pkglog.L()
	l.Info().Str("client_id", client.ID).Str("room_id", roomID).Msg("client left room")
}

// BroadcastToRoom sends a message to all clients in a room.
func (h *Hub) BroadcastToRoom(roomID string, message interface{}, exclude string) error {
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}

	h.broadcast <- &RoomMessage{
		RoomID:  roomID,
		Message: data,
		Exclude: exclude,
	}
	return nil
}

// SendToClient sends a message to a specific client.
func (h *Hub) SendToClient(clientID string, message interface{}) error {
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}

	h.mu.RLock()
	client, ok := h.clients[clientID]
	h.mu.RUnlock()

	if !ok {
		return nil
	}

	select {
	case client.Send <- data:
	default:
		go h.removeClient(client)
	}
	return nil
}


// GetBroadcasterForRoom returns the broadcaster client for a room.
func (h *Hub) GetBroadcasterForRoom(roomID string) *Client {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if roomClients, ok := h.rooms[roomID]; ok {
		for _, client := range roomClients {
			if client.Session != nil && client.Session.IsBroadcasting() {
				return client
			}
		}
	}
	return nil
}

func (h *Hub) removeClient(client *Client) {
	h.unregister <- client
}

// ReadPump pumps messages from the WebSocket connection to the hub.
func (c *Client) ReadPump(handler func(*Client, []byte)) {
	defer func() {
		// Call disconnect handler before unregistering
		if c.disconnectHandler != nil {
			c.disconnectHandler(c)
		}
		c.Hub.Unregister(c)
		c.Conn.Close()
	}()

	c.Conn.SetReadLimit(c.Hub.config.MaxMessageSize)
	c.Conn.SetReadDeadline(time.Now().Add(c.Hub.config.PongWait))
	c.Conn.SetPongHandler(func(string) error {
		c.Conn.SetReadDeadline(time.Now().Add(c.Hub.config.PongWait))
		return nil
	})

	for {
		_, message, err := c.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				l := pkglog.L()
				l.Error().Err(err).Msg("websocket error")
			}
			break
		}

		if c.Session != nil {
			c.Session.UpdateActivity()
		}

		handler(c, message)
	}
}

// WritePump pumps messages from the hub to the WebSocket connection.
func (c *Client) WritePump() {
	ticker := time.NewTicker(c.Hub.config.PingInterval)
	defer func() {
		ticker.Stop()
		c.Conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.Send:
			c.Conn.SetWriteDeadline(time.Now().Add(c.Hub.config.WriteWait))
			if !ok {
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.Conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			c.Conn.SetWriteDeadline(time.Now().Add(c.Hub.config.WriteWait))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// Send sends a message to the client.
func (c *Client) SendMessage(message interface{}) error {
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}

	select {
	case c.Send <- data:
	default:
		return nil
	}
	return nil
}
