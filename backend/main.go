package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/joho/godotenv"
	"github.com/kressinluiz/chat/internal/cache"
	"github.com/kressinluiz/chat/internal/db"
	"github.com/kressinluiz/chat/internal/hub"
	"github.com/kressinluiz/chat/internal/log"
	"github.com/kressinluiz/chat/internal/repository"
	"github.com/kressinluiz/chat/internal/server"
)

func DevLoadEnv() {
	err := godotenv.Load()
	if err != nil {
		slog.Warn("no .env file found, reading from environment")
	}
}

func main() {
	DevLoadEnv()
	log.ConfigLogger()

	ctx := context.Background()
	tracerProvider, err := log.InitTracer(ctx)
	if err != nil {
		slog.Error("failed to initialize tracer", "error", err)
		os.Exit(1)

	}
	defer func() {
		if err := tracerProvider.Shutdown(ctx); err != nil {
			slog.Error("failed to shutdown tracer provider", "error", err)
		}
	}()

	redisClient, err := cache.NewRedisClient(os.Getenv("REDIS_ADDR"))
	if err != nil {
		slog.Error("failed to connect to redis", "error", err)
		os.Exit(1)
	}

	db := db.CreateDBAndRunMigrations()
	defer func() {
		if err := db.Close(); err != nil {
			slog.Error("failed to close database connection", "error", err)
		}
	}()
	msgRepo := repository.NewMessageRepository(db)
	reactionRepo := repository.NewReactionRepo(db)
	userRepo := repository.NewUserRepository(db)
	roomRepo := repository.NewRoomRepository(db)
	roomMemberRepo := repository.NewRoomMemberRepo(db)
	hub, err := hub.NewHub(msgRepo, reactionRepo, redisClient, slog.Default().With("component", "hub"))
	if err != nil {
		slog.Error("failed to create hub", "error", err)
		os.Exit(1)
	}
	go hub.Run()

	// prometheus.MustRegister(httpRequests)
	// prometheus.MustRegister(NewActiveConnectionsMetric(hub))
	// prometheus.MustRegister(requestDuration)
	// prometheus.MustRegister(messagesTotal)
	// prometheus.MustRegister(wsUpgradeFailures)

	server.StartServer(hub, userRepo, roomRepo, roomMemberRepo)
}
