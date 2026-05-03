package response

import "time"

// InboxUser represents a user referenced in an inbox entry.
type InboxUser struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
}

// InboxLastMessage is the last message in a conversation/room.
type InboxLastMessage struct {
	Body     string `json:"body"`
	SenderID int64  `json:"sender_id"`
}

// InboxRoom is the room referenced by a conversation cursor entry.
type InboxRoom struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// InboxRes is the response body for a single inbox feed entry.
// EntryType is either "event" (join/leave) or "conversation" (unread messages).
type InboxRes struct {
	EntryType         string            `json:"entry_type"`
	Kind              string            `json:"kind,omitempty"`
	SourceUser        *InboxUser        `json:"source_user,omitempty"`
	Room              *InboxRoom        `json:"room,omitempty"`
	RefConversationID *int64            `json:"ref_conversation_id,omitempty"`
	UnreadCount       int64             `json:"unread_count,omitempty"`
	LastMessage       *InboxLastMessage `json:"last_message,omitempty"`
	CreatedAt         time.Time         `json:"created_at"`
}
