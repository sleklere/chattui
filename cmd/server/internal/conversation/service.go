package conversation

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/sleklere/chattui/cmd/server/internal/bus"
	"github.com/sleklere/chattui/cmd/server/internal/db"
	"github.com/sleklere/chattui/cmd/server/internal/event"
	dbstore "github.com/sleklere/chattui/cmd/server/internal/store"
)

// Store defines the persistence methods required by the conversation Service.
type Store interface {
	ListConversationsByUser(ctx context.Context, params dbstore.ListConversationsByUserParams) ([]dbstore.ListConversationsByUserRow, error)
	ListMessagesByConversation(ctx context.Context, arg dbstore.ListMessagesByConversationParams) ([]dbstore.Message, error)
}

// Service provides conversation-related business logic.
type Service struct {
	store  Store
	logger *slog.Logger
	bus    bus.Bus
	db     db.Beginner
}

// NewService creates a new conversation Service.
func NewService(s Store, l *slog.Logger, b bus.Bus, d db.Beginner) *Service {
	return &Service{store: s, logger: l, bus: b, db: d}
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

// SendDirectMessage persists a direct message inside a transaction so a CreateMessage
// failure never leaves an orphaned empty conversation. Publishes ConversationCreatedEvent
// if the conversation is new. Events are published post-commit.
func (s *Service) SendDirectMessage(ctx context.Context, senderID int64, toUserID int64, body string) (dbstore.Message, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return dbstore.Message{}, err
	}
	// Rollback is a no-op if Commit already succeeded; the error is intentionally ignored.
	defer tx.Rollback(ctx) //nolint:errcheck

	q := dbstore.New(tx)
	conv, err := q.GetOrCreateConversation(ctx, dbstore.GetOrCreateConversationParams{
		UserA: senderID,
		UserB: toUserID,
	})
	if err != nil {
		return dbstore.Message{}, err
	}

	msg, err := q.CreateMessage(ctx, dbstore.CreateMessageParams{
		ConversationID: pgtype.Int8{Int64: conv.ID, Valid: true},
		SenderID:       senderID,
		Body:           body,
	})
	if err != nil {
		return dbstore.Message{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return dbstore.Message{}, err
	}

	if conv.IsNew {
		s.bus.Publish(ctx, event.ConversationCreatedEvent{
			ConversationID: conv.ID,
			UserAID:        conv.UserA,
			UserBID:        conv.UserB,
		})
	}

	s.bus.Publish(ctx, event.DirectMessageSentEvent{
		ConversationID: conv.ID,
		SenderID:       senderID,
		RecipientID:    toUserID,
		MessageID:      msg.ID,
		Body:           body,
	})

	return msg, nil
}
