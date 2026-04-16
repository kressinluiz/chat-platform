package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type ReactionRepo interface {
	Toggle(ctx context.Context, messageID, userID, emoji string) (added bool, err error)
	GetForMessage(ctx context.Context, messageID string) ([]Reaction, error)
}

type Reaction struct {
	MessageID string
	UserID    string
	Emoji     string
	CreatedAt time.Time
}

type ReactionRepository struct {
	db *sql.DB
}

func NewReactionRepo(db *sql.DB) ReactionRepo {
	return &ReactionRepository{db: db}
}

func (r *ReactionRepository) Toggle(ctx context.Context, messageID, userID, emoji string) (bool, error) {
	query := `
        WITH existing AS (
            DELETE FROM message_reactions
            WHERE message_id = $1 AND user_id = $2 AND emoji = $3
            RETURNING message_id
        )
        INSERT INTO message_reactions (message_id, user_id, emoji)
        SELECT $1, $2, $3
        WHERE NOT EXISTS (SELECT 1 FROM existing)
        RETURNING message_id
    `

	var id string
	err := r.db.QueryRowContext(ctx, query, messageID, userID, emoji).Scan(&id)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to toggle reaction: %w", err)
	}
	return true, nil
}

func (r *ReactionRepository) GetForMessage(ctx context.Context, messageID string) ([]Reaction, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT message_id, user_id, emoji, created_at
         FROM message_reactions WHERE message_id = $1`,
		messageID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get reactions: %w", err)
	}
	defer rows.Close()

	var reactions []Reaction
	for rows.Next() {
		var reaction Reaction
		if err := rows.Scan(&reaction.MessageID, &reaction.UserID, &reaction.Emoji, &reaction.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan reaction: %w", err)
		}
		reactions = append(reactions, reaction)
	}
	return reactions, rows.Err()
}
