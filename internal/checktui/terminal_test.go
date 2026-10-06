package checktui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/vt"
)

func TestScreenErasureCursorAndUnicode(t *testing.T) {
	term := &terminal{emu: vt.NewEmulator(width, height)}
	term.mu.Lock()
	_, _ = term.emu.WriteString("\x1b[3;1Hé💬old-body")
	term.mu.Unlock()
	if !strings.Contains(term.history(), "é💬old-body") {
		t.Fatal("unicode cell history absent")
	}
	term.mu.Lock()
	_, _ = term.emu.WriteString("\x1b[3;1H\x1b[2K\x1b[28;1Hcomposer-only")
	term.mu.Unlock()
	if strings.Contains(term.history(), "old-body") || strings.Contains(term.history(), "composer-only") {
		t.Fatal("stale screen or composer matched viewport")
	}
	term.mu.Lock()
	_, _ = term.emu.WriteString("\x1b[3;1Hcomposer-only")
	term.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	if e := term.waitSent(ctx, "composer not cleared", "composer-only"); !errors.Is(e, context.DeadlineExceeded) {
		t.Fatalf("uncleared composer falsely passed: %v", e)
	}
	cancel()
	term.mu.Lock()
	_, _ = term.emu.WriteString("\x1b[28;1H\x1b[2K")
	term.mu.Unlock()
	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	if e := term.waitSent(ctx, "composer cleared", "composer-only"); e != nil {
		t.Fatal(e)
	}
	cancel()
	term.mu.Lock()
	_, _ = term.emu.WriteString("\x1b[?1049h\x1b[3;1Hnew-body")
	term.mu.Unlock()
	if strings.Contains(term.history(), "old-body") || !strings.Contains(term.history(), "new-body") {
		t.Fatal("alternate screen not represented")
	}
}
func TestWaitCancelAndTimeout(t *testing.T) {
	term := &terminal{emu: vt.NewEmulator(width, height)}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if e := term.wait(ctx, "cancel", func(string) bool { return false }); !errors.Is(e, context.Canceled) {
		t.Fatalf("cancel: %v", e)
	}
	ctx, cancel = context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if e := term.waitHistory(ctx, "timeout", "absent"); !errors.Is(e, context.DeadlineExceeded) {
		t.Fatalf("timeout: %v", e)
	}
}
func TestPTYProcessAndCleanup(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "terminal-client")
	if e := os.WriteFile(script, []byte("#!/bin/sh\nstty -echo\nprintf 'secret-from-client-log' > client.log\nprintf '\\033[3;1Hreal-pty'\nread -r line\nprintf '\\033[3;1H\\033[2Kdone'\n"), 0700); e != nil {
		t.Fatal(e)
	}
	term, e := startTerminal(script, "http://127.0.0.1:1", "ws://127.0.0.1:1/api/v1/ws")
	if e != nil {
		t.Fatal(e)
	}
	cwd := term.dir
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if e = term.waitHistory(ctx, "PTY first frame", "real-pty"); e != nil {
		t.Fatal(e)
	}
	if e = term.key("hello\n"); e != nil {
		t.Fatal(e)
	}
	if e = term.waitHistory(ctx, "PTY changed frame", "done"); e != nil {
		t.Fatal(e)
	}
	term.close()
	if _, e = os.Stat(cwd); !os.IsNotExist(e) {
		t.Fatalf("disposable cwd remains: %v", e)
	}
}
func TestPrivateResultsConfiguration(t *testing.T) {
	dir := t.TempDir()
	r, e := Run(context.Background(), Config{ClientBinary: "/bin/sh", HTTPURL: "http://127.0.0.1:1", WSURL: "ws://127.0.0.1:1/api/v1/ws", Seed: "sensitive-password", ResultsDir: dir, Timeout: time.Second})
	if e == nil || len(r.Steps) != 0 {
		t.Fatalf("invalid configuration accepted: %+v %v", r, e)
	}
	if _, e = os.Stat(filepath.Join(dir, "tui-result.json")); !os.IsNotExist(e) {
		t.Fatal("invalid configuration produced result")
	}
}
