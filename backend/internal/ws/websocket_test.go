package ws

// import (
// 	"encoding/json"
// 	"net/http/httptest"
// 	"strings"
// 	"testing"
// 	"time"

// 	"github.com/gorilla/websocket"
// 	"github.com/kressinluiz/chat/internal/ws"
// )

// func newTestServer(t *testing.T) (*httptest.Server, *Hub) {
// 	t.Helper()

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

// 	userRepo := &MockUserRepository{}
// 	roomRepo := &MockRoomRepository{}
// 	roomMemberRepo := &MockRoomMemberRepo{}

// 	server := httptest.NewServer(RegisterRoutes(hub, userRepo, roomRepo, roomMemberRepo))
// 	t.Cleanup(server.Close)

// 	return server, hub
// }

// func wsURL(server *httptest.Server, token, roomID string) string {
// 	url := strings.Replace(server.URL, "http", "ws", 1)
// 	return url + "/ws?token=" + token + "&room_id=" + roomID
// }

// func connectWS(t *testing.T, url string) *websocket.Conn {
// 	t.Helper()
// 	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
// 	if err != nil {
// 		t.Fatalf("failed to connect to websocket: %v", err)
// 	}
// 	t.Cleanup(func() {
// 		if err := conn.Close(); err != nil {
// 			t.Errorf("failed to close websocket connection: %v", err)
// 		}
// 	})
// 	return conn
// }

// func readMessage(t *testing.T, conn *websocket.Conn) MessageContent {
// 	t.Helper()
// 	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
// 		t.Fatalf("failed to set read deadline: %v", err)
// 	}

// 	_, data, err := conn.ReadMessage()
// 	if err != nil {
// 		t.Fatalf("failed to read message: %v", err)
// 	}
// 	var msg MessageContent
// 	if err := json.Unmarshal(data, &msg); err != nil {
// 		t.Fatalf("failed to unmarshal message: %v", err)
// 	}
// 	return msg
// }

// func sendMessage(t *testing.T, conn *websocket.Conn, text string) {
// 	t.Helper()
// 	payload, _ := json.Marshal(map[string]string{"text": text})
// 	if err := conn.WriteMessage(websocket.TextMessage, payload); err != nil {
// 		t.Fatalf("failed to send message: %v", err)
// 	}
// }

// func testToken(t *testing.T, userID, username string) string {
// 	t.Helper()
// 	t.Setenv("JWT_SECRET", "test-secret-at-least-32-characters-long")
// 	token, err := GenerateToken(userID, username)
// 	if err != nil {
// 		t.Fatalf("failed to generate test token: %v", err)
// 	}
// 	return token
// }

// func TestWebSocket_RejectsNoToken(t *testing.T) {
// 	server, _ := newTestServer(t)
// 	url := strings.Replace(server.URL, "http", "ws", 1) + "/ws?room_id=test-room-id"

// 	_, resp, err := websocket.DefaultDialer.Dial(url, nil)
// 	if err == nil {
// 		t.Fatal("expected connection to be rejected, but it succeeded")
// 	}
// 	if resp.StatusCode != 401 {
// 		t.Errorf("expected status 401, got %d", resp.StatusCode)
// 	}
// }

// func TestWebSocket_RejectsInvalidToken(t *testing.T) {
// 	server, _ := newTestServer(t)
// 	t.Setenv("JWT_SECRET", "mICZv3d9RkvkN1U9EjOn5yyUuq7L4bFDXUWssxegCfU=")
// 	url := wsURL(server, "invalidtoken", "test-room-id")

// 	_, resp, err := websocket.DefaultDialer.Dial(url, nil)
// 	if err == nil {
// 		t.Fatal("expected connection to be rejected, but it succeeded")
// 	}
// 	if resp.StatusCode != 401 {
// 		t.Errorf("expected status 401, got %d", resp.StatusCode)
// 	}
// }

// func TestWebSocket_RejectsUnknownRoom(t *testing.T) {
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

// 	roomRepo := &MockRoomRepository{}
// 	roomMemberRepo := &MockRoomMemberRepo{}
// 	server := httptest.NewServer(RegisterRoutes(hub, &MockUserRepository{}, roomRepo, roomMemberRepo))
// 	t.Cleanup(server.Close)

// 	token := testToken(t, "user-id", "user-test")
// 	url := wsURL(server, token, "nonexistent-room-id")

// 	_, resp, err := websocket.DefaultDialer.Dial(url, nil)
// 	if err == nil {
// 		t.Fatal("expected connection to be rejected")
// 	}
// 	if resp.StatusCode != 404 {
// 		t.Errorf("expected status 404, got %d", resp.StatusCode)
// 	}
// }

// func TestWebSocket_ConnectsSuccessfully(t *testing.T) {
// 	server, _ := newTestServer(t)
// 	token := testToken(t, "user-1", "alice")
// 	url := wsURL(server, token, "test-room-id")

// 	conn := connectWS(t, url)
// 	if conn == nil {
// 		t.Fatal("expected successful connection")
// 	}
// }

// func TestWebSocket_MessageBroadcastToRoom(t *testing.T) {
// 	server, _ := newTestServer(t)
// 	token1 := testToken(t, "user-1", "alice")
// 	token2 := testToken(t, "user-2", "bob")

// 	conn1 := connectWS(t, wsURL(server, token1, "test-room-id"))
// 	conn2 := connectWS(t, wsURL(server, token2, "test-room-id"))
// 	time.Sleep(50 * time.Millisecond)

// 	sendMessage(t, conn1, "hello from alice")

// 	msg := readMessage(t, conn1)
// 	if msg.Text != "hello from alice" {
// 		t.Errorf("conn1 got wrong text: %q", msg.Text)
// 	}
// 	if msg.Username != "alice" {
// 		t.Errorf("conn1 got wrong username: %q", msg.Username)
// 	}

// 	msg = readMessage(t, conn2)
// 	if msg.Text != "hello from alice" {
// 		t.Errorf("conn2 got wrong text: %q", msg.Text)
// 	}
// 	if msg.Username != "alice" {
// 		t.Errorf("conn2 got wrong username: %q", msg.Username)
// 	}
// }

// func TestWebSocket_MessageNotBroadcastToOtherRoom(t *testing.T) {
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

// 	roomRepo := &MockRoomRepository{}
// 	roomMemberRepo := &MockRoomMemberRepo{}
// 	server := httptest.NewServer(RegisterRoutes(hub, &MockUserRepository{}, roomRepo, roomMemberRepo))
// 	t.Cleanup(server.Close)

// 	token1 := testToken(t, "user-1", "alice")
// 	token2 := testToken(t, "user-2", "bob")

// 	conn1 := connectWS(t, wsURL(server, token1, "test-room-id"))
// 	conn2 := connectWS(t, wsURL(server, token2, "other-room-id"))
// 	time.Sleep(50 * time.Millisecond)

// 	sendMessage(t, conn1, "hello from alice")
// 	time.Sleep(50 * time.Millisecond)

// 	if err := conn2.SetReadDeadline(time.Now().Add(200 * time.Millisecond)); err != nil {
// 		t.Fatalf("failed to set read deadline: %v", err)
// 	}

// 	_, _, err := conn2.ReadMessage()
// 	if err == nil {
// 		t.Error("conn2 should not have received a message from a different room")
// 	}
// }
