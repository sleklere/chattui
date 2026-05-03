// Package db provides database connection pooling helpers
package db

import (
	"context"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Beginner can begin a database transaction. *pgxpool.Pool satisfies this interface.
// Services that need to run multiple queries atomically depend on this instead of
// the full pool, keeping the dependency minimal and mockable in tests.
type Beginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// NewPool creates a new pgx connection pool.
func NewPool(ctx context.Context) (*pgxpool.Pool, error) {
	url := os.Getenv("DB_URL")
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, err
	}
	cfg.MaxConns = 10
	return pgxpool.NewWithConfig(ctx, cfg)
}

// Ping verifies the database connection is alive.
func Ping(ctx context.Context, p *pgxpool.Pool) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return p.Ping(ctx)
}
