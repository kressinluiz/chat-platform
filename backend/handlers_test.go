package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kressinluiz/chat/internal/ws"
)

func newTestHTTPServer(t *testing.T) *httptest.Server {
	t.Helper()
	mock := &MockMessageRepository{}
	hub := &Hub{
		rooms:      make(map[string]map[*Client]bool),
		broadcast:  make(chan ws.Event),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		logger:     noopLogger(),
		msgRepo:    mock,
	}
	go hub.Run()
	server := httptest.NewServer(RegisterRoutes(hub, newMockUserRepo(), &MockRoomRepository{}, &MockRoomMemberRepo{}))
	t.Cleanup(server.Close)
	return server
}

func postJSON(t *testing.T, url string, body any) *http.Response {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("failed to marshal request body: %v", err)
	}
	resp, err := http.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		t.Fatalf("failed to make POST request: %v", err)
	}
	return resp
}

func getJSON(t *testing.T, url, token string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to make GET request: %v", err)
	}
	return resp
}

func postJSONWithToken(t *testing.T, url, token string, body any) *http.Response {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("failed to marshal request body: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to make POST request: %v", err)
	}
	return resp
}

func decodeJSON(t *testing.T, resp *http.Response, v any) {
	t.Helper()
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Errorf("failed to close response body: %v", err)
		}
	}()
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
}

func assertStatus(t *testing.T, resp *http.Response, expected int) {
	t.Helper()
	if resp.StatusCode != expected {
		t.Errorf("expected status %d, got %d", expected, resp.StatusCode)
	}
}

func TestRegister_Success(t *testing.T) {
	server := newTestHTTPServer(t)

	resp := postJSON(t, server.URL+"/register", RegisterRequest{
		Username: "alice",
		Password: "password123",
	})
	assertStatus(t, resp, http.StatusCreated)

	var body RegisterResponse
	decodeJSON(t, resp, &body)
	if body.Username != "alice" {
		t.Errorf("expected username 'alice', got %q", body.Username)
	}
	if body.ID == "" {
		t.Error("expected non-empty ID")
	}
}

func TestRegister_DuplicateUsername(t *testing.T) {
	server := newTestHTTPServer(t)

	postJSON(t, server.URL+"/register", RegisterRequest{
		Username: "alice",
		Password: "password123",
	})

	resp := postJSON(t, server.URL+"/register", RegisterRequest{
		Username: "alice",
		Password: "differentpassword",
	})
	assertStatus(t, resp, http.StatusConflict)
}

func TestRegister_Validation(t *testing.T) {
	server := newTestHTTPServer(t)

	tests := []struct {
		name     string
		username string
		password string
		status   int
	}{
		{"empty username", "", "password123", http.StatusBadRequest},
		{"username too long", "thisusernameiswaytoolong", "password123", http.StatusBadRequest},
		{"password too short", "alice", "short", http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := postJSON(t, server.URL+"/register", RegisterRequest{
				Username: tt.username,
				Password: tt.password,
			})
			assertStatus(t, resp, tt.status)
		})
	}
}

