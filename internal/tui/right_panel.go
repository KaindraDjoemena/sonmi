package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// RightPanel switches between the TelemetryPanel and MonitorPanel on <tab>,
// forwarding every other message to both so neither falls behind while hidden.
type rightPanelView int

const (
	viewTelemetry rightPanelView = iota
	viewMonitor
)

type RightPanel struct {
	active    rightPanelView
	telemetry TelemetryPanel
	monitor   MonitorPanel
}

var _ tea.Model = RightPanel{}

func InitializeRightPanel(telemetry TelemetryPanel, monitor MonitorPanel) RightPanel {
	return RightPanel{
		active:    viewTelemetry,
		telemetry: telemetry,
		monitor:   monitor,
	}
}

func (p RightPanel) Init() tea.Cmd {
	return tea.Batch(p.telemetry.Init(), p.monitor.Init())
}

func (p RightPanel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok && key.String() == "tab" {
		if p.active == viewTelemetry {
			p.active = viewMonitor
		} else {
			p.active = viewTelemetry
		}
		return p, nil
	}

	tModel, tCmd := p.telemetry.Update(msg)
	p.telemetry = tModel.(TelemetryPanel)

	mModel, mCmd := p.monitor.Update(msg)
	p.monitor = mModel.(MonitorPanel)

	return p, tea.Batch(tCmd, mCmd)
}

func (p RightPanel) View() string {
	if p.active == viewMonitor {
		return p.monitor.View()
	}
	return p.telemetry.View()
}
