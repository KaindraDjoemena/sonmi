package agent

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"sonmi/internal/api"
	"sonmi/internal/config"
	"sonmi/internal/db"
)

// ////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
// TEST DOUBLES

// toggleCall is one recorded call to the fake DeviceController. Duration is
// only meaningful for the water pump; State only for the other three relays.
type toggleCall struct {
	Method    string
	State     bool
	Duration  uint
	Mode      db.Mode_t
	Rationale string
}

// fakeController records every actuator call instead of publishing MQTT.
// Real DeviceController implementations never touch the database (the audit row
// is written reactively when the hardware confirms), so recording the calls is
// the complete observable effect of executeCorrections.
type fakeController struct {
	mu    sync.Mutex
	calls []toggleCall
	err   error // returned by every toggle, to simulate a broker failure
}

var _ api.DeviceController = (*fakeController)(nil)

func (f *fakeController) record(c toggleCall) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, c)
	return f.err
}

func (f *fakeController) ToggleWaterPump(duration uint, mode db.Mode_t, rationale string) error {
	return f.record(toggleCall{Method: "ToggleWaterPump", Duration: duration, Mode: mode, Rationale: rationale})
}

func (f *fakeController) ToggleGrowLight(state bool, mode db.Mode_t, rationale string) error {
	return f.record(toggleCall{Method: "ToggleGrowLight", State: state, Mode: mode, Rationale: rationale})
}

func (f *fakeController) ToggleIntakeFan(state bool, mode db.Mode_t, rationale string) error {
	return f.record(toggleCall{Method: "ToggleIntakeFan", State: state, Mode: mode, Rationale: rationale})
}

func (f *fakeController) ToggleExhaustFan(state bool, mode db.Mode_t, rationale string) error {
	return f.record(toggleCall{Method: "ToggleExhaustFan", State: state, Mode: mode, Rationale: rationale})
}

func (f *fakeController) recorded() []toggleCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]toggleCall(nil), f.calls...)
}

// testConfig is a minimal in-memory config; nothing here reads config.yaml.
func testConfig(maxSoilMoisture float32, maxWateringsPerDay uint) *config.Config {
	cfg := &config.Config{}
	cfg.Ecosystem.IdealConditions.MaxSoilMoisturePercent = maxSoilMoisture
	cfg.FailsafeDefaults.MaxWateringEventsPerDay = maxWateringsPerDay
	return cfg
}

// correctionCtxWithSoil builds the slice of context the watering path reads.
// Passing no readings models "no telemetry at all", where the soil veto cannot
// fire.
func correctionCtxWithSoil(soil ...float32) *correctionContextWindow {
	ctx := &correctionContextWindow{}
	for _, s := range soil {
		ctx.LatestTelemetryLog = append(ctx.LatestTelemetryLog, db.SensorTelemetryRow{
			SoilHumidity: s,
			Time:         time.Now().UTC(),
		})
	}
	return ctx
}

// newMemoryDB opens a private in-memory SQLite database: no file on disk, no
// fixture, nothing shared between tests.
func newMemoryDB(t *testing.T) db.Database {
	t.Helper()

	database, err := db.NewDatabase(":memory:")
	if err != nil {
		t.Fatalf("opening in-memory database: %v", err)
	}
	t.Cleanup(func() { database.CloseConn() })

	return database
}

