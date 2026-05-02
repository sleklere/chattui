package conversation

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/sleklere/realtime-chat/cmd/server/internal/bus"
	"github.com/sleklere/realtime-chat/cmd/server/internal/event"
	dbstore "github.com/sleklere/realtime-chat/cmd/server/internal/store"
)

// Store defines the persistence methods required by the conversation Service.
type Store interface {
	ListConversationsByUser(ctx context.Context, params dbstore.ListConversationsByUserParams) ([]dbstore.ListConversationsByUserRow, error)
	ListMessagesByConversation(ctx context.Context, arg dbstore.ListMessagesByConversationParams) ([]dbstore.Message, error)
	GetOrCreateConversation(ctx context.Context, arg dbstore.GetOrCreateConversationParams) (dbstore.GetOrCreateConversationRow, error)
	CreateMessage(ctx context.Context, arg dbstore.CreateMessageParams) (dbstore.Message, error)
}

// Service provides conversation-related business logic.
type Service struct {
	store  Store
	logger *slog.Logger
	bus    bus.Bus
}

// NewService creates a new conversation Service.
func NewService(s Store, l *slog.Logger, b bus.Bus) *Service {
	return &Service{store: s, logger: l, bus: b}
}

// ListByUser returns conversations for the given user up to the specified limit.
func (s *Service) ListByUser(ctx context.Context, userID int64, limit int32) ([]dbstore.ListConversationsByUserRow, error) {
	return s.store.ListConversationsByUser(ctx, dbstore.ListConversationsByUserParams{UserID: userID, Lim: limit})
}

// ListMessages returns messages for the given conversation up to the specified limit.
func (s *Service) ListMessages(ctx context.Context, conversationID int64, limit int32) ([]dbstore.Message, error) {
	return s.store.ListMessagesByConversation(
		ctx,
		dbstore.ListMessagesByConversationParams{
			ConversationID: pgtype.Int8{Int64: conversationID, Valid: true},
			Limit:          limit,
		})
}

// SendDirectMessage persists a direct message. Publishes ConversationCreatedEvent if the conversation is new.
func (s *Service) SendDirectMessage(ctx context.Context, senderID int64, toUserID int64, body string) (dbstore.Message, error) {
	conv, err := s.store.GetOrCreateConversation(ctx, dbstore.GetOrCreateConversationParams{
		UserA: senderID,
		UserB: toUserID,
	})
	if err != nil {
		return dbstore.Message{}, err
	}

	msg, err := s.store.CreateMessage(ctx, dbstore.CreateMessageParams{
		ConversationID: pgtype.Int8{Int64: conv.ID, Valid: true},
		SenderID:       senderID,
		Body:           body,
	})
	if err != nil {
		return dbstore.Message{}, err
	}

	if conv.IsNew {
		s.bus.Publish(ctx, event.ConversationCreatedEvent{
			ConversationID: conv.ID,
			UserAID:        conv.UserA,
			UserBID:        conv.UserB,
		})
	}

	return msg, nil
}
