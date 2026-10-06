package checkproto

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"time"
)

// Profile selects the recipient topology for offered load.
type Profile string

// Supported load profiles exercise room broadcasts or direct-message pairs.
const (
	RoomProfile Profile = "room"
	DMProfile   Profile = "dm"
)

// LoadConfig bounds the schedule and selects seeded fixtures and private output.
type LoadConfig struct {
	HTTPURL, WSURL            string
	Profile                   Profile
	Users                     int
	Rooms, MembersPerRoom     int // room profile only; zero means one room / all users
	Rate                      float64
	Duration, Warmup, Drain   time.Duration
	Seed                      int64
	Steps                     int
	MaxRate                   float64
	MaxDuration               time.Duration
	FullTracePath, ResultsDir string
}

// LoadSummary separates offered sends from verified delivery and generator metrics.
type LoadSummary struct {
	RunID                  string                   `json:"run_id"`
	Profile                Profile                  `json:"profile"`
	Seed                   int64                    `json:"seed"`
	WarmupSent             int                      `json:"warmup_sent"`
	WarmupObserved         int                      `json:"warmup_observed"`
	Offered                int                      `json:"offered"`
	Sent                   int                      `json:"sent"`
	Observed               int                      `json:"observed"`
	Correlated             int                      `json:"correlated"`
	Duplicates             int                      `json:"duplicates"`
	Missing                int                      `json:"missing"`
	WrongRecipient         int                      `json:"wrong_recipient"`
	Mismatch               int                      `json:"mismatch"`
	SendErrors             int                      `json:"send_errors"`
	Disconnects            int                      `json:"disconnects"`
	SchedulerMaxDelay      time.Duration            `json:"scheduler_max_delay"`
	SchedulerP95Delay      time.Duration            `json:"scheduler_p95_delay"`
	Latency                map[string]time.Duration `json:"latency"`
	GeneratorMaxHeap       uint64                   `json:"generator_max_heap_bytes"`
	GeneratorMaxGoroutines int                      `json:"generator_max_goroutines"`
	HistoryScope           string                   `json:"history_scope"`
	Failure                string                   `json:"failure,omitempty"`
}

// Validate enforces explicit budgets and JWT lifetime safety margins.
func (c LoadConfig) Validate() error {
	if e := validateEndpoints(c.HTTPURL, c.WSURL); e != nil {
		return e
	}
	if !filepath.IsAbs(c.ResultsDir) {
		return errors.New("absolute results directory required")
	}
	if c.Profile != RoomProfile && c.Profile != DMProfile {
		return errors.New("profile must be room or dm")
	}
	if c.Seed < 0 || c.Users < 2 || c.Users > 32 || (c.Profile == DMProfile && c.Users%2 != 0) || c.Rate <= 0 || c.Rate > 100 || math.IsNaN(c.Rate) || math.IsInf(c.Rate, 0) || math.IsNaN(c.MaxRate) || math.IsInf(c.MaxRate, 0) || c.Duration <= 0 || c.Duration > 2*time.Minute || c.Warmup < 0 || c.Warmup > 30*time.Second || c.Drain < 100*time.Millisecond || c.Drain > 30*time.Second || c.Steps < 1 || c.Steps > 8 || c.MaxRate < c.Rate || c.MaxRate > 100 || c.MaxDuration < c.Duration || c.MaxDuration > 2*time.Minute {
		return errors.New("invalid load bounds (users 2..32, rate <=100/s, duration <=2m, steps <=8, drain 100ms..30s)")
	}
	if c.Profile == RoomProfile {
		if c.Rooms < 0 || c.Rooms > 16 || c.MembersPerRoom < 0 || (c.MembersPerRoom != 0 && (c.MembersPerRoom < 2 || c.MembersPerRoom > c.Users)) {
			return errors.New("room bounds: rooms 1..16 (zero defaults to 1), members per room 2..users (zero defaults to all)")
		}
		if c.Rooms > int(math.Ceil(c.Duration.Seconds()*c.Rate)) {
			return errors.New("measured schedule must include every configured room")
		}
	} else if c.Rooms != 0 || c.MembersPerRoom != 0 {
		return errors.New("room settings apply only to room profile")
	}
	if c.MaxDuration.Seconds()*c.MaxRate > 1000 || c.Warmup.Seconds()*c.MaxRate > 1000 {
		return errors.New("at most 1000 messages per phase")
	}
	if c.Duration+c.Warmup+c.Drain > 4*time.Minute {
		return errors.New("JWT lifetime safety margin exceeded")
	}
	return nil
}
func (c LoadConfig) stepped(step int) LoadConfig {
	c.Rate = math.Min(c.MaxRate, c.Rate*math.Pow(2, float64(step)))
	c.Duration = time.Duration(math.Min(float64(c.MaxDuration), float64(c.Duration)*math.Pow(2, float64(step))))
	return c
}
func writeUnique(path string, b []byte) (err error) {
	f, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return e
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	_, e = f.Write(append(b, '\n'))
	return e
}
func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, e := rand.Read(b); e != nil {
		return "", e
	}
	return hex.EncodeToString(b), nil
}

