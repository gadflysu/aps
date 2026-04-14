package display

import (
	"github.com/mattn/go-runewidth"
)

const (
	MaxTitleLimit = 40
	ColTime       = 19
	ColMsgCount   = 6
	ColSrcWidth   = 11 // len("Claude Code")
	ColIDClaude   = 12 // truncated UUID for interactive
	ColIDClaudeFull = 36 // full UUID for list mode
	ColIDOpencode = 30
)

// Width returns the terminal display width of s (CJK chars count as 2).
func Width(s string) int {
	return runewidth.StringWidth(s)
}

// Pad right-pads s to the given display width.
func Pad(s string, width int) string {
	w := Width(s)
	if w >= width {
		return s
	}
	for i := 0; i < width-w; i++ {
		s += " "
	}
	return s
}

// Truncate cuts s to at most maxWidth display columns, appending "..." if truncated.
func Truncate(s string, maxWidth int) string {
	const suffix = "..."
	suffixW := Width(suffix)
	target := maxWidth - suffixW
	if target <= 0 {
		return suffix
	}

	cur := 0
	for i, r := range s {
		cw := runewidth.RuneWidth(r)
		if cur+cw > target {
			return s[:i] + suffix
		}
		cur += cw
	}
	return s
}

// AdaptiveTitleWidth calculates the adaptive title column width given a slice
// of title strings. Returns min(MaxTitleLimit, maxActualWidth).
func AdaptiveTitleWidth(titles []string) int {
	max := 0
	for _, t := range titles {
		if w := Width(t); w > max {
			max = w
		}
	}
	if max > MaxTitleLimit {
		return MaxTitleLimit
	}
	return max
}
