package checkenv

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeDocker struct {
	calls [][]string
	label string
	fail  string
	port  string
}

func (f *fakeDocker) Run(ctx context.Context, args ...string) (CommandResult, error) {
	f.calls = append(f.calls, append([]string{}, args...))
	if len(args) > 3 && args[0] == "network" && args[1] == "create" {
		f.label = strings.TrimPrefix(args[3], label+"=")
	}
	if len(args) == 3 && args[0] == "image" && args[1] == "inspect" {
		return CommandResult{}, errors.New("not found")
	}
	if f.fail != "" && args[0] == f.fail {
		return CommandResult{Stderr: "failed"}, errors.New("exit")
	}
	if len(args) > 4 && args[0] == "image" && args[1] == "inspect" && args[2] == "--format" && strings.HasPrefix(args[4], "sha256:") {
		return CommandResult{Stdout: args[4] + "\n"}, nil
	}
	if len(args) > 2 && args[1] == "inspect" && args[2] == "--format" {
		return CommandResult{Stdout: f.label + "\n"}, nil
	}
	if args[0] == "inspect" && len(args) > 1 && args[1] == "--format" {
		return CommandResult{Stdout: f.label + "\n"}, nil
	}
	if args[0] == "port" {
		return CommandResult{Stdout: f.port + "\n"}, nil
	}
	if args[0] == "logs" {
		return CommandResult{Stdout: "password jwt"}, nil
	}
	if args[0] == "inspect" {
		return CommandResult{Stdout: `[{"Id":"id","Config":{"Labels":{"chattui.checkenv.id":"` + f.label + `"}},"State":{"Running":true,"OOMKilled":false},"RestartCount":2,"HostConfig":{"NanoCpus":2000000000,"Memory":2147483648,"MemorySwap":2147483648}}]`}, nil
	}
	return CommandResult{}, nil
}
func TestCollectRedactsAndRecordsLimits(t *testing.T) {
	f := &fakeDocker{label: "run"}
	dir := t.TempDir()
	e := &Environment{ID: "run", Backend: Badger, ResultsDir: dir, runner: f, secrets: []string{"password", "jwt"}, names: map[string]string{"postgres": "pg", "server": "srv"}}
	s, err := e.Collect()
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Resources) != 2 || s.Resources[0].NanoCPUs != 2_000_000_000 || s.Resources[1].MemorySwap != defaultMemory || s.Resources[0].RestartCount != 2 {
		t.Fatalf("inspect evidence: %+v", s)
	}
	if got := e.redact("password jwt"); got != "[REDACTED] [REDACTED]" {
		t.Fatal(got)
	}
	b, err := os.ReadFile(filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(b) || strings.Contains(string(b), "password") {
		t.Fatal("invalid or unredacted state")
	}
	log, err := os.ReadFile(filepath.Join(dir, "server.log"))
	if err != nil || string(log) != "[REDACTED] [REDACTED]" {
		t.Fatalf("unredacted log: %q, %v", log, err)
	}
}
func TestCloseOnlyOwnResourcesAndIgnoresCanceledCaller(t *testing.T) {
	f := &fakeDocker{label: "other"}
	e := &Environment{ID: "run", Backend: Postgres, ResultsDir: t.TempDir(), runner: f, names: map[string]string{"postgres": "pg", "server": "srv", "migration": "migration", "postgres-volume": "pgvol", "badger-volume": "badgervol", "network": "net", "image": "image"}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := e.Close(ctx); err != nil {
		t.Fatal(err)
	}
	for _, c := range f.calls {
		if c[0] == "rm" || (len(c) > 1 && c[1] == "rm") {
			t.Fatalf("removed unowned resource: %v", c)
		}
	}
	if !e.closed {
		t.Fatal("close not complete")
	}
}
func TestCloseOwnResourcesAndRetry(t *testing.T) {
	f := &fakeDocker{label: "run"}
	e := &Environment{ID: "run", ResultsDir: t.TempDir(), runner: f, names: map[string]string{"postgres": "pg", "server": "srv", "migration": "migration", "postgres-volume": "pgvol", "badger-volume": "badgervol", "network": "net", "image": "image"}}
	if err := e.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	var removed int
	for _, c := range f.calls {
		if c[0] == "rm" || (len(c) > 1 && c[1] == "rm") {
			removed++
		}
	}
	if removed != 7 {
		t.Fatalf("removed %d resources, want 7", removed)
	}
}
func TestStartRejectsInvalidConfigWithoutDocker(t *testing.T) {
	f := &fakeDocker{}
	_, err := start(context.Background(), Config{Repository: ".", ResultsDir: t.TempDir(), Backend: Postgres}, f)
	if err == nil || len(f.calls) != 0 {
		t.Fatalf("invalid config: %v calls %v", err, f.calls)
	}
}
func TestStartCleansPartialFailure(t *testing.T) {
	f := &fakeDocker{fail: "build"}
	repo, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	_, err = start(context.Background(), Config{Repository: repo, ResultsDir: dir, Backend: Badger}, f)
	if err == nil || !strings.Contains(err.Error(), "build") {
		t.Fatalf("startup failure: %v", err)
	}
	var removed int
	for _, call := range f.calls {
		if len(call) > 1 && call[1] == "rm" {
			removed++
		}
	}
	if removed < 3 {
		t.Fatalf("partial cleanup removed %d resources", removed)
	}
	if _, err := os.Stat(filepath.Join(dir, "startup-error.log")); err != nil {
		t.Fatal(err)
	}
}
func TestRestartPreservesPublishedURLs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	f := &fakeDocker{label: "run", port: strings.TrimPrefix(server.URL, "http://")}
	e := &Environment{ID: "run", runner: f, ResultsDir: t.TempDir(), names: map[string]string{"server": "server", "postgres": "postgres"}}
	if err := e.port(context.Background()); err != nil {
		t.Fatal(err)
	}
	originalHTTP, originalWS := e.HTTPURL, e.WSURL
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := e.RestartServer(ctx); err != nil {
		t.Fatal(err)
	}
	if e.HTTPURL != originalHTTP || e.WSURL != originalWS {
		t.Fatalf("endpoint changed: %s %s", e.HTTPURL, e.WSURL)
	}
	f.port = "127.0.0.1:1"
	if err := e.port(ctx); err == nil || e.HTTPURL != originalHTTP {
		t.Fatalf("unexpected port change accepted: %v", err)
	}
}
func TestConfiguredLimits(t *testing.T) {
	defaultLimits, err := configuredLimits(0, 0)
	if err != nil || defaultLimits.CPU != 2 || defaultLimits.Memory != defaultMemory {
		t.Fatalf("default limits: %+v %v", defaultLimits, err)
	}
	custom, err := configuredLimits(1.5, 512*1024*1024)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(custom.dockerArgs(), " "); got != "--cpus 1.5 --memory 536870912 --memory-swap 536870912" {
		t.Fatal(got)
	}
	if err := custom.verify(Resource{Kind: "server", NanoCPUs: 1_500_000_000, Memory: custom.Memory, MemorySwap: custom.Memory}); err != nil {
		t.Fatal(err)
	}
	if err := custom.verify(Resource{Kind: "server", NanoCPUs: 1_500_000_000, Memory: custom.Memory, MemorySwap: custom.Memory + 1}); err == nil {
		t.Fatal("accepted swap")
	}
	if _, err := configuredLimits(math.NaN(), 1); err == nil {
		t.Fatal("accepted invalid CPU")
	}
	if _, err := configuredLimits(1, -1); err == nil {
		t.Fatal("accepted invalid memory")
	}
}
func TestSharedPinnedImageDoesNotBuildOrDeleteImage(t *testing.T) {
	// Fail at published port verification, after the pinned server image is used.
	f := &fakeDocker{port: "127.0.0.1:1"}
	repo, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	image := "sha256:" + strings.Repeat("a", 64)
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	_, err = start(context.Background(), Config{Repository: repo, ResultsDir: dir, Backend: Badger, SharedImage: image}, f)
	if err == nil || !strings.Contains(err.Error(), "published port") {
		t.Fatalf("expected port failure after shared-image startup: %v", err)
	}
	config, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if err != nil || !strings.Contains(string(config), image) {
		t.Fatalf("image not pinned in configuration: %v", err)
	}
	var used bool
	var limited int
	for _, call := range f.calls {
		if call[0] == "run" && len(call) > 2 && call[1] == "-d" && strings.Contains(strings.Join(call, " "), "--cpus 2 --memory 2147483648 --memory-swap 2147483648") {
			limited++
		}
		if call[0] == "build" {
			t.Fatalf("rebuilt shared image: %v", call)
		}
		if len(call) > 1 && call[0] == "image" && call[1] == "rm" {
			t.Fatalf("deleted shared image: %v", call)
		}
		if call[0] == "run" && call[len(call)-1] == image {
			used = true
		}
	}
	if !used {
		t.Fatal("server did not run pinned image")
	}
	if limited != 2 {
		t.Fatalf("shared-image startup lost server/postgres limits: %d", limited)
	}
}
func TestSharedImageRejectsMutableTagBeforeDocker(t *testing.T) {
	f := &fakeDocker{}
	_, err := start(context.Background(), Config{Backend: Postgres, SharedImage: "latest"}, f)
	if err == nil || len(f.calls) != 0 {
		t.Fatalf("accepted mutable tag: %v", err)
	}
}
func TestCLICollectsStderrOnSuccess(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "docker")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nprintf stdout\nprintf stderr >&2\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	result, err := (cli{}).Run(context.Background(), "logs", "example")
	if err != nil || result.Stdout != "stdout" || result.Stderr != "stderr" {
		t.Fatalf("stdout/stderr: %+v %v", result, err)
	}
}
func TestCloseReturnsAfterCanceledContext(t *testing.T) {
	f := &fakeDocker{label: "other"}
	e := &Environment{ID: "run", ResultsDir: t.TempDir(), runner: f, names: map[string]string{"postgres": "pg", "server": "srv", "migration": "m", "postgres-volume": "pv", "badger-volume": "bv", "network": "n", "image": "i"}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan error, 1)
	go func() { done <- e.Close(ctx) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("cleanup blocked")
	}
}
