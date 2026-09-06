package tui

import (
	"image"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	lipgloss "github.com/charmbracelet/lipgloss"

	"sonmi/internal/api"
	"sonmi/internal/db"
)

type KeyCmd_t string

const (
	CMD_QUIT_APP       KeyCmd_t = "ctrl+c"
	CMD_TOGGLE_WATER   KeyCmd_t = "ctrl+w"
	CMD_TOGGLE_LIGHT   KeyCmd_t = "ctrl+l"
	CMD_TOGGLE_INTAKE  KeyCmd_t = "ctrl+n"
	CMD_TOGGLE_EXHAUST KeyCmd_t = "ctrl+e"

	// waterPumpPulseDurationS is the safe default run time sent for manual pump activations.
	waterPumpPulseDurationS = 5
)

const relayConfirmTimeout = 12 * time.Second

type pendingToggle struct {
	target      bool
	requestedAt time.Time
}

type pendingRelays map[db.Relay_t]pendingToggle

type relayCmdErr struct {
	relay db.Relay_t
	err   error
}

// relayTimestamps tracks when each relay's state was last hardware-confirmed
// changed (per api.RelayState.Time), for display in TelemetryPanel.
type relayTimestamps struct {
	WaterPump  time.Time
	GrowLight  time.Time
	IntakeFan  time.Time
	ExhaustFan time.Time
}

type AppState int

const (
	StateNormal AppState = iota
	StateTypingDuration
	StateTypingRationale
)

type appModel struct {
	telemetryPipe         chan api.Telemetry
	relayStatePipe        chan api.RelayState
	gatewayHealthPipe     chan api.GatewayHealth
	latestTelemetry       api.Telemetry
	latestRelayTimestamps relayTimestamps

	controller api.DeviceController

	state           AppState
	txtInput        textinput.Model
	pendingDuration uint

	pending   map[db.Relay_t]pendingToggle
	statusMsg string

	leftPanel  tea.Model
	rightPanel tea.Model
	width      int
	height     int
}

var _ tea.Model = appModel{} // assigns appModel{} to a tea.Model _ object to check interface implementation compliance

func InitialModel(pipe chan image.Image, telemetryPipe chan api.Telemetry, relayStatePipe chan api.RelayState, gatewayHealthPipe chan api.GatewayHealth, ctrl api.DeviceController, dbConn db.Database, loopStatus *api.LoopStatus) appModel {

	// Fetch latest telemetry from the db
	var initData api.Telemetry

	telRows, _ := dbConn.SelectPastNHourTelemetryRows(1)
	if len(telRows) > 0 {
		initData.Sensors.Temperature = telRows[0].Temp
		initData.Sensors.AirHumidity = telRows[0].AirHumidity
		initData.Sensors.SoilHumidity = telRows[0].SoilHumidity
	}

	var initRelayTimestamps relayTimestamps
	initData.Relays.WaterPump, initRelayTimestamps.WaterPump = dbConn.GetLastKnownRelayStateWithTime(db.RelayWaterPump)
	initData.Relays.GrowLight, initRelayTimestamps.GrowLight = dbConn.GetLastKnownRelayStateWithTime(db.RelayGrowLight)
	initData.Relays.IntakeFan, initRelayTimestamps.IntakeFan = dbConn.GetLastKnownRelayStateWithTime(db.RelayIntakeFan)
	initData.Relays.ExhaustFan, initRelayTimestamps.ExhaustFan = dbConn.GetLastKnownRelayStateWithTime(db.RelayExhaustFan)

	telemetryPanel := InitializeTelemetryPanel(dbConn, loopStatus, initData)
	telemetryPanel.relayChanged = initRelayTimestamps
	rightPanel := InitializeRightPanel(telemetryPanel, InitializeMonitorPanel(dbConn, loopStatus))

	ti := textinput.New()
	ti.Placeholder = "Duration..."
	ti.CharLimit = 256
	ti.Width = 30

	return appModel{
		telemetryPipe:         telemetryPipe,
		relayStatePipe:        relayStatePipe,
		gatewayHealthPipe:     gatewayHealthPipe,
		latestTelemetry:       initData,
		latestRelayTimestamps: initRelayTimestamps,
		controller:            ctrl,
		state:                 StateNormal,
		txtInput:              ti,
		pending:               make(map[db.Relay_t]pendingToggle),
		leftPanel:             InitializeCameraPanel(pipe),
		rightPanel:            rightPanel,
	}
}

