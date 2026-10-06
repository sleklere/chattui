// Package messagestore defines the message persistence boundary.
package messagestore

import (
	"context"

	dbstore "github.com/sleklere/chattui/cmd/server/internal/store"
)

// RoomStore persists messages and lists history for a room.
type RoomStore interface {
	CreateMessage(context.Context, dbstore.CreateMessageParams) (dbstore.Message, error)
	ListMessagesByRoom(context.Context, dbstore.ListMessagesByRoomParams) ([]dbstore.ListMessagesByRoomRow, error)
}

// ConversationReader lists the history of an existing conversation.
type ConversationReader interface {
	ListMessagesByConversation(context.Context, dbstore.ListMessagesByConversationParams) ([]dbstore.Message, error)
}

// ConversationStore persists DM messages after conversation metadata is committed.
type ConversationStore interface {
	ConversationReader
	CreateMessage(context.Context, dbstore.CreateMessageParams) (dbstore.Message, error)
}

// Store supplies message persistence and history for rooms and conversations.
type Store interface {
	RoomStore
	ConversationStore
}

var _ Store = (*dbstore.Queries)(nil)
