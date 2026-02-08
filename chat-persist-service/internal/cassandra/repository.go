package cassandra

import (
	"context"
	"fmt"

	"github.com/gocql/gocql"
	"github.com/weiawesome/wes-io-live/chat-persist-service/internal/domain"
)

// MessageRepository handles message persistence to Cassandra.
type MessageRepository struct {
	session *gocql.Session
}

// NewMessageRepository creates a new MessageRepository.
func NewMessageRepository(client *Client) *MessageRepository {
	return &MessageRepository{
		session: client.Session(),
	}
}

// SaveMessage persists a chat message to the messages_by_room_session table.
func (r *MessageRepository) SaveMessage(ctx context.Context, msg *domain.ChatMessage) error {
	query := `
		INSERT INTO messages_by_room_session (
			room_id, session_id, message_id, user_id, username, content, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)`

	err := r.session.Query(query,
		msg.RoomID,
		msg.SessionID,
		msg.MessageID,
		msg.UserID,
		msg.Username,
		msg.Content,
		msg.Timestamp,
	).WithContext(ctx).Exec()

	if err != nil {
		return fmt.Errorf("failed to save message: %w", err)
	}

	return nil
}
