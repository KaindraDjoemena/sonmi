package api

import (
	"fmt"
	"sync"
	"testing"

	"sonmi/internal/db"
)

// RationaleCorrelator hands a rationale from the goroutine that sends an
// actuator command to the MQTT callback goroutine that logs the hardware's
// confirmation, so every test here exercises the handoff the way production
// does: Put from one side, Take from the other, exactly once.

func TestRationaleCorrelatorPutTake(t *testing.T) {
	t.Parallel()

	r := NewRationaleCorrelator()
	r.Put(db.RelayWaterPump, db.ModeAgent, "soil below minimum")

	if got, want := r.Take(db.RelayWaterPump, db.ModeAgent), "soil below minimum"; got != want {
		t.Errorf("first Take = %q, want %q", got, want)
	}
	// The value is consumed: the second confirmation for the same command (the
	// pump publishes one on start and one on its self-timed shutoff) must not
	// reuse the first one's rationale.
	if got := r.Take(db.RelayWaterPump, db.ModeAgent); got != "" {
		t.Errorf("second Take = %q, want %q", got, "")
	}
}

func TestRationaleCorrelatorTakeUnknownKey(t *testing.T) {
	t.Parallel()

	r := NewRationaleCorrelator()
	if got := r.Take(db.RelayGrowLight, db.ModeAgent); got != "" {
		t.Errorf("Take on an empty correlator = %q, want %q", got, "")
	}

	r.Put(db.RelayGrowLight, db.ModeAgent, "photoperiod not yet met")
	if got := r.Take(db.RelayGrowLight, db.ModeOverride); got != "" {
		t.Errorf("Take with a different mode = %q, want %q", got, "")
	}
	if got := r.Take(db.RelayIntakeFan, db.ModeAgent); got != "" {
		t.Errorf("Take with a different relay = %q, want %q", got, "")
	}
	if got, want := r.Take(db.RelayGrowLight, db.ModeAgent), "photoperiod not yet met"; got != want {
		t.Errorf("Take with the matching key = %q, want %q", got, want)
	}
}

// Every (relay, mode) pair is its own slot; a command for one relay must never
// be attributed to another, and the same relay driven by the agent and by an
// operator override must not collide.
func TestRationaleCorrelatorKeysAreIndependent(t *testing.T) {
	t.Parallel()

	relays := []db.Relay_t{db.RelayWaterPump, db.RelayGrowLight, db.RelayIntakeFan, db.RelayExhaustFan}
	modes := []db.Mode_t{db.ModeAgent, db.ModeOverride, db.ModeFailsafe}

	r := NewRationaleCorrelator()
	for _, relay := range relays {
		for _, mode := range modes {
			r.Put(relay, mode, fmt.Sprintf("%s/%s", relay, mode))
		}
	}

	for _, relay := range relays {
		for _, mode := range modes {
			want := fmt.Sprintf("%s/%s", relay, mode)
			if got := r.Take(relay, mode); got != want {
				t.Errorf("Take(%s, %s) = %q, want %q", relay, mode, got, want)
			}
			if got := r.Take(relay, mode); got != "" {
				t.Errorf("second Take(%s, %s) = %q, want %q", relay, mode, got, "")
			}
		}
	}
}

// The controller calls Put unconditionally, including for callers that pass no
// rationale; an empty one must neither be stored nor clobber a pending value.
func TestRationaleCorrelatorPutEmptyIsIgnored(t *testing.T) {
	t.Parallel()

	r := NewRationaleCorrelator()

	r.Put(db.RelayExhaustFan, db.ModeAgent, "")
	if got := r.Take(db.RelayExhaustFan, db.ModeAgent); got != "" {
		t.Errorf("Take after an empty Put = %q, want %q", got, "")
	}

	r.Put(db.RelayExhaustFan, db.ModeAgent, "humidity above range")
	r.Put(db.RelayExhaustFan, db.ModeAgent, "")
	if got, want := r.Take(db.RelayExhaustFan, db.ModeAgent), "humidity above range"; got != want {
		t.Errorf("Take after an empty Put over a stored value = %q, want %q", got, want)
	}
}

