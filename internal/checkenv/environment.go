// Package checkenv runs an isolated, disposable server and PostgreSQL for checks.
package checkenv

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const label = "chattui.checkenv.id"
const defaultMemory = int64(2 * 1024 * 1024 * 1024)

// Backend selects the message store; PostgreSQL always retains metadata.
type Backend string

// Supported message backends retain PostgreSQL metadata.
const (
	Postgres Backend = "postgres"
	Badger   Backend = "badger"
)

// Config requires an absolute repository path and a private results directory.
// No credentials are accepted or persisted: every Start generates fresh ones.
type Config struct {
	Repository     string
	ResultsDir     string
	Backend        Backend
	ReadyTimeout   time.Duration
	ServerCPU      float64
	ServerMemory   int64 // bytes
	PostgresCPU    float64
	PostgresMemory int64  // bytes
	SharedImage    string // pinned sha256 image ID, built and owned by caller
}

// CommandResult keeps Docker stdout and stderr separate for redaction.
type CommandResult struct{ Stdout, Stderr string }

// Executor runs Docker commands; tests substitute an ownership-aware fake.
type Executor interface {
	Run(context.Context, ...string) (CommandResult, error)
}
type cli struct{}

func (cli) Run(ctx context.Context, args ...string) (CommandResult, error) {
	c := exec.CommandContext(ctx, "docker", args...)
	var stdout, stderr bytes.Buffer
	c.Stdout, c.Stderr = &stdout, &stderr
	err := c.Run()
	return CommandResult{Stdout: stdout.String(), Stderr: stderr.String()}, err
}

// Resource records inspected container identity, state and effective limits.
type Resource struct {
	Kind         string `json:"kind"`
	Name         string `json:"name"`
	ID           string `json:"id,omitempty"`
	Running      bool   `json:"running,omitempty"`
	RestartCount int    `json:"restart_count,omitempty"`
	OOMKilled    bool   `json:"oom_killed,omitempty"`
	NanoCPUs     int64  `json:"nano_cpus,omitempty"`
	Memory       int64  `json:"memory,omitempty"`
	MemorySwap   int64  `json:"memory_swap,omitempty"`
}

// Snapshot captures resources and inspection failures at a point in time.
type Snapshot struct {
	ID        string     `json:"id"`
	Backend   Backend    `json:"backend"`
	HTTPURL   string     `json:"http_url,omitempty"`
	WSURL     string     `json:"ws_url,omitempty"`
	At        time.Time  `json:"at"`
	Resources []Resource `json:"resources"`
	Errors    []string   `json:"errors,omitempty"`
}

// Environment owns only resources labeled with ID. Snapshot and logs are retained in ResultsDir.
type Environment struct {
	ID, HTTPURL, WSURL, ResultsDir string
	Backend                        Backend
	runner                         Executor
	names                          map[string]string
	secrets                        []string
	mu                             sync.Mutex
	closed                         bool
	sharedImage                    bool
}

// Start creates an owned environment and waits for database and HTTP readiness.
func Start(ctx context.Context, cfg Config) (*Environment, error) { return start(ctx, cfg, cli{}) }

type limits struct {
	CPU    float64
	Memory int64
}

