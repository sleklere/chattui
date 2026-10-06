package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseRejectsUnboundedOrSkippedJobsBeforeWrites(t *testing.T) {
	root := t.TempDir()
	repo, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	cases := [][]string{{"--mode", "nonsense"}, {"--backend", "unknown"}, {"--mode", "compare", "--backend", "postgres"}, {"--mode", "load", "--rate", "101"}, {"--mode", "load", "--steps", "9"}, {"--mode", "load", "--profile", "dm", "--users", "3"}, {"--mode", "load", "--existing-http", "http://127.0.0.1:8080"}, {"--existing-http", "http://127.0.0.1:8080", "--existing-ws", "ws://127.0.0.1:8080/api/v1/ws"}, {"--mode", "load", "--existing-http", "http://example.com:8080", "--existing-ws", "ws://127.0.0.1:8080/api/v1/ws"}, {"--server-cpu", "NaN"}, {"--client-binary", "relative"}, {"--mode", "protocol", "--full-trace"}, {"--mode", "load", "--client-binary", "/tmp/client"}, {"--mode", "load", "--repeats", "2"}, {"--seed", "-1"}, {"--mode", "load", "--profile", "room", "--users", "4", "--rooms", "17"}, {"--mode", "load", "--profile", "room", "--members-per-room", "1"}, {"--mode", "load", "--profile", "dm", "--rooms", "2"}, {"--mode", "protocol", "--rooms", "2"}}
	for _, args := range cases {
		argv := append([]string{"--repository", repo, "--results", filepath.Join(root, "results")}, args...)
		if _, err := parse(argv); err == nil {
			t.Errorf("accepted %q", args)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "results")); !os.IsNotExist(err) {
		t.Fatalf("validation wrote output: %v", err)
	}
}
func TestDefaultRequiresBothProtocolAndTUI(t *testing.T) {
	repo, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	o, err := parse([]string{"--repository", repo, "--results", filepath.Join(t.TempDir(), "results")})
	if err != nil {
		t.Fatal(err)
	}
	if o.mode != "e2e" || len(o.jobs()) != 2 || o.jobs()[0].Backend != "postgres" || o.jobs()[1].Backend != "badger" {
		t.Fatalf("default: %+v %+v", o, o.jobs())
	}
}
func TestCompareOrderAndEquivalentJobs(t *testing.T) {
	repo, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	o, err := parse([]string{"--repository", repo, "--results", filepath.Join(t.TempDir(), "results"), "--mode", "compare", "--repeats", "2", "--steps", "2", "--rate", "1", "--max-rate", "2", "--duration", "1s", "--max-duration", "2s"})
	if err != nil {
		t.Fatal(err)
	}
	j := o.jobs()
	if len(j) != 8 {
		t.Fatal(j)
	}
	want := []string{"postgres", "postgres", "badger", "badger", "badger", "badger", "postgres", "postgres"}
	for i, x := range j {
		if string(x.Backend) != want[i] || x.Step != (i%2)+1 || x.Rate != float64(x.Step) || x.Duration != time.Duration(x.Step)*time.Second {
			t.Fatalf("job %d: %+v", i, x)
		}
	}
}
func TestRunRejectsBadRepositoryWithoutDockerOrResults(t *testing.T) {
	root := t.TempDir()
	o := options{repository: filepath.Join(root, "missing"), results: filepath.Join(root, "result")}
	err := run(context.Background(), o)
	if err == nil || !strings.Contains(err.Error(), "Dockerfile") {
		t.Fatal(err)
	}
	if _, e := os.Stat(o.results); !os.IsNotExist(e) {
		t.Fatalf("created results: %v", e)
	}
}
func TestInvalidCLIExitsWithoutPassOrArtifacts(t *testing.T) {
	root := t.TempDir()
	repo, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	result := filepath.Join(root, "result")
	cmd := exec.Command("go", "run", "./cmd/check", "--repository", repo, "--results", result, "--mode", "load", "--rate", "101")
	cmd.Dir = repo
	out, e := cmd.CombinedOutput()
	if e == nil || strings.Contains(string(out), "PASS") {
		t.Fatalf("invalid CLI status %v: %s", e, out)
	}
	if _, e = os.Stat(result); !os.IsNotExist(e) {
		t.Fatalf("invalid CLI wrote results: %v", e)
	}
}
func TestRoomShapePassesToLoadAndPlan(t *testing.T) {
	repo, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	o, err := parse([]string{"--repository", repo, "--results", filepath.Join(t.TempDir(), "out"), "--mode", "compare", "--profile", "room", "--users", "4", "--rooms", "2", "--members-per-room", "2"})
	if err != nil {
		t.Fatal(err)
	}
	cfg := o.loadConfig("http://127.0.0.1:8080", "ws://127.0.0.1:8080/api/v1/ws", o.results)
	if cfg.Rooms != 2 || cfg.MembersPerRoom != 2 || cfg.Validate() != nil {
		t.Fatalf("room shape: %+v", cfg)
	}
}
func TestMemoryBytes(t *testing.T) {
	for _, x := range []struct {
		in   string
		want uint64
	}{{"1.5MiB / 2GiB", 1572864}, {"512KiB / 2GiB", 524288}, {"2GB / 3GB", 2000000000}} {
		got, e := memoryBytes(x.in)
		if e != nil || got != x.want {
			t.Fatalf("%q: %d %v", x.in, got, e)
		}
	}
	if _, e := memoryBytes("wrong"); e == nil {
		t.Fatal("accepted bad unit")
	}
}
