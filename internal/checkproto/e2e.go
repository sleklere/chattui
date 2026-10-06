package checkproto

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Lifecycle struct {
	Stop  func(context.Context) error
	Start func(context.Context) error
}
type E2EConfig struct {
	HTTPURL, WSURL string
	Seed           int64
	Window         time.Duration
	Lifecycle      Lifecycle
	TracePath      string
}
type E2ETrace struct {
	Seed         int64         `json:"seed"`
	RunID        string        `json:"run_id"`
	Expected     []Expected    `json:"expected"`
	Observed     []Observation `json:"observed"`
	Stages       []string      `json:"stages"`
	FixtureUsers []string      `json:"fixture_users"`
	FixtureRoom  string        `json:"fixture_room"`
	Failure      string        `json:"failure,omitempty"`
}

func saveTrace(path string, t E2ETrace) error {
	if path == "" {
		return nil
	}
	b, e := json.MarshalIndent(t, "", "  ")
	if e != nil {
		return e
	}
	f, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return e
	}
	defer f.Close()
	_, e = f.Write(append(b, '\n'))
	return e
}

// RunE2E requires a fresh disposable server, with no other senders. Every stage
// records evidence before proceeding. Stop/Start must preserve storage.
func RunE2E(ctx context.Context, cfg E2EConfig) (trace E2ETrace, err error) {
	if e := validateEndpoints(cfg.HTTPURL, cfg.WSURL); e != nil {
		return trace, e
	}
	if cfg.TracePath == "" {
		return trace, errors.New("full E2E trace path required")
	}
	if cfg.Lifecycle.Stop == nil || cfg.Lifecycle.Start == nil {
		return trace, errors.New("stop and start callbacks required")
	}
	if cfg.Window < 100*time.Millisecond || cfg.Window > 30*time.Second {
		return trace, errors.New("window must be within 100ms..30s")
	}
	var raw [12]byte
	if _, err = rand.Read(raw[:]); err != nil {
		return
	}
	id := hex.EncodeToString(raw[:])
	trace = E2ETrace{Seed: cfg.Seed, RunID: id}
	if cfg.TracePath != "" {
		if !filepath.IsAbs(cfg.TracePath) {
			return trace, errors.New("trace path must be absolute")
		}
		if e := os.MkdirAll(filepath.Dir(cfg.TracePath), 0700); e != nil {
			return trace, e
		}
	}
	defer func() {
		if err != nil {
			trace.Failure = err.Error()
		}
		if e := saveTrace(cfg.TracePath, trace); e != nil {
			err = errors.Join(err, e)
		}
	}()
	passwordRaw := make([]byte, 24)
	if _, err = rand.Read(passwordRaw); err != nil {
		return
	}
	password := hex.EncodeToString(passwordRaw)
	users := []*Client{NewClient(cfg.HTTPURL, cfg.WSURL), NewClient(cfg.HTTPURL, cfg.WSURL), NewClient(cfg.HTTPURL, cfg.WSURL)}
	defer func() {
		for _, u := range users {
			u.Close()
		}
	}()
	for i, u := range users {
		logicalName := fixtureUser("check", cfg.Seed, i)
		trace.FixtureUsers = append(trace.FixtureUsers, logicalName)
		name := fixtureNamespace(logicalName, id)
		if err = u.Register(ctx, name, password); err != nil {
			return
		}
		if err = u.Login(ctx, name, password); err != nil {
			return
		}
		if err = u.ValidFor(2*cfg.Window + 2*time.Minute); err != nil {
			return
		}
	}
	a, b := users[0], users[1]
	peer, e := a.Lookup(ctx, b.User.Username)
	if e != nil {
		return trace, e
	}
	if peer.ID != b.User.ID {
		return trace, errors.New("lookup returned wrong peer")
	}
	trace.FixtureRoom = fmt.Sprintf("check-%d", cfg.Seed)
	room, e := a.CreateRoom(ctx, fixtureNamespace(trace.FixtureRoom, id))
	if e != nil {
		return trace, e
	}
	if room.ID <= 0 {
		return trace, errors.New("room has no ID")
	}
	if err = b.JoinRoom(ctx, room.ID); err != nil {
		return
	}
	for _, u := range users {
		if err = u.Connect(ctx); err != nil {
			return
		}
	}
	trace.Stages = append(trace.Stages, "registered, logged in, discovered peer, joined room, connected")
	// Collect through the negative window, including while B is offline.
	collect := func(offlineUserID int64) error {
		sources := make([]observationSource, 0, len(users))
		for _, u := range users {
			sources = append(sources, observationSource{userID: u.User.ID, events: u.Events(), readErr: u.ReadError(), offline: u.User.ID == offlineUserID})
		}
		observed, e := collectObservations(ctx, cfg.Window, sources)
		trace.Observed = append(trace.Observed, observed...)
		return e
	}
	send := func(from *Client, kind string, target int64, recipients ...int64) error {
		body := fixtureBody("check", cfg.Seed, len(trace.Expected))
		x := Expected{Type: kind, Body: body, SenderID: from.User.ID, TargetID: target, Recipients: recipients, SentAt: time.Now(), Deadline: time.Now().Add(2 * cfg.Window)}
		trace.Expected = append(trace.Expected, x)
		return from.Send(ctx, kind, target, body)
	}
	if err = send(a, "room_message", room.ID, a.User.ID, b.User.ID); err != nil {
		return
	}
	if err = send(a, "direct_message", b.User.ID, a.User.ID, b.User.ID); err != nil {
		return
	}
	if err = collect(0); err != nil {
		return
	}
	first := Verify(trace.Expected, trace.Observed)
	if err = first.Err(); err != nil {
		return
	}
	trace.Stages = append(trace.Stages, "live room and DM recipients/outsider verified")
	convs, e := a.Conversations(ctx)
	if e != nil {
		return trace, e
	}
	var convID int64
	for _, v := range convs {
		if v.PeerID == b.User.ID {
			convID = v.ID
		}
	}
	if convID <= 0 {
		return trace, errors.New("DM conversation not discoverable")
	}
	other, e := b.Conversations(ctx)
	if e != nil {
		return trace, e
	}
	found := false
	for _, v := range other {
		if v.ID == convID && v.PeerID == a.User.ID {
			found = true
		}
	}
	if !found {
		return trace, errors.New("recipient DM discovery failed")
	}
	checkHistory := func() error {
		rooms, e := a.History(ctx, "rooms", room.ID)
		if e != nil {
			return e
		}
		dms, e := b.History(ctx, "conversations", convID)
		if e != nil {
			return e
		}
		var r, d []Expected
		for _, x := range trace.Expected {
			if x.Type == "room_message" {
				r = append(r, x)
			} else {
				d = append(d, x)
			}
		}
		if e = VerifyHistory(rooms, r, "rooms", room.ID); e != nil {
			return e
		}
		return VerifyHistory(dms, d, "conversations", convID)
	}
	if err = checkHistory(); err != nil {
		return
	}
	trace.Stages = append(trace.Stages, "complete room and DM history verified")
	b.Close()
	if err = send(a, "direct_message", b.User.ID, a.User.ID); err != nil {
		return
	}
	if err = collect(b.User.ID); err != nil {
		return
	}
	v := Verify(trace.Expected, trace.Observed)
	if err = v.Err(); err != nil {
		return
	}
	if err = b.Connect(ctx); err != nil {
		return
	}
	if err = collect(0); err != nil {
		return
	}
	v = Verify(trace.Expected, trace.Observed)
	if err = v.Err(); err != nil {
		return
	}
	if err = checkHistory(); err != nil {
		return
	}
	trace.Stages = append(trace.Stages, "offline delivery absent; offline DM present in history after reconnect")
	for _, u := range users {
		u.Close()
	}
	if err = cfg.Lifecycle.Stop(ctx); err != nil {
		return
	}
	if err = cfg.Lifecycle.Start(ctx); err != nil {
		return
	}
	for _, u := range users {
		if err = u.Login(ctx, u.User.Username, password); err != nil {
			return
		}
		if err = u.Connect(ctx); err != nil {
			return
		}
	}
	if err = checkHistory(); err != nil {
		return
	}
	if err = collect(0); err != nil {
		return
	}
	v = Verify(trace.Expected, trace.Observed)
	if err = v.Err(); err != nil {
		return
	}
	trace.Stages = append(trace.Stages, "history survived server restart")
	if err = send(b, "room_message", room.ID, a.User.ID, b.User.ID); err != nil {
		return
	}
	if err = collect(0); err != nil {
		return
	}
	v = Verify(trace.Expected, trace.Observed)
	if err = v.Err(); err != nil {
		return
	}
	if err = checkHistory(); err != nil {
		return
	}
	trace.Stages = append(trace.Stages, "post-restart live routing and history verified")
	return trace, nil
}
