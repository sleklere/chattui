package conversation

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/sleklere/chattui/cmd/server/internal/bus"
	"github.com/sleklere/chattui/cmd/server/internal/db"
	"github.com/sleklere/chattui/cmd/server/internal/event"
	"github.com/sleklere/chattui/cmd/server/internal/messagestore"
	dbstore "github.com/sleklere/chattui/cmd/server/internal/store"
)

// Store defines the persistence methods required by the conversation Service.
type Store interface {
	ListConversationsByUser(ctx context.Context, params dbstore.ListConversationsByUserParams) ([]dbstore.ListConversationsByUserRow, error)
	ListMessagesByConversation(ctx context.Context, arg dbstore.ListMessagesByConversationParams) ([]dbstore.Message, error)
}

// Service provides conversation-related business logic.
type Service struct {
	store    Store
	logger   *slog.Logger
	bus      bus.Bus
	db       db.Beginner
	messages messagestore.ConversationStore
}

// NewService creates a new conversation Service.
func NewService(s Store, l *slog.Logger, b bus.Bus, d db.Beginner, options ...Option) *Service {
	svc := &Service{store: s, logger: l, bus: b, db: d}
	for _, option := range options {
		option(svc)
	}
	return svc
}

type Option func(*Service)

// WithMessageStore opts into separate message storage. PostgreSQL metadata and
// messages no longer share a transaction when this option is used.
func WithMessageStore(messages messagestore.ConversationStore) Option {
	return func(s *Service) { s.messages = messages }
}

// ListByUser returns conversations for the given user up to the specified limit.
func (s *Service) ListByUser(ctx context.Context, userID int64, limit int32) ([]dbstore.ListConversationsByUserRow, error) {
	return s.store.ListConversationsByUser(ctx, dbstore.ListConversationsByUserParams{UserID: userID, Lim: limit})
}

// ListMessages returns messages for the given conversation up to the specified limit.
func (s *Service) ListMessages(ctx context.Context, conversationID int64, limit int32) ([]dbstore.Message, error) {
	var messages messagestore.ConversationReader = s.store
	if s.messages != nil {
		messages = s.messages
	}
	return messages.ListMessagesByConversation(
		ctx,
		dbstore.ListMessagesByConversationParams{
			ConversationID: pgtype.Int8{Int64: conversationID, Valid: true},
			Limit:          limit,
		})
}

// SendDirectMessage uses one transaction in PostgreSQL mode. With separate message
// storage, it commits the conversation first; a failed message can leave an empty
// conversation. Message events are published only after message persistence.
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

	if s.messages != nil {
		// Persist cursors with the conversation so a failed first write can be
		// retried without depending on delivery of ConversationCreatedEvent.
		for _, userID := range []int64{conv.UserA, conv.UserB} {
			if err := q.UpsertInboxConversationCursor(ctx, dbstore.UpsertInboxConversationCursorParams{
				UserID: userID, RefConversationID: pgtype.Int8{Int64: conv.ID, Valid: true},
			}); err != nil {
				return dbstore.Message{}, err
			}
		}
		if err := tx.Commit(ctx); err != nil {
			return dbstore.Message{}, err
		}
		if conv.IsNew {
			s.bus.Publish(ctx, event.ConversationCreatedEvent{
				ConversationID: conv.ID, UserAID: conv.UserA, UserBID: conv.UserB,
			})
		}
		msg, err := s.messages.CreateMessage(ctx, dbstore.CreateMessageParams{
			ConversationID: pgtype.Int8{Int64: conv.ID, Valid: true}, SenderID: senderID, Body: body,
		})
		if err != nil {
			return dbstore.Message{}, err
		}
		s.bus.Publish(ctx, event.DirectMessageSentEvent{
			ConversationID: conv.ID, SenderID: senderID, RecipientID: toUserID, MessageID: msg.ID, Body: body,
		})
		return msg, nil
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
