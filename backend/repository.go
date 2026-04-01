package main

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

var tracer = otel.Tracer("chat/repository")

type RoomRepository struct {
	db *sql.DB
}

type Room struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type UserRepository struct {
	db *sql.DB
}

type MessageRepository struct {
	db *sql.DB
}

type StoredMessage struct {
	Username  string
	Content   string
	CreatedAt time.Time
}

func NewRoomRepository(db *sql.DB) *RoomRepository {
	return &RoomRepository{db: db}
}

func NewMessageRepository(db *sql.DB) *MessageRepository {
	return &MessageRepository{
		db: db,
	}
}

func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{
		db: db,
	}
}

func (r *RoomRepository) Create(ctx context.Context, name string) (Room, error) {
	ctx, span := tracer.Start(ctx, "RoomRepository.Create")
	defer span.End()
	span.SetAttributes(attribute.String("room_name", name))
	query := `
        INSERT INTO rooms (name)
        VALUES ($1)
        RETURNING id, name, created_at
    `
	var room Room
	err := r.db.QueryRowContext(ctx, query, name).Scan(&room.ID, &room.Name, &room.CreatedAt)
	if err != nil {
		span.RecordError(err)
		return Room{}, fmt.Errorf("failed to create room: %w", err)
	}
	return room, nil
}

func (r *RoomRepository) List(ctx context.Context) ([]Room, error) {
	ctx, span := tracer.Start(ctx, "RoomRepository.List")
	defer span.End()
	query := `
        SELECT id, name, created_at
        FROM rooms
        ORDER BY created_at ASC
    `
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to list rooms: %w", err)
	}
	defer rows.Close()

	rooms := make([]Room, 0)
	for rows.Next() {
		var room Room
		if err := rows.Scan(&room.ID, &room.Name, &room.CreatedAt); err != nil {
			span.RecordError(err)
			return nil, fmt.Errorf("failed to scan room: %w", err)
		}
		rooms = append(rooms, room)
	}

	if err := rows.Err(); err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to iterate rooms: %w", err)
	}

	return rooms, nil
}

func (r *UserRepository) Create(ctx context.Context, username, hashedPassword string) (string, error) {
	ctx, span := tracer.Start(ctx, "UserRepository.Create")
	defer span.End()
	span.SetAttributes(attribute.String("username", username))

	query := `
		INSERT INTO users (username, password)
		VALUES ($1, $2)
		RETURNING id
	`
	var id string
	err := r.db.QueryRowContext(ctx, query, username, hashedPassword).Scan(&id)
	if err != nil {
		span.RecordError(err)
		return "", fmt.Errorf("failed to create user: %w", err)
	}
	return id, nil
}

func (r *UserRepository) GetByUsername(ctx context.Context, username string) (string, string, error) {
	ctx, span := tracer.Start(ctx, "UserRepository.GetByUsername")
	defer span.End()
	span.SetAttributes(attribute.String("username", username))

	query := `
		SELECT id, password
		FROM users
		WHERE username = $1
	`
	var id, hashedPassword string
	err := r.db.QueryRowContext(ctx, query, username).Scan(&id, &hashedPassword)
	if err == sql.ErrNoRows {
		return "", "", nil
	}
	if err != nil {
		span.RecordError(err)
		return "", "", fmt.Errorf("failed to get user: %w", err)
	}
	return id, hashedPassword, nil

}

func (r *RoomRepository) GetByID(ctx context.Context, id string) (bool, error) {
	ctx, span := tracer.Start(ctx, "RoomRepository.GetByID")
	defer span.End()
	span.SetAttributes(attribute.String("room_id", id))

	query := `SELECT EXISTS(SELECT 1 FROM rooms WHERE id = $1)`
	var exists bool
	err := r.db.QueryRowContext(ctx, query, id).Scan(&exists)
	if err != nil {
		span.RecordError(err)
		return false, fmt.Errorf("failed to check room: %w", err)
	}
	return exists, nil
}

func (r *MessageRepository) GetHistory(ctx context.Context, roomID string, limit int) ([]StoredMessage, error) {
	ctx, span := tracer.Start(ctx, "MessageRepository.GetHistory")
	defer span.End()
	span.SetAttributes(attribute.String("room_id", roomID))

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
		span.RecordError(err)
		return nil, fmt.Errorf("failed to query history: %w", err)
	}
	defer rows.Close()

	var messages []StoredMessage
	for rows.Next() {
		var msg StoredMessage
		if err := rows.Scan(&msg.Username, &msg.Content, &msg.CreatedAt); err != nil {
			span.RecordError(err)
			return nil, fmt.Errorf("failed to scan message: %w", err)
		}
		messages = append(messages, msg)
	}

	if err := rows.Err(); err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to iterate rows: %w", err)
	}

	return messages, nil
}

func (r *MessageRepository) Save(ctx context.Context, roomID, userID, content string) error {
	ctx, span := tracer.Start(ctx, "MessageRepository.Save")
	defer span.End()
	span.SetAttributes(attribute.String("room_id", roomID))

	query := `
		INSERT INTO messages (room_id, user_id, content)
		VALUES ($1, $2, $3)
	`

	_, err := r.db.ExecContext(ctx, query, roomID, userID, content)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to save message: %w", err)
	}

	return nil
}
