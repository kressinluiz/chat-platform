package main

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/gorilla/websocket"
	"github.com/kressinluiz/chat/internal/middleware"
	"github.com/kressinluiz/chat/internal/repository"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func RegisterRoutes(hub *Hub, userRepo UserRepo, roomRepo RoomRepo, roomMemberRepo repository.RoomMemberRepo) *http.ServeMux {
	var upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}

	router := http.NewServeMux()
	router.Handle("/login", TracingMiddleware(PrometheusMiddleware(http.HandlerFunc(Login(userRepo)), "/login"), "login"))
	router.Handle("/register", TracingMiddleware(PrometheusMiddleware(http.HandlerFunc(Register(userRepo)), "/register"), "register"))
	router.Handle("/ws", TracingMiddleware(PrometheusMiddleware(middleware.RequireMembership(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		WebSocket(w, r, hub, roomRepo, upgrader)
	}), roomMemberRepo), "ws"), "ws"))
	router.Handle("POST /rooms", TracingMiddleware(PrometheusMiddleware(AuthMiddleware(http.HandlerFunc(CreateRoom(roomRepo))), "/rooms"), "create_room"))
	router.Handle("GET /rooms", TracingMiddleware(PrometheusMiddleware(AuthMiddleware((http.HandlerFunc(ListRooms(roomRepo)))), "/rooms"), "list_rooms"))

	fs := http.FileServer(http.Dir("./frontend"))
	router.Handle("/", fs)

	router.Handle("/metrics", promhttp.Handler())

	return router
}

func StartServer(hub *Hub, userRepo UserRepo, roomRepo RoomRepo, roomMemberRepo repository.RoomMemberRepo) {
	router := RegisterRoutes(hub, userRepo, roomRepo, roomMemberRepo)
	address := os.Getenv("SERVER_ADDRESS")
	if address == "" {
		address = "0.0.0.0:8080"
		slog.Warn("SERVER_ADDRESS not set, defaulting to " + address)
	}
	slog.Info("Starting server on " + address)
	if err := http.ListenAndServe(address, router); err != nil {
		slog.Error("http server error", "error", err)
		os.Exit(1)
	}
}