func (l limits) dockerArgs() []string {
	return []string{"--cpus", strconv.FormatFloat(l.CPU, 'f', -1, 64), "--memory", strconv.FormatInt(l.Memory, 10), "--memory-swap", strconv.FormatInt(l.Memory, 10)}
}
func (l limits) evidence() map[string]any {
	return map[string]any{"cpu": l.CPU, "memory_bytes": l.Memory, "memory_swap_bytes": l.Memory}
}
func (l limits) verify(r Resource) error {
	if r.NanoCPUs != int64(math.Round(l.CPU*1e9)) || r.Memory != l.Memory || r.MemorySwap != l.Memory {
		return fmt.Errorf("%s container limits differ from requested limits", r.Kind)
	}
	return nil
}
func configuredLimits(cpu float64, memory int64) (limits, error) {
	if cpu == 0 {
		cpu = 2
	}
	if memory == 0 {
		memory = defaultMemory
	}
	if math.IsNaN(cpu) || math.IsInf(cpu, 0) || cpu <= 0 || cpu*1e9 >= float64(math.MaxInt64) || memory <= 0 {
		return limits{}, errors.New("CPU and memory limits must be finite positive values")
	}
	return limits{CPU: cpu, Memory: memory}, nil
}
func start(ctx context.Context, cfg Config, x Executor) (env *Environment, err error) {
	serverLimits, e := configuredLimits(cfg.ServerCPU, cfg.ServerMemory)
	if e != nil {
		return nil, fmt.Errorf("server limits: %w", e)
	}
	postgresLimits, e := configuredLimits(cfg.PostgresCPU, cfg.PostgresMemory)
	if e != nil {
		return nil, fmt.Errorf("postgres limits: %w", e)
	}
	if cfg.SharedImage != "" {
		if len(cfg.SharedImage) != 71 || !strings.HasPrefix(cfg.SharedImage, "sha256:") {
			return nil, errors.New("shared image must be a pinned sha256 image ID")
		}
		if _, e := hex.DecodeString(strings.TrimPrefix(cfg.SharedImage, "sha256:")); e != nil {
			return nil, errors.New("invalid shared image ID")
		}
	}
	if cfg.Backend != Postgres && cfg.Backend != Badger {
		return nil, errors.New("backend must be postgres or badger")
	}
	if !filepath.IsAbs(cfg.Repository) || !filepath.IsAbs(cfg.ResultsDir) || cfg.Repository == "" || cfg.ResultsDir == "" {
		return nil, errors.New("repository and results directory must be absolute")
	}
	if _, e := os.Stat(filepath.Join(cfg.Repository, "Dockerfile")); e != nil {
		return nil, fmt.Errorf("stat Dockerfile: %w", e)
	}
	if e := os.MkdirAll(cfg.ResultsDir, 0700); e != nil {
		return nil, e
	}
	info, e := os.Lstat(cfg.ResultsDir)
	if e != nil {
		return nil, e
	}
	if !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("results directory must be private")
	}
	entries, e := os.ReadDir(cfg.ResultsDir)
	if e != nil {
		return nil, e
	}
	if len(entries) != 0 {
		return nil, errors.New("results directory must be empty to preserve earlier runs")
	}
	id, e := random(12)
	if e != nil {
		return nil, e
	}
	pwd, e := random(32)
	if e != nil {
		return nil, e
	}
	jwt, e := random(32)
	if e != nil {
		return nil, e
	}
	env = &Environment{ID: id, Backend: cfg.Backend, ResultsDir: cfg.ResultsDir, runner: x, secrets: []string{pwd, jwt}, sharedImage: cfg.SharedImage != "", names: map[string]string{"network": "chattui-check-" + id, "postgres-volume": "chattui-check-pg-" + id, "badger-volume": "chattui-check-badger-" + id, "postgres": "chattui-check-pg-" + id, "server": "chattui-check-server-" + id, "migration": "chattui-check-migrate-" + id, "image": "chattui-check:" + id}}
	if env.sharedImage {
		env.names["image"] = cfg.SharedImage
	}
	phase := "resource creation"
	defer func(created *Environment) {
		if err != nil {
			_ = created.saveLog("startup-error.log", phase+": "+created.redact(err.Error())+"\n")
			_, _ = created.Collect()
			cleanCtx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
			defer cancel()
			if cleanupErr := created.Close(cleanCtx); cleanupErr != nil {
				err = errors.Join(err, cleanupErr)
				_ = created.saveLog("cleanup-error.log", created.redact(cleanupErr.Error())+"\n")
			}
		}
	}(env)
	if e = env.save("config.json", map[string]any{"id": id, "backend": cfg.Backend, "repository": cfg.Repository, "resource_names": env.names, "server_limits": serverLimits.evidence(), "postgres_limits": postgresLimits.evidence()}); e != nil {
		return nil, e
	}
	for _, args := range [][]string{{"network", "create", "--label", label + "=" + id, env.names["network"]}, {"volume", "create", "--label", label + "=" + id, env.names["postgres-volume"]}, {"volume", "create", "--label", label + "=" + id, env.names["badger-volume"]}} {
		if _, e = env.run(ctx, args...); e != nil {
			return nil, e
		}
	}
	// The repository's .dockerignore excludes .env; verify it before using the existing Dockerfile.
	ignored, e := os.ReadFile(filepath.Join(cfg.Repository, ".dockerignore"))
	if e != nil {
		return nil, fmt.Errorf("dockerignore: %w", e)
	}
	safe := false
	for _, line := range strings.Split(string(ignored), "\n") {
		if strings.TrimSpace(line) == ".env" {
			safe = true
		}
	}
	if !safe {
		return nil, errors.New(".dockerignore must exclude .env")
	}
	if !env.sharedImage {
		if _, imageErr := env.run(ctx, "image", "inspect", env.names["image"]); imageErr == nil {
			return nil, errors.New("generated image tag already exists")
		}
		phase = "image build"
		buildResult, buildErr := env.run(ctx, "build", "--label", label+"="+id, "-t", env.names["image"], cfg.Repository)
		if e = env.saveLog("build.log", env.redact(buildResult.Stdout+buildResult.Stderr)); e != nil {
			return nil, e
		}
		if buildErr != nil {
			return nil, buildErr
		}
	} else {
		phase = "shared image verification"
		image, inspectErr := env.run(ctx, "image", "inspect", "--format", "{{.Id}}", cfg.SharedImage)
		if inspectErr != nil || strings.TrimSpace(image.Stdout) != cfg.SharedImage {
			return nil, errors.New("pinned shared image ID unavailable")
		}
	}
	pgEnv, e := env.envFile("POSTGRES_USER=chat\nPOSTGRES_DB=chattui\nPOSTGRES_PASSWORD=" + pwd + "\n")
	if e != nil {
		return nil, e
	}
	defer func() { err = errors.Join(err, os.Remove(pgEnv)) }()
	phase = "postgres startup"
	pg := []string{"run", "-d", "--name", env.names["postgres"], "--label", label + "=" + id, "--network", env.names["network"]}
	pg = append(pg, postgresLimits.dockerArgs()...)
	pg = append(pg, "--restart", "no", "--mount", "type=volume,source="+env.names["postgres-volume"]+",target=/var/lib/postgresql/data", "--env-file", pgEnv, "postgres:17-alpine")
	if _, e = env.run(ctx, pg...); e != nil {
		return nil, e
	}
	dbURL := "postgres://chat:" + pwd + "@" + env.names["postgres"] + ":5432/chattui?sslmode=disable"
	serverEnv, e := env.envFile("DB_URL=" + dbURL + "\nJWT_SECRET=" + jwt + "\nMESSAGE_STORE=" + string(cfg.Backend) + "\nBADGER_PATH=/data/messages.badger\nPORT=8080\n")
	if e != nil {
		return nil, e
	}
	defer func() { err = errors.Join(err, os.Remove(serverEnv)) }()
	timeout := cfg.ReadyTimeout
	if timeout <= 0 {
		timeout = 90 * time.Second
	}
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if e = env.waitDB(waitCtx); e != nil {
		return nil, e
	}
	phase = "migrations"
	migration := []string{"run", "--rm", "--name", "chattui-check-migrate-" + id, "--label", label + "=" + id, "--network", env.names["network"], "--env-file", serverEnv, env.names["image"], "sh", "-c", "goose -dir /migrations postgres \"$DB_URL\" up"}
	migrationResult, migrateErr := env.run(ctx, migration...)
	if migrateErr != nil {
		return nil, migrateErr
	}
	if e = env.saveLog("migrations.log", env.redact(migrationResult.Stdout+migrationResult.Stderr)); e != nil {
		return nil, e
	}
	phase = "server startup"
	hostPort, reserveErr := reservePort()
	if reserveErr != nil {
		return nil, reserveErr
	}
	server := []string{"run", "-d", "--name", env.names["server"], "--label", label + "=" + id, "--network", env.names["network"]}
	server = append(server, serverLimits.dockerArgs()...)
	server = append(server, "--restart", "no", "--mount", "type=volume,source="+env.names["badger-volume"]+",target=/data", "--env-file", serverEnv, "-p", "127.0.0.1:"+strconv.Itoa(hostPort)+":8080", env.names["image"])
	if _, e = env.run(ctx, server...); e != nil {
		return nil, e
	}
	if e = env.port(ctx); e != nil {
		return nil, e
	}
	if env.HTTPURL != "http://127.0.0.1:"+strconv.Itoa(hostPort) {
		return nil, errors.New("published port differs from reserved port")
	}
	for _, requested := range []struct {
		kind   string
		limits limits
	}{{"postgres", postgresLimits}, {"server", serverLimits}} {
		resource, inspectErr := env.inspect(ctx, requested.kind)
		if inspectErr != nil {
			return nil, inspectErr
		}
		if e = requested.limits.verify(resource); e != nil {
			return nil, e
		}
	}
	phase = "HTTP readiness"
	if e = env.waitHTTP(waitCtx); e != nil {
		return nil, e
	}
	_, e = env.Collect()
	if e != nil {
		return nil, e
	}
	return env, nil
}

