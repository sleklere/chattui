package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sleklere/chattui/internal/checkenv"
)

func writeJSON(path string, v any) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	return os.WriteFile(path, append(b, '\n'), 0600)
}
func buildClient(ctx context.Context, repo, binary, log string) error {
	c := exec.CommandContext(ctx, "go", "build", "-o", binary, "./cmd/client")
	c.Dir = repo
	output, err := c.CombinedOutput()
	// Build output can include local paths and diagnostics; keep it in the private run directory.
	if e := os.WriteFile(log, output, 0600); e != nil {
		return e
	}
	if err != nil {
		return fmt.Errorf("client build failed: %w; see %s", err, log)
	}
	return nil
}

type sample struct {
	At                    time.Time `json:"at"`
	Container             string    `json:"container"`
	CPUPercent            float64   `json:"cpu_percent"`
	MemoryBytes           uint64    `json:"memory_bytes"`
	ThrottledPeriods      uint64    `json:"throttled_periods"`
	ThrottledMicroseconds uint64    `json:"throttled_microseconds"`
}

func memoryBytes(text string) (uint64, error) {
	parts := strings.Fields(strings.TrimSpace(strings.Split(text, "/")[0]))
	if len(parts) != 1 {
		return 0, errors.New("unexpected docker memory format")
	}
	t := parts[0]
	units := []struct {
		suffix string
		factor float64
	}{{"GiB", 1 << 30}, {"MiB", 1 << 20}, {"KiB", 1 << 10}, {"GB", 1e9}, {"MB", 1e6}, {"kB", 1e3}, {"B", 1}}
	for _, u := range units {
		if strings.HasSuffix(t, u.suffix) {
			v, e := strconv.ParseFloat(strings.TrimSuffix(t, u.suffix), 64)
			if e != nil || v < 0 {
				return 0, errors.New("invalid memory value")
			}
			return uint64(v * u.factor), nil
		}
	}
	return 0, errors.New("unknown memory unit")
}
func dockerSample(ctx context.Context, id, kind string) (sample, error) {
	s := sample{At: time.Now().UTC(), Container: kind}
	c := exec.CommandContext(ctx, "docker", "stats", "--no-stream", "--format", "{{json .}}", id)
	out, e := c.Output()
	if e != nil {
		return s, fmt.Errorf("docker stats %s: %w", kind, e)
	}
	var v struct{ CPUPerc, MemUsage string }
	if e = json.Unmarshal(out, &v); e != nil {
		return s, e
	}
	s.CPUPercent, e = strconv.ParseFloat(strings.TrimSuffix(v.CPUPerc, "%"), 64)
	if e != nil {
		return s, e
	}
	s.MemoryBytes, e = memoryBytes(v.MemUsage)
	if e != nil {
		return s, e
	}
	c = exec.CommandContext(ctx, "docker", "exec", id, "cat", "/sys/fs/cgroup/cpu.stat")
	out, e = c.Output()
	if e != nil {
		return s, fmt.Errorf("docker cpu.stat %s: %w", kind, e)
	}
	for _, line := range strings.Split(string(out), "\n") {
		p := strings.Fields(line)
		if len(p) != 2 {
			continue
		}
		n, parseErr := strconv.ParseUint(p[1], 10, 64)
		if parseErr != nil {
			return s, parseErr
		}
		switch p[0] {
		case "nr_throttled":
			s.ThrottledPeriods = n
		case "throttled_usec":
			s.ThrottledMicroseconds = n
		}
	}
	return s, nil
}
func startSampling(env *checkenv.Environment, dir string) func() error {
	var wg sync.WaitGroup
	done := make(chan struct{})
	samples := make([]sample, 0)
	var errs []string
	snap, e := env.Collect()
	ids := map[string]string{}
	if e != nil {
		errs = append(errs, e.Error())
	} else {
		for _, r := range snap.Resources {
			if r.Kind == "server" || r.Kind == "postgres" {
				ids[r.Kind] = r.ID
			}
		}
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		tick := time.NewTicker(time.Second)
		defer tick.Stop()
		take := func() {
			for _, kind := range []string{"server", "postgres"} {
				if ids[kind] == "" {
					errs = append(errs, "missing "+kind+" container ID")
					continue
				}
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				s, e := dockerSample(ctx, ids[kind], kind)
				cancel()
				if e != nil {
					errs = append(errs, e.Error())
				} else {
					samples = append(samples, s)
				}
			}
		}
		take()
		for {
			select {
			case <-done:
				return
			case <-tick.C:
				take()
			}
		}
	}()
	return func() error {
		close(done)
		wg.Wait()
		if e := writeJSON(filepath.Join(dir, "container-samples.json"), struct {
			Samples []sample `json:"samples"`
			Errors  []string `json:"errors,omitempty"`
		}{samples, errs}); e != nil {
			return e
		}
		if len(errs) > 0 {
			return fmt.Errorf("container sampling incomplete: %s", strings.Join(errs, "; "))
		}
		return nil
	}
}
