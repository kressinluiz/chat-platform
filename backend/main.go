package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/joho/godotenv"
)

func ServeIndex(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "../frontend/index.html")
}

const PingInterval = 5 * time.Second
const DeadlineInterval = 10 * time.Second
const MessageSizeLimit = 4096 //4KB
const ClientBufferSize = 256  //256 messages
const ServerAddress = "localhost:8080"

const HistoryLimit = 50

type Hub struct {
	clients    map[*Client]bool
	broadcast  chan Message
	register   chan *Client
	unregister chan *Client
	logger     *slog.Logger
	msgRepo    *MessageRepository
}

type Message struct {
	Content     []byte
	MessageType int
	RoomID      string
	UserID      string
	TextContent string
}

type MessageContent struct {
	Username  string `json:"username"`
	UserID    string `json:"user_id"`
	RoomID    string `json:"room_id"`
	Text      string `json:"text"`
	Timestamp int64  `json:"timestamp"`
}

func NewHub(msgRepo *MessageRepository) *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		broadcast:  make(chan Message),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		logger:     slog.Default().With("component", "hub"),
		msgRepo:    msgRepo,
	}
}

func (h *Hub) Run() {
	for {
		select {
		case newClient := <-h.register:
			h.clients[newClient] = true

		case unregisterClient := <-h.unregister:
			_, exists := h.clients[unregisterClient]
			if exists {
				close(unregisterClient.receivedMessages)
				delete(h.clients, unregisterClient)
			}

		case message := <-h.broadcast:
			for client := range h.clients {
				select {
				case client.receivedMessages <- message:

				default:
					close(client.receivedMessages)
					delete(h.clients, client)
				}
			}

			go func(msg Message) {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()

				if err := h.msgRepo.Save(ctx, msg.RoomID, msg.UserID, msg.TextContent); err != nil {
					h.logger.Error("failed to save message", "error", err)
				}
			}(message)
		}
	}
}

type Client struct {
	conn             *websocket.Conn
	receivedMessages chan Message
	hub              *Hub
	Username         string
	RoomID           string
	UserID           string
	logger           *slog.Logger
}

func (c *Client) ReadRoutine() {
	c.conn.SetReadLimit(MessageSizeLimit)
	c.conn.SetReadDeadline(time.Now().Add(DeadlineInterval))
	c.conn.SetPongHandler(func(appData string) error {
		c.conn.SetReadDeadline(time.Now().Add(DeadlineInterval))
		return nil
	})

	_, usernameBytes, err := c.conn.ReadMessage()
	if err != nil {
		c.logger.Warn("failed to read username", "error", err)
		return
	}
	username := strings.TrimSpace(string(usernameBytes))
	if username == "" || len(username) > 20 {
		c.logger.Warn("invalid username", "username", username)
		return
	}
	c.Username = username
	c.RoomID = "35e699ad-b7a2-46e2-9945-29d8c201a2de"
	c.UserID = "cae24c7c-497b-44ca-bd31-2d4a67bb6797"
	c.logger = c.logger.With("username", username)
	c.logger.Info("client connected")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	history, err := c.hub.msgRepo.GetHistory(ctx, c.RoomID, HistoryLimit)
	if err != nil {
		c.logger.Error("failed to load message history", "error", err)
	} else {
		c.SendHistory(history)
	}

	for {
		messageType, content, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				c.logger.Error("unexpected close error", "error", err)
			} else {
				c.logger.Info("client disconnected", "error", err)
			}
			break
		}

		var messageContent MessageContent
		err = json.Unmarshal(content, &messageContent)
		if err != nil {
			c.logger.Warn("failed to unmarshal message", "error", err)
			continue
		}

		messageContent.Timestamp = time.Now().UnixMilli()
		messageContent.Username = c.Username
		content, err = json.Marshal(messageContent)
		if err != nil {
			c.logger.Error("failed to marshal message", "error", err)
			continue
		}
		message := Message{
			Content:     content,
			MessageType: messageType,
			RoomID:      c.RoomID,
			UserID:      c.UserID,
			TextContent: messageContent.Text,
		}
		c.hub.broadcast <- message
	}

	c.hub.unregister <- c
	c.conn.Close()
}

func (c *Client) WriteRoutine() {
	ticker := time.NewTicker(PingInterval)
	defer ticker.Stop()
loop:
	for {
		select {
		case message, ok := <-c.receivedMessages:
			if !ok {
				break loop
			}
			c.conn.SetWriteDeadline(time.Now().Add(DeadlineInterval))
			if err := c.conn.WriteMessage(message.MessageType, message.Content); err != nil {
				c.logger.Error("failed to write message", "error", err)
				return
			}
		case _ = <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(DeadlineInterval))
			err := c.conn.WriteMessage(websocket.PingMessage, nil)
			if err != nil {
				c.logger.Error("failed to send ping", "error", err)
				return
			}
		}
	}

	c.conn.SetWriteDeadline(time.Now().Add(1 * time.Second))
	c.conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "ws closed"))
}

func (c *Client) SendHistory(messages []StoredMessage) {
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		messageContent := MessageContent{
			Username:  msg.Username,
			Text:      msg.Content,
			Timestamp: msg.CreatedAt.UnixMilli(),
		}
		content, err := json.Marshal(messageContent)
		if err != nil {
			c.logger.Error("failed to marshal history message", "error", err)
			continue
		}
		c.receivedMessages <- Message{
			Content:     content,
			MessageType: websocket.TextMessage,
		}
	}
}

func WebSocket(w http.ResponseWriter, r *http.Request, hub *Hub, upgrader websocket.Upgrader) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		http.Error(w, "Error to upgrade to websocket connection", http.StatusInternalServerError)
		return
	}

	client := Client{
		conn:             conn,
		receivedMessages: make(chan Message, ClientBufferSize),
		hub:              hub,
		logger:           slog.Default().With("component", "client"),
	}

	hub.register <- &client

	go client.ReadRoutine()
	go client.WriteRoutine()
}

func configLogger() {
	levelMap := map[string]slog.Level{
		"debug": slog.LevelDebug,
		"info":  slog.LevelInfo,
		"warn":  slog.LevelWarn,
		"error": slog.LevelError,
	}

	logLevel, ok := levelMap[strings.ToLower(os.Getenv("LOG_LEVEL"))]
	if !ok {
		logLevel = slog.LevelInfo
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	}))

	slog.SetDefault(logger)
}

func main() {

	err := godotenv.Load()
	if err != nil {
		slog.Warn("no .env file found, reading from environment")
	}

	configLogger()

	db, err := NewDB(
		os.Getenv("DB_HOST"),
		os.Getenv("DB_PORT"),
		os.Getenv("DB_USER"),
		os.Getenv("DB_PASSWORD"),
		os.Getenv("DB_NAME"),
	)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := RunMigrations(db); err != nil {
		slog.Error("failed to run migrations", "error", err)
		os.Exit(1)
	}

	slog.Info("database connected and migrations applied")

	msgRepo := NewMessageRepository(db)

	hub := NewHub(msgRepo)
	go hub.Run()

	var upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}

	router := http.NewServeMux()
	router.HandleFunc("GET /ws", func(w http.ResponseWriter, r *http.Request) {
		WebSocket(w, r, hub, upgrader)
	})
	router.HandleFunc("GET /", ServeIndex)
	if err := http.ListenAndServe(ServerAddress, router); err != nil {
		slog.Error("http server error", "error", err)
		os.Exit(1)
	}
}
