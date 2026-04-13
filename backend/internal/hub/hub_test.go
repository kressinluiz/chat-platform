package hub

// import (
// 	"context"
// 	"fmt"
// 	"io"
// 	"log/slog"
// 	"sync"
// 	"testing"
// 	"time"

// 	"github.com/kressinluiz/chat/internal/repository"
// 	"github.com/kressinluiz/chat/internal/ws"
// )

// type MockUserRepository struct {
// 	users map[string]MockUser
// }

// type MockUser struct {
// 	id       string
// 	password string
// }

// func newMockUserRepo() *MockUserRepository {
// 	return &MockUserRepository{
// 		users: make(map[string]MockUser),
// 	}
// }

// func (m *MockUserRepository) Create(ctx context.Context, username, hashedPassword string) (string, error) {
// 	if _, exists := m.users[username]; exists {
// 		return "", fmt.Errorf("unique constraint violation")
// 	}
// 	id := "user-" + username
// 	m.users[username] = MockUser{id: id, password: hashedPassword}
// 	return id, nil
// }

// func (m *MockUserRepository) GetByUsername(ctx context.Context, username string) (string, string, error) {
// 	user, exists := m.users[username]
// 	if !exists {
// 		return "", "", nil
// 	}
// 	return user.id, user.password, nil
// }

// type MockRoomRepository struct{}

// type MockRoomMemberRepo struct{}

// func (m *MockRoomMemberRepo) IsMember(ctx context.Context, roomID, userID string) (bool, error) {
// 	return true, nil
// }

// func (m *MockRoomMemberRepo) GetRole(ctx context.Context, roomID, userID string) (repository.RoomRole, error) {
// 	return repository.RoleMember, nil
// }

// func (m *MockRoomMemberRepo) GetRoomsForUser(ctx context.Context, userID string) ([]string, error) {
// 	return []string{"test-room-id"}, nil
// }

// func (m *MockRoomMemberRepo) AddMember(ctx context.Context, roomID, userID string, role repository.RoomRole) error {
// 	return nil
// }

// func (m *MockRoomRepository) Create(ctx context.Context, name string) (repository.Room, error) {
// 	return repository.Room{ID: "test-room-id", Name: name}, nil
// }

// func (m *MockRoomRepository) List(ctx context.Context) ([]repository.Room, error) {
// 	return []repository.Room{{ID: "test-room-id", Name: "general"}, {ID: "other-room-id", Name: "general"}}, nil
// }

// func (m *MockRoomRepository) GetByID(ctx context.Context, id string) (bool, error) {
// 	rooms, err := m.List(ctx)
// 	if err != nil {
// 		return false, err
// 	}
// 	for _, room := range rooms {
// 		if room.ID == id {
// 			return true, nil
// 		}
// 	}
// 	return false, nil
// }

// type SavedMessage struct {
// 	roomID  string
// 	userID  string
// 	content string
// }

// type MockMessageRepository struct {
// 	mu    sync.RWMutex
// 	saved []SavedMessage
// }

// func (mock *MockMessageRepository) Save(ctx context.Context, roomID, userID, content string) error {
// 	mock.mu.Lock()
// 	defer mock.mu.Unlock()
// 	mock.saved = append(mock.saved, SavedMessage{roomID: roomID, userID: userID, content: content})
// 	return nil
// }

// func (mock *MockMessageRepository) GetHistory(ctx context.Context, roomID string, limit int) ([]repository.StoredMessage, error) {
// 	return []repository.StoredMessage{}, nil
// }

// func noopLogger() *slog.Logger {
// 	return slog.New(slog.NewTextHandler(io.Discard, nil))
// }

// func newTestClient(hub *Hub, roomID, userID, username string) *Client {
// 	return &Client{
// 		receivedMessages: make(chan ws.Event, ClientBufferSize),
// 		hub:              hub,
// 		Username:         username,
// 		RoomID:           roomID,
// 		UserID:           userID,
// 		logger:           noopLogger(),
// 	}
// }

