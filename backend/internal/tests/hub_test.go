package tests

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/kressinluiz/chat/internal/hub"
	"github.com/kressinluiz/chat/internal/ws"
)

func TestHub_RegisterClient(t *testing.T) {
	h, err := NewTestHub()
	if err != nil {
		t.Fatalf("failed to create hub: %v", err)
	}
	go h.Run()
	client := hub.NewClient(nil, h, "room-1", "user-1", "alice", NoopLogger())
	h.Join(client)

	time.Sleep(10 * time.Millisecond)

	if h.ConnectedClients() != 1 {
		t.Errorf("expected 1 connected client, got %d", h.ConnectedClients())
	}
}

func TestHub_UnregisterClient(t *testing.T) {
	h, err := NewTestHub()
	if err != nil {
		t.Fatalf("failed to create hub: %v", err)
	}
	go h.Run()
	client := hub.NewClient(nil, h, "room-1", "user-1", "alice", NoopLogger())
	h.Join(client)
	time.Sleep(10 * time.Millisecond)

	h.Leave(client)
	time.Sleep(10 * time.Millisecond)

	if h.ConnectedClients() != 0 {
		t.Errorf("expected 0 connected clients, got %d", h.ConnectedClients())
	}
}

func TestHub_BroadcastToRoom(t *testing.T) {
	h, err := NewTestHub()
	if err != nil {
		t.Fatalf("failed to create hub: %v", err)
	}
	go h.Run()
	client1 := hub.NewClient(nil, h, "user-1", "room-1", "alice", NoopLogger())
	client2 := hub.NewClient(nil, h, "user-2", "room-1", "bob", NoopLogger())
	client3 := hub.NewClient(nil, h, "user-3", "room-2", "carol", NoopLogger())

	h.Join(client1)
	h.Join(client2)
	h.Join(client3)
	time.Sleep(10 * time.Millisecond)

	event, err := CreateTestMessage()
	if err != nil {
		t.Fatalf("failed to create test message: %v", err)
	}

	h.Broadcast <- event
	time.Sleep(10 * time.Millisecond)

	select {
	case received := <-client1.ReceivedMessages:
		var testPayload ws.NewMessagePayload
		err = json.Unmarshal(received.Payload, &testPayload)
		if string(testPayload.Content) != "hello" {
			t.Errorf("client1 got wrong content: %s", testPayload.Content)
		}
	default:
		t.Error("client1 did not receive message")
	}

	select {
	case received := <-client2.ReceivedMessages:
		var testPayload ws.NewMessagePayload
		err = json.Unmarshal(received.Payload, &testPayload)
		if string(testPayload.Content) != "hello" {
			t.Errorf("client2 got wrong content: %s", testPayload.Content)
		}
	default:
		t.Error("client2 did not receive message")
	}

	select {
	case <-client3.ReceivedMessages:
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
