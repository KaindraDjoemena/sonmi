package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"sonmi/internal/api"
	"sonmi/internal/style"
)

type TelemetryPanel struct {
	currTelemetry api.Telemetry

	w, h int
}

var _ tea.Model = TelemetryPanel{}

func InitializeTelemetryPanel(initialTelemetry api.Telemetry) TelemetryPanel {
	return TelemetryPanel{
		currTelemetry: initialTelemetry,
	}
}

func (p TelemetryPanel) Init() tea.Cmd {
	return nil
}

func (p TelemetryPanel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case api.Telemetry:
		p.currTelemetry = msg
	case tea.WindowSizeMsg:
		p.w = msg.Width
		p.h = msg.Height
	}

	return p, nil
}

func (p TelemetryPanel) View() string {
	s := style.Header.Render("Telemetry Panel Data!")

	t := p.currTelemetry

	s += "\n" + fmt.Sprintf("Temp: %.2gC", t.Sensors.Temperature)
	s += "\n" + fmt.Sprintf("Air: %.2g%%", t.Sensors.AirHumidity)
	s += "\n" + fmt.Sprintf("Soil: %.2g%%", t.Sensors.SoilHumidity)

	s += "\n"
	s += "\n" + fmt.Sprintf("[w]ater pump ON: %t", t.Relays.WaterPump)
	s += "\n" + fmt.Sprintf("grow [l]ight ON: %t", t.Relays.GrowLight)
	s += "\n" + fmt.Sprintf("i[n]take fan ON: %t", t.Relays.IntakeFan)
	s += "\n" + fmt.Sprintf("[e]xhaust fan ON: %t", t.Relays.ExhaustFan)

	s += "\n"
	s += "\n" + "(<ctrl>+C to quit) | (<ctrl>+[ ] to toggle relays) | (<tab> for monitor)"

	return s
}
