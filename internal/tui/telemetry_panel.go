package tui

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"sonmi/internal/api"
	"sonmi/internal/db"
	"sonmi/internal/style"
)

type TelemetryPanel struct {
	dbConn     db.Database
	loopStatus *api.LoopStatus

	currTelemetry api.Telemetry
	relayChanged  relayTimestamps
	pending       pendingRelays

	sysState    db.SystemStateRow
	sysStateErr error

	w, h int
}

var _ tea.Model = TelemetryPanel{}

func InitializeTelemetryPanel(dbConn db.Database, loopStatus *api.LoopStatus, initialTelemetry api.Telemetry) TelemetryPanel {
	return TelemetryPanel{
		dbConn:        dbConn,
		loopStatus:    loopStatus,
		currTelemetry: initialTelemetry,
	}
}

func (p TelemetryPanel) Init() tea.Cmd {
	// Deliberately no tea.Tick here — MonitorPanel owns the shared 5s tick
	// chain (monitorTickCmd/monitorTickMsg) and RightPanel forwards every
	// message to both panels. Re-arming from here too would double the
	// tick rate every cycle.
	return nil
}

func (p TelemetryPanel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case api.Telemetry:
		p.currTelemetry = msg
	case relayTimestamps:
		p.relayChanged = msg
	case pendingRelays:
		p.pending = msg
	case monitorTickMsg:
		p.sysState, p.sysStateErr = p.dbConn.SelectCurrentSystemState()
	case tea.WindowSizeMsg:
		p.w = msg.Width
		p.h = msg.Height
	}

	return p, nil
}

func (p TelemetryPanel) View() string {
	s := style.Header.Render("Telemetry Panel Data!")

	snap := p.loopStatus.Snapshot()
	s += "\n" + "previous correction loop: " + agoOrNever(snap.LastCorrectionSuccess)
	s += "\n" + "previous journal loop: " + agoOrNever(snap.LastJournalSuccess)

	s += "\n"
	if p.sysStateErr != nil {
		s += "\n" + "System State: UNKNOWN"
	} else {
		s += "\n" + "System State: " + string(p.sysState.State)
	}

	t := p.currTelemetry

	s += "\n" + fmt.Sprintf("Temp: %.2gC", t.Sensors.Temperature)
	s += "\n" + fmt.Sprintf("Air: %.2g%%", t.Sensors.AirHumidity)
	s += "\n" + fmt.Sprintf("Soil: %.2g%%", t.Sensors.SoilHumidity)

	s += "\n"
	s += "\n" + p.relayLine("[w]ater pump ON", db.RelayWaterPump, t.Relays.WaterPump, p.relayChanged.WaterPump)
	s += "\n" + p.relayLine("grow [l]ight ON", db.RelayGrowLight, t.Relays.GrowLight, p.relayChanged.GrowLight)
	s += "\n" + p.relayLine("i[n]take fan ON", db.RelayIntakeFan, t.Relays.IntakeFan, p.relayChanged.IntakeFan)
	s += "\n" + p.relayLine("[e]xhaust fan ON", db.RelayExhaustFan, t.Relays.ExhaustFan, p.relayChanged.ExhaustFan)

	s += "\n"
	s += "\n" + "(<ctrl>+C to quit) | (<ctrl>+[ ] to toggle relays) | (<tab> for monitor)"

	return s
}

func (p TelemetryPanel) relayLine(label string, relay db.Relay_t, value bool, changedAt time.Time) string {
	if pt, ok := p.pending[relay]; ok {
		return fmt.Sprintf("%s: %t → %t  [WAITING…] (requested %s)", label, value, pt.target, agoOrNever(pt.requestedAt))
	}
	return fmt.Sprintf("%s: %t (%s)", label, value, agoOrNever(changedAt))
}
