package checktui

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sleklere/chattui/internal/checkproto"
)

type Config struct {
	ClientBinary, HTTPURL, WSURL, Seed, ResultsDir string
	Timeout                                        time.Duration
	LifecycleStop, LifecycleStart                  func(context.Context) error
}
type Step struct {
	Name       string `json:"name"`
	DurationMS int64  `json:"duration_ms"`
}
type RunIdentity struct {
	Username string `json:"username"`
	UserID   int64  `json:"user_id,omitempty"`
}
type RunConfig struct {
	Seed            string      `json:"seed"`
	HTTPURL         string      `json:"http_url"`
	WSURL           string      `json:"ws_url"`
	TerminalColumns int         `json:"terminal_columns"`
	TerminalRows    int         `json:"terminal_rows"`
	Theme           string      `json:"theme"`
	UserA           RunIdentity `json:"user_a"`
	UserB           RunIdentity `json:"user_b"`
	RoomName        string      `json:"room_name"`
	RoomID          int64       `json:"room_id,omitempty"`
}
type Result struct {
	Config RunConfig `json:"config"`
	Steps  []Step    `json:"steps"`
	Error  string    `json:"error,omitempty"`
}

// Run uses REST only to check identities and persisted messages after keyboard
// interactions. The returned steps and result.json never contain input frames.
func Run(ctx context.Context, cfg Config) (result Result, err error) {
	if err = validateBinary(cfg.ClientBinary); err != nil {
		return
	}
	if cfg.LifecycleStop == nil || cfg.LifecycleStart == nil || cfg.Seed == "" || !filepath.IsAbs(cfg.ResultsDir) || cfg.Timeout <= 0 {
		err = errors.New("seed, absolute results directory, timeout and lifecycle callbacks required")
		return
	}
	if st, e := os.Stat(cfg.ResultsDir); e != nil || !st.IsDir() || st.Mode().Perm()&0077 != 0 {
		err = errors.New("results directory must exist and be private")
		return
	}
	a := checkproto.NewClient(cfg.HTTPURL, cfg.WSURL)
	b := checkproto.NewClient(cfg.HTTPURL, cfg.WSURL)
	// Validate loopback endpoints before any keyboard activity; REST failures do
	// not replace account creation through the UI.
	if !loopback(cfg.HTTPURL, cfg.WSURL) {
		err = errors.New("HTTP and WS URLs must use numeric loopback hosts")
		return
	}
	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()
	var ta, tb *terminal
	defer func() {
		if ta != nil {
			if e := saveClientLog(cfg.ResultsDir, "client-a", ta.dir); e != nil && err == nil {
				err = fmt.Errorf("sanitizing client A log: %w", e)
			}
			ta.close()
		}
		if tb != nil {
			if e := saveClientLog(cfg.ResultsDir, "client-b", tb.dir); e != nil && err == nil {
				err = fmt.Errorf("sanitizing client B log: %w", e)
			}
			tb.close()
		}
		if err != nil {
			result.Error = err.Error()
		}
		data, e := json.MarshalIndent(result, "", "  ")
		if e == nil {
			_ = os.WriteFile(filepath.Join(cfg.ResultsDir, "tui-result.json"), append(data, '\n'), 0600)
		}
	}()
	nonce := make([]byte, 8)
	if _, err = rand.Read(nonce); err != nil {
		return
	}
	id := hex.EncodeToString(nonce)
	// Generated credentials remain in memory and are never used in error strings.
	pass := make([]byte, 24)
	if _, err = rand.Read(pass); err != nil {
		return
	}
	password := hex.EncodeToString(pass)
	userA, userB := "ta"+id, "tb"+id
	roomName := "room" + id
	bodyRoom, bodyAfter, bodyOffline, bodyDM := logicalBodies(cfg.Seed)
	result.Config = RunConfig{Seed: cfg.Seed, HTTPURL: cfg.HTTPURL, WSURL: cfg.WSURL, TerminalColumns: width, TerminalRows: height, Theme: "catppuccin", UserA: RunIdentity{Username: userA}, UserB: RunIdentity{Username: userB}, RoomName: roomName}
	step := func(name string, f func() error) error {
		start := time.Now()
		e := f()
		result.Steps = append(result.Steps, Step{Name: name, DurationMS: time.Since(start).Milliseconds()})
		if e != nil {
			return fmt.Errorf("%s: %w", name, e)
		}
		return nil
	}
	wait := func(t *terminal, name, text string) error {
		return t.wait(ctx, name, func(screen string) bool { return strings.Contains(screen, text) })
	}
	send := func(t *terminal, keys ...string) error {
		for _, key := range keys {
			if e := t.key(key); e != nil {
				return e
			}
		}
		return nil
	}
	if err = step("register both clients", func() error {
		var e error
		ta, e = startTerminal(cfg.ClientBinary, cfg.HTTPURL, cfg.WSURL)
		if e != nil {
			return e
		}
		tb, e = startTerminal(cfg.ClientBinary, cfg.HTTPURL, cfg.WSURL)
		if e != nil {
			return e
		}
		for _, u := range []struct {
			t    *terminal
			name string
		}{{ta, userA}, {tb, userB}} {
			if e = wait(u.t, "auth", "Sign in"); e != nil {
				return e
			}
			if e = send(u.t, "\x14", u.name, "\t", password, "\r"); e != nil {
				return e
			}
			if e = wait(u.t, "rooms after register", "Rooms"); e != nil {
				return e
			}
			if e = u.t.wait(ctx, "live after register", func(s string) bool { return strings.Contains(s, "● live") }); e != nil {
				return e
			}
		}
		if e = a.Login(ctx, userA, password); e != nil {
			return e
		}
		if e = b.Login(ctx, userB, password); e != nil {
			return e
		}
		result.Config.UserA.UserID = a.User.ID
		result.Config.UserB.UserID = b.User.ID
		return nil
	}); err != nil {
		return
	}
	var room checkproto.Room
	if err = step("create and join room in both clients", func() error {
		if e := send(ta, "n", roomName, "\r"); e != nil {
			return e
		}
		var e error
		if e = ta.wait(ctx, "room listing", func(s string) bool { return strings.Contains(s, roomName) && !strings.Contains(s, "New room") }); e != nil {
			return e
		}
		if e = send(ta, "\r"); e != nil {
			return e
		}
		if e = wait(ta, "room chat", "# "+roomName); e != nil {
			return e
		}
		room, e = roomByName(ctx, cfg.HTTPURL, userA, password, roomName)
		if e != nil {
			return e
		}
		result.Config.RoomID = room.ID
		if e = send(tb, "r"); e != nil {
			return e
		}
		if e = wait(tb, "room visible", roomName); e != nil {
			return e
		}
		if e = send(tb, "\r"); e != nil {
			return e
		}
		return wait(tb, "joined room", "# "+room.Slug)
	}); err != nil {
		return
	}
	if err = step("live room delivery", func() error {
		if e := send(ta, bodyRoom, "\r"); e != nil {
			return e
		}
		if e := ta.waitSent(ctx, "room sender viewport and cleared composer", bodyRoom); e != nil {
			return e
		}
		if e := tb.waitHistory(ctx, "room peer delivery", bodyRoom); e != nil {
			return e
		}
		return verifyHistory(ctx, b, "rooms", room.ID, a.User.ID, bodyRoom)
	}); err != nil {
		return
	}
	if err = step("server stop and reconnect", func() error {
		if e := cfg.LifecycleStop(ctx); e != nil {
			return e
		}
		for _, t := range []*terminal{ta, tb} {
			if e := t.wait(ctx, "disconnected", func(s string) bool {
				return strings.Contains(s, "● connecting") || strings.Contains(s, "● offline")
			}); e != nil {
				return e
			}
		}
		if e := cfg.LifecycleStart(ctx); e != nil {
			return e
		}
		for _, t := range []*terminal{ta, tb} {
			if e := t.wait(ctx, "reconnected live", func(s string) bool { return strings.Contains(s, "● live") }); e != nil {
				return e
			}
		}
		if e := send(ta, bodyAfter, "\r"); e != nil {
			return e
		}
		if e := ta.waitSent(ctx, "post-restart sender viewport and cleared composer", bodyAfter); e != nil {
			return e
		}
		if e := tb.waitHistory(ctx, "post-restart delivery", bodyAfter); e != nil {
			return e
		}
		return verifyHistory(ctx, b, "rooms", room.ID, a.User.ID, bodyAfter)
	}); err != nil {
		return
	}
	if err = step("offline room history on reentry", func() error {
		if e := tb.quit(ctx); e != nil {
			return e
		}
		if e := saveClientLog(cfg.ResultsDir, "client-b-before-reentry", tb.dir); e != nil {
			return e
		}
		tb.close()
		tb = nil
		if e := send(ta, bodyOffline, "\r"); e != nil {
			return e
		}
		if e := ta.waitSent(ctx, "offline-history sender viewport and cleared composer", bodyOffline); e != nil {
			return e
		}
		if e := verifyHistory(ctx, a, "rooms", room.ID, a.User.ID, bodyOffline); e != nil {
			return e
		}
		var e error
		tb, e = startTerminal(cfg.ClientBinary, cfg.HTTPURL, cfg.WSURL)
		if e != nil {
			return e
		}
		if e = wait(tb, "login screen", "Sign in"); e != nil {
			return e
		}
		if e = send(tb, userB, "\t", password, "\r"); e != nil {
			return e
		}
		if e = wait(tb, "rooms after login", "Rooms"); e != nil {
			return e
		}
		if e = wait(tb, "room after login", roomName); e != nil {
			return e
		}
		if e = send(tb, "\r"); e != nil {
			return e
		}
		if e = wait(tb, "room history title", "# "+room.Slug); e != nil {
			return e
		}
		return tb.waitHistory(ctx, "offline history", bodyOffline)
	}); err != nil {
		return
	}
	if err = step("DM lookup, delivery and history", func() error {
		if e := send(ta, "\x1b"); e != nil {
			return e
		}
		if e := wait(ta, "rooms before DM", "Rooms"); e != nil {
			return e
		}
		if e := send(tb, "\x1b"); e != nil {
			return e
		}
		if e := wait(tb, "rooms peer before DM", "Rooms"); e != nil {
			return e
		}
		if e := send(ta, "\t"); e != nil {
			return e
		}
		if e := wait(ta, "DM list", "new DM"); e != nil {
			return e
		}
		if e := send(ta, "n", userB, "\r"); e != nil {
			return e
		}
		if e := wait(ta, "DM chat", "@ "+userB); e != nil {
			return e
		}
		if e := send(tb, "\t"); e != nil {
			return e
		}
		if e := wait(tb, "peer DM list", "new DM"); e != nil {
			return e
		}
		if e := send(tb, "n", userA, "\r"); e != nil {
			return e
		}
		if e := wait(tb, "recipient DM chat", "@ "+userA); e != nil {
			return e
		}
		if e := send(ta, bodyDM, "\r"); e != nil {
			return e
		}
		if e := ta.waitSent(ctx, "DM sender viewport and cleared composer", bodyDM); e != nil {
			return e
		}
		if e := tb.waitHistory(ctx, "live DM recipient", bodyDM); e != nil {
			return e
		}
		if e := verifyDM(ctx, b, a.User.ID, bodyDM); e != nil {
			return e
		}
		if e := tb.waitHistory(ctx, "DM history", bodyDM); e != nil {
			return e
		}
		if e := send(tb, "\x1b"); e != nil {
			return e
		}
		if e := wait(tb, "DM list return", "new DM"); e != nil {
			return e
		}
		if e := send(tb, "\r"); e != nil {
			return e
		}
		return tb.waitHistory(ctx, "DM history reentry", bodyDM)
	}); err != nil {
		return
	}
	err = step("clean ctrl+c exit", func() error {
		if e := ta.quit(ctx); e != nil {
			return e
		}
		return tb.quit(ctx)
	})
	return
}

