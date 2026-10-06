package checktui

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestResultRecordsSeedAndOmitsClientSecrets(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "fake-client")
	if e := os.WriteFile(binary, []byte("#!/bin/sh\nprintf 'fixture-client-log-password-token' > client.log\nprintf 'Sign in'\nread -r line\n"), 0700); e != nil {
		t.Fatal(e)
	}
	resultDir := filepath.Join(dir, "results")
	if e := os.Mkdir(resultDir, 0700); e != nil {
		t.Fatal(e)
	}
	_, e := Run(context.Background(), Config{ClientBinary: binary, HTTPURL: "http://127.0.0.1:1", WSURL: "ws://127.0.0.1:1/api/v1/ws", Seed: "public-seed", ResultsDir: resultDir, Timeout: 100 * time.Millisecond, LifecycleStop: func(context.Context) error { return nil }, LifecycleStart: func(context.Context) error { return nil }})
	if e == nil {
		t.Fatal("expected timeout")
	}
	files, e := os.ReadDir(resultDir)
	if e != nil {
		t.Fatal(e)
	}
	if len(files) == 0 {
		t.Fatal("no result")
	}
	for _, f := range files {
		if f.Name() != "tui-result.json" && f.Name() != "client-a.log" && f.Name() != "client-b.log" {
			t.Fatalf("unexpected artifact: %s", f.Name())
		}
		data, e := os.ReadFile(filepath.Join(resultDir, f.Name()))
		if e != nil {
			t.Fatal(e)
		}
		if strings.Contains(string(data), "fixture-client-log-password-token") {
			t.Fatal("client log secret persisted")
		}
		if f.Name() == "tui-result.json" {
			var result Result
			if e = json.Unmarshal(data, &result); e != nil {
				t.Fatal(e)
			}
			if result.Config.Seed != "public-seed" || result.Config.UserA.Username == "" || result.Config.UserB.Username == "" || result.Config.RoomName == "" || result.Config.TerminalColumns != width || result.Config.TerminalRows != height {
				t.Fatalf("configuration/identities missing: %+v", result.Config)
			}
		}
	}
}
