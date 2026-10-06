// Package messagestore defines the message persistence boundary.
package messagestore

import (
	"context"

	dbstore "github.com/sleklere/chattui/cmd/server/internal/store"
)

type RoomStore interface {
	CreateMessage(context.Context, dbstore.CreateMessageParams) (dbstore.Message, error)
	ListMessagesByRoom(context.Context, dbstore.ListMessagesByRoomParams) ([]dbstore.ListMessagesByRoomRow, error)
}

type ConversationReader interface {
	ListMessagesByConversation(context.Context, dbstore.ListMessagesByConversationParams) ([]dbstore.Message, error)
}

type ConversationStore interface {
	ConversationReader
	CreateMessage(context.Context, dbstore.CreateMessageParams) (dbstore.Message, error)
}

type Store interface {
	RoomStore
	ConversationStore
}

var _ Store = (*dbstore.Queries)(nil)
