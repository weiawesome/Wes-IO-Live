package repository

import (
	"context"
	"errors"

	"github.com/weiawesome/wes-io-live/room-service/internal/domain"
)

var (
	ErrRoomNotFound = errors.New("room not found")
)

// RoomRepository defines the interface for room data persistence.
type RoomRepository interface {
	Create(ctx context.Context, room *domain.Room) error
	GetByID(ctx context.Context, id string) (*domain.Room, error)
	List(ctx context.Context, page, pageSize int, status string) ([]domain.Room, int, error)
	Search(ctx context.Context, query string, page, pageSize int) ([]domain.Room, int, error)
	GetUserRooms(ctx context.Context, userID string) ([]domain.Room, error)
	CountActiveRoomsByUser(ctx context.Context, userID string) (int, error)
	Close(ctx context.Context, id string) error
}
