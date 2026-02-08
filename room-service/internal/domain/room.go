package domain

import (
	"time"
)

// RoomStatus represents room status.
type RoomStatus string

const (
	RoomStatusActive RoomStatus = "active"
	RoomStatusClosed RoomStatus = "closed"
)

// Room represents a streaming room.
type Room struct {
	ID            string     `json:"id"`
	OwnerID       string     `json:"owner_id"`
	OwnerUsername string     `json:"owner_username"`
	Title         string     `json:"title"`
	Description   string     `json:"description,omitempty"`
	Status        RoomStatus `json:"status"`
	ViewerCount   int        `json:"viewer_count"`
	Tags          []string   `json:"tags,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	ClosedAt      *time.Time `json:"closed_at,omitempty"`
}

// CreateRoomRequest represents a create room request.
type CreateRoomRequest struct {
	Title       string   `json:"title" binding:"required,min=1,max=200"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
}

// UpdateRoomRequest represents an update room request.
type UpdateRoomRequest struct {
	Title       *string  `json:"title"`
	Description *string  `json:"description"`
	Tags        []string `json:"tags"`
}

// ListRoomsRequest represents a list rooms request.
type ListRoomsRequest struct {
	Page     int    `form:"page"`
	PageSize int    `form:"page_size"`
	Status   string `form:"status"`
}

// SearchRoomsRequest represents a search rooms request.
type SearchRoomsRequest struct {
	Query    string `form:"q" binding:"required,min=1"`
	Page     int    `form:"page"`
	PageSize int    `form:"page_size"`
}

// RoomResponse represents a room in API responses.
type RoomResponse struct {
	ID            string     `json:"id"`
	OwnerID       string     `json:"owner_id"`
	OwnerUsername string     `json:"owner_username"`
	Title         string     `json:"title"`
	Description   string     `json:"description,omitempty"`
	Status        RoomStatus `json:"status"`
	ViewerCount   int        `json:"viewer_count"`
	Tags          []string   `json:"tags,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	ClosedAt      *time.Time `json:"closed_at,omitempty"`
}

// ListRoomsResponse represents a paginated list response.
type ListRoomsResponse struct {
	Rooms      []RoomResponse `json:"rooms"`
	Total      int            `json:"total"`
	Page       int            `json:"page"`
	PageSize   int            `json:"page_size"`
	TotalPages int            `json:"total_pages"`
}

// ToResponse converts Room to RoomResponse.
func (r *Room) ToResponse() RoomResponse {
	return RoomResponse{
		ID:            r.ID,
		OwnerID:       r.OwnerID,
		OwnerUsername: r.OwnerUsername,
		Title:         r.Title,
		Description:   r.Description,
		Status:        r.Status,
		ViewerCount:   r.ViewerCount,
		Tags:          r.Tags,
		CreatedAt:     r.CreatedAt,
		ClosedAt:      r.ClosedAt,
	}
}