func waitForTelemetry(pipe chan api.Telemetry) tea.Cmd {
	return func() tea.Msg {
		return <-pipe
	}
}

// waits for hardware confirmation before piping to the ui
func waitForRelayState(pipe chan api.RelayState) tea.Cmd {
	return func() tea.Msg {
		return <-pipe
	}
}

func waitForGatewayHealth(pipe chan api.GatewayHealth) tea.Cmd {
	return func() tea.Msg {
		return <-pipe
	}
}

func (m appModel) Init() tea.Cmd {
	return tea.Batch(
		waitForTelemetry(m.telemetryPipe),
		waitForRelayState(m.relayStatePipe),
		waitForGatewayHealth(m.gatewayHealthPipe),
		m.leftPanel.Init(),
		m.rightPanel.Init(),
	)
}

func (m appModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {

	if telMsg, ok := msg.(api.Telemetry); ok {
		m.latestTelemetry.Sensors = telMsg.Sensors

		// Reconcile relays from the 5-min telemetry too, as a backstop for a
		// missed api.RelayState confirmation — but never stomp a toggle that is
		// still waiting for its own confirmation.
		if _, waiting := m.pending[db.RelayWaterPump]; !waiting {
			m.latestTelemetry.Relays.WaterPump = telMsg.Relays.WaterPump
		}
		if _, waiting := m.pending[db.RelayGrowLight]; !waiting {
			m.latestTelemetry.Relays.GrowLight = telMsg.Relays.GrowLight
		}
		if _, waiting := m.pending[db.RelayIntakeFan]; !waiting {
			m.latestTelemetry.Relays.IntakeFan = telMsg.Relays.IntakeFan
		}
		if _, waiting := m.pending[db.RelayExhaustFan]; !waiting {
			m.latestTelemetry.Relays.ExhaustFan = telMsg.Relays.ExhaustFan
		}

		msg = m.latestTelemetry // Replace the message with our merged version
	}

	var cmdLeft, cmdRight tea.Cmd
	m.leftPanel, cmdLeft = m.leftPanel.Update(msg)
	m.rightPanel, cmdRight = m.rightPanel.Update(msg)

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, tea.Batch(cmdLeft, cmdRight)

	case tea.KeyMsg:

		// Text Input Field
		if m.state != StateNormal {
			switch msg.String() {
			case string(CMD_QUIT_APP):
				return m, tea.Batch(cmdLeft, cmdRight, tea.Quit)
			case "esc":
				m.state = StateNormal
				m.txtInput.Blur()
				m.txtInput.Reset()
				return m, tea.Batch(cmdLeft, cmdRight)
			case "enter":
				val := m.txtInput.Value()

				switch m.state {
				case StateTypingDuration:
					parsedVal, err := strconv.Atoi(val)

					if err == nil {
						m.pendingDuration = uint(parsedVal)
					} else {
						m.pendingDuration = 5
					}

					m.state = StateTypingRationale
					m.txtInput.Reset()
					m.txtInput.Placeholder = "Why are you watering?"
					return m, tea.Batch(cmdLeft, cmdRight)

				case StateTypingRationale:
					m.state = StateNormal
					m.txtInput.Blur()
					m.txtInput.Reset()
					m.pending[db.RelayWaterPump] = pendingToggle{target: true, requestedAt: time.Now()}
					m.statusMsg = ""
					m.rightPanel, _ = m.rightPanel.Update(m.pendingSnapshot())
					return m, tea.Batch(cmdLeft, cmdRight, triggerWaterPumpCmd(m.controller, m.pendingDuration, val))
				}
			}

			var cmd tea.Cmd
			m.txtInput, cmd = m.txtInput.Update(msg)
			return m, tea.Batch(cmdLeft, cmdRight, cmd)
		}

		// Normal Page
		switch strings.ToLower(msg.String()) {
		case string(CMD_QUIT_APP):
			return m, tea.Batch(cmdLeft, cmdRight, tea.Quit)
		case string(CMD_TOGGLE_WATER):
			m.state = StateTypingDuration
			m.txtInput.Focus()
			m.txtInput.Placeholder = "Enter pump duration (s): "
			return m, tea.Batch(cmdLeft, cmdRight, textinput.Blink)
		case string(CMD_TOGGLE_LIGHT):
			target := !m.latestTelemetry.Relays.GrowLight
			m.pending[db.RelayGrowLight] = pendingToggle{target: target, requestedAt: time.Now()}
			m.statusMsg = ""
			m.rightPanel, _ = m.rightPanel.Update(m.pendingSnapshot())
			return m, tea.Batch(cmdLeft, cmdRight, triggerGrowLightCmd(m.controller, target, ""))
		case string(CMD_TOGGLE_INTAKE):
			target := !m.latestTelemetry.Relays.IntakeFan
			m.pending[db.RelayIntakeFan] = pendingToggle{target: target, requestedAt: time.Now()}
			m.statusMsg = ""
			m.rightPanel, _ = m.rightPanel.Update(m.pendingSnapshot())
			return m, tea.Batch(cmdLeft, cmdRight, triggerIntakeFanCmd(m.controller, target, ""))
		case string(CMD_TOGGLE_EXHAUST):
			target := !m.latestTelemetry.Relays.ExhaustFan
			m.pending[db.RelayExhaustFan] = pendingToggle{target: target, requestedAt: time.Now()}
			m.statusMsg = ""
			m.rightPanel, _ = m.rightPanel.Update(m.pendingSnapshot())
			return m, tea.Batch(cmdLeft, cmdRight, triggerExhautFanCmd(m.controller, target, ""))
		}
		return m, tea.Batch(cmdLeft, cmdRight)

	case api.Telemetry:
		m.latestTelemetry.Sensors = msg.Sensors

		return m, tea.Batch(cmdLeft, cmdRight, waitForTelemetry(m.telemetryPipe))

	case api.RelayState:
		// Hardware has confirmed a relay state change — update the UI to mirror reality.
		switch msg.Relay {
		case db.RelayWaterPump:
			m.latestTelemetry.Relays.WaterPump = msg.Value
			m.latestRelayTimestamps.WaterPump = msg.Time
		case db.RelayGrowLight:
			m.latestTelemetry.Relays.GrowLight = msg.Value
			m.latestRelayTimestamps.GrowLight = msg.Time
		case db.RelayIntakeFan:
			m.latestTelemetry.Relays.IntakeFan = msg.Value
			m.latestRelayTimestamps.IntakeFan = msg.Time
		case db.RelayExhaustFan:
			m.latestTelemetry.Relays.ExhaustFan = msg.Value
			m.latestRelayTimestamps.ExhaustFan = msg.Time
		}

		if p, ok := m.pending[msg.Relay]; ok {
			if p.target != msg.Value {
				m.statusMsg = string(msg.Relay) + ": hardware confirmed opposite of request"
			} else {
				m.statusMsg = ""
			}
		}

		delete(m.pending, msg.Relay)

		m.rightPanel, _ = m.rightPanel.Update(m.latestRelayTimestamps)
		m.rightPanel, _ = m.rightPanel.Update(m.latestTelemetry)
		m.rightPanel, _ = m.rightPanel.Update(m.pendingSnapshot())

		return m, tea.Batch(cmdLeft, cmdRight, waitForRelayState(m.relayStatePipe))

	case monitorTickMsg:
		// Sweep pending toggles that never got a hardware confirmation. Don't
		// re-arm the tick here — MonitorPanel.Update owns monitorTickCmd().
		changed := false
		for relay, pt := range m.pending {
			if time.Since(pt.requestedAt) > relayConfirmTimeout {
				delete(m.pending, relay)
				m.statusMsg = string(relay) + ": no hardware confirmation — reverted to last known state"
				changed = true
			}
		}
		if changed {
			m.rightPanel, _ = m.rightPanel.Update(m.pendingSnapshot())
		}
		return m, tea.Batch(cmdLeft, cmdRight)

	case relayCmdErr:
		delete(m.pending, msg.relay)
		m.statusMsg = string(msg.relay) + ": send failed: " + msg.err.Error()
		m.rightPanel, _ = m.rightPanel.Update(m.pendingSnapshot())
		return m, tea.Batch(cmdLeft, cmdRight)

	case api.GatewayHealth:
		return m, tea.Batch(cmdLeft, cmdRight, waitForGatewayHealth(m.gatewayHealthPipe))
	}

	return m, tea.Batch(cmdLeft, cmdRight)
}

