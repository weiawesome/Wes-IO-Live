package processor

import "context"

// AvatarProcessor resizes raw avatar images into multiple sizes and uploads them.
// Implementations also satisfy mq.MinIOEventHandler by parsing the userID from the key
// and delegating to Process.
type AvatarProcessor interface {
	Process(ctx context.Context, rawKey, userID string) error
}
