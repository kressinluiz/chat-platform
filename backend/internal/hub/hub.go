package hub

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/kressinluiz/chat/internal/cache"
	"github.com/kressinluiz/chat/internal/repository"
	"github.com/kressinluiz/chat/internal/ws"
)

const PingInterval = 5 * time.Second
const DeadlineInterval = 10 * time.Second
const MessageSizeLimit = 4096 //4KB
const ClientBufferSize = 256
const HistoryLimit = 50
const TypingTTL = 1000000 * time.Second

type Hub struct {
	Rooms          map[string]map[*Client]bool
	Broadcast      chan ws.Event
	register       chan *Client
	unregister     chan *Client
	Logger         *slog.Logger
	MsgRepo        repository.MessageRepo
	ClientsCounter atomic.Int64
	Cache          cache.Client
	LocalSeq       atomic.Int64
}

type Client struct {
	Conn             *websocket.Conn
	ReceivedMessages chan ws.Event
	Hub              *Hub
	Username         string
	RoomID           string
	UserID           string
	Logger           *slog.Logger
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

func NewHub(msgRepo repository.MessageRepo, redisClient cache.Client, logger *slog.Logger) (*Hub, error) {
	if redisClient == nil {
		return nil, fmt.Errorf("redis client cannot be nil")
	}
	hub := Hub{
		Rooms:      make(map[string]map[*Client]bool),
		Broadcast:  make(chan ws.Event),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		Logger:     logger,
		MsgRepo:    msgRepo,
		Cache:      redisClient,
	}
	return &hub, nil
}

func (h *Hub) Join(client *Client) {
	h.register <- client
}

func (h *Hub) Leave(client *Client) {
	h.unregister <- client
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			if _, exists := h.Rooms[client.RoomID]; !exists {
				h.Rooms[client.RoomID] = make(map[*Client]bool)
			}
			h.Rooms[client.RoomID][client] = true
			h.Logger.Info("client registered", "room", client.RoomID, "username", client.Username)
			h.ClientsCounter.Add(1)

		case client := <-h.unregister:
			if _, exists := h.Rooms[client.RoomID][client]; exists {
				close(client.ReceivedMessages)
				delete(h.Rooms[client.RoomID], client)
				h.Logger.Info("client unregistered", "room", client.RoomID, "username", client.Username)
				h.ClientsCounter.Add(-1)

				if len(h.Rooms[client.RoomID]) == 0 {
					delete(h.Rooms, client.RoomID)
					h.Logger.Info("room deleted", "room", client.RoomID)
				}
			}

		case message := <-h.Broadcast:
			if room, exists := h.Rooms[message.RoomID]; exists {
				for client := range room {
					select {
					case client.ReceivedMessages <- message:
					default:
						close(client.ReceivedMessages)
						delete(room, client)
						h.ClientsCounter.Add(-1)
						h.Logger.Warn("client dropped, buffer full", "room", client.RoomID, "username", client.Username)
						if len(room) == 0 {
							h.Logger.Warn("room deleted, buffer full", "room", client.RoomID)
							delete(h.Rooms, client.RoomID)
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

func (h *Hub) nextSeq(roomID string) int64 {
	key := fmt.Sprintf("seq:%s", roomID)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	val, err := h.Cache.Incr(ctx, key)
	if err != nil {
		return h.LocalSeq.Add(1)
	}
	return val
}

func NewClient(conn *websocket.Conn, hub *Hub, username, roomID, userID string, logger *slog.Logger) *Client {
	return &Client{
		Conn:             conn,
		ReceivedMessages: make(chan ws.Event, ClientBufferSize),
		Hub:              hub,
		Username:         username,
		RoomID:           roomID,
		UserID:           userID,
		Logger:           logger,
	}
}

func (c *Client) ReadRoutine() {
	c.Conn.SetReadLimit(MessageSizeLimit)
	if err := c.Conn.SetReadDeadline(time.Now().Add(DeadlineInterval)); err != nil {
		c.Logger.Error("failed to set read deadline", "error", err)
	}
	c.Conn.SetPongHandler(func(appData string) error {
		if err := c.Conn.SetReadDeadline(time.Now().Add(DeadlineInterval)); err != nil {
			c.Logger.Error("failed to set read deadline", "error", err)
		}
		return nil
	})

	c.Logger.Info("client connected")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	history, err := c.Hub.MsgRepo.GetHistory(ctx, c.RoomID, HistoryLimit)
	if err != nil {
		c.Logger.Error("failed to load message history", "error", err)
	} else {
		c.SendHistory(history)
	}

	for {
		_, content, err := c.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				c.Logger.Error("unexpected close error", "error", err)
			} else {
				c.Logger.Info("client disconnected", "error", err)
			}
			break
		}

		var event ws.Event
		err = json.Unmarshal(content, &event)
		if err != nil {
			c.Logger.Warn("failed to unmarshal event", "error", err)
			continue
		}

		event.RoomID = c.RoomID
		event.SentAt = time.Now().UTC().Format(time.RFC3339)
		event.Seq = c.Hub.nextSeq(c.RoomID)

		switch event.Type {
		case ws.EventTypeSendMessage:
			var payload ws.SendMessagePayload
			err = json.Unmarshal(event.Payload, &payload)
			if err != nil {
				c.Logger.Warn("failed to unmarshal message", "error", err)
				continue
			}

			content, err = json.Marshal(payload)
			outboundPayload := ws.NewMessagePayload{
				UserID:    c.UserID,
				RoomID:    c.RoomID,
				Username:  c.Username,
				Content:   payload.Content,
				ParentID:  payload.ParentID,
				CreatedAt: time.Now().UTC().Format(time.RFC3339),
			}
			data, err := json.Marshal(outboundPayload)
			if err != nil {
				c.Logger.Error("failed to marshal outbound payload", "error", err)
				continue
			}
			message := ws.Event{
				ID:      event.ID,
				Type:    ws.EventTypeNewMessage,
				RoomID:  c.RoomID,
				Seq:     event.Seq,
				Payload: data,
				SentAt:  event.SentAt,
			}
			c.Hub.Broadcast <- message
			go func(roomID, userID, content string) {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()

				if err := c.Hub.MsgRepo.Save(ctx, roomID, userID, content); err != nil {
					c.Hub.Logger.Error("failed to save message", "error", err)
				}
			}(c.RoomID, c.UserID, payload.Content)
		case ws.EventTypeDeliveryAck:
		case ws.EventTypeTypingStart, ws.EventTypeTypingStop:
			c.HandleTyping(event)
		default:
			c.Logger.Warn("unknown event type", "type", event.Type)
			continue
		}
	}

	c.Hub.Leave(c)
	if err := c.Conn.Close(); err != nil {
		c.Logger.Error("failed to close connection", "error", err)
	}
}

func (c *Client) HandleTyping(event ws.Event) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	key := fmt.Sprintf("typing:%s:%s", c.RoomID, c.UserID)

	switch event.Type {
	case ws.EventTypeTypingStart:
		if err := c.Hub.Cache.Set(ctx, key, c.Username, TypingTTL); err != nil {
			c.Logger.Error("failed to set typing key", "error", err)
			return
		}

	case ws.EventTypeTypingStop:
		if err := c.Hub.Cache.Del(ctx, key); err != nil {
			c.Logger.Error("failed to delete typing key", "error", err)
			return
		}
	}

	c.Hub.broadcastTypingUpdate(c.RoomID)
}

func (h *Hub) broadcastTypingUpdate(roomID string) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	pattern := fmt.Sprintf("typing:%s:*", roomID)
	keys, err := h.Cache.Keys(ctx, pattern)
	if err != nil {
		h.Logger.Error("failed to fetch typing keys", "error", err)
		return
	}

	usernames := make([]string, 0, len(keys))
	for _, key := range keys {
		username, err := h.Cache.Get(ctx, key)
		if err != nil {
			continue
		}
		usernames = append(usernames, username)
	}

	payload, err := json.Marshal(ws.TypingUpdatePayload{TypingUsers: usernames})
	if err != nil {
		h.Logger.Error("failed to marshal typing update", "error", err)
		return
	}

	h.Broadcast <- ws.Event{
		Type:    ws.EventTypeTypingUpdate,
		RoomID:  roomID,
		Payload: payload,
		SentAt:  time.Now().UTC().Format(time.RFC3339),
	}
}

func (c *Client) WriteRoutine() {
	ticker := time.NewTicker(PingInterval)
	defer ticker.Stop()
loop:
	for {
		select {
		case message, ok := <-c.ReceivedMessages:
			if !ok {
				break loop
			}
			if err := c.Conn.SetWriteDeadline(time.Now().Add(DeadlineInterval)); err != nil {
				c.Logger.Error("failed to set write deadline", "error", err)
				return
			}

			data, err := json.Marshal(message)
			if err != nil {
				c.Logger.Error("failed to marshal event", "error", err)
				return
			}
			if err := c.Conn.WriteMessage(websocket.TextMessage, data); err != nil {
				c.Logger.Error("failed to write message", "error", err)
				return
			}
		case <-ticker.C:
			if err := c.Conn.SetWriteDeadline(time.Now().Add(DeadlineInterval)); err != nil {
				c.Logger.Error("failed to set write deadline", "error", err)
				return
			}
			err := c.Conn.WriteMessage(websocket.PingMessage, nil)
			if err != nil {
				c.Logger.Error("failed to send ping", "error", err)
				return
			}
		}
	}

	if err := c.Conn.SetWriteDeadline(time.Now().Add(1 * time.Second)); err != nil {
		c.Logger.Error("failed to set write deadline", "error", err)
	}

	if err := c.Conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "ws closed")); err != nil {
		c.Logger.Error("failed to write close message", "error", err)
	}

}

func (c *Client) SendHistory(messages []repository.StoredMessage) {
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		messageContent := ws.NewMessagePayload{
			Username:  msg.Username,
			Content:   msg.Content,
			CreatedAt: msg.CreatedAt.Format(time.RFC3339),
		}
		content, err := json.Marshal(messageContent)
		if err != nil {
			c.Logger.Error("failed to marshal history message", "error", err)
			continue
		}
		c.ReceivedMessages <- ws.Event{
			ID:      "", // need to fix this later
			Seq:     0,  // need to fix this later
			RoomID:  c.RoomID,
			Type:    ws.EventTypeNewMessage,
			Payload: content,
			SentAt:  time.Now().Format(time.RFC3339),
		}
	}
}