// RunLoad measures the whole server, not the storage engine in isolation. It
// requires a fresh disposable environment; all accounts connect once each.
func RunLoad(ctx context.Context, cfg LoadConfig) (summary LoadSummary, err error) {
	if err = cfg.Validate(); err != nil {
		return
	}
	summary.Profile = cfg.Profile
	summary.Seed = cfg.Seed
	summary.HistoryScope = "last 100 per chat only; older history not accessible via REST"
	id, e := randomHex(12)
	if e != nil {
		return summary, e
	}
	summary.RunID = id
	password, e := randomHex(24)
	if e != nil {
		return summary, e
	}
	if e = os.MkdirAll(cfg.ResultsDir, 0700); e != nil {
		return summary, e
	}
	info, e := os.Stat(cfg.ResultsDir)
	if e != nil {
		return summary, e
	}
	if info.Mode().Perm()&0077 != 0 {
		return summary, errors.New("results directory must be private")
	}
	defer func() {
		if err != nil {
			summary.Failure = err.Error()
		}
		if b, e := json.MarshalIndent(struct {
			Config  LoadConfig  `json:"config"`
			Summary LoadSummary `json:"summary"`
		}{cfg, summary}, "", "  "); e == nil {
			err = errors.Join(err, writeUnique(filepath.Join(cfg.ResultsDir, "load-"+id+".json"), b))
		} else {
			err = errors.Join(err, e)
		}
	}()
	users := make([]*Client, cfg.Users)
	defer func() {
		for _, u := range users {
			if u != nil {
				u.Close()
			}
		}
	}()
	for i := range users {
		u := NewClient(cfg.HTTPURL, cfg.WSURL)
		users[i] = u
		name := fixtureNamespace(fixtureUser("load", cfg.Seed, i), id)
		if err = u.Register(ctx, name, password); err != nil {
			return
		}
		if err = u.Login(ctx, name, password); err != nil {
			return
		}
		if err = u.ValidFor(cfg.Warmup + cfg.Duration + cfg.Drain + 30*time.Second); err != nil {
			return
		}
	}
	type loadRoom struct {
		id      int64
		members []*Client
	}
	var rooms []loadRoom
	if cfg.Profile == RoomProfile {
		roomCount := cfg.Rooms
		if roomCount == 0 {
			roomCount = 1
		}
		memberCount := cfg.MembersPerRoom
		if memberCount == 0 {
			memberCount = len(users)
		}
		rooms = make([]loadRoom, 0, roomCount)
		for i := 0; i < roomCount; i++ {
			members := make([]*Client, 0, memberCount)
			for j := 0; j < memberCount; j++ {
				members = append(members, users[(i*memberCount+j+int(cfg.Seed%int64(len(users))))%len(users)])
			}
			room, createErr := members[0].CreateRoom(ctx, fixtureNamespace(fmt.Sprintf("load-%d-%d", cfg.Seed, i), id))
			if createErr != nil {
				err = createErr
				return
			}
			for _, u := range members[1:] {
				if err = u.JoinRoom(ctx, room.ID); err != nil {
					return
				}
			}
			rooms = append(rooms, loadRoom{id: room.ID, members: members})
		}
	}
	for _, u := range users {
		if err = u.Connect(ctx); err != nil {
			return
		}
	}
	type recorded struct{ ob Observation }
	events := make(chan recorded, 65536)
	stopped := make(chan struct{})
	for _, u := range users {
		go func(u *Client) {
			for {
				select {
				case <-stopped:
					return
				case e := <-u.ReadError():
					if e != nil {
						select {
						case events <- recorded{ob: Observation{UserID: u.User.ID, Event: Event{Type: "__disconnect"}}}:
						case <-stopped:
							return
						}
						return
					}
				case o, ok := <-u.Events():
					if !ok {
						select {
						case events <- recorded{ob: Observation{UserID: u.User.ID, Event: Event{Type: "__disconnect"}}}:
						case <-stopped:
						}
						return
					}
					select {
					case events <- recorded{ob: o}:
					case <-stopped:
						return
					}
				}
			}
		}(u)
	}
	defer close(stopped)
	expected := []Expected{}
	warmExpected := []Expected{}
	observed := []Observation{}
	delays := []time.Duration{}
	var mem runtime.MemStats
	// Warmup uses unique bodies but is excluded from counters. Drain warmup before measurement.
	sendPhase := func(duration time.Duration, rate float64, warm bool) error {
		interval := time.Duration(float64(time.Second) / rate)
		if interval < time.Millisecond {
			return errors.New("scheduler resolution below 1ms")
		}
		start := time.Now()
		count := int(math.Ceil(duration.Seconds() * rate))
		if count > 1000 {
			return errors.New("scheduled message limit exceeded")
		}
		if !warm {
			summary.Offered = count
		}
		for i := 0; i < count; i++ {
			due := start.Add(time.Duration(i) * interval)
			if !due.Before(start.Add(duration)) {
				break
			}
			if wait := time.Until(due); wait > 0 {
				t := time.NewTimer(wait)
				select {
				case <-ctx.Done():
					t.Stop()
					return ctx.Err()
				case <-t.C:
				}
			}
			delay := time.Since(due)
			if !warm {
				delays = append(delays, delay)
			}
			var from *Client
			var target int64
			var recipients []int64
			kind := "room_message"
			if cfg.Profile == DMProfile {
				fromIndex := 2*((i/2+int(cfg.Seed%int64(len(users)/2)))%(len(users)/2)) + i%2
				from = users[fromIndex]
				to := users[fromIndex^1]
				target = to.User.ID
				recipients = []int64{from.User.ID, to.User.ID}
				kind = "direct_message"
			} else {
				room := rooms[(i+int(cfg.Seed%int64(len(rooms))))%len(rooms)]
				from = room.members[(i/len(rooms)+int(cfg.Seed%int64(len(room.members))))%len(room.members)]
				target = room.id
				for _, u := range room.members {
					recipients = append(recipients, u.User.ID)
				}
			}
			prefix := "load/measured"
			if warm {
				prefix = "load/warm"
			}
			body := fixtureBody(prefix, cfg.Seed, i)
			x := Expected{Type: kind, Body: body, SenderID: from.User.ID, TargetID: target, Recipients: recipients, SentAt: time.Now()}
			if warm {
				warmExpected = append(warmExpected, x)
			} else {
				expected = append(expected, x)
			}
			if e := from.Send(ctx, kind, target, body); e != nil {
				if !warm {
					summary.SendErrors++
				}
				return e
			}
			if warm {
				summary.WarmupSent++
			} else {
				summary.Sent++
			}
			runtime.ReadMemStats(&mem)
			if mem.HeapAlloc > summary.GeneratorMaxHeap {
				summary.GeneratorMaxHeap = mem.HeapAlloc
			}
			if n := runtime.NumGoroutine(); n > summary.GeneratorMaxGoroutines {
				summary.GeneratorMaxGoroutines = n
			}
		}
		return nil
	}
	if cfg.Warmup > 0 {
		if err = sendPhase(cfg.Warmup, cfg.Rate, true); err != nil {
			return
		}
		t := time.NewTimer(cfg.Drain)
		select {
		case <-ctx.Done():
			t.Stop()
			return summary, ctx.Err()
		case <-t.C:
		}
		var warmObserved []Observation
		var disconnects int
		for len(events) > 0 {
			r := <-events
			switch r.ob.Event.Type {
			case "__disconnect":
				disconnects++
			case "room_message", "direct_message":
				warmObserved = append(warmObserved, r.ob)
			}
		}
		summary.WarmupObserved = len(warmObserved)
		if e := verifyWarmup(warmExpected, warmObserved, disconnects); e != nil {
			return summary, e
		}
	}
	if err = sendPhase(cfg.Duration, cfg.Rate, false); err != nil {
		summary.Failure = err.Error()
	}
	for i := range expected {
		expected[i].Deadline = time.Now().Add(cfg.Drain)
	}
	t := time.NewTimer(cfg.Drain)
	select {
	case <-ctx.Done():
		t.Stop()
		return summary, ctx.Err()
	case <-t.C:
	}
	for len(events) > 0 {
		r := <-events
		switch r.ob.Event.Type {
		case "__disconnect":
			summary.Disconnects++
		case "room_message", "direct_message":
			observed = append(observed, r.ob)
		}
	}
	v := Verify(expected, observed)
	summary.Observed = len(observed)
	summary.Correlated = v.Correlated
	summary.Duplicates = len(v.Duplicate)
	summary.Missing = len(v.Missing)
	summary.WrongRecipient = len(v.WrongRecipient)
	summary.Mismatch = len(v.Mismatch) + len(v.Unknown)
	summary.Latency = Percentiles(v.Latencies)
	summary.SchedulerMaxDelay = 0
	for _, d := range delays {
		if d > summary.SchedulerMaxDelay {
			summary.SchedulerMaxDelay = d
		}
	}
	summary.SchedulerP95Delay = Percentiles(delays)["p95"]
	if err == nil {
		err = v.Err()
	}
	if summary.Disconnects > 0 && err == nil {
		err = errors.New("websocket disconnect during load")
	}
	if summary.SchedulerP95Delay > time.Duration(float64(time.Second)/cfg.Rate) && err == nil {
		err = errors.New("generator missed schedule")
	}
	// Verify only the latest 100 for each chat; zero messages is a failure.
	if err == nil {
		chats := map[int64][]Expected{}
		for _, x := range expected {
			key := x.TargetID
			if cfg.Profile == DMProfile {
				key = x.Recipients[0]
				if x.Recipients[1] < key {
					key = x.Recipients[1]
				}
			}
			chats[key] = append(chats[key], x)
		}
		for target, xs := range chats {
			kind := "rooms"
			owner := users[0]
			if cfg.Profile == RoomProfile {
				found := false
				for _, room := range rooms {
					if room.id == target {
						owner = room.members[0]
						found = true
						break
					}
				}
				if !found {
					err = errors.New("unknown room target")
					break
				}
			}
			if cfg.Profile == DMProfile {
				kind = "conversations"
				var peer int64
				for _, u := range users {
					if u.User.ID == target {
						owner = u
						break
					}
				}
				for _, id := range xs[0].Recipients {
					if id != target {
						peer = id
					}
				}
				convs, e := owner.Conversations(ctx)
				if e != nil {
					err = e
					break
				}
				convID := int64(0)
				for _, cv := range convs {
					if cv.PeerID == peer {
						convID = cv.ID
					}
				}
				if convID == 0 {
					err = errors.New("DM conversation not discovered")
					break
				}
				target = convID
			}
			history, e := owner.History(ctx, kind, target)
			if e != nil {
				err = e
				break
			}
			sort.Slice(xs, func(i, j int) bool { return xs[i].MessageID < xs[j].MessageID })
			if len(xs) > 100 {
				xs = xs[len(xs)-100:]
			}
			if len(history) > len(xs) {
				history = history[:len(xs)]
			}
			if e = VerifyHistory(history, xs, kind, target); e != nil {
				err = e
				break
			}
		}
	}
	if err != nil {
		summary.Failure = err.Error()
	}
	if cfg.FullTracePath != "" {
		if !filepath.IsAbs(cfg.FullTracePath) {
			err = errors.Join(err, errors.New("trace path must be absolute"))
		} else {
			b, e := json.MarshalIndent(struct {
				Expected []Expected    `json:"expected"`
				Observed []Observation `json:"observed"`
				Summary  LoadSummary   `json:"summary"`
			}{expected, observed, summary}, "", "  ")
			if e == nil {
				e = writeUnique(cfg.FullTracePath, b)
			}
			err = errors.Join(err, e)
		}
	}
	if err != nil {
		summary.Failure = err.Error()
	}
	return
}

