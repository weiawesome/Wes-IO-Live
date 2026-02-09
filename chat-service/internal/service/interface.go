package service

import (
	"context"

	"github.com/weiawesome/wes-io-live/chat-service/internal/hub"
)

type ChatService interface {
	HandleAuth(ctx context.Context, client *hub.Client, token string) error
	HandleJoinRoom(ctx context.Context, client *hub.Client, roomID, sessionID string) error
	HandleChatMessage(ctx context.Context, client *hub.Client, content string) error
	HandleLeaveRoom(ctx context.Context, client *hub.Client) error
	HandleDisconnect(ctx context.Context, client *hub.Client) error
	Start(ctx context.Context) error
	Stop() error
}
