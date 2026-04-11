package conversation

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgtype"
	dbstore "github.com/sleklere/realtime-chat/cmd/server/internal/store"
)

type Store interface {
	ListConversationsByUser(ctx context.Context, params dbstore.ListConversationsByUserParams) ([]dbstore.ListConversationsByUserRow, error)
	ListMessagesByConversation(ctx context.Context, arg dbstore.ListMessagesByConversationParams) ([]dbstore.Message, error)
}

type Service struct {
	store  Store
	logger *slog.Logger
}

func NewService(s Store, l *slog.Logger) *Service {
	return &Service{store: s, logger: l}
}

func (s *Service) ListByUser(ctx context.Context, userID int64, limit int32) ([]dbstore.ListConversationsByUserRow, error) {
	return s.store.ListConversationsByUser(ctx, dbstore.ListConversationsByUserParams{UserID: userID, Lim: limit})
}

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
