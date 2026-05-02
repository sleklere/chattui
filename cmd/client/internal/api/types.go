package api

import "time"

// AuthRequest represents the login or register request body.
type AuthRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// AuthResponse represents the response from login or register endpoints.
type AuthResponse struct {
	User      UserResponse `json:"user"`
	Token     string       `json:"token"`
	ExpiresAt int64        `json:"expires_at"`
}

// UserResponse represents a user in API responses.
type UserResponse struct {
	ID        int64     `json:"id"`
	Username  string    `json:"username"`
	CreatedAt time.Time `json:"created_at"`
}

// RoomResponse represents a room in API responses.
type RoomResponse struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	CreatedAt time.Time `json:"created_at"`
}

// MessageResponse represents a message in API responses.
type MessageResponse struct {
	ID             int64     `json:"id"`
	RoomID         *int64    `json:"room_id,omitempty"`
	ConversationID *int64    `json:"conversation_id,omitempty"`
	SenderID       int64     `json:"sender_id"`
	SenderUsername string    `json:"sender_username"`
	Body           string    `json:"body"`
	CreatedAt      time.Time `json:"created_at"`
}

// ConversationResponse represents a DM conversation in API responses.
type ConversationResponse struct {
	ID           int64  `json:"id"`
	PeerID       int64  `json:"peer_id"`
	PeerUsername string `json:"peer_username"`
}

// CreateRoomRequest represents the request body for creating a room.
type CreateRoomRequest struct {
	Name string `json:"name"`
}

// InboxEvent represents an inbox event from the server.
type InboxEvent struct {
	ID             int64      `json:"id"`
	Kind           string     `json:"kind"`
	RoomID         int64      `json:"room_id"`
	CreatedAt      time.Time  `json:"created_at"`
	SourceUserID   int64      `json:"source_user_id"`
	SourceUsername string     `json:"source_username"`
	ReadAt         *time.Time `json:"read_at"`
}

// Error represents a structured error response from the server.
type Error struct {
	Code    string `json:"code,omitempty"`
	Error   string `json:"error"`
	Details any    `json:"details,omitempty"`
}
