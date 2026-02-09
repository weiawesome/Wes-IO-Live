package registry

import "context"

// RegistryLookup provides read-only access to the chat service registry.
type RegistryLookup interface {
	Lookup(ctx context.Context, roomID, sessionID string) (string, error)
	Close() error
}
