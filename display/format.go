package display

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"local/aps/source"
)

// Column widths (display columns, not bytes).
const (
	MaxTitleLimit   = 40 // cap for adaptive title width
	colTime         = 19 // fixed: "2006-01-02 15:04:05"
	colMsgCount     = 6
	colSrcWidth     = 11 // len("Claude Code")
	colIDClaudeFull = 36 // full UUID, list mode
	colIDOpencode   = 30
	colSep          = "｜" // U+FF5C FULLWIDTH VERTICAL LINE
)

// List-mode lipgloss styles (directory = white, matching original ColorWhite).
var (
	listTimeStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Width(colTime)
	listTitleStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	listIDStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	listMsgStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("5")).Width(colMsgCount)
	listSrcStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("5")).Width(colSrcWidth)
	listDirStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("7")) // white for list
	listSepStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	listHeaderStyle = lipgloss.NewStyle().Underline(true).Foreground(lipgloss.Color("7"))
)

// AdaptiveTitleWidth returns min(MaxTitleLimit, maxActualDisplayWidth) across titles.
func AdaptiveTitleWidth(titles []string) int {
	max := 0
	for _, t := range titles {
		if w := lipgloss.Width(t); w > max {
			max = w
		}
	}
	if max > MaxTitleLimit {
		return MaxTitleLimit
	}
	return max
}

// FormatListRow formats a session for plain list output (no hidden TAB fields).
func FormatListRow(s source.Session, titleWidth int, includeSource bool) string {
	idW := listIDColWidth(s)
	sep := listSepStyle.Render(colSep)

	row := listTimeStyle.Render(formatTime(s.Time)) + sep +
		listTitleStyle.Copy().Width(titleWidth).MaxWidth(titleWidth).Render(truncateWidth(sanitize(s.Title), titleWidth)) + sep +
		listIDStyle.Copy().Width(idW).Render(sanitize(s.ID)) + sep +
		listMsgStyle.Render(fmt.Sprintf("%d", s.MsgCount))
	if includeSource {
		row += sep + listSrcStyle.Render(s.Client.String())
	}
	row += sep + listDirStyle.Render(sanitize(s.CWDDisplay))
	return row
}

// Header returns a formatted header row for list mode.
func Header(titleWidth int, includeSource bool) string {
	sep := listSepStyle.Render(colSep)
	h := listHeaderStyle

	row := h.Copy().Width(colTime).Render("TIME") + sep +
		h.Copy().Width(titleWidth).Render("TITLE") + sep +
		h.Copy().Width(colIDClaudeFull).Render("ID") + sep +
		h.Copy().Width(colMsgCount).Render("MSG")
	if includeSource {
		row += sep + h.Copy().Width(colSrcWidth).Render("SRC")
	}
	row += sep + h.Render("DIRECTORY")
	return row
}

// --- internal helpers ---

func listIDColWidth(s source.Session) int {
	if s.Client == source.ClientClaude {
		return colIDClaudeFull
	}
	return colIDOpencode
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return "No time"
	}
	return t.Format("2006-01-02 15:04:05")
}

// Sanitize replaces tab and newline characters with spaces.
func Sanitize(s string) string {
	s = strings.ReplaceAll(s, "\t", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}

// TruncateWidth truncates s to at most maxCols display columns, CJK-aware.
// tail (e.g. "…") is appended when truncation occurs and counts toward maxCols.
// Uses lipgloss.Width (which wraps go-runewidth) to measure each candidate.
func TruncateWidth(s string, maxCols int, tail string) string {
	runes := []rune(s)
	tailCols := lipgloss.Width(tail)
	if lipgloss.Width(string(runes)) <= maxCols {
		return string(runes)
	}
	for len(runes) > 0 && lipgloss.Width(string(runes))+tailCols > maxCols {
		runes = runes[:len(runes)-1]
	}
	return string(runes) + tail
}

// keep unexported alias so internal callers are unchanged
func sanitize(s string) string { return Sanitize(s) }
func truncateWidth(s string, n int) string { return TruncateWidth(s, n, "…") }