// Docker cannot accept an open listener; another process claiming the port
// between reservation and docker run causes startup to fail, never a retarget.
func reservePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		return 0, err
	}
	return port, nil
}
func random(n int) (string, error) {
	b := make([]byte, n)
	if _, e := rand.Read(b); e != nil {
		return "", e
	}
	return hex.EncodeToString(b), nil
}
func (e *Environment) run(ctx context.Context, args ...string) (CommandResult, error) {
	r, err := e.runner.Run(ctx, args...)
	if err != nil {
		return CommandResult{Stdout: e.redact(r.Stdout), Stderr: e.redact(r.Stderr)}, fmt.Errorf("docker %s: %s: %w", strings.Join(args[:min(2, len(args))], " "), e.redact(r.Stderr), err)
	}
	return r, nil
}
func (e *Environment) redact(s string) string {
	for _, secret := range e.secrets {
		if secret != "" {
			s = strings.ReplaceAll(s, secret, "[REDACTED]")
		}
	}
	return s
}
func (e *Environment) envFile(text string) (name string, err error) {
	f, err := os.CreateTemp(e.ResultsDir, ".docker-env-")
	if err != nil {
		return "", err
	}
	defer func() {
		err = errors.Join(err, f.Close())
		if err != nil {
			err = errors.Join(err, os.Remove(f.Name()))
			name = ""
		}
	}()
	if err = f.Chmod(0600); err != nil {
		return "", err
	}
	if _, err = f.WriteString(text); err != nil {
		return "", err
	}
	return f.Name(), nil
}
func (e *Environment) waitDB(ctx context.Context) error {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		if _, err := e.run(ctx, "exec", e.names["postgres"], "pg_isready", "-U", "chat", "-d", "chattui"); err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
		}
	}
}
func (e *Environment) port(ctx context.Context) error {
	r, err := e.run(ctx, "port", e.names["server"], "8080/tcp")
	if err != nil {
		return err
	}
	host, port, err := net.SplitHostPort(strings.TrimSpace(r.Stdout))
	if err != nil {
		return err
	}
	if host != "127.0.0.1" {
		return fmt.Errorf("unexpected published host %q", host)
	}
	if _, err = strconv.Atoi(port); err != nil {
		return err
	}
	publishedURL := "http://127.0.0.1:" + port
	if e.HTTPURL != "" && e.HTTPURL != publishedURL {
		return fmt.Errorf("server published port changed across restart")
	}
	e.HTTPURL = publishedURL
	e.WSURL = "ws://127.0.0.1:" + port + "/api/v1/ws"
	return nil
}
func (e *Environment) waitHTTP(ctx context.Context) error {
	t := time.NewTicker(300 * time.Millisecond)
	defer t.Stop()
	client := http.Client{Timeout: time.Second}
	for {
		req, err := http.NewRequestWithContext(ctx, "GET", e.HTTPURL+"/healthz", nil)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err == nil {
			if err := resp.Body.Close(); err != nil {
				return err
			}
			if resp.StatusCode == 200 {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
		}
	}
}

// StopServer stops the writer while retaining its storage and endpoint.
func (e *Environment) StopServer(ctx context.Context) error {
	_, err := e.run(ctx, "stop", "-t", "12", e.names["server"])
	_, _ = e.Collect()
	return err
}

// StartServer restarts the retained container and verifies readiness and endpoint.
func (e *Environment) StartServer(ctx context.Context) error {
	_, err := e.run(ctx, "start", e.names["server"])
	if err != nil {
		return err
	}
	if err := e.port(ctx); err != nil {
		return err
	}
	if err := e.waitHTTP(ctx); err != nil {
		return err
	}
	_, err = e.Collect()
	return err
}

// RestartServer stops and starts the same server without resetting its dataset.
func (e *Environment) RestartServer(ctx context.Context) error {
	if err := e.StopServer(ctx); err != nil {
		return err
	}
	return e.StartServer(ctx)
}
func (e *Environment) save(name string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(e.ResultsDir, name), append(b, '\n'), 0600)
}
func (e *Environment) inspect(ctx context.Context, kind string) (Resource, error) {
	name := e.names[kind]
	r, err := e.run(ctx, "inspect", name)
	if err != nil {
		return Resource{Kind: kind, Name: name}, err
	}
	var values []struct {
		ID     string
		Config struct{ Labels map[string]string }
		State  struct {
			Running    bool
			Restarting bool
			OOMKilled  bool
		}
		RestartCount int
		HostConfig   struct {
			NanoCpus   int64
			Memory     int64
			MemorySwap int64
		}
	}
	if err = json.Unmarshal([]byte(r.Stdout), &values); err != nil || len(values) != 1 {
		return Resource{}, errors.New("invalid docker inspect response")
	}
	v := values[0]
	if v.Config.Labels[label] != e.ID {
		return Resource{}, fmt.Errorf("resource %s not owned by run", name)
	}
	return Resource{Kind: kind, Name: name, ID: v.ID, Running: v.State.Running, RestartCount: v.RestartCount, OOMKilled: v.State.OOMKilled, NanoCPUs: v.HostConfig.NanoCpus, Memory: v.HostConfig.Memory, MemorySwap: v.HostConfig.MemorySwap}, nil
}
func (e *Environment) owned(ctx context.Context, kind string) bool {
	r, err := e.run(ctx, "inspect", "--format", "{{ index .Config.Labels \""+label+"\" }}", e.names[kind])
	return err == nil && strings.TrimSpace(r.Stdout) == e.ID
}
func (e *Environment) ownedOther(ctx context.Context, kind string) bool {
	if e.names[kind] == "" {
		return false
	}
	typeName := "volume"
	if kind == "network" {
		typeName = "network"
	}
	if kind == "image" {
		typeName = "image"
	}
	path := ".Labels"
	if kind == "image" {
		path = ".Config.Labels"
	}
	r, err := e.run(ctx, typeName, "inspect", "--format", "{{ index "+path+" \""+label+"\" }}", e.names[kind])
	return err == nil && strings.TrimSpace(r.Stdout) == e.ID
}

