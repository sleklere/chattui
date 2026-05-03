// Package testhelper provides shared test infrastructure for integration tests.
package testhelper

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/sleklere/chattui/cmd/server/internal/event"
	dbstore "github.com/sleklere/chattui/cmd/server/internal/store"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

// NewPool starts a Postgres container, runs migrations, and returns a pool and a cleanup func.
func NewPool(ctx context.Context) (*pgxpool.Pool, func(), error) {
	pgc, err := tcpostgres.Run(ctx,
		"postgres:16-alpine",
		tcpostgres.WithDatabase("testdb"),
		tcpostgres.WithUsername("test"),
		tcpostgres.WithPassword("test"),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("start postgres container: %w", err)
	}

	connStr, err := pgc.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		_ = testcontainers.TerminateContainer(pgc)
		return nil, nil, fmt.Errorf("get connection string: %w", err)
	}

	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		_ = testcontainers.TerminateContainer(pgc)
		return nil, nil, fmt.Errorf("create pool: %w", err)
	}

	if err := runMigrations(pool); err != nil {
		pool.Close()
		_ = testcontainers.TerminateContainer(pgc)
		return nil, nil, fmt.Errorf("run migrations: %w", err)
	}

	cleanup := func() {
		pool.Close()
		_ = testcontainers.TerminateContainer(pgc)
	}

	return pool, cleanup, nil
}

func runMigrations(pool *pgxpool.Pool) error {
	db := stdlib.OpenDBFromPool(pool)
	defer db.Close() //nolint:errcheck

	goose.SetLogger(goose.NopLogger())
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}

	// This file is at cmd/server/internal/testhelper/testdb.go.
	// Migrations are at cmd/server/migrations/ — two levels up.
	_, thisFile, _, _ := runtime.Caller(0)
	migrationsDir := filepath.Join(filepath.Dir(thisFile), "..", "..", "migrations")

	return goose.Up(db, migrationsDir)
}

// WithTx returns a Queries backed by a transaction that rolls back when the test ends.
// Use for tests that need a clean state without explicit cleanup.
func WithTx(t *testing.T, pool *pgxpool.Pool) *dbstore.Queries {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(ctx) })
	return dbstore.New(tx)
}

// NewTx begins a transaction that rolls back when the test ends.
// Returns the raw pgx.Tx so tests can run arbitrary SQL for verification,
// and a *dbstore.Queries backed by the same transaction.
func NewTx(t *testing.T, pool *pgxpool.Pool) (pgx.Tx, *dbstore.Queries) {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(ctx) })
	return tx, dbstore.New(tx)
}

// CaptureBus is a bus.Bus implementation that records every published event.
// Subscribe is a no-op — use it when you want to call service handlers directly.
type CaptureBus struct {
	mu     sync.Mutex
	events []event.Event
}

// Publish records the event so tests can assert on it.
func (b *CaptureBus) Publish(_ context.Context, e event.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, e)
}

// Subscribe is a no-op; use EventsOfKind to inspect published events instead.
func (b *CaptureBus) Subscribe(_ string, _ func(context.Context, event.Event) error) {}

// EventsOfKind returns all captured events matching the given kind.
func (b *CaptureBus) EventsOfKind(kind string) []event.Event {
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []event.Event
	for _, e := range b.events {
		if e.Kind() == kind {
			out = append(out, e)
		}
	}
	return out
}

// DiscardLogger returns a no-op slog logger.
func DiscardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
