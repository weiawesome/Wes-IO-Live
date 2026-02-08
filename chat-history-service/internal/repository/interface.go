package repository

import (
	"context"

	"github.com/weiawesome/wes-io-live/chat-history-service/internal/domain"
)

type Direction string

const (
	DirectionBackward Direction = "backward" // DESC - from newest to oldest
	DirectionForward  Direction = "forward"  // ASC - from oldest to newest
)

func ParseDirection(s string) Direction {
	if s == "forward" {
		return DirectionForward
	}
	return DirectionBackward
}

type MessageRepository interface {
	GetMessages(
		ctx context.Context,
		roomID, sessionID string,
		cursor string,
		limit int,
		direction Direction,
	) (messages []domain.ChatMessage, nextCursor string, hasMore bool, err error)

	Close() error
}
