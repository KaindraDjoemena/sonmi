package style

import (
	"github.com/charmbracelet/lipgloss"
)

var Header = lipgloss.NewStyle().
	Bold(true).
	Foreground(lipgloss.Color("#FAFAFA")).
	Background(lipgloss.Color("#7D56F4"))
