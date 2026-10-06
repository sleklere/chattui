package checkproto

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sleklere/chattui/internal/checkenv"
)

func TestRealLoadProfiles(t *testing.T) {
	if os.Getenv("CHECKPROTO_LOAD_REAL") != "1" {
		t.Skip("set CHECKPROTO_LOAD_REAL=1 to run isolated load checks")
	}
	backend := checkenv.Backend(os.Getenv("CHECKPROTO_BACKEND"))
	if backend != checkenv.Postgres && backend != checkenv.Badger {
		t.Fatal("set CHECKPROTO_BACKEND=postgres or badger")
	}
	cwd, e := os.Getwd()
	if e != nil {
		t.Fatal(e)
	}
	results, e := os.MkdirTemp("/tmp/opencode", "checkproto-load-")
	if e != nil {
		t.Fatal(e)
	}
	t.Logf("retained results: %s", results)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	env, e := checkenv.Start(ctx, checkenv.Config{Repository: filepath.Clean(filepath.Join(cwd, "../..")), ResultsDir: filepath.Join(results, "environment"), Backend: backend})
	if e != nil {
		t.Fatal(e)
	}
	defer func() {
		if e := env.Close(context.Background()); e != nil {
			t.Error(e)
		}
	}()
	for _, profile := range []Profile{RoomProfile, DMProfile} {
		t.Run(string(profile), func(t *testing.T) {
			cfg := LoadConfig{HTTPURL: env.HTTPURL, WSURL: env.WSURL, ResultsDir: results, Profile: profile, Users: 2, Rate: 2, Duration: 2 * time.Second, Warmup: time.Second, Drain: 500 * time.Millisecond, Seed: 17, Steps: 1, MaxRate: 2, MaxDuration: 2 * time.Second, FullTracePath: filepath.Join(results, string(profile)+"-trace.json")}
			summary, e := RunLoad(ctx, cfg)
			if e != nil {
				t.Fatalf("%+v: %v", summary, e)
			}
			t.Logf("offered=%d sent=%d observed=%d correlated=%d", summary.Offered, summary.Sent, summary.Observed, summary.Correlated)
		})
	}
}