func (m appModel) View() string {

	leftStr := m.leftPanel.View()
	rightStr := m.rightPanel.View()

	leftStyle := lipgloss.NewStyle().Width((m.width / 2) - 2).Height(m.height - 2).Border(lipgloss.NormalBorder())
	rightStyle := lipgloss.NewStyle().Width((m.width / 2) - 2).Height(m.height - 2).Border(lipgloss.NormalBorder())

	renderedLeft := leftStyle.Render(lipgloss.PlaceHorizontal((m.width/2)-2, lipgloss.Center, leftStr))
	renderedRight := rightStyle.Render(rightStr)

	body := lipgloss.JoinHorizontal(lipgloss.Top, renderedLeft, renderedRight)

	if m.state != StateNormal {
		body = lipgloss.JoinVertical(lipgloss.Left, body, "\n  "+m.txtInput.View())
	}
	if m.statusMsg != "" {
		body = lipgloss.JoinVertical(lipgloss.Left, body, "  ⚠ "+m.statusMsg)
	}

	return body
}

// //////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
func triggerWaterPumpCmd(ctrl api.DeviceController, duration uint, rationale string) tea.Cmd {
	return func() tea.Msg {
		if err := ctrl.ToggleWaterPump(duration, db.ModeOverride, rationale); err != nil {
			return relayCmdErr{relay: db.RelayWaterPump, err: err}
		}
		return nil
	}
}
func triggerGrowLightCmd(ctrl api.DeviceController, state bool, rationale string) tea.Cmd {
	return func() tea.Msg {
		if err := ctrl.ToggleGrowLight(state, db.ModeOverride, rationale); err != nil {
			return relayCmdErr{relay: db.RelayGrowLight, err: err}
		}
		return nil
	}
}
func triggerIntakeFanCmd(ctrl api.DeviceController, state bool, rationale string) tea.Cmd {
	return func() tea.Msg {
		if err := ctrl.ToggleIntakeFan(state, db.ModeOverride, rationale); err != nil {
			return relayCmdErr{relay: db.RelayIntakeFan, err: err}
		}
		return nil
	}
}
func triggerExhautFanCmd(ctrl api.DeviceController, state bool, rationale string) tea.Cmd {
	return func() tea.Msg {
		if err := ctrl.ToggleExhaustFan(state, db.ModeOverride, rationale); err != nil {
			return relayCmdErr{relay: db.RelayExhaustFan, err: err}
		}
		return nil
	}
}

func (m appModel) pendingSnapshot() pendingRelays {
	cp := make(pendingRelays, len(m.pending))
	for k, v := range m.pending {
		cp[k] = v
	}

	return cp
}