func assertCalls(t *testing.T, got, want []toggleCall) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("got %d actuator calls, want %d\n got: %+v\nwant: %+v", len(got), len(want), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("actuator call %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// ////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
// isBudgetOnlyError

// budgetErr is the exact error shape DecrementWateringBudget produces.
func budgetErr() error {
	return fmt.Errorf("%w for today (%s)", db.ErrBudgetExceeded, "2026-09-08")
}

// customJoined is a joined error that is not errors.Join — errors.Join never
// stores nil elements, but the Unwrap() []error contract allows them.
type customJoined struct{ errs []error }

func (c customJoined) Error() string   { return "custom joined" }
func (c customJoined) Unwrap() []error { return c.errs }

func TestIsBudgetOnlyError(t *testing.T) {
	other := errors.New("mqtt: not connected")

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil is not a budget error",
			err:  nil,
			want: false,
		},
		{
			name: "the sentinel itself",
			err:  db.ErrBudgetExceeded,
			want: true,
		},
		{
			name: "wrapped sentinel, as DecrementWateringBudget returns it",
			err:  budgetErr(),
			want: true,
		},
		{
			name: "doubly wrapped sentinel",
			err:  fmt.Errorf("executing corrections: %w", budgetErr()),
			want: true,
		},
		{
			name: "unrelated error",
			err:  other,
			want: false,
		},
		{
			name: "error whose text merely mentions the budget",
			err:  errors.New("watering budget exceeded"),
			want: false,
		},
		{
			name: "join of a single budget error",
			err:  errors.Join(budgetErr()),
			want: true,
		},
		{
			name: "join of two budget errors",
			err:  errors.Join(budgetErr(), budgetErr()),
			want: true,
		},
		{
			name: "budget error joined with an unrelated one",
			err:  errors.Join(budgetErr(), other),
			want: false,
		},
		{
			name: "unrelated error joined with a budget one, reversed order",
			err:  errors.Join(other, budgetErr()),
			want: false,
		},
		{
			name: "join of unrelated errors",
			err:  errors.Join(other, errors.New("boom")),
			want: false,
		},
		{
			name: "join dropping nils around a budget error",
			err:  errors.Join(nil, budgetErr(), nil),
			want: true,
		},
		{
			name: "custom join containing an explicit nil element",
			err:  customJoined{errs: []error{nil, budgetErr()}},
			want: true,
		},
		{
			name: "custom join with only nil elements is vacuously budget-only",
			err:  customJoined{errs: []error{nil}},
			want: true,
		},
		{
			name: "custom join containing an unrelated error",
			err:  customJoined{errs: []error{budgetErr(), other}},
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isBudgetOnlyError(tc.err); got != tc.want {
				t.Errorf("isBudgetOnlyError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

// ////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
// durationUntilNext

// durationUntilNext reads the wall clock itself, so the assertions below are
// properties rather than fixed values: the deadline it points at must be the
// *earliest* future occurrence of hour:minute in UTC.
func TestDurationUntilNext(t *testing.T) {
	tests := []struct {
		name string
		// offset from "now" used to derive the target hour and minute.
		offset time.Duration
		// optional bounds on the returned duration; zero means unchecked.
		min, max time.Duration
	}{
		{
			name:   "one minute from now",
			offset: time.Minute,
			max:    2 * time.Minute,
		},
		{
			name:   "one hour from now",
			offset: time.Hour,
			min:    59 * time.Minute,
			max:    time.Hour + time.Minute,
		},
		{
			name:   "twelve hours from now",
			offset: 12 * time.Hour,
			min:    11*time.Hour + 59*time.Minute,
			max:    12*time.Hour + time.Minute,
		},
		{
			// The target hour:minute is now, so today's occurrence has already
			// passed (by fractions of a second) and the wait must roll a full
			// day rather than return zero or a negative duration.
			name:   "exactly the current minute rolls to tomorrow",
			offset: 0,
			min:    23 * time.Hour,
			max:    24 * time.Hour,
		},
		{
			name:   "one minute ago rolls to tomorrow",
			offset: -time.Minute,
			min:    23 * time.Hour,
			max:    24 * time.Hour,
		},
		{
			name:   "one hour ago rolls to tomorrow",
			offset: -time.Hour,
			min:    22*time.Hour + 58*time.Minute,
			max:    24 * time.Hour,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			target := time.Now().UTC().Add(tc.offset)
			hour, minute := target.Hour(), target.Minute()

			before := time.Now().UTC()
			got := durationUntilNext(hour, minute)
			after := time.Now().UTC()

			// The clock must not have moved appreciably during the call, or the
			// deadline reconstruction below is meaningless.
			if elapsed := after.Sub(before); elapsed > 250*time.Millisecond {
				t.Fatalf("durationUntilNext took %v; the machine is too loaded for this assertion", elapsed)
			}

			if got <= 0 {
				t.Fatalf("durationUntilNext(%02d, %02d) = %v, want a positive duration", hour, minute, got)
			}
			if got > 24*time.Hour {
				t.Fatalf("durationUntilNext(%02d, %02d) = %v, want at most 24h", hour, minute, got)
			}

			// Reconstruct the absolute deadline. `before` is at most 250ms
			// earlier than the instant inside the function, so rounding to the
			// second recovers the exact deadline.
			deadline := before.Add(got).Round(time.Second)

			if deadline.Hour() != hour || deadline.Minute() != minute || deadline.Second() != 0 {
				t.Errorf("durationUntilNext(%02d, %02d) points at %v, want %02d:%02d:00", hour, minute, deadline, hour, minute)
			}
			if !deadline.After(before) {
				t.Errorf("durationUntilNext(%02d, %02d) points at %v, which is not after %v", hour, minute, deadline, before)
			}
			// It must be the *soonest* such occurrence: one day earlier must
			// already be in the past, otherwise a whole day was skipped.
			if prev := deadline.AddDate(0, 0, -1); prev.After(after) {
				t.Errorf("durationUntilNext(%02d, %02d) skipped the occurrence at %v (now %v)", hour, minute, prev, after)
			}
			// A UTC-day-based schedule only ever lands today or tomorrow.
			today := before.Format(time.DateOnly)
			tomorrow := before.AddDate(0, 0, 1).Format(time.DateOnly)
			if d := deadline.Format(time.DateOnly); d != today && d != tomorrow {
				t.Errorf("durationUntilNext(%02d, %02d) lands on %s, want %s or %s", hour, minute, d, today, tomorrow)
			}

			if tc.min != 0 && got < tc.min {
				t.Errorf("durationUntilNext(%02d, %02d) = %v, want at least %v", hour, minute, got, tc.min)
			}
			if tc.max != 0 && got > tc.max {
				t.Errorf("durationUntilNext(%02d, %02d) = %v, want at most %v", hour, minute, got, tc.max)
			}
		})
	}
}

// The journal loop is pinned to 23:59 UTC and the snapshot ticker to 23:55; the
// four minutes between them must survive the day boundary.
func TestDurationUntilNextOrdersFixedSchedule(t *testing.T) {
	snapshot := durationUntilNext(23, 55)
	journal := durationUntilNext(23, 59)

	// The two calls read the clock a moment apart, so compare the gap against
	// both expectations with a tolerance rather than for exact equality.
	near := func(got, want time.Duration) bool {
		diff := got - want
		if diff < 0 {
			diff = -diff
		}
		return diff < time.Second
	}

	// Either both are later today (journal 4 minutes after the snapshot), or the
	// snapshot has just passed and it wraps to tomorrow (journal comes first).
	if diff := journal - snapshot; !near(diff, 4*time.Minute) && !near(diff, 4*time.Minute-24*time.Hour) {
		t.Errorf("journal(23:59) - snapshot(23:55) = %v, want ~4m or ~-23h56m", diff)
	}
}

func TestDurationUntilNextEveryHourIsWithinADay(t *testing.T) {
	for hour := range 24 {
		got := durationUntilNext(hour, 0)
		if got <= 0 || got > 24*time.Hour {
			t.Errorf("durationUntilNext(%02d, 00) = %v, want a duration in (0, 24h]", hour, got)
		}
	}
}

// ////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
// executeCorrections

// The cases below never reach the database: the grow light, both fans, turning
// the pump off, and both veto paths all return before DecrementWateringBudget.
// They are handed a zero-value db.Database on purpose — a regression that made
// any of them touch SQLite would panic on the nil connection instead of quietly
// passing.
func TestExecuteCorrections(t *testing.T) {
	brokerDown := errors.New("mqtt: not connected")

	tests := []struct {
		name          string
		resp          string
		soil          []float32 // telemetry readings, newest first; empty = no telemetry
		controllerErr error
		want          []toggleCall
		wantErr       bool
		wantErrIs     error
	}{
		{
			name: "grow light on",
			resp: `[{"relay":"GROW_LIGHT","value":true,"rationale":"photoperiod not yet met"}]`,
			want: []toggleCall{
				{Method: "ToggleGrowLight", State: true, Mode: db.ModeAgent, Rationale: "photoperiod not yet met"},
			},
		},
		{
			name: "grow light off",
			resp: `[{"relay":"GROW_LIGHT","value":false,"rationale":"14h of light delivered"}]`,
			want: []toggleCall{
				{Method: "ToggleGrowLight", State: false, Mode: db.ModeAgent, Rationale: "14h of light delivered"},
			},
		},
		{
			name: "several relays are applied in order",
			resp: `[
				{"relay":"INTAKE_FAN","value":true,"rationale":"temp above range"},
				{"relay":"EXHAUST_FAN","value":true,"rationale":"temp above range"},
				{"relay":"GROW_LIGHT","value":false,"rationale":"reduce heat load"}
			]`,
			want: []toggleCall{
				{Method: "ToggleIntakeFan", State: true, Mode: db.ModeAgent, Rationale: "temp above range"},
				{Method: "ToggleExhaustFan", State: true, Mode: db.ModeAgent, Rationale: "temp above range"},
				{Method: "ToggleGrowLight", State: false, Mode: db.ModeAgent, Rationale: "reduce heat load"},
			},
		},
		{
			// The duration field is ignored when value is false: the pump is
			// commanded off with 0, which is what the firmware reads as "off now".
			name: "water pump off ignores the requested duration",
			resp: `[{"relay":"WATER_PUMP","value":false,"duration":30,"rationale":"stop watering"}]`,
			soil: []float32{20},
			want: []toggleCall{
				{Method: "ToggleWaterPump", Duration: 0, Mode: db.ModeAgent, Rationale: "stop watering"},
			},
		},
		{
			// Soil-moisture veto (docs/concepts.md §4.2): wetter than the
			// configured maximum, so nothing reaches the hardware and no error
			// is reported.
			name: "watering is vetoed when the soil is already too wet",
			resp: `[{"relay":"WATER_PUMP","value":true,"duration":30,"rationale":"soil looks dry"}]`,
			soil: []float32{81.5, 20},
			want: nil,
		},
		{
			name: "a vetoed watering does not block the other relays",
			resp: `[
				{"relay":"WATER_PUMP","value":true,"duration":30,"rationale":"soil looks dry"},
				{"relay":"GROW_LIGHT","value":true,"rationale":"photoperiod not yet met"}
			]`,
			soil: []float32{81.5},
			want: []toggleCall{
				{Method: "ToggleGrowLight", State: true, Mode: db.ModeAgent, Rationale: "photoperiod not yet met"},
			},
		},
		{
			name: "unrecognised relay names are silently dropped",
			resp: `[{"relay":"HEATING_PAD","value":true,"rationale":"warm the roots"}]`,
			want: nil,
		},
		{
			name: "an unrecognised relay does not block the recognised ones",
			resp: `[
				{"relay":"HEATING_PAD","value":true,"rationale":"warm the roots"},
				{"relay":"EXHAUST_FAN","value":true,"rationale":"humidity above range"}
			]`,
			want: []toggleCall{
				{Method: "ToggleExhaustFan", State: true, Mode: db.ModeAgent, Rationale: "humidity above range"},
			},
		},
		{
			name: "relay names are case sensitive",
			resp: `[{"relay":"grow_light","value":true,"rationale":"lower case"}]`,
			want: nil,
		},
		{
			name: "an empty correction list is a no-op",
			resp: `[]`,
			want: nil,
		},
		{
			name:    "malformed json is an error",
			resp:    `{"relay":"GROW_LIGHT"}`,
			want:    nil,
			wantErr: true,
		},
		{
			name:    "truncated json is an error",
			resp:    `[{"relay":"GROW_LIGHT",`,
			want:    nil,
			wantErr: true,
		},
		{
			// A failing controller must not abort the batch: every remaining
			// correction is still attempted and the errors are joined.
			name: "controller failures are collected without stopping the batch",
			resp: `[
				{"relay":"GROW_LIGHT","value":true,"rationale":"photoperiod not yet met"},
				{"relay":"INTAKE_FAN","value":true,"rationale":"temp above range"}
			]`,
			controllerErr: brokerDown,
			want: []toggleCall{
				{Method: "ToggleGrowLight", State: true, Mode: db.ModeAgent, Rationale: "photoperiod not yet met"},
				{Method: "ToggleIntakeFan", State: true, Mode: db.ModeAgent, Rationale: "temp above range"},
			},
			wantErr:   true,
			wantErrIs: brokerDown,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := &fakeController{err: tc.controllerErr}
			cfg := testConfig(60, 1)
			ctx := correctionCtxWithSoil(tc.soil...)

			err := executeCorrections([]byte(tc.resp), c, db.Database{}, cfg, ctx)

			if tc.wantErr && err == nil {
				t.Fatalf("executeCorrections returned nil, want an error")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("executeCorrections returned unexpected error: %v", err)
			}
			if tc.wantErrIs != nil && !errors.Is(err, tc.wantErrIs) {
				t.Errorf("executeCorrections error = %v, want it to wrap %v", err, tc.wantErrIs)
			}
			// Nothing here is a budget failure, so the correction loop must read
			// every one of these outcomes as a real failure (or a success).
			if isBudgetOnlyError(err) {
				t.Errorf("isBudgetOnlyError(%v) = true, want false", err)
			}

			assertCalls(t, c.recorded(), tc.want)
		})
	}
}

// The watering path is the only one that reaches SQLite, via the hard daily cap
// in DecrementWateringBudget (docs/concepts.md §4.1).
func TestExecuteCorrectionsWatering(t *testing.T) {
	const resp = `[{"relay":"WATER_PUMP","value":true,"duration":45,"rationale":"soil below minimum"}]`

	tests := []struct {
		name string
		soil []float32
	}{
		{
			name: "soil below the maximum",
			soil: []float32{20},
		},
		{
			// The veto is an equality check: exactly at the maximum still waters.
			name: "soil exactly at the maximum",
			soil: []float32{60},
		},
		{
			// With no telemetry the veto cannot fire; the budget is the only
			// remaining guard.
			name: "no telemetry at all",
			soil: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := &fakeController{}
			cfg := testConfig(60, 2)
			database := newMemoryDB(t)
			today := time.Now().UTC().Format(time.DateOnly)

			if err := executeCorrections([]byte(resp), c, database, cfg, correctionCtxWithSoil(tc.soil...)); err != nil {
				t.Fatalf("executeCorrections returned unexpected error: %v", err)
			}

			assertCalls(t, c.recorded(), []toggleCall{
				{Method: "ToggleWaterPump", Duration: 45, Mode: db.ModeAgent, Rationale: "soil below minimum"},
			})

			remaining, err := database.SelectWateringBudget(today)
			if err != nil {
				t.Fatalf("reading the watering budget for %s: %v", today, err)
			}
			if want := 1; remaining != want {
				t.Errorf("watering budget for %s = %d, want %d", today, remaining, want)
			}
		})
	}
}

// A vetoed watering must not spend budget — the plant gets its allowance back
// for a later tick in the same day.
func TestExecuteCorrectionsVetoDoesNotSpendBudget(t *testing.T) {
	c := &fakeController{}
	cfg := testConfig(60, 1)
	database := newMemoryDB(t)
	today := time.Now().UTC().Format(time.DateOnly)

	resp := `[{"relay":"WATER_PUMP","value":true,"duration":30,"rationale":"soil looks dry"}]`
	if err := executeCorrections([]byte(resp), c, database, cfg, correctionCtxWithSoil(81.5)); err != nil {
		t.Fatalf("executeCorrections returned unexpected error: %v", err)
	}

	assertCalls(t, c.recorded(), nil)

	// No row at all means the budget was never touched.
	if _, err := database.SelectWateringBudget(today); err == nil {
		t.Errorf("a vetoed watering wrote a watering_budgets row for %s, want none", today)
	}
}

// Exhausting the daily cap is an expected condition, not a fault: the second
// watering is refused, no command reaches the pump, and the resulting error is
// budget-only so StartCorrectionLoop keeps the system NOMINAL instead of
// dropping into FAILSAFE (docs/concepts.md §5.1).
func TestExecuteCorrectionsBudgetExhausted(t *testing.T) {
	cfg := testConfig(60, 1)
	database := newMemoryDB(t)
	ctx := correctionCtxWithSoil(20)
	resp := []byte(`[{"relay":"WATER_PUMP","value":true,"duration":30,"rationale":"soil below minimum"}]`)

	first := &fakeController{}
	if err := executeCorrections(resp, first, database, cfg, ctx); err != nil {
		t.Fatalf("first watering returned unexpected error: %v", err)
	}
	assertCalls(t, first.recorded(), []toggleCall{
		{Method: "ToggleWaterPump", Duration: 30, Mode: db.ModeAgent, Rationale: "soil below minimum"},
	})

	second := &fakeController{}
	err := executeCorrections(resp, second, database, cfg, ctx)
	if err == nil {
		t.Fatal("second watering returned nil, want a budget error")
	}
	if !errors.Is(err, db.ErrBudgetExceeded) {
		t.Errorf("second watering error = %v, want it to wrap db.ErrBudgetExceeded", err)
	}
	if !isBudgetOnlyError(err) {
		t.Errorf("isBudgetOnlyError(%v) = false, want true — the loop would enter FAILSAFE", err)
	}
	assertCalls(t, second.recorded(), nil)
}

// A batch where watering is capped but other relays succeed must still read as
// budget-only, or one exhausted watering would degrade the whole tick.
func TestExecuteCorrectionsBudgetExhaustedAlongsideOtherRelays(t *testing.T) {
	cfg := testConfig(60, 0) // no watering allowed at all today
	database := newMemoryDB(t)
	c := &fakeController{}

	resp := `[
		{"relay":"WATER_PUMP","value":true,"duration":30,"rationale":"soil below minimum"},
		{"relay":"GROW_LIGHT","value":true,"rationale":"photoperiod not yet met"}
	]`

	err := executeCorrections([]byte(resp), c, database, cfg, correctionCtxWithSoil(20))
	if err == nil {
		t.Fatal("executeCorrections returned nil, want a budget error")
	}
	if !isBudgetOnlyError(err) {
		t.Errorf("isBudgetOnlyError(%v) = false, want true", err)
	}

	assertCalls(t, c.recorded(), []toggleCall{
		{Method: "ToggleGrowLight", State: true, Mode: db.ModeAgent, Rationale: "photoperiod not yet met"},
	})
}
