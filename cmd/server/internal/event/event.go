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
