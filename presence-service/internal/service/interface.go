package service

import (
	"context"

	"github.com/weiawesome/wes-io-live/presence-service/internal/domain"
	"github.com/weiawesome/wes-io-live/presence-service/internal/hub"
	"github.com/weiawesome/wes-io-live/presence-service/internal/kafka"
)

// PresenceService defines the interface for presence management.
type PresenceService interface {
	// HandleJoin handles a user joining a room.
	HandleJoin(ctx context.Context, c *hub.Client, roomID, token, deviceHash string) error

	// HandleLeave handles a user leaving a room.
	HandleLeave(ctx context.Context, c *hub.Client, roomID string) error

	// HandleHeartbeat handles a heartbeat from a client.
	HandleHeartbeat(ctx context.Context, c *hub.Client) error

	// HandleDisconnect handles a client disconnecting.
	HandleDisconnect(ctx context.Context, c *hub.Client) error

	// GetRoomCount returns the presence count for a room.
	GetRoomCount(ctx context.Context, roomID string) (domain.PresenceCount, error)

	// GetRoomInfo returns complete room info including presence count and live status.
	GetRoomInfo(ctx context.Context, roomID string) (*domain.RoomInfo, error)

	// GetAllLiveRooms returns all room IDs that are currently live.
	GetAllLiveRooms(ctx context.Context) ([]string, error)

	// HandleBroadcastEvent handles a broadcast event from Kafka.
	HandleBroadcastEvent(ctx context.Context, event *kafka.BroadcastEvent) error

	// Start starts the presence service background tasks.
	Start(ctx context.Context) error

	// Stop stops the presence service.
	Stop() error
}
