package checkenv

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDockerSmoke(t *testing.T) {
	if os.Getenv("CHECKENV_SMOKE") != "1" {
		t.Skip("set CHECKENV_SMOKE=1 to build and run isolated Docker containers")
	}
	repo, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	results, err := os.MkdirTemp("/tmp/opencode", "checkenv-smoke-")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("retained results: %s", results)
	env, err := Start(ctx, Config{Repository: repo, ResultsDir: results, Backend: Badger})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := env.Close(context.Background()); err != nil {
			t.Errorf("close: %v", err)
		}
	}()
	endpoint := env.HTTPURL
	client := &http.Client{Timeout: time.Second}
	assertHealth := func() {
		t.Helper()
		resp, err := client.Get(endpoint + "/healthz")
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			if err := resp.Body.Close(); err != nil {
				t.Errorf("close health response: %v", err)
			}
		}()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("health status: %d", resp.StatusCode)
		}
	}
	assertHealth()
	if err := env.StopServer(ctx); err != nil {
		t.Fatal(err)
	}
	if resp, err := client.Get(endpoint + "/healthz"); err == nil {
		if err := resp.Body.Close(); err != nil {
			t.Errorf("close unexpected health response: %v", err)
		}
		t.Fatal("stopped server still responds")
	}
	if err := env.StartServer(ctx); err != nil {
		t.Fatal(err)
	}
	if env.HTTPURL != endpoint {
		t.Fatalf("endpoint changed: %s -> %s", endpoint, env.HTTPURL)
	}
	assertHealth()
	if err := env.RestartServer(ctx); err != nil {
		t.Fatal(err)
	}
	if env.HTTPURL != endpoint {
		t.Fatalf("endpoint changed on restart: %s -> %s", endpoint, env.HTTPURL)
	}
	assertHealth()
	state, err := env.Collect()
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Resources) < 2 || !state.Resources[1].Running {
		t.Fatalf("restart: %+v", state)
	}
}
