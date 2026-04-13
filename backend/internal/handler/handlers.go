package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/kressinluiz/chat/internal/auth"
	"github.com/kressinluiz/chat/internal/hub"
	"github.com/kressinluiz/chat/internal/repository"
	"github.com/kressinluiz/chat/internal/ws"
	"golang.org/x/crypto/bcrypt"
)

type RegisterRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type RegisterResponse struct {
	ID       string `json:"id"`
	Username string `json:"username"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Token    string `json:"token"`
	UserID   string `json:"user_id"`
	Username string `json:"username"`
}

type CreateRoomRequest struct {
	Name string `json:"name"`
}

func Register(userRepo repository.UserRepo) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req RegisterRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		req.Username = strings.TrimSpace(req.Username)
		if req.Username == "" || len(req.Username) > 20 {
			WriteError(w, http.StatusBadRequest, "username must be between 1 and 20 characters")
			return
		}
		if len(req.Password) < 8 {
			WriteError(w, http.StatusBadRequest, "password must be at least 8 characters")
			return
		}

		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "failed to process password")
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		id, err := userRepo.Create(ctx, req.Username, string(hashedPassword))
		if err != nil {
			if strings.Contains(err.Error(), "unique") {
				WriteError(w, http.StatusConflict, "username already taken")
				return
			}
			WriteError(w, http.StatusInternalServerError, "failed to create user")
			return
		}

		WriteJSON(w, http.StatusCreated, RegisterResponse{
			ID:       id,
			Username: req.Username,
		})
	}
}

func Login(userRepo repository.UserRepo) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req LoginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		req.Username = strings.TrimSpace(req.Username)
		if req.Username == "" || req.Password == "" {
			WriteError(w, http.StatusBadRequest, "username and password are required")
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		id, hashedPassword, err := userRepo.GetByUsername(ctx, req.Username)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "failed to fetch user")
			return
		}
		if id == "" {
			WriteError(w, http.StatusUnauthorized, "invalid credentials")
			return
		}

		if err := bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(req.Password)); err != nil {
			WriteError(w, http.StatusUnauthorized, "invalid credentials")
			return
		}

		token, err := auth.GenerateToken(id, req.Username)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "failed to generate token")
			return
		}

		WriteJSON(w, http.StatusOK, LoginResponse{
			Token:    token,
			UserID:   id,
			Username: req.Username,
		})
	}
}

func CreateRoom(roomRepo repository.RoomRepo) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req CreateRoomRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		req.Name = strings.TrimSpace(req.Name)
		if req.Name == "" || len(req.Name) > 50 {
			WriteError(w, http.StatusBadRequest, "room name must be between 1 and 50 characters")
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		room, err := roomRepo.Create(ctx, req.Name)
		if err != nil {
			if strings.Contains(err.Error(), "unique") {
				WriteError(w, http.StatusConflict, "room name already taken")
				return
			}
			WriteError(w, http.StatusInternalServerError, "failed to create room")
			return
		}

		WriteJSON(w, http.StatusCreated, room)
	}
}

func ListRooms(roomRepo repository.RoomRepo) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		rooms, err := roomRepo.List(ctx)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "failed to list rooms")
			return
		}

		WriteJSON(w, http.StatusOK, rooms)
	}
}

func WebSocket(w http.ResponseWriter, r *http.Request, h *hub.Hub, roomRepo repository.RoomRepo, upgrader websocket.Upgrader) {
	tokenString := r.URL.Query().Get("token")
	if tokenString == "" {
		// wsUpgradeFailures.Inc()
		http.Error(w, "missing token", http.StatusUnauthorized)
		return
	}

	roomID := r.URL.Query().Get("room_id")
	if roomID == "" {
		// wsUpgradeFailures.Inc()
		http.Error(w, "missing room_id", http.StatusUnauthorized)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	roomFound, err := roomRepo.GetByID(ctx, roomID)

	if err != nil {
		// wsUpgradeFailures.Inc()
		http.Error(w, "failed to check room", http.StatusInternalServerError)
		return
	}

	if !roomFound {
		// wsUpgradeFailures.Inc()
		http.Error(w, "room not found", http.StatusNotFound)
		return
	}

	claims, err := auth.ValidateToken(tokenString)
	if err != nil {
		// wsUpgradeFailures.Inc()
		slog.Warn("invalid token on websocket upgrade", "error", err)
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		// wsUpgradeFailures.Inc()
		http.Error(w, "failed to upgrade to websocket connection", http.StatusInternalServerError)
		return
	}

	client := hub.Client{
		Conn:             conn,
		ReceivedMessages: make(chan ws.Event, hub.ClientBufferSize),
		Hub:              h,
		Username:         claims.Username,
		UserID:           claims.UserID,
		RoomID:           roomID,
		Logger:           slog.Default().With("component", "client"),
	}

	h.Register <- &client

	go client.ReadRoutine()
	go client.WriteRoutine()
}

func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	err := json.NewEncoder(w).Encode(v)
	if err != nil {
		slog.Error("failed to encode JSON response", "error", err)
	}
}

func WriteError(w http.ResponseWriter, status int, msg string) {
	WriteJSON(w, status, ErrorResponse{Error: msg})
}
