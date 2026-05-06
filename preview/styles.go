package preview

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/gadflysu/aps/display"
)

// Label styles use the same color constants as the list/picker for consistency.
var (
	previewLabelTitle = lipgloss.NewStyle().Foreground(display.ColorTitle).Bold(true)
	previewLabelTime  = lipgloss.NewStyle().Foreground(display.ColorTime).Bold(true)
	previewLabelMsg   = lipgloss.NewStyle().Foreground(display.ColorMsg).Bold(true)
	previewLabelDir   = lipgloss.NewStyle().Foreground(display.ColorDir).Bold(true)

	previewHeader  = lipgloss.NewStyle().Foreground(display.ColorDir).Bold(true)
	previewBullet  = lipgloss.NewStyle().Foreground(display.ColorMuted).Bold(true)
	previewMissing = lipgloss.NewStyle().Foreground(display.ColorMuted)
)
