package service

import (
	"context"
)

// MediaService handles media streaming operations.
type MediaService interface {
	// HandleStartBroadcast handles a start broadcast request.
	HandleStartBroadcast(ctx context.Context, roomID, userID, offerSDP string) error

	// HandleICECandidate handles an ICE candidate from the client.
	HandleICECandidate(ctx context.Context, roomID, candidateJSON string) error

	// HandleStopBroadcast handles a stop broadcast request.
	HandleStopBroadcast(ctx context.Context, roomID, reason string) error

	// Start starts the service and subscribes to events.
	Start(ctx context.Context) error

	// Stop stops the service.
	Stop() error

	// IsRoomLive returns whether a room has an active live stream.
	IsRoomLive(ctx context.Context, roomID string) bool

	// GetActiveSession returns the active session for a room.
	GetActiveSession(ctx context.Context, roomID string) (*VODSession, error)
}
