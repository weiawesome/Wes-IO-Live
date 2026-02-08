package registry

import "context"

type Registry interface {
	Register(ctx context.Context, roomID, sessionID string) error
	Deregister(ctx context.Context, roomID, sessionID string) error
	Lookup(ctx context.Context, roomID, sessionID string) (string, error)
	StartHeartbeat(ctx context.Context) error
	StopHeartbeat()
	Close() error
}
