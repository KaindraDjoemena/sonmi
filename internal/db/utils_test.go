package db

import (
	"testing"
	"time"
)

// The whole system stores timestamps as the fixed string TIME_FORMAT and reads
// them back assuming UTC (see docs/concepts.md §8 "Time is UTC text,
// everywhere"). These tests pin down the three properties callers rely on:
//   1. the format has 1-second resolution and no timezone suffix,
//   2. ParseTime always yields a time.Time located in UTC,
//   3. FormatTime -> ParseTime is a round trip for any UTC, whole-second time.

func TestFormatTime(t *testing.T) {
	tests := []struct {
		name string
		in   time.Time
		want string
	}{
		{
			name: "typical utc timestamp",
			in:   time.Date(2026, 9, 8, 14, 32, 7, 0, time.UTC),
			want: "2026-09-08 14:32:07",
		},
		{
			name: "zero time",
			in:   time.Time{},
			want: "0001-01-01 00:00:00",
		},
		{
			name: "midnight is zero padded",
			in:   time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
			want: "2026-01-02 00:00:00",
		},
		{
			name: "subsecond precision is dropped",
			in:   time.Date(2026, 9, 8, 14, 32, 7, 999999999, time.UTC),
			want: "2026-09-08 14:32:07",
		},
		{
			name: "leap day",
			in:   time.Date(2028, 2, 29, 23, 59, 59, 0, time.UTC),
			want: "2028-02-29 23:59:59",
		},
		{
			name: "non-utc location is formatted as wall clock, not converted",
			in:   time.Date(2026, 9, 8, 12, 0, 0, 0, time.FixedZone("WIB", 7*60*60)),
			want: "2026-09-08 12:00:00",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := FormatTime(tc.in); got != tc.want {
				t.Errorf("FormatTime(%v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestParseTime(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    time.Time
		wantErr bool
	}{
		{
			name: "typical timestamp is parsed as utc",
			in:   "2026-09-08 14:32:07",
			want: time.Date(2026, 9, 8, 14, 32, 7, 0, time.UTC),
		},
		{
			name: "zero time",
			in:   "0001-01-01 00:00:00",
			want: time.Time{},
		},
		{
			name: "leap day",
			in:   "2028-02-29 23:59:59",
			want: time.Date(2028, 2, 29, 23, 59, 59, 0, time.UTC),
		},
		{
			name:    "empty string",
			in:      "",
			wantErr: true,
		},
		{
			name:    "rfc3339 is not accepted",
			in:      "2026-09-08T14:32:07Z",
			wantErr: true,
		},
		{
			// Go's parser accepts a fractional second after a seconds field even
			// though the layout has none. Nothing in this codebase writes one,
			// but a hand-edited row or a foreign writer would be read back with
			// sub-second precision rather than rejected.
			name: "fractional seconds are tolerated",
			in:   "2026-09-08 14:32:07.500",
			want: time.Date(2026, 9, 8, 14, 32, 7, 500000000, time.UTC),
		},
		{
			name:    "trailing timezone suffix is not accepted",
			in:      "2026-09-08 14:32:07 UTC",
			wantErr: true,
		},
		{
			name:    "date only",
			in:      "2026-09-08",
			wantErr: true,
		},
		{
			name:    "unpadded month and day",
			in:      "2026-9-8 14:32:07",
			wantErr: true,
		},
		{
			name:    "impossible month",
			in:      "2026-13-08 14:32:07",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseTime(tc.in)

			if tc.wantErr {
				if err == nil {
					t.Fatalf("ParseTime(%q) = %v, want an error", tc.in, got)
				}
				// On failure the caller must get the zero time, never a
				// partially parsed value.
				if !got.IsZero() {
					t.Errorf("ParseTime(%q) returned %v on error, want the zero time", tc.in, got)
				}
				return
			}

			if err != nil {
				t.Fatalf("ParseTime(%q) returned unexpected error: %v", tc.in, err)
			}
			if !got.Equal(tc.want) {
				t.Errorf("ParseTime(%q) = %v, want %v", tc.in, got, tc.want)
			}
			if loc := got.Location(); loc != time.UTC {
				t.Errorf("ParseTime(%q) located in %v, want UTC", tc.in, loc)
			}
		})
	}
}

func TestFormatParseRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		in   time.Time
		// want is the instant the round trip must produce; for whole-second UTC
		// times it is `in` itself, otherwise the second-truncated value.
		want time.Time
	}{
		{
			name: "whole second utc time survives unchanged",
			in:   time.Date(2026, 9, 8, 14, 32, 7, 0, time.UTC),
			want: time.Date(2026, 9, 8, 14, 32, 7, 0, time.UTC),
		},
		{
			name: "zero time",
			in:   time.Time{},
			want: time.Time{},
		},
		{
			name: "subsecond precision is truncated, not rounded",
			in:   time.Date(2026, 9, 8, 14, 32, 7, 999999999, time.UTC),
			want: time.Date(2026, 9, 8, 14, 32, 7, 0, time.UTC),
		},
		{
			name: "one nanosecond",
			in:   time.Date(2026, 9, 8, 14, 32, 7, 1, time.UTC),
			want: time.Date(2026, 9, 8, 14, 32, 7, 0, time.UTC),
		},
		{
			name: "utc midnight, the day boundary the watering budget keys on",
			in:   time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC),
			want: time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "23:59, when the journal loop fires",
			in:   time.Date(2026, 9, 8, 23, 59, 0, 0, time.UTC),
			want: time.Date(2026, 9, 8, 23, 59, 0, 0, time.UTC),
		},
		{
			name: "a time already normalised to utc from another zone",
			in:   time.Date(2026, 9, 8, 12, 0, 0, 0, time.FixedZone("WIB", 7*60*60)).UTC(),
			want: time.Date(2026, 9, 8, 5, 0, 0, 0, time.UTC),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseTime(FormatTime(tc.in))
			if err != nil {
				t.Fatalf("ParseTime(FormatTime(%v)) returned error: %v", tc.in, err)
			}
			if !got.Equal(tc.want) {
				t.Errorf("round trip of %v = %v, want %v", tc.in, got, tc.want)
			}
			if loc := got.Location(); loc != time.UTC {
				t.Errorf("round trip of %v located in %v, want UTC", tc.in, loc)
			}
		})
	}
}

// TestRoundTripDropsNonUTCOffset documents the sharp edge behind the
// "every time.Now() is time.Now().UTC()" convention: FormatTime writes the
// wall clock of whatever location it is handed, and ParseTime reads it back as
// UTC. Handing it a non-UTC time therefore shifts the stored instant by that
// zone's offset instead of failing loudly.
func TestRoundTripDropsNonUTCOffset(t *testing.T) {
	wib := time.FixedZone("WIB", 7*60*60)
	local := time.Date(2026, 9, 8, 12, 0, 0, 0, wib)

	got, err := ParseTime(FormatTime(local))
	if err != nil {
		t.Fatalf("ParseTime(FormatTime(%v)) returned error: %v", local, err)
	}

	if got.Equal(local) {
		t.Fatalf("round trip preserved the instant %v; FormatTime now converts to UTC and this test is obsolete", local)
	}
	if want := 7 * time.Hour; got.Sub(local) != want {
		t.Errorf("round trip of %v shifted the instant by %v, want %v", local, got.Sub(local), want)
	}
	if want := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Errorf("round trip of %v = %v, want the same wall clock in UTC (%v)", local, got, want)
	}
}
