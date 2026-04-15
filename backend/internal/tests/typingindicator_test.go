package tests

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/kressinluiz/chat/internal/hub"
	"github.com/kressinluiz/chat/internal/ws"
)

func CreateTypingEventFromClient(roomID string) ws.Event {
	typingFromClient := ws.Event{
		ID:      "test-random-uuid",
		Type:    ws.EventTypeTypingStart,
		RoomID:  roomID,
		Seq:     0,
		Payload: nil,
		SentAt:  time.Now().UTC().Format(time.RFC3339),
	}

	return typingFromClient
}

func TestTypingIndicator_Success(t *testing.T) {
	time.Sleep(50 * time.Millisecond)

	testHub, err := NewTestHub()
	if err != nil {
		t.Errorf("failed to create test hub %v", err)
	}
	go testHub.Run()
	testClient1 := hub.NewClient(nil, testHub, "alice", "test-room-id", "user-1", NoopLogger())
	testClient2 := hub.NewClient(nil, testHub, "bob", "test-room-id", "user-2", NoopLogger())
	testClient3 := hub.NewClient(nil, testHub, "charlie", "test-room-id", "user-3", NoopLogger())
	testHub.Join(testClient1)
	testHub.Join(testClient2)
	testHub.Join(testClient3)
	time.Sleep(10 * time.Millisecond)

	testClient1.HandleTyping(CreateTypingEventFromClient("test-room-id"))
	testClient1.HandleTyping(CreateTypingEventFromClient("test-room-id"))
	testClient1.HandleTyping(CreateTypingEventFromClient("test-room-id"))

	testClient2.HandleTyping(CreateTypingEventFromClient("test-room-id"))
	testClient3.HandleTyping(CreateTypingEventFromClient("test-room-id"))
	time.Sleep(10 * time.Millisecond)

	for {
		select {
		case message := <-testClient1.ReceivedMessages:
			var payload ws.TypingUpdatePayload
			err = json.Unmarshal(message.Payload, &payload)
			if len(payload.TypingUsers) == 0 {
				t.Errorf("expected 3 typing users, got 0")
				return
			}
			if len(payload.TypingUsers) > 3 {
				t.Errorf("expected 3 typing users, got %d", len(payload.TypingUsers))
				return
			}

			for _, username := range payload.TypingUsers {
				if username != "alice" && username != "bob" && username != "charlie" {
					t.Errorf("unexpected typing user %s", username)
				}
			}

			if len(payload.TypingUsers) == 3 {
				t.Log("typing users:", payload.TypingUsers)
				return
			}
		default:
			t.Error("client1 did not receive any message")
		}
	}

}
