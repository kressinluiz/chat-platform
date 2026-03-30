package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

const PingInterval = 5 * time.Second
const DeadlineInterval = 10 * time.Second
const MessageSizeLimit = 4096 //4KB
const ClientBufferSize = 256
const HistoryLimit = 50

type Hub struct {
	rooms          map[string]map[*Client]bool
	broadcast      chan Message
	register       chan *Client
	unregister     chan *Client
	logger         *slog.Logger
	msgRepo        *MessageRepository
	ClientsCounter atomic.Int64
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

func StartHub(msgRepo *MessageRepository) *Hub {
	hub := Hub{
		rooms:      make(map[string]map[*Client]bool),
		broadcast:  make(chan Message),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		logger:     slog.Default().With("component", "hub"),
		msgRepo:    msgRepo,
	}
	go hub.Run()
	return &hub
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			if _, exists := h.rooms[client.RoomID]; !exists {
				h.rooms[client.RoomID] = make(map[*Client]bool)
			}
			h.rooms[client.RoomID][client] = true
			h.logger.Info("client registered", "room", client.RoomID, "username", client.Username)
			h.ClientsCounter.Add(1)

		case client := <-h.unregister:
			if _, exists := h.rooms[client.RoomID][client]; exists {
				close(client.receivedMessages)
				delete(h.rooms[client.RoomID], client)
				h.logger.Info("client unregistered", "room", client.RoomID, "username", client.Username)
				h.ClientsCounter.Add(-1)

				if len(h.rooms[client.RoomID]) == 0 {
					delete(h.rooms, client.RoomID)
					h.logger.Info("room deleted", "room", client.RoomID)
				}
			}

		case message := <-h.broadcast:
			if room, exists := h.rooms[message.RoomID]; exists {
				for client := range room {
					select {
					case client.receivedMessages <- message:
					default:
						close(client.receivedMessages)
						delete(room, client)
						h.ClientsCounter.Add(-1)
						h.logger.Warn("client dropped, buffer full", "room", client.RoomID, "username", client.Username)
						if len(room) == 0 {
							h.logger.Warn("room deleted, buffer full", "room", client.RoomID)
							delete(h.rooms, client.RoomID)
						}
					}
				}
				messagesTotal.WithLabelValues(message.RoomID).Inc()
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

func (h *Hub) ConnectedClients() int {
	return int(h.ClientsCounter.Load())
}

func (c *Client) ReadRoutine() {
	c.conn.SetReadLimit(MessageSizeLimit)
	c.conn.SetReadDeadline(time.Now().Add(DeadlineInterval))
	c.conn.SetPongHandler(func(appData string) error {
		c.conn.SetReadDeadline(time.Now().Add(DeadlineInterval))
		return nil
	})

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
