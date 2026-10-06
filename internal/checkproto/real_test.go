package checkproto

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sleklere/chattui/internal/checkenv"
)

// Explicit opt-in; each invocation owns disposable, labeled Docker resources.
// Logs, configuration, snapshots and traces remain under /tmp/opencode.
func TestRealProtocol(t *testing.T) {
	if os.Getenv("CHECKPROTO_REAL") != "1" {
		t.Skip("set CHECKPROTO_REAL=1 to run isolated protocol E2E")
	}
	backend := checkenv.Backend(os.Getenv("CHECKPROTO_BACKEND"))
	if backend != checkenv.Postgres && backend != checkenv.Badger {
		t.Fatal("set CHECKPROTO_BACKEND=postgres or badger")
	}
	cwd, e := os.Getwd()
	if e != nil {
		t.Fatal(e)
	}
	repo := filepath.Clean(filepath.Join(cwd, "../.."))
	results, e := os.MkdirTemp("/tmp/opencode", "checkproto-real-")
	if e != nil {
		t.Fatal(e)
	}
	t.Logf("retained results: %s", results)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	env, e := checkenv.Start(ctx, checkenv.Config{Repository: repo, ResultsDir: filepath.Join(results, "environment"), Backend: backend})
	if e != nil {
		t.Fatal(e)
	}
	defer func() {
		if e := env.Close(context.Background()); e != nil {
			t.Error(e)
		}
	}()
	_, e = RunE2E(ctx, E2EConfig{HTTPURL: env.HTTPURL, WSURL: env.WSURL, Window: 500 * time.Millisecond, TracePath: filepath.Join(results, "e2e.json"), Lifecycle: Lifecycle{Stop: env.StopServer, Start: env.StartServer}})
	if e != nil {
		t.Fatal(e)
	}
}
