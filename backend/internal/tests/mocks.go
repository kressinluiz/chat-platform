package tests

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/kressinluiz/chat/internal/hub"
	"github.com/kressinluiz/chat/internal/repository"
	"github.com/kressinluiz/chat/internal/ws"
)

type MockUserRepository struct {
	users map[string]MockUser
}

type MockUser struct {
	id       string
	password string
}

func newMockUserRepo() *MockUserRepository {
	return &MockUserRepository{
		users: make(map[string]MockUser),
	}
}

func (m *MockUserRepository) Create(ctx context.Context, username, hashedPassword string) (string, error) {
	if _, exists := m.users[username]; exists {
		return "", fmt.Errorf("unique constraint violation")
	}
	id := "user-" + username
	m.users[username] = MockUser{id: id, password: hashedPassword}
	return id, nil
}

func (m *MockUserRepository) GetByUsername(ctx context.Context, username string) (string, string, error) {
	user, exists := m.users[username]
	if !exists {
		return "", "", nil
	}
	return user.id, user.password, nil
}

type MockRoomRepository struct{}

type MockRoomMemberRepo struct{}

func (m *MockRoomMemberRepo) IsMember(ctx context.Context, roomID, userID string) (bool, error) {
	return true, nil
}

func (m *MockRoomMemberRepo) GetRole(ctx context.Context, roomID, userID string) (repository.RoomRole, error) {
	return repository.RoleMember, nil
}

func (m *MockRoomMemberRepo) GetRoomsForUser(ctx context.Context, userID string) ([]string, error) {
	return []string{"test-room-id"}, nil
}

func (m *MockRoomMemberRepo) AddMember(ctx context.Context, roomID, userID string, role repository.RoomRole) error {
	return nil
}

func (m *MockRoomRepository) Create(ctx context.Context, name string) (repository.Room, error) {
	return repository.Room{ID: "test-room-id", Name: name}, nil
}

func (m *MockRoomRepository) List(ctx context.Context) ([]repository.Room, error) {
	return []repository.Room{{ID: "test-room-id", Name: "general"}, {ID: "other-room-id", Name: "general"}}, nil
}

func (m *MockRoomRepository) GetByID(ctx context.Context, id string) (bool, error) {
	rooms, err := m.List(ctx)
	if err != nil {
		return false, err
	}
	for _, room := range rooms {
		if room.ID == id {
			return true, nil
		}
	}
	return false, nil
}

type SavedMessage struct {
	roomID  string
	userID  string
	content string
}

type MockMessageRepository struct {
	mu    sync.RWMutex
	saved []SavedMessage
}

func (mock *MockMessageRepository) Save(ctx context.Context, roomID, userID, content string) error {
	slog.Info("mock.Save")
	mock.mu.Lock()
	defer mock.mu.Unlock()
	mock.saved = append(mock.saved, SavedMessage{roomID: roomID, userID: userID, content: content})
	return nil
}

func (mock *MockMessageRepository) GetHistory(ctx context.Context, roomID string, limit int) ([]repository.StoredMessage, error) {
	return []repository.StoredMessage{}, nil
}

func NoopLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func NewTestHub() (*hub.Hub, error) {
	mock := &MockMessageRepository{}
	redisClient := NewMockRedisClient()
	hub, err := hub.NewHub(mock, redisClient, NoopLogger())
	return hub, err
}

func CreateTestMessage() (ws.Event, error) {
	newMessageTest := ws.NewMessagePayload{
		MessageID: "message-test-id",
		UserID:    "user-1",
		RoomID:    "room-1",
		Username:  "alice",
		Content:   "hello",
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}

	testPayload, err := json.Marshal(newMessageTest)
	if err != nil {
		return ws.Event{}, err
	}

	event := ws.Event{
		ID:      "event-test-id",
		Type:    ws.EventTypeNewMessage,
		RoomID:  "room-1",
		Seq:     0,
		Payload: testPayload,
		SentAt:  time.Now().UTC().Format(time.RFC3339),
	}

	return event, err
}

func NewMockRedisClient() *MockRedisClient {
	return &MockRedisClient{
		store: make(map[string]MockRedisItem),
	}
}

type MockRedisItem struct {
	value string
	ttl   time.Time
}

type MockRedisClient struct {
	mu    sync.RWMutex
	store map[string]MockRedisItem
}

func (c *MockRedisClient) isExpired(item MockRedisItem) bool {
	return !item.ttl.IsZero() && time.Now().After(item.ttl)
}

func (c *MockRedisClient) Set(ctx context.Context, key string, value any, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	strVal := fmt.Sprintf("%v", value)
	item := MockRedisItem{
		value: strVal,
	}
	if ttl > 0 {
		item.ttl = time.Now().Add(ttl)
	}
	c.store[key] = item
	return nil
}

func (c *MockRedisClient) Get(ctx context.Context, key string) (string, error) {
	c.mu.RLock()
	item, ok := c.store[key]
	defer c.mu.RUnlock()

	if !ok {
		return "", fmt.Errorf("redis: nil")
	}

	if c.isExpired(item) {
		c.mu.Lock()
		delete(c.store, key)
		c.mu.Unlock()
		return "", fmt.Errorf("redis: nil")
	}
	return item.value, nil
}

func (c *MockRedisClient) Keys(ctx context.Context, pattern string) ([]string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var result []string

	for key, item := range c.store {
		if c.isExpired(item) {
			continue
		}

		match, err := filepath.Match(pattern, key)
		if err != nil {
			return nil, err
		}

		if match {
			result = append(result, key)
		}
	}

	return result, nil
}

func (c *MockRedisClient) Del(ctx context.Context, keys ...string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, key := range keys {
		delete(c.store, key)
	}
	return nil
}

func (c *MockRedisClient) Exists(ctx context.Context, keys ...string) (int64, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var count int64

	for _, key := range keys {
		item, ok := c.store[key]
		if ok && !c.isExpired(item) {
			count++
		}
	}

	return count, nil
}

func (c *MockRedisClient) Expire(ctx context.Context, key string, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	item, ok := c.store[key]
	if !ok {
		return errors.New("redis: nil")
	}

	item.ttl = time.Now().Add(ttl)
	c.store[key] = item

	return nil
}

func (c *MockRedisClient) Incr(ctx context.Context, key string) (int64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	item, ok := c.store[key]

	var val int64
	if ok && !c.isExpired(item) {
		parsed, err := strconv.ParseInt(item.value, 10, 64)
		if err != nil {
			return 0, err
		}
		val = parsed
	}

	val++

	c.store[key] = MockRedisItem{
		value: strconv.FormatInt(val, 10),
	}

	return val, nil
}
