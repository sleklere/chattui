package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/sleklere/chattui/internal/checkenv"
	"github.com/sleklere/chattui/internal/checkproto"
	"github.com/sleklere/chattui/internal/checktui"
)

type options struct {
	mode, backend, profile, results, repository, clientBinary, existingHTTP, existingWS string
	seed                                                                                int64
	repeats, users, steps, rooms, membersPerRoom                                        int
	rate, maxRate, serverCPU, postgresCPU                                               float64
	duration, maxDuration, warmup, drain, window, tuiTimeout                            time.Duration
	serverMemory, postgresMemory                                                        int64
	fullTrace, diagnostic                                                               bool
}

func parse(args []string) (options, error) {
	var o options
	f := flag.NewFlagSet("check", flag.ContinueOnError)
	f.SetOutput(io.Discard)
	f.StringVar(&o.mode, "mode", "e2e", "e2e, protocol, load, compare")
	f.StringVar(&o.backend, "backend", "both", "postgres, badger, both")
	f.StringVar(&o.profile, "profile", "room", "room or dm (load and compare)")
	f.StringVar(&o.results, "results", "", "new absolute private results directory")
	f.StringVar(&o.repository, "repository", "", "absolute repository directory")
	f.StringVar(&o.clientBinary, "client-binary", "", "prebuilt client binary (default: build outside measurement)")
	f.StringVar(&o.existingHTTP, "existing-http", "", "opt-in existing loopback server, load only")
	f.StringVar(&o.existingWS, "existing-ws", "", "existing server WebSocket endpoint")
	f.Int64Var(&o.seed, "seed", 17, "logical fixture seed")
	f.IntVar(&o.repeats, "repeats", 2, "comparison repetitions per backend")
	f.IntVar(&o.users, "users", 2, "load users")
	f.IntVar(&o.rooms, "rooms", 0, "room count, 0 defaults to one (room profile)")
	f.IntVar(&o.membersPerRoom, "members-per-room", 0, "members per room, 0 defaults to all users (room profile)")
	f.IntVar(&o.steps, "steps", 1, "bounded load steps")
	f.Float64Var(&o.rate, "rate", 2, "initial offered messages per second")
	f.Float64Var(&o.maxRate, "max-rate", 2, "maximum offered messages per second")
	f.DurationVar(&o.duration, "duration", 2*time.Second, "initial measured duration")
	f.DurationVar(&o.maxDuration, "max-duration", 2*time.Second, "maximum measured duration")
	f.DurationVar(&o.warmup, "warmup", time.Second, "unmeasured warmup")
	f.DurationVar(&o.drain, "drain", time.Second, "receive drain")
	f.DurationVar(&o.window, "window", 500*time.Millisecond, "protocol receive window")
	f.DurationVar(&o.tuiTimeout, "tui-timeout", 90*time.Second, "TUI interaction timeout")
	f.Float64Var(&o.serverCPU, "server-cpu", 2, "server CPU quota")
	f.Float64Var(&o.postgresCPU, "postgres-cpu", 2, "Postgres CPU quota")
	f.Int64Var(&o.serverMemory, "server-memory", 2*1024*1024*1024, "server memory bytes (also swap limit)")
	f.Int64Var(&o.postgresMemory, "postgres-memory", 2*1024*1024*1024, "Postgres memory bytes (also swap limit)")
	f.BoolVar(&o.fullTrace, "full-trace", false, "record all load events, instead of errors and metrics only")
	f.BoolVar(&o.diagnostic, "diagnostic", false, "one separately recorded rerun after load failure; original failure retained")
	if err := f.Parse(args); err != nil {
		return o, err
	}
	if f.NArg() != 0 {
		return o, errors.New("unexpected positional arguments")
	}
	if !filepath.IsAbs(o.results) || !filepath.IsAbs(o.repository) {
		return o, errors.New("--results and --repository must be absolute")
	}
	if o.seed < 0 {
		return o, errors.New("seed must be nonnegative")
	}
	var unsupported string
	f.Visit(func(x *flag.Flag) {
		if (o.mode == "e2e" || o.mode == "protocol") && (x.Name == "profile" || x.Name == "users" || x.Name == "rooms" || x.Name == "members-per-room" || x.Name == "rate" || x.Name == "max-rate" || x.Name == "duration" || x.Name == "max-duration" || x.Name == "warmup" || x.Name == "drain" || x.Name == "steps" || x.Name == "repeats" || x.Name == "full-trace" || x.Name == "diagnostic") {
			unsupported = x.Name
		}
		if o.mode != "e2e" && (x.Name == "client-binary" || x.Name == "tui-timeout") {
			unsupported = x.Name
		}
		if o.mode != "compare" && x.Name == "repeats" {
			unsupported = x.Name
		}
	})
	if unsupported != "" {
		return o, fmt.Errorf("--%s is not used in %s mode", unsupported, o.mode)
	}
	if o.mode != "e2e" && o.mode != "protocol" && o.mode != "load" && o.mode != "compare" {
		return o, errors.New("invalid mode")
	}
	if o.backend != "postgres" && o.backend != "badger" && o.backend != "both" {
		return o, errors.New("invalid backend")
	}
	if o.repeats < 1 || o.repeats > 5 || o.window < 100*time.Millisecond || o.window > 30*time.Second || o.tuiTimeout <= 0 || o.tuiTimeout > 5*time.Minute {
		return o, errors.New("invalid repetition or timeout bounds")
	}
	if (o.existingHTTP == "") != (o.existingWS == "") {
		return o, errors.New("both existing endpoints required")
	}
	if o.existingHTTP != "" && o.mode != "load" {
		return o, errors.New("existing server supports partial load mode only; restart and TUI require an owned environment")
	}
	if o.existingHTTP != "" {
		h, e := url.Parse(o.existingHTTP)
		if e != nil {
			return o, e
		}
		w, e := url.Parse(o.existingWS)
		if e != nil {
			return o, e
		}
		if h.Scheme != "http" || h.Path != "" || w.Scheme != "ws" || w.Path != "/api/v1/ws" || h.Host != w.Host {
			return o, errors.New("existing endpoints require matching loopback http/ws hosts and /api/v1/ws path")
		}
	}
	if o.existingHTTP != "" && o.backend == "both" {
		return o, errors.New("select the known existing backend explicitly")
	}
	if o.mode == "compare" && o.backend != "both" {
		return o, errors.New("compare requires both backends")
	}
	if o.mode == "e2e" && o.clientBinary != "" {
		if s, e := os.Stat(o.clientBinary); !filepath.IsAbs(o.clientBinary) || e != nil || !s.Mode().IsRegular() || s.Mode()&0111 == 0 {
			return o, errors.New("client binary must be executable")
		}
	}
	if o.mode == "load" || o.mode == "compare" {
		c := o.loadConfig("http://127.0.0.1:8080", "ws://127.0.0.1:8080/api/v1/ws", o.results)
		if o.existingHTTP != "" {
			c.HTTPURL = o.existingHTTP
			c.WSURL = o.existingWS
		}
		if err := c.Validate(); err != nil {
			return o, err
		}
	}
	if o.serverCPU <= 0 || o.postgresCPU <= 0 || math.IsNaN(o.serverCPU) || math.IsInf(o.serverCPU, 0) || math.IsNaN(o.postgresCPU) || math.IsInf(o.postgresCPU, 0) || o.serverCPU*1e9 >= float64(math.MaxInt64) || o.postgresCPU*1e9 >= float64(math.MaxInt64) || o.serverMemory <= 0 || o.postgresMemory <= 0 {
		return o, errors.New("container limits must be positive")
	}
	if o.mode == "e2e" || o.mode == "protocol" {
		if o.existingHTTP != "" {
			return o, errors.New("E2E requires owned lifecycle")
		}
	}
	return o, nil
}
func (o options) loadConfig(http, ws, dir string) checkproto.LoadConfig {
	return checkproto.LoadConfig{HTTPURL: http, WSURL: ws, ResultsDir: dir, Profile: checkproto.Profile(o.profile), Users: o.users, Rooms: o.rooms, MembersPerRoom: o.membersPerRoom, Rate: o.rate, Duration: o.duration, Warmup: o.warmup, Drain: o.drain, Seed: o.seed, Steps: o.steps, MaxRate: o.maxRate, MaxDuration: o.maxDuration}
}
func (o options) backends() []checkenv.Backend {
	if o.backend == "both" {
		return []checkenv.Backend{checkenv.Postgres, checkenv.Badger}
	}
	return []checkenv.Backend{checkenv.Backend(o.backend)}
}

