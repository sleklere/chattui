// Package user contains domain logic
package user

import (
	"context"
	"errors"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/sleklere/realtime-chat/cmd/server/internal/errs"
	dbstore "github.com/sleklere/realtime-chat/cmd/server/internal/store"
)

// Store defines the persistence methods required by the user Service.
type Store interface {
	GetUserByUsername(ctx context.Context, username string) (dbstore.User, error)
}

// Service provides user-related business logic.
type Service struct {
	store  Store
	logger *slog.Logger
}

// NewService creates a new user Service.
func NewService(s Store, l *slog.Logger) *Service {
	return &Service{store: s, logger: l}
}

// GetByUsername returns the user with the given username.
func (s *Service) GetByUsername(ctx context.Context, username string) (dbstore.User, error) {
	user, err := s.store.GetUserByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return dbstore.User{}, errs.ErrNotFound
		}
		return dbstore.User{}, err
	}

	return user, nil
}
