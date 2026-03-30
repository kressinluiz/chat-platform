package main

import (
	"log/slog"

	"github.com/joho/godotenv"
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
	db := CreateDBAndRunMigrations()
	defer db.Close()
	msgRepo := NewMessageRepository(db)
	userRepo := NewUserRepository(db)
	roomRepo := NewRoomRepository(db)
	hub := StartHub(msgRepo)

	prometheus.MustRegister(httpRequests)
	prometheus.MustRegister(NewActiveConnectionsMetric(hub))
	prometheus.MustRegister(requestDuration)
	prometheus.MustRegister(messagesTotal)
	prometheus.MustRegister(wsUpgradeFailures)

	StartServer(hub, userRepo, roomRepo)
}
