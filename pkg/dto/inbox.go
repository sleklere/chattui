package dto

import "time"

// InboxUser is a user referenced in an inbox entry.
type InboxUser struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
}

// InboxRoom is a room referenced in an inbox entry.
type InboxRoom struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// InboxLastMessage is the last message in a conversation or room cursor entry.
type InboxLastMessage struct {
	Body     string `json:"body"`
	SenderID int64  `json:"sender_id"`
}

// InboxFeed is a single inbox feed entry.
// EntryType is "event" (join/leave) or "conversation" (message cursor).
type InboxFeed struct {
	EntryType         string            `json:"entry_type"`
	Kind              string            `json:"kind,omitempty"`
	SourceUser        *InboxUser        `json:"source_user,omitempty"`
	Room              *InboxRoom        `json:"room,omitempty"`
	RefConversationID *int64            `json:"ref_conversation_id,omitempty"`
	UnreadCount       int64             `json:"unread_count,omitempty"`
	LastMessage       *InboxLastMessage `json:"last_message,omitempty"`
	CreatedAt         time.Time         `json:"created_at"`
}
