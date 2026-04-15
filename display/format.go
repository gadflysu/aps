package display

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	cterm "github.com/charmbracelet/x/term"

	"local/aps/source"
)

// Column widths (display columns, not bytes).
const (
	MaxTitleLimit = 40 // baseline cap for adaptive title width
	colTime       = 19 // fixed: "2006-01-02 15:04:05"
	colSrcWidth   = 11 // len("Claude Code")
	colSep        = "｜" // U+FF5C FULLWIDTH VERTICAL LINE
)

// List-mode lipgloss styles (directory = white, matching original ColorWhite).
var (
	listTimeStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Width(colTime)
	listTitleStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	listIDStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	listMsgStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("5"))
	listSrcStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("5")).Width(colSrcWidth)
	listDirStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("7")) // white for list
	listSepStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	listHeaderStyle = lipgloss.NewStyle().Underline(true).Foreground(lipgloss.Color("7"))
)

// TermWidth returns the terminal width of w, or 0 if w is not a TTY.
// Callers treat 0 as "unconstrained" (pipe / redirect).
func TermWidth(w io.Writer) int {
	type fdder interface{ Fd() uintptr }
	f, ok := w.(fdder)
	if !ok {
		return 0
	}
	fd := f.Fd()
	if !cterm.IsTerminal(fd) {
		return 0
	}
	width, _, err := cterm.GetSize(fd)
	if err != nil || width <= 0 {
		return 0
	}
	return width
}

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

// AdaptiveMsgWidth returns the column width needed to display the widest message count.
// The minimum is len("MSG") so the header always fits.
func AdaptiveMsgWidth(sessions []source.Session) int {
	max := len("MSG")
	for _, s := range sessions {
		if w := len(fmt.Sprintf("%d", s.MsgCount)); w > max {
			max = w
		}
	}
	return max
}

// AdaptiveIDWidth returns the column width needed to display the widest session ID.
func AdaptiveIDWidth(sessions []source.Session) int {
	max := 0
	for _, s := range sessions {
		if w := lipgloss.Width(s.ID); w > max {
			max = w
		}
	}
	return max
}

// AdaptiveDirWidth returns the column width needed to display the widest CWDDisplay.
// When termWidth > 0, the result is capped at termWidth.
// When termWidth == 0, result is the natural maximum (no cap).
func AdaptiveDirWidth(sessions []source.Session, termWidth int) int {
	max := 0
	for _, s := range sessions {
		if w := lipgloss.Width(s.CWDDisplay); w > max {
			max = w
		}
	}
	if termWidth > 0 && max > termWidth {
		return termWidth
	}
	return max
}

// ListWidths holds pre-computed column widths for list-mode rendering.
type ListWidths struct {
	Title  int
	ID     int
	Msg    int
	Dir    int // 0 means unconstrained (pipe mode — Dir rendered without Width padding)
	Source int // 0 when not combined
}

// ComputeListWidths computes adaptive column widths for all sessions.
// termWidth==0 means stdout is not a TTY; no bonus space is allocated.
func ComputeListWidths(sessions []source.Session, includeSource bool, termWidth int) ListWidths {
	titles := extractTitles(sessions)
	titleW := AdaptiveTitleWidth(titles) // min(max, 40)
	idW := AdaptiveIDWidth(sessions)
	msgW := AdaptiveMsgWidth(sessions)
	dirW := AdaptiveDirWidth(sessions, termWidth)

	srcW := 0
	if includeSource {
		srcW = colSrcWidth
	}

	// Separators: one between each adjacent column pair.
	// colSep is U+FF5C FULLWIDTH VERTICAL LINE = 2 display columns.
	// Columns: TIME, TITLE, ID, MSG, [SRC,] DIR = 5 or 6 columns → 4 or 5 separators.
	numCols := 5
	if includeSource {
		numCols = 6
	}
	seps := (numCols - 1) * lipgloss.Width(colSep)

	naturalW := colTime + titleW + idW + msgW + srcW + dirW + seps

	// Bonus: surplus terminal width goes entirely to TITLE.
	if termWidth > 0 && naturalW < termWidth {
		titleW += termWidth - naturalW
	}

	return ListWidths{
		Title:  titleW,
		ID:     idW,
		Msg:    msgW,
		Dir:    dirW,
		Source: srcW,
	}
}

// FormatListRow formats a session for plain list output.
func FormatListRow(s source.Session, w ListWidths) string {
	sep := listSepStyle.Render(colSep)

	row := listTimeStyle.Render(formatTime(s.Time)) + sep +
		listTitleStyle.Copy().Width(w.Title).Render(TruncateWidth(Sanitize(s.Title), w.Title, "…")) + sep +
		listIDStyle.Copy().Width(w.ID).Render(Sanitize(s.ID)) + sep +
		listMsgStyle.Copy().Width(w.Msg).Render(fmt.Sprintf("%d", s.MsgCount))

	if w.Source > 0 {
		row += sep + listSrcStyle.Render(s.Client.String())
	}

	if w.Dir > 0 {
		row += sep + listDirStyle.Copy().Width(w.Dir).Render(Sanitize(s.CWDDisplay))
	} else {
		row += sep + listDirStyle.Render(Sanitize(s.CWDDisplay))
	}

	return row
}

// Header returns a formatted header row for list mode.
func Header(w ListWidths) string {
	sep := listSepStyle.Render(colSep)
	h := listHeaderStyle

	row := h.Copy().Width(colTime).Render("TIME") + sep +
		h.Copy().Width(w.Title).Render("TITLE") + sep +
		h.Copy().Width(w.ID).Render("ID") + sep +
		h.Copy().Width(w.Msg).Render("MSG")

	if w.Source > 0 {
		row += sep + h.Copy().Width(colSrcWidth).Render("SRC")
	}

	if w.Dir > 0 {
		row += sep + h.Copy().Width(w.Dir).Render("DIRECTORY")
	} else {
		row += sep + h.Render("DIRECTORY")
	}

	return row
}

// --- internal helpers ---

func extractTitles(sessions []source.Session) []string {
	titles := make([]string, len(sessions))
	for i, s := range sessions {
		titles[i] = s.Title
	}
	return titles
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

// keep unexported aliases so internal callers (if any) remain unchanged
func sanitize(s string) string        { return Sanitize(s) }
func truncateWidth(s string, n int) string { return TruncateWidth(s, n, "…") }
