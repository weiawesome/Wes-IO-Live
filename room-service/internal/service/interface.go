package service

import (
	"context"

	"github.com/weiawesome/wes-io-live/room-service/internal/domain"
)

// RoomService defines the interface for room business logic.
type RoomService interface {
	CreateRoom(ctx context.Context, userID, username string, req *domain.CreateRoomRequest) (*domain.RoomResponse, error)
	GetRoom(ctx context.Context, roomID string) (*domain.RoomResponse, error)
	ListRooms(ctx context.Context, page, pageSize int, status string) (*domain.ListRoomsResponse, error)
	SearchRooms(ctx context.Context, query string, page, pageSize int) (*domain.ListRoomsResponse, error)
	GetMyRooms(ctx context.Context, userID string) ([]domain.RoomResponse, error)
	CloseRoom(ctx context.Context, userID, roomID string) error
	GetRoomStats(ctx context.Context, userID string) (activeCount, maxAllowed int, err error)
}
