package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

func ServeIndex(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "../frontend/index.html")
}

const PingInterval = 5 * time.Second
const DeadlineInterval = 10 * time.Second
const MessageSizeLimit = 4096 //4KB
const ClientBufferSize = 256  //256 messages
const ServerAddress = "localhost:8080"

type Hub struct {
	clients    map[*Client]bool
	broadcast  chan Message
	register   chan *Client
	unregister chan *Client
}

type Message struct {
	Content     []byte
	MessageType int
}

type MessageContent struct {
	Username  string `json:"username"`
	Text      string `json:"text"`
	Timestamp int64  `json:"timestamp"`
}

func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		broadcast:  make(chan Message),
		register:   make(chan *Client),
		unregister: make(chan *Client),
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
		}
	}
}

type Client struct {
	conn             *websocket.Conn
	receivedMessages chan Message
	hub              *Hub
	Username         string
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
		log.Println(err)
		return
	}
	username := strings.TrimSpace(string(usernameBytes))
	if username == "" || len(username) > 20 {
		log.Println("Username cannot be empty")
		return
	}
	c.Username = username

	for {
		messageType, content, err := c.conn.ReadMessage()
		if err != nil {
			log.Println(err)
			break
		}

		var messageContent MessageContent
		err = json.Unmarshal(content, &messageContent)
		if err != nil {
			log.Println(err)
			continue
		}

		messageContent.Timestamp = time.Now().UnixMilli()
		messageContent.Username = c.Username
		content, err = json.Marshal(messageContent)
		if err != nil {
			log.Println(err)
			continue
		}
		message := Message{
			Content:     content,
			MessageType: messageType,
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
				log.Println(err)
				return
			}
		case _ = <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(DeadlineInterval))
			err := c.conn.WriteMessage(websocket.PingMessage, nil)
			if err != nil {
				log.Println(err)
				return
			}
		}
	}

	c.conn.SetWriteDeadline(time.Now().Add(1 * time.Second))
	c.conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "ws closed"))
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
	}

	hub.register <- &client

	go client.ReadRoutine()
	go client.WriteRoutine()
}

func main() {
	hub := NewHub()
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
		log.Fatalf("HTTP Server Error: %s", err.Error())
	}
}
