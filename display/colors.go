package display

import lipgloss "charm.land/lipgloss/v2"

// Shared ANSI 16-color palette used by both list mode and the interactive picker.
// Using ANSI 16 respects the user's terminal color theme.
var (
	ColorTime    = lipgloss.Color("2") // green
	ColorTitle   = lipgloss.Color("3") // yellow
	ColorID      = lipgloss.Color("7") // white
	ColorMsg     = lipgloss.Color("5") // magenta
	ColorSrc     = lipgloss.Color("5") // magenta (same as msg)
	ColorDir     = lipgloss.Color("6") // cyan
	ColorMuted   = lipgloss.Color("8") // dark grey (separators, borders, dim text)
	ColorHeader  = lipgloss.Color("7") // white (header row)
	ColorSpinner = lipgloss.Color("9") // bright red (renders as orange in most terminals, matches Claude brand)
	ColorError   = lipgloss.Color("1") // red (fatal/non-fatal errors)
)
