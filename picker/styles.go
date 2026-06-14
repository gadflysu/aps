package picker

import (
	lipgloss "charm.land/lipgloss/v2"

	"github.com/gadflysu/aps/display"
)

// Column content widths (padding excluded). Outer width = colW + 2 (PaddingLeft+PaddingRight).
const (
	titleColWidth = 40 // fixed in TUI mode; adaptive in list mode via display.AdaptiveTitleWidth
	timeColW      = 19 // "2006-01-02 15:04:05"
	srcColW       = 11 // "claude" / "opencode"

	// spinnerColW is the fixed width of the leading spinner cell (never scrolled).
	spinnerColW = 2
	// hScrollStep is the number of display columns advanced per left/right key press.
	hScrollStep = 4
)

var (
	// Each field owns its surrounding spaces via PaddingLeft/PaddingRight.
	// Boundaries between fields are: left field's right-pad + right field's left-pad,
	// so under Reverse each space gets its own background color.
	// Width = content + PaddingLeft(1) + PaddingRight(1) because in lipgloss v2
	// Width sets the outer box width (padding included).
	timeStyle  = lipgloss.NewStyle().Foreground(display.ColorTime).Width(timeColW+2).PaddingLeft(1).PaddingRight(1)
	titleStyle = lipgloss.NewStyle().Foreground(display.ColorTitle).Width(titleColWidth+2).PaddingLeft(1).PaddingRight(1)
	idStyle    = lipgloss.NewStyle().Foreground(display.ColorID).Width(12+2).PaddingLeft(1).PaddingRight(1)
	msgStyle   = lipgloss.NewStyle().Foreground(display.ColorMsg).Width(6+2).PaddingLeft(1).PaddingRight(1)
	srcStyle   = lipgloss.NewStyle().Foreground(display.ColorSrc).Width(srcColW+2).PaddingLeft(1).PaddingRight(1)
	dirStyle   = lipgloss.NewStyle().Foreground(display.ColorMuted)

	// Selected-state variants: every cell gets Reverse(true) so that the
	// highlight survives each cell's own ANSI reset sequence.
	timeStyleSel  = timeStyle.Copy().Reverse(true)
	titleStyleSel = titleStyle.Copy().Bold(true).Reverse(true)
	idStyleSel    = idStyle.Copy().Reverse(true)
	msgStyleSel   = msgStyle.Copy().Reverse(true)
	srcStyleSel   = srcStyle.Copy().Reverse(true)
	// dir is rendered via FormatDirCell; PaddingLeft(1) provides the leading space.
	dirStyleSel = lipgloss.NewStyle().Foreground(display.ColorDir).PaddingLeft(1).PaddingRight(1).Reverse(true)

	headerStyle = lipgloss.NewStyle().
			Underline(true).
			UnderlineSpaces(true).
			Foreground(display.ColorHeader)

	matchStyle = lipgloss.NewStyle().Foreground(display.ColorSpinner).Bold(true)

	spinnerStyleConfirmed = lipgloss.NewStyle().Foreground(display.ColorSpinner)
	spinnerStyleGuessed   = lipgloss.NewStyle().Foreground(display.ColorSpinner).Faint(true)

	statusStyle     = lipgloss.NewStyle().Foreground(display.ColorMuted).Background(display.ColorBackground)
	statusErrStyle  = lipgloss.NewStyle().Foreground(display.ColorError).Background(display.ColorBackground)
	statusHintStyle = lipgloss.NewStyle().Foreground(display.ColorID).Background(display.ColorBackground)
)
