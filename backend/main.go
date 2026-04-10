package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/joho/godotenv"
	"github.com/kressinluiz/chat/internal/cache"
	"github.com/kressinluiz/chat/internal/repository"
	"github.com/prometheus/client_golang/prometheus"
)

func DevLoadEnv() {
	err := godotenv.Load()
	if err != nil {
		slog.Warn("no .env file found, reading from environment")
	}
}

func main() {
	DevLoadEnv()
	ConfigLogger()

	ctx := context.Background()
	tracerProvider, err := InitTracer(ctx)
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

	db := CreateDBAndRunMigrations()
	defer func() {
		if err := db.Close(); err != nil {
			slog.Error("failed to close database connection", "error", err)
		}
	}()
	msgRepo := NewMessageRepository(db)
	userRepo := NewUserRepository(db)
	roomRepo := NewRoomRepository(db)
	roomMemberRepo := repository.NewRoomMemberRepo(db)
	hub := StartHub(msgRepo, redisClient)

	prometheus.MustRegister(httpRequests)
	prometheus.MustRegister(NewActiveConnectionsMetric(hub))
	prometheus.MustRegister(requestDuration)
	prometheus.MustRegister(messagesTotal)
	prometheus.MustRegister(wsUpgradeFailures)

	StartServer(hub, userRepo, roomRepo, roomMemberRepo)
}
