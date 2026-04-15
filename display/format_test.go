package display

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/gadflysu/aps/source"
)

func TestHeader_MsgColumnLabelIsTurns(t *testing.T) {
	w := ListWidths{Title: 10, ID: 36, Msg: 5, Dir: 20}
	h := stripANSI(Header(w))
	if !strings.Contains(h, "TURNS") {
		t.Errorf("header does not contain TURNS: %q", h)
	}
	if strings.Contains(h, "MSG") {
		t.Errorf("header still contains old label MSG: %q", h)
	}
}

func TestAdaptiveMsgWidth_MinIsTurnsHeaderLen(t *testing.T) {
	// minimum width must accommodate "TURNS" (5), not "MSG" (3)
	got := AdaptiveMsgWidth(nil)
	if got != len("TURNS") {
		t.Errorf("AdaptiveMsgWidth(nil) = %d, want %d (len(\"TURNS\"))", got, len("TURNS"))
	}
}


func TestFormatListRow_DimDirContentPreserved(t *testing.T) {
	// dimDir=true must still include the directory text (just styled differently).
	s := source.Session{
		Title:      "test",
		ID:         "1ab683ce-f9fc-4799-a67e-48211866f4de",
		MsgCount:   1,
		CWDDisplay: "~/projects.local/aps",
		Client:     source.ClientClaude,
	}
	w := ListWidths{Title: 10, ID: 36, Msg: 5, Dir: 25}
	dimmed := stripANSI(FormatListRow(s, w, true))
	if !strings.Contains(dimmed, "~/projects.local/aps") {
		t.Errorf("dimDir row missing directory text: %q", dimmed)
	}
}

func TestAdaptiveTitleWidth_Empty(t *testing.T) {
	if got := AdaptiveTitleWidth(nil); got != 0 {
		t.Errorf("AdaptiveTitleWidth(nil) = %d, want 0", got)
	}
	if got := AdaptiveTitleWidth([]string{}); got != 0 {
		t.Errorf("AdaptiveTitleWidth([]) = %d, want 0", got)
	}
}

func TestAdaptiveTitleWidth_Normal(t *testing.T) {
	titles := []string{"hello", "world!!", "hi"}
	got := AdaptiveTitleWidth(titles)
	want := 7 // "world!!" = 7 ASCII cols
	if got != want {
		t.Errorf("AdaptiveTitleWidth = %d, want %d", got, want)
	}
}

func TestAdaptiveTitleWidth_CappedAtMaxTitleLimit(t *testing.T) {
	titles := []string{strings.Repeat("a", MaxTitleLimit+10)}
	got := AdaptiveTitleWidth(titles)
	if got != MaxTitleLimit {
		t.Errorf("AdaptiveTitleWidth with oversized title = %d, want MaxTitleLimit=%d", got, MaxTitleLimit)
	}
}

func TestAdaptiveTitleWidth_ExactlyAtLimit(t *testing.T) {
	titles := []string{strings.Repeat("x", MaxTitleLimit)}
	got := AdaptiveTitleWidth(titles)
	if got != MaxTitleLimit {
		t.Errorf("AdaptiveTitleWidth exactly at limit = %d, want %d", got, MaxTitleLimit)
	}
}

func TestAdaptiveTitleWidth_CJK(t *testing.T) {
	// Each CJK char is 2 display columns. 10 CJK chars = 20 cols < MaxTitleLimit.
	titles := []string{strings.Repeat("中", 10)}
	got := AdaptiveTitleWidth(titles)
	if got != 20 {
		t.Errorf("AdaptiveTitleWidth CJK 10 chars = %d, want 20", got)
	}
}

func TestAdaptiveMsgWidth_MinIsHeaderLen(t *testing.T) {
	// nil sessions: width should equal len("TURNS") = 5
	got := AdaptiveMsgWidth(nil)
	if got != len("TURNS") {
		t.Errorf("AdaptiveMsgWidth(nil) = %d, want %d", got, len("TURNS"))
	}
}

func TestAdaptiveMsgWidth_WiderThanHeader(t *testing.T) {
	sessions := []source.Session{
		{MsgCount: 12345},
	}
	got := AdaptiveMsgWidth(sessions)
	want := len(fmt.Sprintf("%d", 12345)) // 5
	if got != want {
		t.Errorf("AdaptiveMsgWidth = %d, want %d", got, want)
	}
}

func TestAdaptiveIDWidth_Claude(t *testing.T) {
	sessions := []source.Session{
		{ID: "1ab683ce-f9fc-4799-a67e-48211866f4de"},
	}
	got := AdaptiveIDWidth(sessions)
	if got != 36 {
		t.Errorf("AdaptiveIDWidth Claude UUID = %d, want 36", got)
	}
}

func TestAdaptiveDirWidth_NoCap(t *testing.T) {
	// termWidth==0: no cap, returns natural max
	sessions := []source.Session{
		{CWDDisplay: "~/projects.local/aps"},
		{CWDDisplay: "~/projects.local/dotfiles_sd"},
	}
	got := AdaptiveDirWidth(sessions, 0)
	want := lipgloss.Width("~/projects.local/dotfiles_sd")
	if got != want {
		t.Errorf("AdaptiveDirWidth no cap = %d, want %d", got, want)
	}
}

func TestAdaptiveDirWidth_Capped(t *testing.T) {
	// termWidth==40: very long dir capped at 40
	sessions := []source.Session{
		{CWDDisplay: "/Volumes/Work/main/drive/Syncthing/dev/scripts/very/long/path"},
	}
	got := AdaptiveDirWidth(sessions, 40)
	if got != 40 {
		t.Errorf("AdaptiveDirWidth capped = %d, want 40", got)
	}
}

