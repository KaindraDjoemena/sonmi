package tui

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"sonmi/internal/api"
	"sonmi/internal/db"
	"sonmi/internal/style"
)

// gatewayHealthStaleAfter and telemetryStaleAfter bound how long since the last
// report before the monitor panel flags that source as stale/unreachable rather
// than showing a possibly-ancient last-known value as if it were current.
const (
	gatewayHealthStaleAfter = 3 * time.Minute // health reporter publishes ~every 60s
	telemetryStaleAfter     = 15 * time.Minute // ESP32 publishes ~every 5min
)

type monitorTickMsg time.Time

func monitorTickCmd() tea.Cmd {
	return tea.Tick(5*time.Second, func(t time.Time) tea.Msg {
		return monitorTickMsg(t)
	})
}

type MonitorPanel struct {
	dbConn     db.Database
	loopStatus *api.LoopStatus

	latestHealth     api.GatewayHealth
	healthReceivedAt time.Time
	lastTelemetryAt  time.Time

	sysState    db.SystemStateRow
	sysStateErr error

	w, h int
}

var _ tea.Model = MonitorPanel{}

func InitializeMonitorPanel(dbConn db.Database, loopStatus *api.LoopStatus) MonitorPanel {
	return MonitorPanel{
		dbConn:     dbConn,
		loopStatus: loopStatus,
	}
}

func (p MonitorPanel) Init() tea.Cmd {
	return monitorTickCmd()
}

func (p MonitorPanel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case api.GatewayHealth:
		p.latestHealth = msg
		p.healthReceivedAt = time.Now()

	case api.Telemetry:
		p.lastTelemetryAt = time.Now()

	case monitorTickMsg:
		p.sysState, p.sysStateErr = p.dbConn.SelectCurrentSystemState()
		return p, monitorTickCmd()

	case tea.WindowSizeMsg:
		p.w = msg.Width
		p.h = msg.Height
	}

	return p, nil
}

func (p MonitorPanel) View() string {
	s := style.Header.Render("System Monitor")

	s += "\n\n" + "Agent State:"
	switch {
	case p.sysStateErr != nil:
		s += "\n  UNKNOWN (no system_states row yet)"
	default:
		s += "\n  " + string(p.sysState.State) + " (since " + agoString(p.sysState.Time) + ")"
	}

	snap := p.loopStatus.Snapshot()
	s += "\n\n" + "Correction Loop:"
	s += "\n  last attempt: " + agoOrNever(snap.LastCorrectionAttempt)
	s += "\n  last success: " + agoOrNever(snap.LastCorrectionSuccess)

	s += "\n\n" + "Journal Loop:"
	s += "\n  last attempt: " + agoOrNever(snap.LastJournalAttempt)
	s += "\n  last success: " + agoOrNever(snap.LastJournalSuccess)

	s += "\n\n" + "Telemetry:"
	if p.lastTelemetryAt.IsZero() {
		s += "\n  no telemetry received this session"
	} else {
		age := time.Since(p.lastTelemetryAt)
		s += "\n  last received: " + agoString(p.lastTelemetryAt)
		if age > telemetryStaleAfter {
			s += "  [STALE]"
		}
	}

	s += "\n\n" + "Pi Edge Gateway:"
	if p.healthReceivedAt.IsZero() {
		s += "\n  no health report received this session"
	} else {
		age := time.Since(p.healthReceivedAt)
		stale := age > gatewayHealthStaleAfter
		s += "\n  last report: " + agoString(p.healthReceivedAt)
		if stale {
			s += "  [STALE — gateway may be unreachable]"
		}
		s += "\n  " + healthLine("hostapd", p.latestHealth.Hostapd, stale)
		s += "\n  " + healthLine("dnsmasq", p.latestHealth.Dnsmasq, stale)
		s += "\n  " + healthLine("uap0", p.latestHealth.Uap0, stale)
		s += "\n  " + healthLine("mosquitto", p.latestHealth.Mosquitto, stale)
		s += "\n  " + healthLine("bridge->aws", p.latestHealth.Bridge, stale)
		s += "\n  " + healthLine("pi-relay", p.latestHealth.PiRelay, stale)
	}

	s += "\n\n" + "(<tab> to switch panel | <ctrl>+C to quit)"

	return s
}

func healthLine(name string, up bool, stale bool) string {
	status := "DOWN"
	if up {
		status = "UP"
	}
	if stale {
		status = "?"
	}
	return fmt.Sprintf("%-12s %s", name, status)
}

func agoOrNever(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	return agoString(t)
}

func agoString(t time.Time) string {
	d := time.Since(t).Round(time.Second)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	default:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
}
