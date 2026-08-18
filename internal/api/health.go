package api

import (
	"sync"
	"time"
)

// GatewayHealth is the JSON health report published by the Pi's edge gateway
// (see the health-reporter script deployed to the Pi, roadmap.md Phase 4).
type GatewayHealth struct {
	Timestamp time.Time `json:"timestamp"`
	Hostapd   bool      `json:"hostapd"`
	Dnsmasq   bool      `json:"dnsmasq"`
	Uap0      bool      `json:"uap0"`
	Mosquitto bool      `json:"mosquitto"`
	Bridge    bool      `json:"bridge"`
	PiRelay   bool      `json:"pi_relay"`
}

// LoopStatus tracks when the correction/journal agent loops last attempted and
// last succeeded, so the TUI's monitor panel can show whether they're actually
// running on schedule. Safe for concurrent use by the agent loops (writers) and
// the TUI (reader).
type LoopStatus struct {
	mu sync.RWMutex

	lastCorrectionAttempt time.Time
	lastCorrectionSuccess time.Time
	lastJournalAttempt    time.Time
	lastJournalSuccess    time.Time
}

func (s *LoopStatus) MarkCorrectionAttempt() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastCorrectionAttempt = time.Now().UTC()
}

func (s *LoopStatus) MarkCorrectionSuccess() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastCorrectionSuccess = time.Now().UTC()
}

func (s *LoopStatus) MarkJournalAttempt() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastJournalAttempt = time.Now().UTC()
}

func (s *LoopStatus) MarkJournalSuccess() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastJournalSuccess = time.Now().UTC()
}

// LoopSnapshot is a point-in-time read of LoopStatus's timestamps.
type LoopSnapshot struct {
	LastCorrectionAttempt time.Time
	LastCorrectionSuccess time.Time
	LastJournalAttempt    time.Time
	LastJournalSuccess    time.Time
}

func (s *LoopStatus) Snapshot() LoopSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return LoopSnapshot{
		LastCorrectionAttempt: s.lastCorrectionAttempt,
		LastCorrectionSuccess: s.lastCorrectionSuccess,
		LastJournalAttempt:    s.lastJournalAttempt,
		LastJournalSuccess:    s.lastJournalSuccess,
	}
}
