package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// RoomClient wraps the Room Service HTTP client.
type RoomClient struct {
	baseURL    string
	httpClient *http.Client
	cache      map[string]*cachedRoom
	cacheTTL   time.Duration
	mu         sync.RWMutex
}

type cachedRoom struct {
	room      *Room
	expiresAt time.Time
}

// Room represents room information from the Room Service.
type Room struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	OwnerID     string    `json:"owner_id"`
	OwnerName   string    `json:"owner_name"`
	Status      string    `json:"status"` // "active", "closed"
	CreatedAt   time.Time `json:"created_at"`
}

// RoomResponse represents the API response wrapper.
type RoomResponse struct {
	Success bool   `json:"success"`
	Data    *Room  `json:"data"`
	Error   string `json:"error,omitempty"`
}

// NewRoomClient creates a new Room Service client.
func NewRoomClient(baseURL string, cacheTTL time.Duration) *RoomClient {
	return &RoomClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		cache:    make(map[string]*cachedRoom),
		cacheTTL: cacheTTL,
	}
}

// GetRoom retrieves room information by ID.
func (c *RoomClient) GetRoom(ctx context.Context, roomID string) (*Room, error) {
	// Check cache first
	if room := c.getFromCache(roomID); room != nil {
		return room, nil
	}

	// Fetch from service
	url := fmt.Sprintf("%s/api/v1/rooms/%s", c.baseURL, roomID)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch room: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrRoomNotFound
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("room service returned status: %d", resp.StatusCode)
	}

	var roomResp RoomResponse
	if err := json.NewDecoder(resp.Body).Decode(&roomResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if !roomResp.Success || roomResp.Data == nil {
		return nil, fmt.Errorf("room service error: %s", roomResp.Error)
	}

	// Cache the result
	c.addToCache(roomID, roomResp.Data)

	return roomResp.Data, nil
}

// InvalidateCache removes a room from the cache.
func (c *RoomClient) InvalidateCache(roomID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.cache, roomID)
}

func (c *RoomClient) getFromCache(roomID string) *Room {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if cached, ok := c.cache[roomID]; ok {
		if time.Now().Before(cached.expiresAt) {
			return cached.room
		}
	}
	return nil
}

func (c *RoomClient) addToCache(roomID string, room *Room) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cache[roomID] = &cachedRoom{
		room:      room,
		expiresAt: time.Now().Add(c.cacheTTL),
	}
}

// Errors
var (
	ErrRoomNotFound = fmt.Errorf("room not found")
)
