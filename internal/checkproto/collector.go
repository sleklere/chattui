package checkproto

import (
	"context"
	"errors"
	"fmt"
	"time"
)

type observationSource struct {
	userID  int64
	events  <-chan Observation
	readErr <-chan error
	offline bool
}

// collectObservations waits for the entire window, including when an expected
// offline socket is closed. An unexpected closed socket fails the check.
func collectObservations(ctx context.Context, window time.Duration, sources []observationSource) ([]Observation, error) {
	if window <= 0 {
		return nil, errors.New("collection window must be positive")
	}
	deadline := time.Now().Add(window)
	observations := []Observation{}
	closed := make([]bool, len(sources))
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		if e := ctx.Err(); e != nil {
			return observations, e
		}
		for i, source := range sources {
			if closed[i] {
				continue
			}
			select {
			case e := <-source.readErr:
				if e != nil {
					return observations, fmt.Errorf("user %d websocket: %w", source.userID, e)
				}
			default:
			}
			// Bound a busy socket so cancellation and the deadline are checked again.
			for n := 0; n < 4096; n++ {
				if e := ctx.Err(); e != nil {
					return observations, e
				}
				select {
				case o, ok := <-source.events:
					if !ok {
						if !source.offline {
							return observations, fmt.Errorf("user %d websocket closed unexpectedly", source.userID)
						}
						closed[i] = true
						break
					}
					observations = append(observations, o)
				default:
					n = 4096
				}
				if closed[i] {
					break
				}
			}
		}
		if !time.Now().Before(deadline) {
			return observations, nil
		}
		select {
		case <-ctx.Done():
			return observations, ctx.Err()
		case <-ticker.C:
		}
	}
}
