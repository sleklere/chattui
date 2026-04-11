package response

import "time"

// RoomRes is the response body for a room.
type RoomRes struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	CreatedAt time.Time `json:"created_at"`
}

// MessageRes is the base response body for a message.
type MessageRes struct {
	ID        int64     `json:"id"`
	SenderID  int64     `json:"sender_id"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

// RoomMessageRes is the response body for a room message.
type RoomMessageRes struct {
	MessageRes
	RoomID         int64  `json:"room_id"`
	SenderUsername string `json:"sender_username"`
}

// ConversationMessageRes is the response body for a direct message.
type ConversationMessageRes struct {
	MessageRes
	ConversationID int64 `json:"conversation_id"`
}