func verifyWarmup(expected []Expected, observed []Observation, disconnects int) error {
	if disconnects > 0 {
		return fmt.Errorf("warmup websocket disconnects: %d", disconnects)
	}
	if err := Verify(expected, observed).Err(); err != nil {
		return fmt.Errorf("warmup: %w", err)
	}
	return nil
}

// RunSteps stops at the first failing or scheduler-limited step. Preserve each
// result and the original error; a diagnostic replay never replaces either.
func RunSteps(ctx context.Context, cfg LoadConfig) ([]LoadSummary, error) {
	if e := cfg.Validate(); e != nil {
		return nil, e
	}
	results := []LoadSummary{}
	for i := 0; i < cfg.Steps; i++ {
		stepCfg := cfg.stepped(i)
		if i > 0 && stepCfg.FullTracePath != "" {
			stepCfg.FullTracePath = fmt.Sprintf("%s.step-%d", cfg.FullTracePath, i)
		}
		s, e := RunLoad(ctx, stepCfg)
		results = append(results, s)
		if e != nil {
			return results, e
		}
		if i+1 < cfg.Steps {
			next := cfg.stepped(i + 1)
			if next.Rate == stepCfg.Rate && next.Duration == stepCfg.Duration {
				break
			}
		}
	}
	return results, nil
}

// DiagnosticReplay retains the original failure and runs one separate diagnostic job.
func DiagnosticReplay(ctx context.Context, cfg LoadConfig, original LoadSummary, originalErr error) (LoadSummary, error) {
	if originalErr == nil || original.RunID == "" {
		return LoadSummary{}, errors.New("diagnostic replay requires retained original failed run")
	}
	cfg.Steps = 1
	if cfg.FullTracePath != "" {
		cfg.FullTracePath += ".diagnostic"
	}
	replay, e := RunLoad(ctx, cfg)
	return replay, errors.Join(originalErr, e)
}