// Collect records current container limits, restart/OOM state and redacted logs.
func (e *Environment) Collect() (Snapshot, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	s := Snapshot{ID: e.ID, Backend: e.Backend, HTTPURL: e.HTTPURL, WSURL: e.WSURL, At: time.Now().UTC()}
	stamp := strconv.FormatInt(s.At.UnixNano(), 10)
	var errs []error
	for _, k := range []string{"postgres", "server"} {
		v, err := e.inspect(ctx, k)
		if err != nil {
			s.Errors = append(s.Errors, e.redact(err.Error()))
			errs = append(errs, err)
		}
		s.Resources = append(s.Resources, v)
		if e.owned(ctx, k) {
			r, er := e.run(ctx, "logs", "--timestamps", e.names[k])
			if er != nil {
				errs = append(errs, er)
			} else {
				text := e.redact(r.Stdout + r.Stderr)
				if er = e.saveLog(k+"-"+stamp+".log", text); er != nil {
					errs = append(errs, er)
				}
				if er = e.saveLog(k+".log", text); er != nil {
					errs = append(errs, er)
				}
			}
		}
	}
	for _, kind := range []string{"network", "postgres-volume", "badger-volume", "image"} {
		if kind == "image" && e.sharedImage {
			continue
		}
		if e.ownedOther(ctx, kind) {
			s.Resources = append(s.Resources, Resource{Kind: kind, Name: e.names[kind]})
		}
	}
	if err := e.save("state-"+stamp+".json", s); err != nil {
		errs = append(errs, err)
	}
	if err := e.save("state.json", s); err != nil {
		errs = append(errs, err)
	}
	return s, errors.Join(errs...)
}
func (e *Environment) saveLog(name, text string) error {
	return os.WriteFile(filepath.Join(e.ResultsDir, name), []byte(text), 0600)
}

// Close keeps evidence and removes only labeled resources for this run. It uses
// an independent bounded context even if the caller was canceled.
func (e *Environment) Close(_ context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return nil
	}
	_, _ = e.Collect()
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	var errs []error
	for _, k := range []string{"server", "postgres", "migration"} {
		if e.owned(ctx, k) {
			if _, err := e.run(ctx, "rm", "-f", e.names[k]); err != nil {
				errs = append(errs, err)
			}
		}
	}
	for _, k := range []string{"badger-volume", "postgres-volume"} {
		if e.ownedOther(ctx, k) {
			if _, err := e.run(ctx, "volume", "rm", e.names[k]); err != nil {
				errs = append(errs, err)
			}
		}
	}
	if e.ownedOther(ctx, "network") {
		if _, err := e.run(ctx, "network", "rm", e.names["network"]); err != nil {
			errs = append(errs, err)
		}
	}
	if !e.sharedImage && e.ownedOther(ctx, "image") {
		if _, err := e.run(ctx, "image", "rm", e.names["image"]); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) == 0 {
		e.closed = true
	}
	return errors.Join(errs...)
}
