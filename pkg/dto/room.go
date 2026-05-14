package dto

import "time"

// Room represents a chat room in API responses.
type Room struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	CreatedAt time.Time `json:"created_at"`
}

// Message represents a chat message in API responses.
// RoomID and ConversationID are mutually exclusive.
type Message struct {
	ID             int64     `json:"id"`
	RoomID         *int64    `json:"room_id,omitempty"`
	ConversationID *int64    `json:"conversation_id,omitempty"`
	SenderID       int64     `json:"sender_id"`
	SenderUsername string    `json:"sender_username,omitempty"`
	Body           string    `json:"body"`
	CreatedAt      time.Time `json:"created_at"`
}
