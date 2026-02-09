package hub

import (
	"encoding/json"
	"log"
	"time"

	"github.com/gorilla/websocket"
	"github.com/weiawesome/wes-io-live/chat-service/internal/config"
	"github.com/weiawesome/wes-io-live/chat-service/internal/domain"
)

type Client struct {
	ID      string
	Hub     *Hub
	Conn    *websocket.Conn
	Send    chan []byte
	Session *domain.Session
	config  config.WebSocketConfig
}

func NewClient(id string, hub *Hub, conn *websocket.Conn, cfg config.WebSocketConfig) *Client {
	return &Client{
		ID:      id,
		Hub:     hub,
		Conn:    conn,
		Send:    make(chan []byte, 256),
		Session: domain.NewSession(id),
		config:  cfg,
	}
}

func (c *Client) ReadPump(handler func(*Client, []byte)) {
	defer func() {
		c.Hub.Unregister(c)
		c.Conn.Close()
	}()

	c.Conn.SetReadLimit(c.config.MaxMessageSize)
	c.Conn.SetReadDeadline(time.Now().Add(c.config.PongWait))
	c.Conn.SetPongHandler(func(string) error {
		c.Conn.SetReadDeadline(time.Now().Add(c.config.PongWait))
		return nil
	})

	for {
		_, message, err := c.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}

		if c.Session != nil {
			c.Session.UpdateActivity()
		}

		handler(c, message)
	}
}

func (c *Client) WritePump() {
	ticker := time.NewTicker(c.config.PingInterval)
	defer func() {
		ticker.Stop()
		c.Conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.Send:
			c.Conn.SetWriteDeadline(time.Now().Add(c.config.WriteWait))
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
			c.Conn.SetWriteDeadline(time.Now().Add(c.config.WriteWait))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

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