type job struct {
	Backend    checkenv.Backend `json:"backend"`
	Profile    string           `json:"profile,omitempty"`
	Repetition int              `json:"repetition"`
	Step       int              `json:"step"`
	Rate       float64          `json:"rate,omitempty"`
	Duration   time.Duration    `json:"duration,omitempty"`
}

func (o options) jobs() []job {
	var out []job
	if o.mode == "compare" {
		for repeat := 0; repeat < o.repeats; repeat++ {
			bs := o.backends()
			if repeat%2 == 1 {
				bs[0], bs[1] = bs[1], bs[0]
			}
			for _, b := range bs {
				for step := 0; step < o.steps; step++ {
					rate := o.rate
					duration := o.duration
					for i := 0; i < step; i++ {
						rate = min(o.maxRate, rate*2)
						duration = min(o.maxDuration, duration*2)
					}
					out = append(out, job{Backend: b, Profile: o.profile, Repetition: repeat + 1, Step: step + 1, Rate: rate, Duration: duration})
				}
			}
		}
		return out
	}
	for _, b := range o.backends() {
		if o.mode == "load" {
			for step := 0; step < o.steps; step++ {
				rate := o.rate
				duration := o.duration
				for i := 0; i < step; i++ {
					rate = min(o.maxRate, rate*2)
					duration = min(o.maxDuration, duration*2)
				}
				out = append(out, job{Backend: b, Profile: o.profile, Repetition: 1, Step: step + 1, Rate: rate, Duration: duration})
			}
		} else {
			out = append(out, job{Backend: b, Repetition: 1})
		}
	}
	return out
}
func run(ctx context.Context, o options) (err error) {
	if o.existingHTTP == "" {
		if _, e := os.Stat(filepath.Join(o.repository, "Dockerfile")); e != nil {
			return fmt.Errorf("repository Dockerfile: %w", e)
		}
		ignored, e := os.ReadFile(filepath.Join(o.repository, ".dockerignore"))
		if e != nil {
			return fmt.Errorf("repository .dockerignore: %w", e)
		}
		found := false
		for _, line := range strings.Split(string(ignored), "\n") {
			if strings.TrimSpace(line) == ".env" {
				found = true
			}
		}
		if !found {
			return errors.New("repository .dockerignore must exclude .env")
		}
	}
	if err := os.Mkdir(o.results, 0700); err != nil {
		return err
	}
	st, err := os.Stat(o.results)
	if err != nil {
		return err
	}
	if st.Mode().Perm()&0077 != 0 {
		return errors.New("results directory must be private")
	}
	entries, err := os.ReadDir(o.results)
	if err != nil {
		return err
	}
	if len(entries) != 0 {
		return errors.New("results directory must be empty")
	}
	jobs := o.jobs()
	if err := writeJSON(filepath.Join(o.results, "plan.json"), struct {
		Mode           string        `json:"mode"`
		Seed           int64         `json:"seed"`
		Existing       bool          `json:"existing_server_partial"`
		Jobs           []job         `json:"jobs"`
		ServerCPU      float64       `json:"server_cpu"`
		PostgresCPU    float64       `json:"postgres_cpu"`
		ServerMemory   int64         `json:"server_memory_bytes"`
		PostgresMemory int64         `json:"postgres_memory_bytes"`
		Users          int           `json:"users"`
		Rooms          int           `json:"rooms"`
		MembersPerRoom int           `json:"members_per_room"`
		Warmup         time.Duration `json:"warmup"`
		Drain          time.Duration `json:"drain"`
		MaxRate        float64       `json:"max_rate"`
		MaxDuration    time.Duration `json:"max_duration"`
		Window         time.Duration `json:"protocol_window"`
		TUITimeout     time.Duration `json:"tui_timeout"`
		FullTrace      bool          `json:"load_full_trace"`
		Diagnostic     bool          `json:"diagnostic_rerun"`
		Repository     string        `json:"repository"`
		ClientBinary   string        `json:"client_binary,omitempty"`
		ExistingHTTP   string        `json:"existing_http,omitempty"`
		ExistingWS     string        `json:"existing_ws,omitempty"`
	}{o.mode, o.seed, o.existingHTTP != "", jobs, o.serverCPU, o.postgresCPU, o.serverMemory, o.postgresMemory, o.users, o.rooms, o.membersPerRoom, o.warmup, o.drain, o.maxRate, o.maxDuration, o.window, o.tuiTimeout, o.fullTrace, o.diagnostic, o.repository, o.clientBinary, o.existingHTTP, o.existingWS}); err != nil {
		return err
	}
	binary := o.clientBinary
	if o.mode == "e2e" && binary == "" {
		binary = filepath.Join(o.results, "client")
		if err := buildClient(ctx, o.repository, binary, filepath.Join(o.results, "client-build.log")); err != nil {
			return err
		}
	}
	var shared imageRecord
	if o.existingHTTP == "" && len(jobs) > 1 {
		shared, err = buildSharedImage(ctx, o.repository, o.results)
		if err != nil {
			return err
		}
		defer func() { err = errors.Join(err, removeSharedImage(shared)) }()
	}
	for i, j := range jobs {
		dir := filepath.Join(o.results, fmt.Sprintf("job-%02d-%s-r%d-s%d", i+1, j.Backend, j.Repetition, j.Step))
		if err := os.Mkdir(dir, 0700); err != nil {
			return err
		}
		if err := runJob(ctx, o, j, dir, binary, shared.ID); err != nil {
			return fmt.Errorf("job %d (%s): %w; results %s", i+1, j.Backend, err, dir)
		}
	}
	return nil
}
func runJob(ctx context.Context, o options, j job, dir, binary, imageID string) (err error) {
	var env *checkenv.Environment
	httpURL, wsURL := o.existingHTTP, o.existingWS
	if httpURL == "" {
		env, err = checkenv.Start(ctx, checkenv.Config{Repository: o.repository, ResultsDir: filepath.Join(dir, "environment"), Backend: j.Backend, ServerCPU: o.serverCPU, PostgresCPU: o.postgresCPU, ServerMemory: o.serverMemory, PostgresMemory: o.postgresMemory, SharedImage: imageID})
		if err != nil {
			return err
		}
		httpURL, wsURL = env.HTTPURL, env.WSURL
		defer func() { err = errors.Join(err, env.Close(context.Background())) }()
		if imageID != "" {
			snap, e := env.Collect()
			if e != nil {
				return e
			}
			found := false
			for _, r := range snap.Resources {
				if r.Kind == "server" {
					found = true
					if e = verifyServerImage(ctx, r.ID, imageID, dir); e != nil {
						return e
					}
				}
			}
			if !found {
				return errors.New("server missing from Docker inspect")
			}
		}
	}
	defer func() {
		if env != nil {
			_, e := env.Collect()
			err = errors.Join(err, e)
		}
	}()
	if o.mode == "e2e" || o.mode == "protocol" {
		_, err = checkproto.RunE2E(ctx, checkproto.E2EConfig{HTTPURL: httpURL, WSURL: wsURL, Seed: o.seed, Window: o.window, TracePath: filepath.Join(dir, "protocol-trace.json"), Lifecycle: checkproto.Lifecycle{Stop: env.StopServer, Start: env.StartServer}})
		if err != nil {
			return err
		}
		if o.mode == "e2e" {
			tuiDir := filepath.Join(dir, "tui")
			if err = os.Mkdir(tuiDir, 0700); err != nil {
				return err
			}
			_, err = checktui.Run(ctx, checktui.Config{ClientBinary: binary, HTTPURL: httpURL, WSURL: wsURL, Seed: strconv.FormatInt(o.seed, 10), ResultsDir: tuiDir, Timeout: o.tuiTimeout, LifecycleStop: env.StopServer, LifecycleStart: env.StartServer})
		}
		return err
	}
	cfg := o.loadConfig(httpURL, wsURL, dir)
	cfg.Rate = j.Rate
	cfg.Duration = j.Duration
	cfg.Steps = 1
	if o.fullTrace {
		cfg.FullTracePath = filepath.Join(dir, "load-trace.json")
	}
	var stop func() error
	if env != nil {
		stop = startSampling(env, dir)
	}
	var before, after syscall.Rusage
	if e := syscall.Getrusage(syscall.RUSAGE_SELF, &before); e != nil {
		return e
	}
	summary, loadErr := checkproto.RunLoad(ctx, cfg)
	originalLoadErr := loadErr
	if e := syscall.Getrusage(syscall.RUSAGE_SELF, &after); e != nil {
		loadErr = errors.Join(loadErr, e)
	} else {
		loadErr = errors.Join(loadErr, writeJSON(filepath.Join(dir, "generator.json"), struct {
			UserCPUSeconds   float64 `json:"user_cpu_seconds"`
			SystemCPUSeconds float64 `json:"system_cpu_seconds"`
			PeakRSSBytes     int64   `json:"peak_rss_bytes"`
			MaxHeapBytes     uint64  `json:"max_heap_bytes"`
			MaxGoroutines    int     `json:"max_goroutines"`
		}{timevalSeconds(after.Utime) - timevalSeconds(before.Utime), timevalSeconds(after.Stime) - timevalSeconds(before.Stime), after.Maxrss * 1024, summary.GeneratorMaxHeap, summary.GeneratorMaxGoroutines}))
	}
	if stop != nil {
		err = stop()
	}
	if originalLoadErr != nil && o.diagnostic && summary.RunID != "" {
		replayCfg := cfg
		replayCfg.FullTracePath = filepath.Join(dir, "diagnostic-trace.json")
		_, replayErr := checkproto.DiagnosticReplay(ctx, replayCfg, summary, originalLoadErr)
		err = errors.Join(err, replayErr)
	}
	return errors.Join(err, loadErr)
}
func timevalSeconds(t syscall.Timeval) float64 { return float64(t.Sec) + float64(t.Usec)/1e6 }
func main() {
	o, err := parse(os.Args[1:])
	if err == nil {
		err = run(context.Background(), o)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "check:", err)
		os.Exit(1)
	}
	fmt.Println("PASS; results:", strings.TrimSpace(o.results))
}
