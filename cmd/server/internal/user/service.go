// Package user contains domain logic
package user

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/sleklere/realtime-chat/cmd/server/internal/httpx"
	dbstore "github.com/sleklere/realtime-chat/cmd/server/internal/store"
)

type Store interface {
	GetUserByUsername(ctx context.Context, username string) (dbstore.User, error)
}

type Service struct {
	store  Store
	logger *slog.Logger
}

func NewService(s Store, l *slog.Logger) *Service {
	return &Service{store: s, logger: l}
}

func (s *Service) GetByUsername(ctx context.Context, username string) (dbstore.User, error) {
	user, err := s.store.GetUserByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return dbstore.User{}, httpx.New(http.StatusNotFound, "not_found", "user not found", err)
		}
		return dbstore.User{}, err
	}

	return user, nil
}
