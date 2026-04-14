package display

import (
	"fmt"
	"strings"
	"time"

	"local/aps/source"
)

// FormatInteractiveClaude formats a Claude session for fzf input.
// Output: <session_id>\t<project_path>\t<cwd>\t<display>
func FormatInteractiveClaude(s source.Session, titleWidth int) string {
	dID := s.ID
	if len(dID) > ColIDClaude {
		dID = dID[:ColIDClaude]
	}
	display := buildLine(s, dID, titleWidth, ColorDarkGrey, false)
	return fmt.Sprintf("%s\t%s\t%s\t%s",
		sanitize(s.ID), sanitize(s.ProjectPath), sanitize(s.CWD), display)
}

// FormatInteractiveOpencode formats an Opencode session for fzf input.
// Output: <session_id>\t<cwd>\t<display>
func FormatInteractiveOpencode(s source.Session, titleWidth int) string {
	display := buildLine(s, s.ID, titleWidth, ColorDarkGrey, false)
	return fmt.Sprintf("%s\t%s\t%s",
		sanitize(s.ID), sanitize(s.CWD), display)
}

// FormatInteractiveAll formats a session for combined fzf input.
// Output: <source>\t<id>\t<project_path_or_empty>\t<cwd>\t<display>
// project_path is set for Claude sessions; empty string for Opencode.
func FormatInteractiveAll(s source.Session, titleWidth int) string {
	display := buildLineWithSrc(s, titleWidth, ColorDarkGrey)
	return fmt.Sprintf("%s\t%s\t%s\t%s\t%s",
		sanitize(s.Client.String()), sanitize(s.ID),
		sanitize(s.ProjectPath), sanitize(s.CWD), display)
}

// FormatListRow formats a session for plain list output (no hidden TAB fields).
func FormatListRow(s source.Session, titleWidth int, includeSource bool) string {
	if includeSource {
		return buildLineWithSrc(s, titleWidth, ColorWhite)
	}
	idWidth := idWidth(s)
	return buildLineWithID(s, s.ID, idWidth, titleWidth, ColorWhite)
}

// Header returns a formatted header row for list mode.
func Header(titleWidth int, includeSource bool) string {
	h := ColorHeaderUL
	line := h + Pad("TIME", ColTime) + ColorReset + ColSep +
		h + Pad("TITLE", titleWidth) + ColorReset + ColSep +
		h + Pad("ID", idHeaderWidth(includeSource)) + ColorReset + ColSep +
		h + Pad("MSG", ColMsgCount) + ColorReset
	if includeSource {
		line += ColSep + h + Pad("SRC", ColSrcWidth) + ColorReset
	}
	line += ColSep + h + "DIRECTORY" + ColorReset
	return line
}

// --- internal helpers ---

func buildLine(s source.Session, displayID string, titleWidth int, dirColor string, _ bool) string {
	return buildLineWithID(s, displayID, Width(displayID), titleWidth, dirColor)
}

func buildLineWithID(s source.Session, displayID string, idColWidth int, titleWidth int, dirColor string) string {
	timeStr := formatTime(s.Time)
	title := s.Title
	if Width(title) > titleWidth {
		title = Truncate(title, titleWidth)
	}

	return ColorGreen + Pad(timeStr, ColTime) + ColorReset + ColSep +
		ColorYellow + Pad(sanitize(title), titleWidth) + ColorReset + ColSep +
		ColorCyan + Pad(sanitize(displayID), idColWidth) + ColorReset + ColSep +
		ColorMagenta + Pad(fmt.Sprintf("%d", s.MsgCount), ColMsgCount) + ColorReset + ColSep +
		dirColor + sanitize(s.CWDDisplay) + ColorReset
}

func buildLineWithSrc(s source.Session, titleWidth int, dirColor string) string {
	idW := ColIDClaudeFull // full UUID width for combined mode
	timeStr := formatTime(s.Time)
	title := s.Title
	if Width(title) > titleWidth {
		title = Truncate(title, titleWidth)
	}

	return ColorGreen + Pad(timeStr, ColTime) + ColorReset + ColSep +
		ColorYellow + Pad(sanitize(title), titleWidth) + ColorReset + ColSep +
		ColorCyan + Pad(sanitize(s.ID), idW) + ColorReset + ColSep +
		ColorMagenta + Pad(fmt.Sprintf("%d", s.MsgCount), ColMsgCount) + ColorReset + ColSep +
		ColorMagenta + Pad(s.Client.String(), ColSrcWidth) + ColorReset + ColSep +
		dirColor + sanitize(s.CWDDisplay) + ColorReset
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return "No time"
	}
	return t.Format("2006-01-02 15:04:05")
}

func sanitize(s string) string {
	s = strings.ReplaceAll(s, "\t", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}

func idWidth(s source.Session) int {
	switch s.Client {
	case source.ClientClaude:
		return ColIDClaudeFull
	default:
		return ColIDOpencode
	}
}

func idHeaderWidth(includeSource bool) int {
	if includeSource {
		return ColIDClaudeFull
	}
	return ColIDClaudeFull // use max for header
}