func TestLogin_Success(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-at-least-32-characters-long")

	userRepo := newMockUserRepo()
	mock := &MockMessageRepository{}
	roomMemberRepo := &MockRoomMemberRepo{}
	hub := &Hub{
		rooms:      make(map[string]map[*Client]bool),
		broadcast:  make(chan ws.Event),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		logger:     noopLogger(),
		msgRepo:    mock,
	}
	go hub.Run()
	server := httptest.NewServer(RegisterRoutes(hub, userRepo, &MockRoomRepository{}, roomMemberRepo))
	t.Cleanup(server.Close)

	postJSON(t, server.URL+"/register", RegisterRequest{
		Username: "alice",
		Password: "password123",
	})

	resp := postJSON(t, server.URL+"/login", LoginRequest{
		Username: "alice",
		Password: "password123",
	})
	assertStatus(t, resp, http.StatusOK)

	var body LoginResponse
	decodeJSON(t, resp, &body)
	if body.Token == "" {
		t.Error("expected non-empty token")
	}
	if body.Username != "alice" {
		t.Errorf("expected username 'alice', got %q", body.Username)
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-at-least-32-characters-long")

	userRepo := newMockUserRepo()
	mock := &MockMessageRepository{}
	hub := &Hub{
		rooms:      make(map[string]map[*Client]bool),
		broadcast:  make(chan ws.Event),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		logger:     noopLogger(),
		msgRepo:    mock,
	}
	go hub.Run()
	server := httptest.NewServer(RegisterRoutes(hub, userRepo, &MockRoomRepository{}, &MockRoomMemberRepo{}))
	t.Cleanup(server.Close)

	postJSON(t, server.URL+"/register", RegisterRequest{
		Username: "alice",
		Password: "password123",
	})

	resp := postJSON(t, server.URL+"/login", LoginRequest{
		Username: "alice",
		Password: "wrongpassword",
	})
	assertStatus(t, resp, http.StatusUnauthorized)

	var body ErrorResponse
	decodeJSON(t, resp, &body)
	if body.Error != "invalid credentials" {
		t.Errorf("expected 'invalid credentials', got %q", body.Error)
	}
}

func TestLogin_UnknownUser(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-at-least-32-characters-long")
	server := newTestHTTPServer(t)

	resp := postJSON(t, server.URL+"/login", LoginRequest{
		Username: "nobody",
		Password: "password123",
	})
	assertStatus(t, resp, http.StatusUnauthorized)

	var body ErrorResponse
	decodeJSON(t, resp, &body)
	if body.Error != "invalid credentials" {
		t.Errorf("expected same error as wrong password, got %q", body.Error)
	}
}

func TestListRooms_RequiresAuth(t *testing.T) {
	server := newTestHTTPServer(t)

	resp := getJSON(t, server.URL+"/rooms", "")
	assertStatus(t, resp, http.StatusUnauthorized)
}

func TestListRooms_Success(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-at-least-32-characters-long")
	server := newTestHTTPServer(t)
	token := testToken(t, "user-1", "alice")

	resp := getJSON(t, server.URL+"/rooms", token)
	assertStatus(t, resp, http.StatusOK)

	var rooms []Room
	decodeJSON(t, resp, &rooms)
	if len(rooms) != 2 {
		t.Errorf("expected 2 rooms, got %d", len(rooms)) //MockRepository has 2 rooms
	}
}

func TestCreateRoom_RequiresAuth(t *testing.T) {
	server := newTestHTTPServer(t)

	resp := postJSON(t, server.URL+"/rooms", CreateRoomRequest{Name: "general"})
	assertStatus(t, resp, http.StatusUnauthorized)
}

func TestCreateRoom_Success(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-at-least-32-characters-long")
	server := newTestHTTPServer(t)
	token := testToken(t, "user-1", "alice")

	resp := postJSONWithToken(t, server.URL+"/rooms", token, CreateRoomRequest{
		Name: "general",
	})
	assertStatus(t, resp, http.StatusCreated)

	var room Room
	decodeJSON(t, resp, &room)
	if room.Name != "general" {
		t.Errorf("expected room name 'general', got %q", room.Name)
	}
	if room.ID == "" {
		t.Error("expected non-empty room ID")
	}
}

func TestCreateRoom_Validation(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-at-least-32-characters-long")
	server := newTestHTTPServer(t)
	token := testToken(t, "user-1", "alice")

	tests := []struct {
		name   string
		input  string
		status int
	}{
		{"empty name", "", http.StatusBadRequest},
		{"name too long", strings.Repeat("a", 51), http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := postJSONWithToken(t, server.URL+"/rooms", token, CreateRoomRequest{
				Name: tt.input,
			})
			assertStatus(t, resp, tt.status)
		})
	}
}
