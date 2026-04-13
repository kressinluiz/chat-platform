package server

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/gorilla/websocket"
	"github.com/kressinluiz/chat/internal/handler"
	"github.com/kressinluiz/chat/internal/hub"
	"github.com/kressinluiz/chat/internal/log"
	"github.com/kressinluiz/chat/internal/middleware"
	"github.com/kressinluiz/chat/internal/repository"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func RegisterRoutes(hub *hub.Hub, userRepo repository.UserRepo, roomRepo repository.RoomRepo, roomMemberRepo repository.RoomMemberRepo) *http.ServeMux {
	var upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}

	router := http.NewServeMux()
	router.Handle("/login", middleware.TracingMiddleware(log.PrometheusMiddleware(http.HandlerFunc(handler.Login(userRepo)), "/login"), "login"))
	router.Handle("/register", middleware.TracingMiddleware(log.PrometheusMiddleware(http.HandlerFunc(handler.Register(userRepo)), "/register"), "register"))
	router.Handle("/ws", middleware.TracingMiddleware(log.PrometheusMiddleware(middleware.RequireMembership(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.WebSocket(w, r, hub, roomRepo, upgrader)
	}), roomMemberRepo), "ws"), "ws"))
	router.Handle("POST /rooms", middleware.TracingMiddleware(log.PrometheusMiddleware(middleware.AuthMiddleware(http.HandlerFunc(handler.CreateRoom(roomRepo))), "/rooms"), "create_room"))
	router.Handle("GET /rooms", middleware.TracingMiddleware(log.PrometheusMiddleware(middleware.AuthMiddleware((http.HandlerFunc(handler.ListRooms(roomRepo)))), "/rooms"), "list_rooms"))

	fs := http.FileServer(http.Dir("./frontend"))
	router.Handle("/", fs)

	router.Handle("/metrics", promhttp.Handler())

	return router
}

func StartServer(hub *hub.Hub, userRepo repository.UserRepo, roomRepo repository.RoomRepo, roomMemberRepo repository.RoomMemberRepo) {
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