// A later Put for the same key replaces the pending value — the newest command
// is the one the next confirmation belongs to.
func TestRationaleCorrelatorPutOverwrites(t *testing.T) {
	t.Parallel()

	r := NewRationaleCorrelator()
	r.Put(db.RelayIntakeFan, db.ModeAgent, "first")
	r.Put(db.RelayIntakeFan, db.ModeAgent, "second")

	if got, want := r.Take(db.RelayIntakeFan, db.ModeAgent), "second"; got != want {
		t.Errorf("Take = %q, want %q", got, want)
	}
	if got := r.Take(db.RelayIntakeFan, db.ModeAgent); got != "" {
		t.Errorf("second Take = %q, want %q", got, "")
	}
}

// ActuatorController holds a *RationaleCorrelator that may be nil (the TUI and
// the agent share one, but a controller built without one must not panic).
func TestRationaleCorrelatorNilReceiver(t *testing.T) {
	t.Parallel()

	var r *RationaleCorrelator

	r.Put(db.RelayGrowLight, db.ModeOverride, "operator flipped the light")
	if got := r.Take(db.RelayGrowLight, db.ModeOverride); got != "" {
		t.Errorf("Take on a nil correlator = %q, want %q", got, "")
	}
}

// Producers (the agent loop, the TUI, failsafe) and the consumer (the MQTT
// callback) run on different goroutines. Run with -race.
func TestRationaleCorrelatorConcurrentPutTake(t *testing.T) {
	t.Parallel()

	const (
		writers   = 8
		perWriter = 200
		readers   = 8
	)

	relays := []db.Relay_t{db.RelayWaterPump, db.RelayGrowLight, db.RelayIntakeFan, db.RelayExhaustFan}
	modes := []db.Mode_t{db.ModeAgent, db.ModeOverride, db.ModeFailsafe}

	r := NewRationaleCorrelator()

	// Every value written is unique, so a taken value identifies exactly which
	// Put produced it.
	produced := make(map[string]struct{}, writers*perWriter)
	for w := range writers {
		for i := range perWriter {
			produced[fmt.Sprintf("w%d-%d", w, i)] = struct{}{}
		}
	}

	var (
		mu    sync.Mutex
		taken = make(map[string]int)
	)
	recordTaken := func(v string) {
		if v == "" {
			return
		}
		mu.Lock()
		taken[v]++
		mu.Unlock()
	}

	done := make(chan struct{})

	var writeGroup, readGroup sync.WaitGroup

	for w := range writers {
		writeGroup.Add(1)
		go func() {
			defer writeGroup.Done()
			for i := range perWriter {
				relay := relays[(w+i)%len(relays)]
				mode := modes[i%len(modes)]
				r.Put(relay, mode, fmt.Sprintf("w%d-%d", w, i))
			}
		}()
	}

	for range readers {
		readGroup.Add(1)
		go func() {
			defer readGroup.Done()
			for {
				select {
				case <-done:
					return
				default:
				}
				for _, relay := range relays {
					for _, mode := range modes {
						recordTaken(r.Take(relay, mode))
					}
				}
			}
		}()
	}

	writeGroup.Wait()
	close(done)
	readGroup.Wait()

	// Drain whatever the readers did not get to.
	for _, relay := range relays {
		for _, mode := range modes {
			recordTaken(r.Take(relay, mode))
		}
	}

	if len(taken) == 0 {
		t.Fatal("no rationale was ever taken; the test did not exercise the handoff")
	}

	for v, n := range taken {
		if _, ok := produced[v]; !ok {
			t.Errorf("Take returned %q, which was never Put", v)
		}
		// The whole point of Take is that it consumes: no rationale may be
		// handed to two different relay confirmations.
		if n != 1 {
			t.Errorf("rationale %q was taken %d times, want exactly 1", v, n)
		}
	}

	// The correlator must be fully drained: nothing is left to leak into a
	// later, unrelated confirmation.
	for _, relay := range relays {
		for _, mode := range modes {
			if got := r.Take(relay, mode); got != "" {
				t.Errorf("Take(%s, %s) after draining = %q, want %q", relay, mode, got, "")
			}
		}
	}
}
