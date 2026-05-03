package auth

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sleklere/realtime-chat/cmd/server/internal/testhelper"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	if testing.Short() {
		os.Exit(m.Run())
	}
	pool, cleanup, err := testhelper.NewPool(context.Background())
	if err != nil {
		// Docker not available — skip integration tests, unit tests still run.
		os.Exit(m.Run())
	}
	testPool = pool
	code := m.Run()
	cleanup()
	os.Exit(code)
}
