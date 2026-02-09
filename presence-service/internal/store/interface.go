package store

import (
	"context"
	"time"

	"github.com/weiawesome/wes-io-live/presence-service/internal/domain"
)

// PresenceStore defines the interface for presence data storage.
type PresenceStore interface {
	// AddUser adds an authenticated user to a room.
	AddUser(ctx context.Context, roomID, userID string, ttl time.Duration) error

	// RemoveUser removes an authenticated user from a room.
	RemoveUser(ctx context.Context, roomID, userID string) error

	// AddDevice adds an anonymous device to a room.
	AddDevice(ctx context.Context, roomID, deviceHash string, ttl time.Duration) error

	// RemoveDevice removes an anonymous device from a room.
	RemoveDevice(ctx context.Context, roomID, deviceHash string) error

	// GetCount returns the presence count for a room.
	GetCount(ctx context.Context, roomID string) (domain.PresenceCount, error)

	// SetUserRoom sets the room mapping for a user (for disconnect cleanup).
	SetUserRoom(ctx context.Context, userID, roomID string, ttl time.Duration) error

	// GetUserRoom gets the room that a user is in.
	GetUserRoom(ctx context.Context, userID string) (string, error)

	// DeleteUserRoom removes the room mapping for a user.
	DeleteUserRoom(ctx context.Context, userID string) error

	// SetDeviceRoom sets the room mapping for a device (for disconnect cleanup).
	SetDeviceRoom(ctx context.Context, deviceHash, roomID string, ttl time.Duration) error

	// GetDeviceRoom gets the room that a device is in.
	GetDeviceRoom(ctx context.Context, deviceHash string) (string, error)

	// DeleteDeviceRoom removes the room mapping for a device.
	DeleteDeviceRoom(ctx context.Context, deviceHash string) error

	// RefreshTTL refreshes the TTL for a user or device presence.
	RefreshTTL(ctx context.Context, identity *domain.UserIdentity, roomID string, ttl time.Duration) error

	// PublishRoomUpdate publishes a room count update to the Redis Pub/Sub channel for multi-instance sync.
	PublishRoomUpdate(ctx context.Context, roomID string, count domain.PresenceCount) error

	// SetRoomLive marks a room as live with the given broadcaster.
	SetRoomLive(ctx context.Context, roomID, broadcasterID string) error

	// SetRoomOffline marks a room as offline.
	SetRoomOffline(ctx context.Context, roomID string) error

	// GetRoomLiveStatus returns the live status of a room.
	GetRoomLiveStatus(ctx context.Context, roomID string) (*domain.LiveStatus, error)

	// GetAllLiveRooms returns all room IDs that are currently live.
	GetAllLiveRooms(ctx context.Context) ([]string, error)

	// PublishLiveStatusUpdate publishes a live status update for multi-instance sync.
	PublishLiveStatusUpdate(ctx context.Context, roomID string, isLive bool) error

	// Close closes the store connection.
	Close() error
}
