package preview

import (
	lipgloss "charm.land/lipgloss/v2"

	"github.com/gadflysu/aps/display"
)

// Label styles use the same color constants as the list/picker for consistency.
var (
	previewLabelAgent = lipgloss.NewStyle().Foreground(display.ColorSrc).Bold(true)
	previewLabelTitle = lipgloss.NewStyle().Foreground(display.ColorTitle).Bold(true)
	previewLabelID    = lipgloss.NewStyle().Foreground(display.ColorID).Bold(true)
	previewLabelTime  = lipgloss.NewStyle().Foreground(display.ColorTime).Bold(true)
	previewLabelMsg   = lipgloss.NewStyle().Foreground(display.ColorMsg).Bold(true)
	previewLabelData  = lipgloss.NewStyle().Foreground(display.ColorDir).Bold(true)
	previewLabelDir   = lipgloss.NewStyle().Foreground(display.ColorDir).Bold(true)

	previewHeader  = lipgloss.NewStyle().Foreground(display.ColorDir).Bold(true)
	previewBullet  = lipgloss.NewStyle().Foreground(display.ColorMuted).Bold(true)
	previewMissing = lipgloss.NewStyle().Foreground(display.ColorMuted)
)
