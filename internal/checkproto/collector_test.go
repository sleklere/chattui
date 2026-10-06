package checkproto

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestCollectObservationsClosedOffline(t *testing.T) {
	events := make(chan Observation, 1)
	events <- Observation{UserID: 2}
	close(events)
	started := time.Now()
	observed, e := collectObservations(context.Background(), 20*time.Millisecond, []observationSource{{userID: 2, events: events, offline: true}})
	if e != nil || len(observed) != 1 {
		t.Fatalf("observed=%d error=%v", len(observed), e)
	}
	if time.Since(started) < 20*time.Millisecond {
		t.Fatal("negative window skipped when channel closed")
	}
}
func TestCollectObservationsUnexpectedClose(t *testing.T) {
	events := make(chan Observation)
	close(events)
	_, e := collectObservations(context.Background(), time.Second, []observationSource{{userID: 2, events: events}})
	if e == nil || !strings.Contains(e.Error(), "closed unexpectedly") {
		t.Fatalf("expected unexpected close, got %v", e)
	}
}
func TestCollectObservationsCancelOffline(t *testing.T) {
	events := make(chan Observation)
	close(events)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, e := collectObservations(ctx, time.Minute, []observationSource{{userID: 2, events: events, offline: true}})
		done <- e
	}()
	cancel()
	select {
	case e := <-done:
		if !errors.Is(e, context.Canceled) {
			t.Fatalf("want canceled, got %v", e)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("closed offline socket spun past cancellation")
	}
}
func TestCollectObservationsUnexpectedReadError(t *testing.T) {
	events := make(chan Observation)
	readErr := make(chan error, 1)
	readErr <- errors.New("read failed")
	_, e := collectObservations(context.Background(), time.Second, []observationSource{{userID: 2, events: events, readErr: readErr}})
	if e == nil || !strings.Contains(e.Error(), "read failed") {
		t.Fatalf("want read error, got %v", e)
	}
}
func TestFixtureSeedAndNamespace(t *testing.T) {
	for _, seed := range []int64{0, 17, -4} {
		a := fixtureUser("check", seed, 1)
		body := fixtureBody("check", seed, 2)
		if fixtureUser("check", seed, 1) != a || fixtureBody("check", seed, 2) != body {
			t.Fatal("seed changed logical fixture")
		}
		if fixtureNamespace(a, "one") == fixtureNamespace(a, "two") || strings.Contains(body, "one") {
			t.Fatal("namespace changed workload or collided")
		}
	}
}
