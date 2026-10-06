// Package checktui exercises two real client binaries through Linux pseudo-terminals.
package checktui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/x/vt"
	"github.com/creack/pty"
)

const width, height = 100, 32

type terminal struct {
	cmd       *exec.Cmd
	master    *os.File
	emu       *vt.Emulator
	dir       string
	mu        sync.Mutex
	done      chan error
	copyDone  chan struct{}
	inputDone chan struct{}
}

func startTerminal(binary, httpURL, wsURL string) (*terminal, error) {
	dir, err := os.MkdirTemp("", "chattui-pty-*")
	if err != nil {
		return nil, err
	}
	if err = os.Chmod(dir, 0700); err != nil {
		_ = os.RemoveAll(dir)
		return nil, err
	}
	cmd := exec.Command(binary)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "SERVER_URL="+httpURL, "WS_URL="+strings.TrimSuffix(wsURL, "/api/v1/ws"), "THEME=catppuccin", "TERM=xterm-256color", "COLORTERM=truecolor", "NO_COLOR=")
	// The client writes client.log in its cwd. Keep the disposable cwd private and
	// remove it at close; neither input bytes nor raw terminal frames are persisted.
	cmd.Env = append(cmd.Env, "HOME="+dir, "XDG_CONFIG_HOME="+dir)
	master, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: height, Cols: width})
	if err != nil {
		_ = os.RemoveAll(dir)
		return nil, err
	}
	t := &terminal{cmd: cmd, master: master, emu: vt.NewEmulator(width, height), dir: dir, done: make(chan error, 1), copyDone: make(chan struct{}), inputDone: make(chan struct{})}
	go func() {
		defer close(t.copyDone)
		buf := make([]byte, 8192)
		for {
			n, e := master.Read(buf)
			if n > 0 {
				t.mu.Lock()
				_, _ = t.emu.Write(buf[:n])
				t.mu.Unlock()
			}
			if e != nil {
				return
			}
		}
	}()
	// Emulator answers DA, DSR and cursor position queries on its input pipe.
	go func() { defer close(t.inputDone); _, _ = io.Copy(master, t.emu) }()
	go func() { t.done <- cmd.Wait(); close(t.done) }()
	return t, nil
}

func (t *terminal) screen() string { t.mu.Lock(); defer t.mu.Unlock(); return t.emu.String() }
func (t *terminal) history() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.cells(2, height-7)
}
func (t *terminal) cells(start, end int) string {
	var b strings.Builder
	for y := start; y < end; y++ {
		for x := 0; x < width; x++ {
			if c := t.emu.CellAt(x, y); c != nil {
				b.WriteString(c.Content)
			}
		}
		b.WriteByte('\n')
	}
	return b.String()
}
func (t *terminal) waitSent(ctx context.Context, label, body string) error {
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	for {
		t.mu.Lock()
		// Viewport excludes the composer. Requiring an echoed viewport message
		// also prevents an empty composer before the queued Enter from passing.
		sent := strings.Contains(t.cells(2, height-7), body) && !strings.Contains(t.cells(height-5, height-2), body)
		t.mu.Unlock()
		if sent {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("%s: %w", label, ctx.Err())
		case <-tick.C:
		}
	}
}
func (t *terminal) key(s string) error { _, err := io.WriteString(t.master, s); return err }
func (t *terminal) wait(ctx context.Context, label string, predicate func(string) bool) error {
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	for {
		if predicate(t.screen()) {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("%s (%s): %w", label, t.state(), ctx.Err())
		case <-tick.C:
		}
	}
}
func (t *terminal) state() string {
	s := t.screen()
	flags := []string{}
	for _, label := range []string{"Sign in", "Create account", "Rooms", "DMs", "● live", "● connecting", "● offline", "authenticating", "✗"} {
		if strings.Contains(s, label) {
			flags = append(flags, label)
		}
	}
	return strings.Join(flags, ",")
}
func (t *terminal) waitHistory(ctx context.Context, label, body string) error {
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	for {
		if strings.Contains(t.history(), body) {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("%s: %w", label, ctx.Err())
		case <-tick.C:
		}
	}
}
func (t *terminal) quit(ctx context.Context) error {
	if err := t.key("\x03"); err != nil {
		return err
	}
	select {
	case err := <-t.done:
		if err != nil {
			return fmt.Errorf("client exit: %w", err)
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (t *terminal) close() {
	if t == nil {
		return
	}
	// Unblock a pending emulator response before closing its input pipe.
	_ = t.cmd.Process.Kill()
	_ = t.master.Close()
	if pipe, ok := t.emu.InputPipe().(io.Closer); ok {
		_ = pipe.Close()
	}
	select {
	case <-t.done:
	case <-time.After(3 * time.Second):
	}
	select {
	case <-t.copyDone:
	case <-time.After(3 * time.Second):
	}
	select {
	case <-t.inputDone:
	case <-time.After(3 * time.Second):
	}
	_ = os.RemoveAll(t.dir)
}
func validateBinary(path string) error {
	if !filepath.IsAbs(path) {
		return errors.New("client binary must be absolute")
	}
	st, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !st.Mode().IsRegular() || st.Mode()&0111 == 0 {
		return errors.New("client binary must be executable")
	}
	return nil
}
