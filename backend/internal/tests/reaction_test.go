package tests

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/kressinluiz/chat/internal/hub"
	"github.com/kressinluiz/chat/internal/ws"
)

func CreateReactionEventFromClient(roomID string) ws.Event {
	newReaction := ws.ReactionPayload{
		MessageID: "test-message-id",
		Emoji:     "😀",
	}
	payloadBytes, _ := json.Marshal(newReaction)

	reactionFromClient := ws.Event{
		ID:      "test-random-uuid",
		Type:    ws.EventTypeReaction,
		RoomID:  roomID,
		Seq:     0,
		Payload: payloadBytes,
		SentAt:  time.Now().UTC().Format(time.RFC3339),
	}

	return reactionFromClient
}

func TestReaction_Toggle(t *testing.T) {
	testHub, err := NewTestHub()
	if err != nil {
		t.Errorf("failed to create test hub %v", err)
	}
	go testHub.Run()
	testClient1 := hub.NewClient(nil, testHub, "alice", "test-room-id", "user-1", NoopLogger())
	testHub.Join(testClient1)
	time.Sleep(10 * time.Millisecond)

	testClient1.HandleReaction(CreateReactionEventFromClient("test-room-id"))
	time.Sleep(10 * time.Millisecond)

	select {
	case message := <-testClient1.ReceivedMessages:
		var payload ws.ReactionUpdatePayload
		err = json.Unmarshal(message.Payload, &payload)
		if payload.MessageID != "test-message-id" {
			t.Errorf("expected message ID 'test-message-id', got '%s'", payload.MessageID)
			return
		}
		if len(payload.Reactions) != 1 {
			t.Errorf("expected 1 reaction, got %d", len(payload.Reactions))
			return
		}
		for reaction := range payload.Reactions {
			if reaction != "😀" {
				t.Errorf("expected emoji '😀', got '%s'", reaction)
				return
			}
		}
	default:
		t.Error("client1 did not receive any message")
		return
	}

	testClient1.HandleReaction(CreateReactionEventFromClient("test-room-id"))
	time.Sleep(10 * time.Millisecond)

	select {
	case message := <-testClient1.ReceivedMessages:
		var payload ws.ReactionUpdatePayload
		err = json.Unmarshal(message.Payload, &payload)
		if payload.MessageID != "test-message-id" {
			t.Errorf("expected message ID 'test-message-id', got '%s'", payload.MessageID)
			return
		}
		if len(payload.Reactions) != 0 {
			t.Errorf("expected 0 reactions, got %d", len(payload.Reactions))
			return
		}
	default:
		t.Error("client1 did not receive any message")
		return
	}
}
