package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type RoomRole string

const (
	RoleAdmin     RoomRole = "admin"
	RoleModerator RoomRole = "moderator"
	RoleMember    RoomRole = "member"
)

type RoomMember struct {
	RoomID   string
	UserID   string
	Role     RoomRole
	JoinedAt time.Time
}

type RoomMemberRepo interface {
	AddMember(ctx context.Context, roomID, userID string, role RoomRole) error
	GetRole(ctx context.Context, roomID, userID string) (RoomRole, error)
	GetRoomsForUser(ctx context.Context, userID string) ([]string, error)
	IsMember(ctx context.Context, roomID, userID string) (bool, error)
}

type RoomMemberRepository struct {
	db *sql.DB
}

func NewRoomMemberRepo(db *sql.DB) RoomMemberRepo {
	return &RoomMemberRepository{db: db}
}

func (r *RoomMemberRepository) AddMember(ctx context.Context, roomID, userID string, role RoomRole) error {
	query := `
		INSERT INTO room_members (room_id, user_id, role)
		VALUES ($1, $2, $3)
		RETURNING id
	`

	err := r.db.QueryRowContext(ctx, query, roomID, userID, role).Scan(new(int64))
	if err != nil {
		return fmt.Errorf("failed to add member: %w", err)
	}

	return nil
}

func (r *RoomMemberRepository) GetRole(ctx context.Context, roomID, userID string) (RoomRole, error) {
	var role RoomRole
	err := r.db.QueryRowContext(ctx,
		`SELECT role FROM room_members WHERE room_id = $1 AND user_id = $2`,
		roomID, userID,
	).Scan(&role)

	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to get role: %w", err)
	}
	return role, err
}

func (r *RoomMemberRepository) GetRoomsForUser(ctx context.Context, userID string) ([]string, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT room_id FROM room_members WHERE user_id = $1`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var roomIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		roomIDs = append(roomIDs, id)
	}
	return roomIDs, rows.Err()
}

func (r *RoomMemberRepository) IsMember(ctx context.Context, roomID, userID string) (bool, error) {
	query := `
		SELECT 1
		FROM room_members
		WHERE room_id = $1 AND user_id = $2
	`

	err := r.db.QueryRowContext(ctx, query, roomID, userID).Scan(new(int64))
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to check membership: %w", err)
	}

	return true, nil
}
