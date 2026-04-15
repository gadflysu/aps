package display

import "github.com/charmbracelet/lipgloss"

// Shared ANSI 16-color palette used by both list mode and the interactive picker.
// Using ANSI 16 respects the user's terminal color theme.
const (
	ColorTime   = lipgloss.Color("2") // green
	ColorTitle  = lipgloss.Color("3") // yellow
	ColorID     = lipgloss.Color("7") // white
	ColorMsg    = lipgloss.Color("5") // magenta
	ColorSrc    = lipgloss.Color("5") // magenta (same as msg)
	ColorDir    = lipgloss.Color("6") // cyan
	ColorMuted  = lipgloss.Color("8") // dark grey (separators, borders, dim text)
	ColorHeader = lipgloss.Color("7") // white (header row)
)
