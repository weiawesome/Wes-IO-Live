package service

import (
	"context"

	"github.com/weiawesome/wes-io-live/chat-history-service/internal/domain"
)

type ChatHistoryService interface {
	GetChatHistory(
		ctx context.Context,
		roomID, sessionID string,
		cursor string,
		limit int,
		direction string,
	) (*domain.ChatHistoryResponse, error)
}
