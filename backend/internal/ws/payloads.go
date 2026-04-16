package ws

// // The message types are defined in RFC 6455, section 11.8.
// const (
// 	// TextMessage denotes a text data message. The text message payload is
// 	// interpreted as UTF-8 encoded text data.
// 	TextMessage = 1

// 	// BinaryMessage denotes a binary data message.
// 	BinaryMessage = 2

// 	// CloseMessage denotes a close control message. The optional message
// 	// payload contains a numeric code and text. Use the FormatCloseMessage
// 	// function to format a close message payload.
// 	CloseMessage = 8

// 	// PingMessage denotes a ping control message. The optional message payload
// 	// is UTF-8 encoded text.
// 	PingMessage = 9

// 	// PongMessage denotes a pong control message. The optional message payload
// 	// is UTF-8 encoded text.
// 	PongMessage = 10
// )

type SendMessagePayload struct {
	Content   string `json:"content"`
	ParentID  string `json:"parent_id,omitempty"`
	FileToken string `json:"file_token,omitempty"`
}

type DeliveryAckPayload struct {
	Seq int64 `json:"seq"`
}

type NewMessagePayload struct {
	MessageID string `json:"message_id"`
	UserID    string `json:"user_id"`
	RoomID    string `json:"room_id"`
	Username  string `json:"username"`
	Content   string `json:"content"`
	ParentID  string `json:"parent_id,omitempty"`
	CreatedAt string `json:"created_at"`
}

type TypingUpdatePayload struct {
	TypingUsers []string `json:"typing_users"`
}

type ErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ReactionPayload struct {
	MessageID string `json:"message_id"`
	Emoji     string `json:"emoji"`
}

type ReactionUpdatePayload struct {
	MessageID string              `json:"message_id"`
	Reactions map[string][]string `json:"reactions"` // map[emoji][]userID
}
