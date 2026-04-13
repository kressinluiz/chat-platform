package hub

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

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

func noopLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestHub_RegisterClient(t *testing.T) {
	mock := &MockMessageRepository{}
	hub := NewHub(mock, nil, noopLogger())
	go hub.Run()

	client := NewClient(nil, hub, "room-1", "user-1", "alice", noopLogger())
	hub.Join(client)

	time.Sleep(10 * time.Millisecond)

	if hub.ConnectedClients() != 1 {
		t.Errorf("expected 1 connected client, got %d", hub.ConnectedClients())
	}
}

func TestHub_UnregisterClient(t *testing.T) {
	mock := &MockMessageRepository{}
	hub := NewHub(mock, nil, noopLogger())
	go hub.Run()

	client := NewClient(nil, hub, "room-1", "user-1", "alice", noopLogger())
	hub.Join(client)
	time.Sleep(10 * time.Millisecond)

	hub.Leave(client)
	time.Sleep(10 * time.Millisecond)

	if hub.ConnectedClients() != 0 {
		t.Errorf("expected 0 connected clients, got %d", hub.ConnectedClients())
	}
}

func TestHub_BroadcastToRoom(t *testing.T) {
	mock := &MockMessageRepository{}
	hub := NewHub(mock, nil, noopLogger())
	go hub.Run()

	client1 := NewClient(nil, hub, "user-1", "room-1", "alice", noopLogger())
	client2 := NewClient(nil, hub, "user-2", "room-1", "bob", noopLogger())
	client3 := NewClient(nil, hub, "user-3", "room-2", "carol", noopLogger())

	hub.Join(client1)
	hub.Join(client2)
	hub.Join(client3)
	time.Sleep(10 * time.Millisecond)

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
		t.Error("failed to marshal test payload", "error", err)
		return
	}

	event := ws.Event{
		ID:      "event-test-id",
		Type:    ws.EventTypeNewMessage,
		RoomID:  "room-1",
		Seq:     0,
		Payload: testPayload,
		SentAt:  time.Now().UTC().Format(time.RFC3339),
	}

	hub.broadcast <- event
	time.Sleep(10 * time.Millisecond)

	select {
	case received := <-client1.receivedMessages:
		var testPayload ws.NewMessagePayload
		err = json.Unmarshal(received.Payload, &testPayload)
		if string(testPayload.Content) != "hello" {
			t.Errorf("client1 got wrong content: %s", testPayload.Content)
		}
	default:
		t.Error("client1 did not receive message")
	}

	select {
	case received := <-client2.receivedMessages:
		var testPayload ws.NewMessagePayload
		err = json.Unmarshal(received.Payload, &testPayload)
		if string(testPayload.Content) != "hello" {
			t.Errorf("client2 got wrong content: %s", testPayload.Content)
		}
	default:
		t.Error("client2 did not receive message")
	}

	select {
	case <-client3.receivedMessages:
		t.Error("client3 should not have received message from room-1")
	default:
		// correct — carol is in room-2
	}
}

// need to implement this test at client.ReadRoutine

// func TestHub_MessageSavedToRepository(t *testing.T) {
// 	mock := &MockMessageRepository{}
// 	hub := NewHub(mock, nil, noopLogger())
// 	go hub.Run()

// 	client := NewClient(nil, hub, "user-1", "room-1", "alice", noopLogger())
// 	hub.Join(client)
// 	time.Sleep(10 * time.Millisecond)

// 	newMessageTest := ws.NewMessagePayload{
// 		MessageID: "message-test-id",
// 		UserID:    "user-1",
// 		RoomID:    "room-1",
// 		Username:  "alice",
// 		Content:   "hello",
// 		CreatedAt: time.Now().UTC().Format(time.RFC3339),
// 	}

// 	testPayload, err := json.Marshal(newMessageTest)
// 	if err != nil {
// 		t.Error("failed to marshal test payload", "error", err)
// 		return
// 	}

// 	event := ws.Event{
// 		ID:      "event-test-id",
// 		Type:    ws.EventTypeNewMessage,
// 		RoomID:  "room-1",
// 		Seq:     0,
// 		Payload: testPayload,
// 		SentAt:  time.Now().UTC().Format(time.RFC3339),
// 	}

// 	hub.broadcast <- event
// 	time.Sleep(50 * time.Millisecond) // save is async, give it time

// 	mock.mu.Lock()
// 	defer mock.mu.Unlock()
// 	if len(mock.saved) != 1 {
// 		t.Fatalf("expected 1 saved message, got %d", len(mock.saved))
// 	}
// 	if mock.saved[0].content != "hello" {
// 		t.Errorf("expected content 'hello', got %q", mock.saved[0].content)
// 	}
// 	if mock.saved[0].roomID != "room-1" {
// 		t.Errorf("expected roomID 'room-1', got %q", mock.saved[0].roomID)
// 	}
// }
