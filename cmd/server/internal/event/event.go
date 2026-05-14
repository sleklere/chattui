package event

// Event is implemented by all domain events published to the bus.
type Event interface {
	Kind() string
}

// RoomJoinedEvent is published when a user successfully joins a room.
type RoomJoinedEvent struct {
	UserID int64
	RoomID int64
}

// Kind returns the event kind identifier.
func (e RoomJoinedEvent) Kind() string {
	return "room_join"
}

// RoomLeftEvent is published when a user leaves a room.
type RoomLeftEvent struct {
	UserID int64
	RoomID int64
}

// Kind returns the event kind identifier.
func (e RoomLeftEvent) Kind() string {
	return "room_leave"
}

// RoomMessageSentEvent is published when a message is sent in a room.
type RoomMessageSentEvent struct {
	RoomID    int64
	SenderID  int64
	MessageID int64
	Body      string
}

// Kind returns the event kind identifier.
func (e RoomMessageSentEvent) Kind() string {
	return "room_message"
}

// DirectMessageSentEvent is published when a direct message is sent.
type DirectMessageSentEvent struct {
	ConversationID int64
	SenderID       int64
	RecipientID    int64
	MessageID      int64
	Body           string
}

// Kind returns the event kind identifier.
func (e DirectMessageSentEvent) Kind() string {
	return "direct_message"
}

// ConversationCreatedEvent is published the first time a DM conversation is created.
type ConversationCreatedEvent struct {
	ConversationID int64
	UserAID        int64
	UserBID        int64
}

// Kind returns the event kind identifier.
func (e ConversationCreatedEvent) Kind() string {
	return "conversation_created"
}

// InboxEntryUpdatedEvent is published after an inbox cursor is updated.
// Carries the denormalized entry data so consumers can push it without a DB round-trip.
type InboxEntryUpdatedEvent struct {
	UserID              int64
	EntryType           string
	EntryKind           string
	SourceUserID        int64
	SourceUsername      string
	RefRoomID           *int64
	RefRoomName         *string
	RefConversationID   *int64
	UnreadCount         int64
	LastMessageBody     *string
	LastMessageSenderID *int64
	PeerID              int64
	PeerUsername        string
}

// Kind implements the Event interface.
func (e InboxEntryUpdatedEvent) Kind() string {
	return "inbox_entry_updated"
}
