package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/kressinluiz/chat/internal/cache"
	"github.com/kressinluiz/chat/internal/ws"
)

const PingInterval = 5 * time.Second
const DeadlineInterval = 10 * time.Second
const MessageSizeLimit = 4096 //4KB
const ClientBufferSize = 256
const HistoryLimit = 50

type Hub struct {
	rooms          map[string]map[*Client]bool
	broadcast      chan ws.Event
	register       chan *Client
	unregister     chan *Client
	logger         *slog.Logger
	msgRepo        MessageRepo
	ClientsCounter atomic.Int64
	cache          cache.Client
	localSeq       atomic.Int64
}

type Client struct {
	conn             *websocket.Conn
	receivedMessages chan ws.Event
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

func StartHub(msgRepo MessageRepo, redisClient cache.Client) *Hub {
	hub := Hub{
		rooms:      make(map[string]map[*Client]bool),
		broadcast:  make(chan ws.Event),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		logger:     slog.Default().With("component", "hub"),
		msgRepo:    msgRepo,
		cache:      redisClient,
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
				// messagesTotal.WithLabelValues(message.RoomID).Inc()
			}
		}
	}
}

func (h *Hub) ConnectedClients() int {
	return int(h.ClientsCounter.Load())
}

func (c *Client) ReadRoutine() {
	c.conn.SetReadLimit(MessageSizeLimit)
	if err := c.conn.SetReadDeadline(time.Now().Add(DeadlineInterval)); err != nil {
		c.logger.Error("failed to set read deadline", "error", err)
	}
	c.conn.SetPongHandler(func(appData string) error {
		if err := c.conn.SetReadDeadline(time.Now().Add(DeadlineInterval)); err != nil {
			c.logger.Error("failed to set read deadline", "error", err)
		}
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
		_, content, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				c.logger.Error("unexpected close error", "error", err)
			} else {
				c.logger.Info("client disconnected", "error", err)
			}
			break
		}

		var event ws.Event
		err = json.Unmarshal(content, &event)
		if err != nil {
			c.logger.Warn("failed to unmarshal event", "error", err)
			continue
		}

		event.RoomID = c.RoomID
		event.SentAt = time.Now().UTC().Format(time.RFC3339)
		event.Seq = c.hub.nextSeq(c.RoomID)

		switch event.Type {
		case ws.EventTypeSendMessage:
			var payload ws.NewMessagePayload
			err = json.Unmarshal(event.Payload, &payload)
			if err != nil {
				c.logger.Warn("failed to unmarshal message", "error", err)
				continue
			}

			payload.Username = c.Username
			payload.UserID = c.UserID
			payload.RoomID = c.RoomID
			payload.CreatedAt = time.Now().UTC().Format(time.RFC3339)
			content, err = json.Marshal(payload)
			if err != nil {
				c.logger.Error("failed to marshal message", "error", err)
				continue
			}
			message := ws.Event{
				ID:      event.ID,
				Type:    ws.EventTypeNewMessage,
				RoomID:  c.RoomID,
				Seq:     event.Seq,
				Payload: content,
				SentAt:  event.SentAt,
			}
			c.hub.broadcast <- message
			go func(msg ws.NewMessagePayload) {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()

				if err := c.hub.msgRepo.Save(ctx, msg.RoomID, msg.UserID, msg.Content); err != nil {
					c.hub.logger.Error("failed to save message", "error", err)
				}
			}(payload)
		case ws.EventTypeDeliveryAck:
			// handle delivery ack event
		case ws.EventTypeTypingStart, ws.EventTypeTypingStop:
			// handle typing events
		default:
			c.logger.Warn("unknown event type", "type", event.Type)
			continue
		}
	}

	c.hub.unregister <- c
	if err := c.conn.Close(); err != nil {
		c.logger.Error("failed to close connection", "error", err)
	}
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
			if err := c.conn.SetWriteDeadline(time.Now().Add(DeadlineInterval)); err != nil {
				c.logger.Error("failed to set write deadline", "error", err)
				return
			}

			if err := c.conn.WriteMessage(1, message.Payload); err != nil {
				c.logger.Error("failed to write message", "error", err)
				return
			}
		case <-ticker.C:
			if err := c.conn.SetWriteDeadline(time.Now().Add(DeadlineInterval)); err != nil {
				c.logger.Error("failed to set write deadline", "error", err)
				return
			}
			err := c.conn.WriteMessage(websocket.PingMessage, nil)
			if err != nil {
				c.logger.Error("failed to send ping", "error", err)
				return
			}
		}
	}

	if err := c.conn.SetWriteDeadline(time.Now().Add(1 * time.Second)); err != nil {
		c.logger.Error("failed to set write deadline", "error", err)
	}

	if err := c.conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "ws closed")); err != nil {
		c.logger.Error("failed to write close message", "error", err)
	}

}

func (c *Client) SendHistory(messages []StoredMessage) {
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		messageContent := ws.NewMessagePayload{
			Username:  msg.Username,
			Content:   msg.Content,
			CreatedAt: msg.CreatedAt.Format(time.RFC3339),
		}
		content, err := json.Marshal(messageContent)
		if err != nil {
			c.logger.Error("failed to marshal history message", "error", err)
			continue
		}
		c.receivedMessages <- ws.Event{
			ID:      "", // need to fix this later
			Seq:     0,  // need to fix this later
			RoomID:  c.RoomID,
			Type:    ws.EventTypeNewMessage,
			Payload: content,
			SentAt:  time.Now().Format(time.RFC3339),
		}
	}
}

func (h *Hub) nextSeq(roomID string) int64 {
	key := fmt.Sprintf("seq:%s", roomID)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	val, err := h.cache.Incr(ctx, key)
	if err != nil {
		return h.localSeq.Add(1)
	}
	return val
}
