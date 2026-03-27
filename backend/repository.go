package main

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type MessageRepository struct {
	db *sql.DB
}

type StoredMessage struct {
	Username  string
	Content   string
	CreatedAt time.Time
}

func NewMessageRepository(db *sql.DB) *MessageRepository {
	return &MessageRepository{
		db: db,
	}
}

func (r *MessageRepository) Save(ctx context.Context, roomID, userID, content string) error {

	query := `
		INSERT INTO messages (room_id, user_id, content)
		VALUES ($1, $2, $3)
	`

	_, err := r.db.ExecContext(ctx, query, roomID, userID, content)
	if err != nil {
		return fmt.Errorf("failed to save message: %w", err)
	}

	return nil
}

func (r *MessageRepository) GetHistory(ctx context.Context, roomID string, limit int) ([]StoredMessage, error) {
	query := `
        SELECT u.username, m.content, m.created_at
        FROM messages m
        JOIN users u ON u.id = m.user_id
        WHERE m.room_id = $1
        ORDER BY m.created_at DESC
        LIMIT $2
    `
	rows, err := r.db.QueryContext(ctx, query, roomID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query history: %w", err)
	}
	defer rows.Close()

	var messages []StoredMessage
	for rows.Next() {
		var msg StoredMessage
		if err := rows.Scan(&msg.Username, &msg.Content, &msg.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan message: %w", err)
		}
		messages = append(messages, msg)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate rows: %w", err)
	}

	return messages, nil
}
