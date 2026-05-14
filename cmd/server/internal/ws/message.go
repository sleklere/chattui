// Package ws handles the WebSocket connections for the real-time chat.
package ws

import (
	"encoding/json"
	"time"
)

// WebSocket message types.
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

// Message is the envelope for all WebSocket messages.
// Type determines which payload struct to unmarshal into.
type Message struct {
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload,omitempty"`
	Timestamp time.Time       `json:"timestamp"`
}

// Type returns the WebSocket message type constant for each payload. //nolint:revive
func (p RoomMessagePayload) Type() string   { return TypeRoomMessage }     //nolint:revive
func (p DirectMessagePayload) Type() string { return TypeDirectMessage }   //nolint:revive
func (p RoomPresencePayload) Type() string  { return TypeJoinRoom }        //nolint:revive
func (p UserTypingPayload) Type() string    { return TypeUserTyping }      //nolint:revive
func (p LoadHistoryPayload) Type() string   { return TypeLoadRoomHistory } //nolint:revive
func (p ErrorPayload) Type() string         { return TypeError }           //nolint:revive

// RoomMessagePayload is the payload for room messages.
type RoomMessagePayload struct {
	RoomID  int64  `json:"room_id"`
	Content string `json:"content"`
	// fields populated by the server before broadcast
	SenderID       int64  `json:"sender_id,omitempty"`
	SenderUsername string `json:"sender_username,omitempty"`
	MessageID      int64  `json:"message_id,omitempty"`
}

// DirectMessagePayload is the payload for direct (1-to-1) messages.
type DirectMessagePayload struct {
	ToUserID int64  `json:"to_user_id"`
	Content  string `json:"content"`
	// fields populated by the server before broadcast
	SenderID       int64  `json:"from_user_id,omitempty"`
	SenderUsername string `json:"from_username,omitempty"`
	ConversationID int64  `json:"conversation_id,omitempty"`
	MessageID      int64  `json:"message_id,omitempty"`
}

// RoomPresencePayload is the payload for join/leave room events.
type RoomPresencePayload struct {
	RoomID int64 `json:"room_id"`
}

// UserTypingPayload is the payload for typing indicator events.
type UserTypingPayload struct {
	RoomID   *int64 `json:"room_id,omitempty"`
	ToUserID *int64 `json:"to_user_id,omitempty"`
	IsTyping bool   `json:"is_typing"`
}

// LoadHistoryPayload is the payload for requesting message history.
type LoadHistoryPayload struct {
	RoomID         *int64 `json:"room_id,omitempty"`
	ConversationID *int64 `json:"conversation_id,omitempty"`
	Limit          int    `json:"limit"`
	BeforeID       *int64 `json:"before_id,omitempty"`
}

// InboxUpdatedPayload is sent to connected clients when their inbox entry changes.
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

// Type returns the WebSocket message type constant for InboxUpdatedPayload.
func (p InboxUpdatedPayload) Type() string {
	return TypeInboxUpdated
}

// ErrorPayload is the payload for error responses sent to the client.
type ErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
