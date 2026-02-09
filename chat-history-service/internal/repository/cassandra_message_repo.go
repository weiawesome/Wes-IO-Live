package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/weiawesome/wes-io-live/chat-history-service/internal/config"
	"github.com/weiawesome/wes-io-live/chat-history-service/internal/domain"
	"github.com/gocql/gocql"
)

type CassandraMessageRepository struct {
	session *gocql.Session
}

func NewCassandraMessageRepository(cfg config.CassandraConfig) (*CassandraMessageRepository, error) {
	cluster := gocql.NewCluster(cfg.Hosts...)
	cluster.Keyspace = cfg.Keyspace
	cluster.ConnectTimeout = cfg.ConnectTimeout
	cluster.Timeout = cfg.Timeout

	switch cfg.Consistency {
	case "LOCAL_ONE":
		cluster.Consistency = gocql.LocalOne
	case "LOCAL_QUORUM":
		cluster.Consistency = gocql.LocalQuorum
	case "ONE":
		cluster.Consistency = gocql.One
	case "QUORUM":
		cluster.Consistency = gocql.Quorum
	default:
		cluster.Consistency = gocql.LocalOne
	}

	session, err := cluster.CreateSession()
	if err != nil {
		return nil, fmt.Errorf("failed to create cassandra session: %w", err)
	}

	return &CassandraMessageRepository{session: session}, nil
}

func (r *CassandraMessageRepository) GetMessages(
	ctx context.Context,
	roomID, sessionID string,
	cursor string,
	limit int,
	direction Direction,
) ([]domain.ChatMessage, string, bool, error) {
	// Query limit + 1 to determine if there are more results
	queryLimit := limit + 1

	var query string
	var args []interface{}

	if direction == DirectionBackward {
		// DESC - from newest to oldest
		if cursor == "" {
			query = `SELECT message_id, user_id, username, room_id, session_id, content, created_at
					 FROM messages_by_room_session
					 WHERE room_id = ? AND session_id = ?
					 ORDER BY message_id DESC
					 LIMIT ?`
			args = []interface{}{roomID, sessionID, queryLimit}
		} else {
			query = `SELECT message_id, user_id, username, room_id, session_id, content, created_at
					 FROM messages_by_room_session
					 WHERE room_id = ? AND session_id = ? AND message_id < ?
					 ORDER BY message_id DESC
					 LIMIT ?`
			args = []interface{}{roomID, sessionID, cursor, queryLimit}
		}
	} else {
		// ASC - from oldest to newest
		if cursor == "" {
			query = `SELECT message_id, user_id, username, room_id, session_id, content, created_at
					 FROM messages_by_room_session
					 WHERE room_id = ? AND session_id = ?
					 ORDER BY message_id ASC
					 LIMIT ?`
			args = []interface{}{roomID, sessionID, queryLimit}
		} else {
			query = `SELECT message_id, user_id, username, room_id, session_id, content, created_at
					 FROM messages_by_room_session
					 WHERE room_id = ? AND session_id = ? AND message_id > ?
					 ORDER BY message_id ASC
					 LIMIT ?`
			args = []interface{}{roomID, sessionID, cursor, queryLimit}
		}
	}

	iter := r.session.Query(query, args...).WithContext(ctx).Iter()

	var messages []domain.ChatMessage
	var msg domain.ChatMessage
	var createdAt time.Time

	for iter.Scan(
		&msg.MessageID,
		&msg.UserID,
		&msg.Username,
		&msg.RoomID,
		&msg.SessionID,
		&msg.Content,
		&createdAt,
	) {
		msg.CreatedAt = createdAt
		messages = append(messages, msg)
		msg = domain.ChatMessage{}
	}

	if err := iter.Close(); err != nil {
		return nil, "", false, fmt.Errorf("failed to iterate messages: %w", err)
	}

	// Determine if there are more results
	hasMore := len(messages) > limit
	if hasMore {
		messages = messages[:limit]
	}

	// Get next cursor from the last message
	var nextCursor string
	if len(messages) > 0 {
		nextCursor = messages[len(messages)-1].MessageID
	}

	return messages, nextCursor, hasMore, nil
}

func (r *CassandraMessageRepository) Close() error {
	r.session.Close()
	return nil
}
