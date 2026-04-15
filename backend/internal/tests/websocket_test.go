package tests

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/kressinluiz/chat/internal/auth"
	"github.com/kressinluiz/chat/internal/hub"
	"github.com/kressinluiz/chat/internal/server"
	"github.com/kressinluiz/chat/internal/ws"
)

func newTestServer(t *testing.T) (*httptest.Server, *hub.Hub) {
	t.Helper()
	h, err := NewTestHub()
	if err != nil {
		t.Fatalf("failed to create hub: %v", err)
	}
	go h.Run()

	userRepo := &MockUserRepository{}
	roomRepo := &MockRoomRepository{}
	roomMemberRepo := &MockRoomMemberRepo{}

	server := httptest.NewServer(server.RegisterRoutes(h, userRepo, roomRepo, roomMemberRepo))
	t.Cleanup(server.Close)

	return server, h
}

func wsURL(server *httptest.Server, token, roomID string) string {
	url := strings.Replace(server.URL, "http", "ws", 1)
	return url + "/ws?token=" + token + "&room_id=" + roomID
}

func connectWS(t *testing.T, url string) *websocket.Conn {
	t.Helper()
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("failed to connect to websocket: %v", err)
	}
	t.Cleanup(func() {
		if err := conn.Close(); err != nil {
			t.Errorf("failed to close websocket connection: %v", err)
		}
	})
	return conn
}

func readMessage(t *testing.T, conn *websocket.Conn) ws.SendMessagePayload {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("failed to set read deadline: %v", err)
	}

	_, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read message: %v", err)
	}

	var event ws.Event
	err = json.Unmarshal(data, &event)
	if err != nil {
		t.Fatalf("failed to unmarshal event error %v", err)
	}

	var payload ws.SendMessagePayload
	err = json.Unmarshal(event.Payload, &payload)
	if err != nil {
		t.Fatalf("failed to unmarshal message error %v", err)
	}

	return payload
}

func sendMessage(t *testing.T, conn *websocket.Conn, content string, roomID string) {
	t.Helper()

	newMessageTest := ws.SendMessagePayload{
		Content: content,
	}

	testPayload, err := json.Marshal(newMessageTest)
	if err != nil {
		t.Error("failed to marshal test payload", "error", err)
		return
	}

	event := ws.Event{
		ID:      "event-test-id",
		Type:    ws.EventTypeSendMessage,
		RoomID:  roomID,
		Seq:     0,
		Payload: testPayload,
		SentAt:  time.Now().UTC().Format(time.RFC3339),
	}

	payload, _ := json.Marshal(event)
	if err := conn.WriteMessage(websocket.TextMessage, payload); err != nil {
		t.Fatalf("failed to send message: %v", err)
	}
}

func testToken(t *testing.T, userID, username string) string {
	t.Helper()
	t.Setenv("JWT_SECRET", "test-secret-at-least-32-characters-long")
	token, err := auth.GenerateToken(userID, username)
	if err != nil {
		t.Fatalf("failed to generate test token: %v", err)
	}
	return token
}

func TestWebSocket_RejectsNoToken(t *testing.T) {
	server, _ := newTestServer(t)
	url := strings.Replace(server.URL, "http", "ws", 1) + "/ws?room_id=test-room-id"

	_, resp, err := websocket.DefaultDialer.Dial(url, nil)
	if err == nil {
		t.Fatal("expected connection to be rejected, but it succeeded")
	}
	if resp.StatusCode != 401 {
		t.Errorf("expected status 401, got %d", resp.StatusCode)
	}
}

func TestWebSocket_RejectsInvalidToken(t *testing.T) {
	server, _ := newTestServer(t)
	t.Setenv("JWT_SECRET", "mICZv3d9RkvkN1U9EjOn5yyUuq7L4bFDXUWssxegCfU=")
	url := wsURL(server, "invalidtoken", "test-room-id")

	_, resp, err := websocket.DefaultDialer.Dial(url, nil)
	if err == nil {
		t.Fatal("expected connection to be rejected, but it succeeded")
	}
	if resp.StatusCode != 401 {
		t.Errorf("expected status 401, got %d", resp.StatusCode)
	}
}

func TestWebSocket_RejectsUnknownRoom(t *testing.T) {
	hub, err := NewTestHub()
	if err != nil {
		t.Fatalf("failed to create hub: %v", err)
	}

	roomRepo := &MockRoomRepository{}
	roomMemberRepo := &MockRoomMemberRepo{}
	server := httptest.NewServer(server.RegisterRoutes(hub, &MockUserRepository{}, roomRepo, roomMemberRepo))
	t.Cleanup(server.Close)

	token := testToken(t, "user-id", "user-test")
	url := wsURL(server, token, "nonexistent-room-id")

	_, resp, err := websocket.DefaultDialer.Dial(url, nil)
	if err == nil {
		t.Fatal("expected connection to be rejected")
	}
	if resp.StatusCode != 404 {
		t.Errorf("expected status 404, got %d", resp.StatusCode)
	}
}

func TestWebSocket_ConnectsSuccessfully(t *testing.T) {
	server, _ := newTestServer(t)
	token := testToken(t, "user-1", "alice")
	url := wsURL(server, token, "test-room-id")

	conn := connectWS(t, url)
	if conn == nil {
		t.Fatal("expected successful connection")
	}
}

func TestWebSocket_MessageBroadcastToRoom(t *testing.T) {
	server, _ := newTestServer(t)
	token1 := testToken(t, "user-1", "alice")
	token2 := testToken(t, "user-2", "bob")

	conn1 := connectWS(t, wsURL(server, token1, "test-room-id"))
	conn2 := connectWS(t, wsURL(server, token2, "test-room-id"))
	time.Sleep(50 * time.Millisecond)

	sendMessage(t, conn1, "hello from alice", "test-room-id")

	msg := readMessage(t, conn1)
	if msg.Content != "hello from alice" {
		t.Errorf("conn1 got wrong text: %q", msg.Content)
	}

	msg = readMessage(t, conn2)
	if msg.Content != "hello from alice" {
		t.Errorf("conn2 got wrong text: %q", msg.Content)
	}
}

func TestWebSocket_MessageNotBroadcastToOtherRoom(t *testing.T) {
	hub, err := NewTestHub()
	if err != nil {
		t.Fatalf("failed to create hub: %v", err)
	}

	roomRepo := &MockRoomRepository{}
	roomMemberRepo := &MockRoomMemberRepo{}
	server := httptest.NewServer(server.RegisterRoutes(hub, &MockUserRepository{}, roomRepo, roomMemberRepo))
	t.Cleanup(server.Close)

	token1 := testToken(t, "user-1", "alice")
	token2 := testToken(t, "user-2", "bob")

	conn1 := connectWS(t, wsURL(server, token1, "test-room-id"))
	conn2 := connectWS(t, wsURL(server, token2, "other-room-id"))
	time.Sleep(50 * time.Millisecond)

	sendMessage(t, conn1, "hello from alice", "test-room-id")
	time.Sleep(50 * time.Millisecond)

	if err := conn2.SetReadDeadline(time.Now().Add(200 * time.Millisecond)); err != nil {
		t.Fatalf("failed to set read deadline: %v", err)
	}

	_, _, err = conn2.ReadMessage()
	if err == nil {
		t.Error("conn2 should not have received a message from a different room")
	}
}
