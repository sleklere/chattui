package checktui

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sleklere/chattui/internal/checkenv"
)

func TestRealTwoClients(t *testing.T) {
	if os.Getenv("CHECKTUI_REAL") != "1" {
		t.Skip("set CHECKTUI_REAL=1 with CHECKTUI_CLIENT_BINARY to run Docker and two PTYs")
	}
	binary := os.Getenv("CHECKTUI_CLIENT_BINARY")
	if binary == "" {
		t.Fatal("CHECKTUI_CLIENT_BINARY required")
	}
	repo, e := filepath.Abs("../..")
	if e != nil {
		t.Fatal(e)
	}
	results, e := os.MkdirTemp("/tmp/opencode", "checktui-real-")
	if e != nil {
		t.Fatal(e)
	}
	t.Logf("retained diagnostics: %s", results)
	backend := checkenv.Badger
	if os.Getenv("CHECKTUI_BACKEND") == "postgres" {
		backend = checkenv.Postgres
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()
	env, e := checkenv.Start(ctx, checkenv.Config{Repository: repo, ResultsDir: results, Backend: backend})
	if e != nil {
		t.Fatal(e)
	}
	defer func() {
		if e := env.Close(context.Background()); e != nil {
			t.Errorf("environment close: %v", e)
		}
	}()
	runDir := filepath.Join(results, "tui")
	if e = os.Mkdir(runDir, 0700); e != nil {
		t.Fatal(e)
	}
	result, e := Run(ctx, Config{ClientBinary: binary, HTTPURL: env.HTTPURL, WSURL: env.WSURL, Seed: "two-client", ResultsDir: runDir, Timeout: 60 * time.Second, LifecycleStop: env.StopServer, LifecycleStart: env.StartServer})
	_, _ = env.Collect()
	for _, step := range result.Steps {
		t.Logf("step %s: %dms", step.Name, step.DurationMS)
	}
	if e != nil {
		t.Fatal(e)
	}
}
