package picker

import "github.com/charmbracelet/lipgloss"

// titleColWidth is the fixed title column width in TUI (interactive) mode.
// In list mode the width is adaptive; see display.AdaptiveTitleWidth.
const titleColWidth = 40

// ANSI 16-color palette — respect the user's terminal color theme.
const (
	colorTime   = lipgloss.Color("2") // ANSI green
	colorTitle  = lipgloss.Color("3") // ANSI yellow
	colorID     = lipgloss.Color("6") // ANSI cyan
	colorMsg    = lipgloss.Color("5") // ANSI magenta
	colorDir    = lipgloss.Color("8") // ANSI dark grey (normal row)
	colorDirSel = lipgloss.Color("7") // ANSI white    (selected row)
	colorBorder = lipgloss.Color("8") // ANSI dark grey
)

var (
	timeStyle = lipgloss.NewStyle().Foreground(colorTime).Width(19)
	titleStyle = lipgloss.NewStyle().Foreground(colorTitle).
			Width(titleColWidth).MaxWidth(titleColWidth)
	idStyle  = lipgloss.NewStyle().Foreground(colorID).Width(12)
	msgStyle = lipgloss.NewStyle().Foreground(colorMsg).Width(6)
	srcStyle = lipgloss.NewStyle().Foreground(colorMsg).Width(11)
	dirStyle = lipgloss.NewStyle().Foreground(colorDir)
	sepStyle = lipgloss.NewStyle().Foreground(colorDir)

	// Selected-state variants: title bold, directory brightens to white.
	titleStyleSel = titleStyle.Copy().Bold(true)
	dirStyleSel   = lipgloss.NewStyle().Foreground(colorDirSel)

	// previewBorder adds BorderLeft(1) + PaddingLeft(1) = 2 cols of chrome.
	// Viewport content width = panel width - 2.
	previewBorder = lipgloss.NewStyle().
			BorderLeft(true).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(colorBorder).
			PaddingLeft(1)
)