func TestComputeListWidths_BonusToTitle(t *testing.T) {
	// Title wider than 40: bonus fills up to natural max, not beyond.
	// maxTitleW = 50 (> 40), maxBonus = 50-40 = 10
	// With a very wide terminal, titleW should reach exactly 50, not exceed it.
	longTitle := strings.Repeat("a", 50) // 50 ASCII cols
	sessions := []source.Session{
		{Title: longTitle, ID: "1ab683ce-f9fc-4799-a67e-48211866f4de", MsgCount: 1, CWDDisplay: "~"},
	}
	w := ComputeListWidths(sessions, false, 300)
	if w.Title != 50 {
		t.Errorf("Title with bonus should reach natural max 50, got %d", w.Title)
	}
	if w.Source != 0 {
		t.Errorf("Source should be 0 when not combined, got %d", w.Source)
	}
}

func TestComputeListWidths_NoBonusWhenTitleFitsIn40(t *testing.T) {
	// Title fits within 40: no bonus should be applied regardless of termWidth.
	sessions := []source.Session{
		{Title: "hi", ID: "1ab683ce-f9fc-4799-a67e-48211866f4de", MsgCount: 1, CWDDisplay: "~"},
	}
	w := ComputeListWidths(sessions, false, 300)
	if w.Title != 2 { // lipgloss.Width("hi") == 2, no bonus
		t.Errorf("Title ≤40 should not receive bonus, got %d want 2", w.Title)
	}
}

func TestComputeListWidths_NoBonus(t *testing.T) {
	sessions := []source.Session{
		{Title: "hi", ID: "1ab683ce-f9fc-4799-a67e-48211866f4de", MsgCount: 1, CWDDisplay: "~"},
	}
	// termWidth==0 (pipe): no bonus, title stays at adaptive baseline
	w := ComputeListWidths(sessions, false, 0)
	if w.Title != 2 { // lipgloss.Width("hi") == 2
		t.Errorf("Title without bonus = %d, want 2", w.Title)
	}
}

func TestComputeListWidths_TotalFitsTermWidth(t *testing.T) {
	// When maxTitleW > 40, bonus fills up to maxTitleW. The total row width
	// must not exceed termWidth. colSep = 2 display cols; 4 seps = 8 cols.
	longTitle := strings.Repeat("a", 50) // maxTitleW=50, maxBonus=10
	sessions := []source.Session{
		{Title: longTitle, ID: "1ab683ce-f9fc-4799-a67e-48211866f4de", MsgCount: 1, CWDDisplay: "~"},
	}
	termWidth := 220
	w := ComputeListWidths(sessions, false, termWidth)
	sepW := lipgloss.Width(colSep)
	numSeps := 4
	total := colTime + w.Title + w.ID + w.Msg + w.Dir + numSeps*sepW
	if total > termWidth {
		t.Errorf("total row width = %d exceeds termWidth %d", total, termWidth)
	}
}

func TestFormatDirCell_ContainsPrefixAndBasename(t *testing.T) {
	// Cell must contain both prefix and basename text.
	got := formatDirCell("~/projects.local/aps", 30)
	plain := stripANSI(got)
	if !strings.Contains(plain, "~/projects.local/") {
		t.Errorf("prefix not found in output: %q", plain)
	}
	if !strings.Contains(plain, "aps") {
		t.Errorf("basename 'aps' not found in output: %q", plain)
	}
}

func TestFormatDirCell_PaddedToWidth(t *testing.T) {
	// Cell must be exactly colWidth display columns wide.
	got := formatDirCell("~/projects.local/aps", 30)
	if w := lipgloss.Width(got); w != 30 {
		t.Errorf("formatDirCell width = %d, want 30", w)
	}
}

func TestFormatDirCell_BareBasename(t *testing.T) {
	// When there is no slash (e.g. "~"), the whole string is the basename.
	got := formatDirCell("~", 10)
	plain := stripANSI(got)
	if !strings.Contains(plain, "~") {
		t.Errorf("bare basename '~' not found in output: %q", plain)
	}
	if w := lipgloss.Width(got); w != 10 {
		t.Errorf("formatDirCell bare width = %d, want 10", w)
	}
}

func TestFormatDirCell_TruncatedPath(t *testing.T) {
	// Very long path truncated to colWidth: must still fit exactly.
	dir := "/Volumes/Work/main/drive/Syncthing/dev/scripts"
	got := formatDirCell(dir, 20)
	if w := lipgloss.Width(got); w != 20 {
		t.Errorf("truncated formatDirCell width = %d, want 20", w)
	}
}


// stripANSI removes ANSI escape sequences for plain-text assertions.
func stripANSI(s string) string {
	var out strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '[' {
			i += 2
			for i < len(s) && s[i] != 'm' {
				i++
			}
			i++ // skip 'm'
			continue
		}
		out.WriteByte(s[i])
		i++
	}
	return out.String()
}

func TestComputeListWidths_NaturalFitsNoBonusIfEqual(t *testing.T) {
	sessions := []source.Session{
		{Title: "hi", ID: "1ab683ce-f9fc-4799-a67e-48211866f4de", MsgCount: 1, CWDDisplay: "~"},
	}
	// Compute natural width, then pass exactly that as termWidth → no bonus
	w0 := ComputeListWidths(sessions, false, 0)
	naturalW := colTime + w0.Title + w0.ID + w0.Msg + w0.Dir + 4 // 4 seps
	wExact := ComputeListWidths(sessions, false, naturalW)
	if wExact.Title != w0.Title {
		t.Errorf("Title at exact fit = %d, want %d", wExact.Title, w0.Title)
	}
}