func logicalBodies(seed string) (room, recovered, offline, dm string) {
	hash := sha256.Sum256([]byte(seed))
	id := hex.EncodeToString(hash[:8])
	return "room-live-" + id, "room-recovered-" + id, "room-history-" + id, "dm-live-" + id
}

func verifyHistory(ctx context.Context, c *checkproto.Client, kind string, id, sender int64, body string) error {
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		history, e := c.History(ctx, kind, id)
		if e == nil {
			for _, m := range history {
				if m.ID > 0 && m.SenderID == sender && m.Body == body {
					return nil
				}
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("persisted message not found: %w", ctx.Err())
		case <-tick.C:
		}
	}
}
func verifyDM(ctx context.Context, c *checkproto.Client, sender int64, body string) error {
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		convs, e := c.Conversations(ctx)
		if e == nil {
			for _, v := range convs {
				if v.ID > 0 && v.PeerID == sender {
					h, e := c.History(ctx, "conversations", v.ID)
					if e == nil {
						for _, m := range h {
							if m.ID > 0 && m.SenderID == sender && m.Body == body {
								return nil
							}
						}
					}
				}
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("persisted DM not found: %w", ctx.Err())
		case <-tick.C:
		}
	}
}
func roomByName(ctx context.Context, base, user, password, name string) (checkproto.Room, error) {
	// Read-only REST lookup after the UI creates the room. This second login is
	// only for fetching a bearer token; no credential is serialized to results.
	// The protocol client deliberately keeps its token private. Obtain a short-
	// lived token via the public login endpoint for the read-only room list.
	payload, _ := json.Marshal(map[string]string{"username": user, "password": password})
	req, e := http.NewRequestWithContext(ctx, "POST", base+"/api/v1/auth/login", strings.NewReader(string(payload)))
	if e != nil {
		return checkproto.Room{}, e
	}
	req.Header.Set("Content-Type", "application/json")
	resp, e := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if e != nil {
		return checkproto.Room{}, errors.New("room lookup login failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return checkproto.Room{}, fmt.Errorf("room lookup login status %d", resp.StatusCode)
	}
	var auth struct {
		Token string `json:"token"`
	}
	if e = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&auth); e != nil {
		return checkproto.Room{}, errors.New("room lookup auth response invalid")
	}
	req, e = http.NewRequestWithContext(ctx, "GET", base+"/api/v1/rooms", nil)
	if e != nil {
		return checkproto.Room{}, e
	}
	req.Header.Set("Authorization", "Bearer "+auth.Token)
	resp2, e := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if e != nil {
		return checkproto.Room{}, errors.New("room lookup list failed")
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != 200 {
		return checkproto.Room{}, fmt.Errorf("room list status %d", resp2.StatusCode)
	}
	var rooms []checkproto.Room
	if e = json.NewDecoder(io.LimitReader(resp2.Body, 1<<20)).Decode(&rooms); e != nil {
		return checkproto.Room{}, errors.New("room list response invalid")
	}
	for _, r := range rooms {
		if r.Name == name && r.ID > 0 {
			return r, nil
		}
	}
	return checkproto.Room{}, errors.New("UI-created room absent from REST list")
}
func loopback(httpURL, wsURL string) bool {
	for i, raw := range []string{httpURL, wsURL} {
		u, e := url.Parse(raw)
		if e != nil || u.User != nil || u.RawQuery != "" || u.Fragment != "" || ((i == 0 && u.Scheme != "http") || (i == 1 && u.Scheme != "ws")) {
			return false
		}
		host, _, e := net.SplitHostPort(u.Host)
		if e != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
			return false
		}
	}
	return true
}
