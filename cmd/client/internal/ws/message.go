package ws

import (
	"encoding/json"
	"time"
)

// WebSocket message type constants.
const (
	TypeRoomMessage   = "room_message"
	TypeDirectMessage = "direct_message"

	TypeJoinRoom  = "join_room"
	TypeLeaveRoom = "leave_room"

	TypeUserOnline  = "user_online"
	TypeUserOffline = "user_offline"
	TypeUserTyping  = "user_typing"

	TypeLoadRoomHistory  = "load_room_history"
	TypeLoadConversation = "load_conversation"

	TypePing    = "ping"
	TypePong    = "pong"
	TypeError   = "error"
	TypeSuccess = "success"

	TypeInboxUpdated = "inbox_updated"
)

// Message is the WebSocket envelope containing a type, payload, and timestamp.
type Message struct {
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload,omitempty"`
	Timestamp time.Time       `json:"timestamp"`
}

// RoomMessagePayload is the payload for room_message messages.
type RoomMessagePayload struct {
	RoomID         int64  `json:"room_id"`
	Content        string `json:"content"`
	SenderID       int64  `json:"sender_id,omitempty"`
	SenderUsername string `json:"sender_username,omitempty"`
	MessageID      int64  `json:"message_id,omitempty"`
}

// DirectMessagePayload is the payload for direct_message messages.
type DirectMessagePayload struct {
	ToUserID       int64  `json:"to_user_id"`
	Content        string `json:"content"`
	FromUserID     int64  `json:"from_user_id,omitempty"`
	FromUsername   string `json:"from_username,omitempty"`
	ConversationID int64  `json:"conversation_id,omitempty"`
	MessageID      int64  `json:"message_id,omitempty"`
}

// JoinRoomPayload is the payload for join_room and leave_room messages.
type JoinRoomPayload struct {
	RoomID int64 `json:"room_id"`
}

// UserTypingPayload is the payload for user_typing messages.
type UserTypingPayload struct {
	RoomID   *int64 `json:"room_id,omitempty"`
	ToUserID *int64 `json:"to_user_id,omitempty"`
	IsTyping bool   `json:"is_typing"`
}

// InboxUpdatedPayload is the payload for inbox_updated messages from the server.
type InboxUpdatedPayload struct {
	EntryType           string  `json:"entry_type"`
	Kind                string  `json:"kind"`
	SourceUserID        int64   `json:"source_user_id"`
	SourceUsername      string  `json:"source_username"`
	RefRoomID           *int64  `json:"room_id,omitempty"`
	RefRoomName         *string `json:"room_name,omitempty"`
	RefConversationID   *int64  `json:"conversation_id,omitempty"`
	UnreadCount         int64   `json:"unread_count"`
	LastMessageBody     *string `json:"last_message_body,omitempty"`
	LastMessageSenderID *int64  `json:"last_message_sender_id,omitempty"`
	PeerID              int64   `json:"peer_id"`
	PeerUsername        string  `json:"peer_username"`
}

// ErrorPayload is the payload for error messages from the server.
type ErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