// func TestHub_RegisterClient(t *testing.T) {
// 	mock := &MockMessageRepository{}
// 	hub := &Hub{
// 		rooms:      make(map[string]map[*Client]bool),
// 		broadcast:  make(chan ws.Event),
// 		register:   make(chan *Client),
// 		unregister: make(chan *Client),
// 		logger:     noopLogger(),
// 		msgRepo:    mock,
// 	}
// 	go hub.Run()

// 	client := newTestClient(hub, "room-1", "user-1", "alice")
// 	hub.register <- client

// 	time.Sleep(10 * time.Millisecond)

// 	if hub.ConnectedClients() != 1 {
// 		t.Errorf("expected 1 connected client, got %d", hub.ConnectedClients())
// 	}
// }

// func TestHub_UnregisterClient(t *testing.T) {
// 	mock := &MockMessageRepository{}
// 	hub := &Hub{
// 		rooms:      make(map[string]map[*Client]bool),
// 		broadcast:  make(chan ws.Event),
// 		register:   make(chan *Client),
// 		unregister: make(chan *Client),
// 		logger:     noopLogger(),
// 		msgRepo:    mock,
// 	}
// 	go hub.Run()

// 	client := newTestClient(hub, "room-1", "user-1", "alice")
// 	hub.register <- client
// 	time.Sleep(10 * time.Millisecond)

// 	hub.unregister <- client
// 	time.Sleep(10 * time.Millisecond)

// 	if hub.ConnectedClients() != 0 {
// 		t.Errorf("expected 0 connected clients, got %d", hub.ConnectedClients())
// 	}
// }

// func TestHub_BroadcastToRoom(t *testing.T) {
// 	mock := &MockMessageRepository{}
// 	hub := &Hub{
// 		rooms:      make(map[string]map[*Client]bool),
// 		broadcast:  make(chan ws.Event),
// 		register:   make(chan *Client),
// 		unregister: make(chan *Client),
// 		logger:     noopLogger(),
// 		msgRepo:    mock,
// 	}
// 	go hub.Run()

// 	client1 := newTestClient(hub, "room-1", "user-1", "alice")
// 	client2 := newTestClient(hub, "room-1", "user-2", "bob")
// 	client3 := newTestClient(hub, "room-2", "user-3", "carol")

// 	hub.register <- client1
// 	hub.register <- client2
// 	hub.register <- client3
// 	time.Sleep(10 * time.Millisecond)

// 	msg := Message{
// 		Content:     []byte(`{"text":"hello"}`),
// 		MessageType: websocket.TextMessage,
// 		RoomID:      "room-1",
// 		UserID:      "user-1",
// 		TextContent: "hello",
// 	}
// 	hub.broadcast <- msg
// 	time.Sleep(10 * time.Millisecond)

// 	select {
// 	case received := <-client1.receivedMessages:
// 		if string(received.Content) != `{"text":"hello"}` {
// 			t.Errorf("client1 got wrong content: %s", received.Content)
// 		}
// 	default:
// 		t.Error("client1 did not receive message")
// 	}

// 	select {
// 	case received := <-client2.receivedMessages:
// 		if string(received.Content) != `{"text":"hello"}` {
// 			t.Errorf("client2 got wrong content: %s", received.Content)
// 		}
// 	default:
// 		t.Error("client2 did not receive message")
// 	}

// 	select {
// 	case <-client3.receivedMessages:
// 		t.Error("client3 should not have received message from room-1")
// 	default:
// 		// correct — carol is in room-2
// 	}
// }

// func TestHub_MessageSavedToRepository(t *testing.T) {
// 	mock := &MockMessageRepository{}
// 	hub := &Hub{
// 		rooms:      make(map[string]map[*Client]bool),
// 		broadcast:  make(chan ws.Event),
// 		register:   make(chan *Client),
// 		unregister: make(chan *Client),
// 		logger:     noopLogger(),
// 		msgRepo:    mock,
// 	}
// 	go hub.Run()

// 	client := newTestClient(hub, "room-1", "user-1", "alice")
// 	hub.register <- client
// 	time.Sleep(10 * time.Millisecond)

// 	msg := Message{
// 		Content:     []byte(`{"text":"hello"}`),
// 		MessageType: websocket.TextMessage,
// 		RoomID:      "room-1",
// 		UserID:      "user-1",
// 		TextContent: "hello",
// 	}
// 	hub.broadcast <- msg
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
