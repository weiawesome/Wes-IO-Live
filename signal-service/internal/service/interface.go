package service

import (
	"context"
	"encoding/json"

	"github.com/weiawesome/wes-io-live/signal-service/internal/hub"
)

// SignalService handles signaling operations.
type SignalService interface {
	// HandleAuth handles client authentication.
	HandleAuth(ctx context.Context, client *hub.Client, token string) error

	// HandleJoinRoom handles a client joining a room.
	HandleJoinRoom(ctx context.Context, client *hub.Client, roomID string) error

	// HandleStartBroadcast handles a broadcaster starting a stream.
	HandleStartBroadcast(ctx context.Context, client *hub.Client, roomID string, offer json.RawMessage) error

	// HandleICECandidate handles an ICE candidate from the client.
	HandleICECandidate(ctx context.Context, client *hub.Client, roomID string, candidate json.RawMessage) error

	// HandleStopBroadcast handles a broadcaster stopping a stream.
	HandleStopBroadcast(ctx context.Context, client *hub.Client, roomID string) error

	// HandleLeaveRoom handles a client leaving a room.
	HandleLeaveRoom(ctx context.Context, client *hub.Client, roomID string) error

	// HandleDisconnect handles a client disconnecting.
	HandleDisconnect(ctx context.Context, client *hub.Client) error

	// Start starts background goroutines (e.g., event subscribers).
	Start(ctx context.Context) error

	// Stop stops background goroutines.
	Stop() error
}
