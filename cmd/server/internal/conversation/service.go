package conversation

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgtype"
	dbstore "github.com/sleklere/realtime-chat/cmd/server/internal/store"
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
}

// NewService creates a new conversation Service.
func NewService(s Store, l *slog.Logger) *Service {
	return &Service{store: s, logger: l}
}

// ListByUser returns conversations for the given user up to the specified limit.
func (s *Service) ListByUser(ctx context.Context, userID int64, limit int32) ([]dbstore.ListConversationsByUserRow, error) {
	return s.store.ListConversationsByUser(ctx, dbstore.ListConversationsByUserParams{UserID: userID, Lim: limit})
}

// ListMessages returns messages for the given conversation up to the specified limit.
func (s *Service) ListMessages(ctx context.Context,
	conversationID int64,
	limit int32) ([]dbstore.Message, error) {

	return s.store.ListMessagesByConversation(
		ctx,
		dbstore.ListMessagesByConversationParams{
			ConversationID: pgtype.Int8{Int64: conversationID, Valid: true},
			Limit:          limit,
		})
}
