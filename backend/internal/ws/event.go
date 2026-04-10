package ws

import "encoding/json"

type EventType string

const (
	// client -> server
	EventTypeSendMessage EventType = "message.send"
	EventTypeDeliveryAck EventType = "delivery.ack"
	EventTypeTypingStart EventType = "typing.start"
	EventTypeTypingStop  EventType = "typing.stop"

	// server -> client
	EventTypeNewMessage     EventType = "message.new"
	EventTypeReaction       EventType = "message.reaction"
	EventTypeThreadReply    EventType = "thread.reply"
	EventTypeTypingUpdate   EventType = "typing.update"
	EventTypePresenceUpdate EventType = "presence.update"
	EventTypeError          EventType = "error"
	EventTypeRateLimited    EventType = "rate_limit.exceeded"
)

type Event struct {
	ID      string          `json:"id"`
	Type    EventType       `json:"type"`
	RoomID  string          `json:"room_id"`
	Seq     int64           `json:"seq"`
	Payload json.RawMessage `json:"payload"`
	SentAt  string          `json:"sent_at"`
}
