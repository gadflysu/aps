package picker

import (
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/muesli/termenv"

	"github.com/gadflysu/aps/source"
)

// TestMain forces ANSI color output so tests that inspect ANSI escape sequences
// work correctly in non-TTY environments (e.g., go test pipes).
func TestMain(m *testing.M) {
	lipgloss.SetColorProfile(termenv.ANSI)
	os.Exit(m.Run())
}

func TestVisibleRange_SmallList(t *testing.T) {
	// total < height: show everything from 0
	start, end := visibleRange(2, 5, 10)
	if start != 0 || end != 5 {
		t.Errorf("visibleRange(2,5,10) = (%d,%d), want (0,5)", start, end)
	}
}

func TestVisibleRange_CursorAtTop(t *testing.T) {
	// cursor=0, list larger than height: start at 0
	start, end := visibleRange(0, 100, 20)
	if start != 0 || end != 20 {
		t.Errorf("visibleRange(0,100,20) = (%d,%d), want (0,20)", start, end)
	}
}

func TestVisibleRange_CursorBeyondViewport(t *testing.T) {
	// cursor=25, height=20: window scrolls so cursor is at bottom
	start, end := visibleRange(25, 100, 20)
	if start != 6 || end != 26 {
		t.Errorf("visibleRange(25,100,20) = (%d,%d), want (6,26)", start, end)
	}
}

func TestVisibleRange_CursorAtLast(t *testing.T) {
	// cursor at last element
	start, end := visibleRange(49, 50, 20)
	if start != 30 || end != 50 {
		t.Errorf("visibleRange(49,50,20) = (%d,%d), want (30,50)", start, end)
	}
}

func TestVisibleRange_ExactFit(t *testing.T) {
	// total == height
	start, end := visibleRange(9, 10, 10)
	if start != 0 || end != 10 {
		t.Errorf("visibleRange(9,10,10) = (%d,%d), want (0,10)", start, end)
	}
}

// --- adaptive column widths ---

func TestAdaptiveColWidths_IDFromSessions(t *testing.T) {
	sessions := []source.Session{
		{ID: "abc", MsgCount: 1},
		{ID: "abcdefghij", MsgCount: 9999},
	}
	m := newModel(sessions, false)
	wantID := lipgloss.Width("abcdefghij") // 10
	if m.idColW != wantID {
		t.Errorf("idColW = %d, want %d", m.idColW, wantID)
	}
}

func TestAdaptiveColWidths_MsgFromSessions(t *testing.T) {
	sessions := []source.Session{
		{ID: "a", MsgCount: 1},
		{ID: "b", MsgCount: 9999},
	}
	m := newModel(sessions, false)
	// AdaptiveMsgWidth has a floor of len("TURNS")=5 so the header fits.
	wantMsg := len("TURNS") // 5, because 9999 (4 cols) < floor
	if m.msgColW != wantMsg {
		t.Errorf("msgColW = %d, want %d", m.msgColW, wantMsg)
	}
}

func TestAdaptiveColWidths_StableAfterFilter(t *testing.T) {
	sessions := []source.Session{
		{ID: "abcdefghij", MsgCount: 9999, Title: "alpha"},
		{ID: "x", MsgCount: 1, Title: "beta"},
	}
	m := newModel(sessions, false)
	idBefore := m.idColW
	msgBefore := m.msgColW
	m.query = "beta"
	m.applyFilter() // only "x"/1 survives
	if m.idColW != idBefore {
		t.Errorf("idColW changed after filter: %d → %d", idBefore, m.idColW)
	}
	if m.msgColW != msgBefore {
		t.Errorf("msgColW changed after filter: %d → %d", msgBefore, m.msgColW)
	}
}

// --- applyFilter ---

func makeSessions() []source.Session {
	return []source.Session{
		{Title: "Fix login bug", CWDDisplay: "~/projects/auth"},
		{Title: "Add dark mode", CWDDisplay: "~/projects/ui"},
		{Title: "Refactor database", CWDDisplay: "~/projects/backend"},
	}
}

func TestApplyFilter_EmptyQuery(t *testing.T) {
	sessions := makeSessions()
	m := newModel(sessions, false)
	m.query = ""
	m.applyFilter()
	if len(m.filtered) != len(sessions) {
		t.Errorf("empty query: filtered len=%d, want %d", len(m.filtered), len(sessions))
	}
}

func TestApplyFilter_MatchesTitle(t *testing.T) {
	m := newModel(makeSessions(), false)
	m.query = "login"
	m.applyFilter()
	if len(m.filtered) == 0 {
		t.Fatal("expected matches for query 'login', got none")
	}
	if m.filtered[0].Title != "Fix login bug" {
		t.Errorf("first match title = %q, want \"Fix login bug\"", m.filtered[0].Title)
	}
}

func TestApplyFilter_MatchesCWDDisplay(t *testing.T) {
	m := newModel(makeSessions(), false)
	m.query = "backend"
	m.applyFilter()
	if len(m.filtered) == 0 {
		t.Fatal("expected matches for query 'backend', got none")
	}
	if m.filtered[0].CWDDisplay != "~/projects/backend" {
		t.Errorf("first match CWDDisplay = %q, want \"~/projects/backend\"", m.filtered[0].CWDDisplay)
	}
}

func TestApplyFilter_NoMatches(t *testing.T) {
	m := newModel(makeSessions(), false)
	m.query = "zzznomatch999"
	m.applyFilter()
	if len(m.filtered) != 0 {
		t.Errorf("no-match query: filtered len=%d, want 0", len(m.filtered))
	}
}

func TestApplyFilter_QueryClearedRestoresAll(t *testing.T) {
	sessions := makeSessions()
	m := newModel(sessions, false)
	m.query = "login"
	m.applyFilter()
	m.query = ""
	m.applyFilter()
	if len(m.filtered) != len(sessions) {
		t.Errorf("after clearing query: filtered len=%d, want %d", len(m.filtered), len(sessions))
	}
}

// --- dir rendering alignment with list mode ---

// TestRenderRowDirUsesCyanNotMuted verifies that the dir column uses ColorDir
// (cyan) matching list mode, not ColorMuted (grey).
func TestRenderRowDirUsesCyanNotMuted(t *testing.T) {
	s := source.Session{
		Client:     source.ClientClaude,
		ID:         "abc",
		Title:      "test",
		CWDDisplay: "~/projects/aps",
	}
	m := newModel([]source.Session{s}, false)
	m.width, m.height = 120, 40
	row := m.renderRow(s, false)
	// ColorDir = lipgloss.Color("6") → ANSI foreground 36 (cyan)
	// ColorMuted = lipgloss.Color("8") → ANSI foreground 90 (dark grey)
	// After the last separator the dir cell must contain \x1b[36m (cyan), not only \x1b[90m.
	if !strings.Contains(row, "\x1b[36m") {
		t.Error("dir column must use ColorDir (cyan, \\x1b[36m); got only muted/grey")
	}
}

// TestRenderRowDimDirFaint verifies that a row rendered with dimDir=true
// produces a faint ANSI sequence (SGR 2) for the dir column.
func TestRenderRowDimDirFaint(t *testing.T) {
	s := source.Session{
		Client:     source.ClientClaude,
		ID:         "abc",
		Title:      "test",
		CWDDisplay: "~/projects/aps",
	}
	m := newModel([]source.Session{s}, false)
	m.width, m.height = 120, 40
	row := m.renderRow(s, false)
	dimRow := m.renderRowDim(s, false)
	if row == dimRow {
		t.Error("renderRowDim must produce different output than renderRow (dim dir)")
	}
	// lipgloss may combine SGR codes: \x1b[2;36m or \x1b[2m — both contain ";2;" or start with "[2"
	if !strings.Contains(dimRow, "\x1b[2;") && !strings.Contains(dimRow, "\x1b[2m") {
		t.Errorf("renderRowDim dir must contain faint (SGR 2); got %q", dimRow)
	}
}

// --- renderColumnHeader ---

// stripANSI removes ANSI CSI escape sequences (ESC [ ... m) from s.
func stripANSI(s string) string {
	out := make([]byte, 0, len(s))
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
		out = append(out, s[i])
		i++
	}
	return string(out)
}

func TestRenderColumnHeader_ContainsExpectedLabels(t *testing.T) {
	m := newModel(makeSessions(), false)
	m.width, m.height = 120, 40
	h := stripANSI(m.renderColumnHeader())
	for _, label := range []string{"TIME", "TITLE", "ID", "TURNS", "DIRECTORY"} {
		if !strings.Contains(h, label) {
			t.Errorf("renderColumnHeader missing %q; stripped=%q", label, h)
		}
	}
}

func TestRenderColumnHeader_CombinedIncludesSRC(t *testing.T) {
	m := newModel(makeSessions(), true)
	m.width, m.height = 120, 40
	h := stripANSI(m.renderColumnHeader())
	if !strings.Contains(h, "SRC") {
		t.Error("renderColumnHeader in combined mode must contain \"SRC\"")
	}
}

func TestRenderColumnHeader_NoSRCWhenNotCombined(t *testing.T) {
	m := newModel(makeSessions(), false)
	m.width, m.height = 120, 40
	h := stripANSI(m.renderColumnHeader())
	if strings.Contains(h, "SRC") {
		t.Error("renderColumnHeader in non-combined mode must not contain \"SRC\"")
	}
}

// --- esc behaviour ---

// TestEscInPreviewClosesPreview verifies that pressing esc while in
// stateListPreview collapses the preview pane instead of quitting.
func TestEscInPreviewClosesPreview(t *testing.T) {
	m := newModel(makeSessions(), false)
	m.state = stateListPreview

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m2 := next.(Model)
	if m2.state != stateList {
		t.Errorf("esc in preview: state = %v, want stateList", m2.state)
	}
	if m2.chosen != nil {
		t.Error("esc in preview must not set chosen")
	}
}

// TestEscInListExits verifies that pressing esc in stateList triggers quit.
func TestEscInListExits(t *testing.T) {
	m := newModel(makeSessions(), false)
	m.state = stateList

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Error("esc in list mode must return tea.Quit cmd")
	}
}

// --- search focus ---

// TestSearchFocusedOnInit verifies that the textinput is focused immediately
// after newModel, without needing Init() to be called first.
// Regression: Init() used a value receiver, so Focus() mutated a copy and
// the real model's search.focus stayed false — all keystrokes were silently dropped.
func TestSearchFocusedOnInit(t *testing.T) {
	m := newModel(makeSessions(), false)
	if !m.search.Focused() {
		t.Error("search textinput must be focused immediately after newModel")
	}
}

// --- updatePreviewHeights ---

func TestUpdatePreviewHeights_NoMsgs(t *testing.T) {
	// height=30: info(5) + sep+dir_header(2) + dir_content = 30 → vpDir.Height = 23
	m := newModel(makeSessions(), false)
	m.width = 100
	m.height = 30
	m.hasMsgs = false
	m.updatePreviewHeights()

	if m.vpInfo.Height != 4 {
		t.Errorf("vpInfo.Height = %d, want 4", m.vpInfo.Height)
	}
	if m.vpMsgs.Height != 0 {
		t.Errorf("vpMsgs.Height = %d, want 0 when hasMsgs=false", m.vpMsgs.Height)
	}
	if m.vpDir.Height != 23 {
		t.Errorf("vpDir.Height = %d, want 23", m.vpDir.Height)
	}
}

func TestUpdatePreviewHeights_WithMsgs(t *testing.T) {
	// height=40: available_after_info=35, after_sep+msgs_header=33, msgsH=33/3=11, after_sep+dir_header=22-2=20
	m := newModel(makeSessions(), false)
	m.width = 100
	m.height = 40
	m.hasMsgs = true
	m.updatePreviewHeights()

	if m.vpInfo.Height != 4 {
		t.Errorf("vpInfo.Height = %d, want 4", m.vpInfo.Height)
	}
	if m.vpMsgs.Height != 11 {
		t.Errorf("vpMsgs.Height = %d, want 11", m.vpMsgs.Height)
	}
	if m.vpDir.Height != 20 {
		t.Errorf("vpDir.Height = %d, want 20", m.vpDir.Height)
	}
}

func TestUpdatePreviewHeights_WidthSet(t *testing.T) {
	// pw = 100*4/10 - 2 = 38
	m := newModel(makeSessions(), false)
	m.width = 100
	m.height = 30
	m.hasMsgs = false
	m.updatePreviewHeights()

	pw := 100*4/10 - 2
	if m.vpInfo.Width != pw {
		t.Errorf("vpInfo.Width = %d, want %d", m.vpInfo.Width, pw)
	}
	if m.vpDir.Width != pw {
		t.Errorf("vpDir.Width = %d, want %d", m.vpDir.Width, pw)
	}
}

func TestUpdatePreviewHeights_ClampMsgsToOne(t *testing.T) {
	// height so small that available/3 rounds to 0 → clamp to 1
	// infoTotalHeight=5, sep+sectionHeaderLines=2; available = height-5-2 = height-7
	// need available/3 < 1 → available < 3 → height < 10
	m := newModel(makeSessions(), false)
	m.width = 100
	m.height = 10
	m.hasMsgs = true
	m.updatePreviewHeights()

	if m.vpMsgs.Height < 1 {
		t.Errorf("vpMsgs.Height = %d, want >= 1 (clamp)", m.vpMsgs.Height)
	}
}

// --- renderRow ID truncation ---

// TestRenderRowOpencodeIDSingleLine verifies that Opencode session IDs longer than
// 12 display columns do not cause the rendered row to wrap onto multiple lines.
//
// Regression: the old code only truncated Claude IDs (guarded by s.Client ==
// ClientClaude). For Opencode, the full ID was passed directly to idStyle which
// has Width(12) but no Inline(true): lipgloss Width(N) word-wraps long content
// to N columns, producing multiple lines and breaking the TUI list layout.
func TestRenderRowOpencodeIDSingleLine(t *testing.T) {
	longID := "abcdefghij_klmnopqrstuvwxyz_1234" // 32 ASCII chars → wraps to 3 lines at Width(12)
	s := source.Session{
		Client: source.ClientOpencode,
		ID:     longID,
		Title:  "test session",
	}
	m := newModel([]source.Session{s}, false)
	m.width, m.height = 120, 40
	row := m.renderRow(s, false)

	if lineCount(row) != 1 {
		t.Errorf("renderRow must produce a single line; got %d lines (Opencode ID was wrapped instead of truncated)", lineCount(row))
	}
}

// TestRenderRowClaudeIDSingleLine verifies the same invariant for Claude UUIDs.
func TestRenderRowClaudeIDSingleLine(t *testing.T) {
	uuid := "550e8400-e29b-41d4-a716-446655440000" // 36 chars → wraps to 3 lines at Width(12)
	s := source.Session{
		Client: source.ClientClaude,
		ID:     uuid,
		Title:  "test session",
	}
	m := newModel([]source.Session{s}, false)
	m.width, m.height = 120, 40
	row := m.renderRow(s, false)

	if lineCount(row) != 1 {
		t.Errorf("renderRow must produce a single line; got %d lines (Claude UUID was wrapped instead of truncated)", lineCount(row))
	}
}

// lineCount returns the number of newline-separated lines in s.
func lineCount(s string) int {
	n := 1
	for _, c := range s {
		if c == '\n' {
			n++
		}
	}
	return n
}

// --- renderRow highlight ---

// TestRenderRowSelectedHasReverseVideo verifies that the selected row is
// wrapped in ANSI reverse-video (SGR 7), making the entire row visually
// highlighted regardless of the terminal color theme.
func TestRenderRowSelectedHasReverseVideo(t *testing.T) {
	s := source.Session{Client: source.ClientClaude, ID: "abc", Title: "test"}
	m := newModel([]source.Session{s}, false)
	m.width, m.height = 120, 40

	selected := m.renderRow(s, true)
	unselected := m.renderRow(s, false)

	// ANSI SGR 7 = reverse video; lipgloss emits it as \x1b[7m or inside a
	// compound sequence like \x1b[1;7m.  Check the raw bytes.
	if !containsReverseVideo(selected) {
		t.Error("selected row must contain ANSI reverse-video (SGR 7)")
	}
	if containsReverseVideo(unselected) {
		t.Error("unselected row must not contain ANSI reverse-video")
	}
}

// containsReverseVideo reports whether s contains an ANSI SGR sequence that
// enables reverse video (parameter 7).  It matches both \x1b[7m and compound
// sequences like \x1b[1;7m or \x1b[7;1m.
func containsReverseVideo(s string) bool {
	// Walk every ESC [ ... m sequence and look for a "7" parameter.
	for i := 0; i < len(s)-2; i++ {
		if s[i] != '\x1b' || s[i+1] != '[' {
			continue
		}
		j := i + 2
		for j < len(s) && s[j] != 'm' {
			j++
		}
		if j >= len(s) {
			continue
		}
		params := s[i+2 : j] // everything between "[" and "m"
		for _, p := range strings.Split(params, ";") {
			if p == "7" {
				return true
			}
		}
	}
	return false
}

// TestRenderRowFitsInPreviewListWidth_Combined verifies that combined mode
// (with SRC column) also stays within lw in preview mode.
func TestRenderRowFitsInPreviewListWidth_Combined(t *testing.T) {
	s := source.Session{
		Client:     source.ClientClaude,
		ID:         "550e8400-e29b-41d4",
		Title:      "A long title that would normally overflow the narrow list column",
		CWDDisplay: "~/projects.local/aps",
	}
	termW := 105
	lw := termW * 6 / 10 // 63
	m := newModel([]source.Session{s}, true) // combined=true
	m.width, m.height = termW, 40
	m.state = stateListPreview

	row := m.renderRow(s, false)
	rowW := lipgloss.Width(row)
	if rowW > lw {
		t.Errorf("renderRow combined in preview mode: width=%d exceeds lw=%d; row will word-wrap", rowW, lw)
	}
}

// TestRenderRowFitsInPreviewListWidth verifies that when a row is rendered
// for the narrowed list column in preview mode, its total width does not
// exceed the available list width (lw = width*6/10), so lipgloss.Width(lw)
// will NOT word-wrap it.
func TestRenderRowFitsInPreviewListWidth(t *testing.T) {
	s := source.Session{
		Client:     source.ClientClaude,
		ID:         "550e8400-e29b-41d4",
		Title:      "A long title that would normally overflow the narrow list column",
		CWDDisplay: "~/projects.local/aps",
	}
	termW := 105
	lw := termW * 6 / 10 // 63
	m := newModel([]source.Session{s}, false)
	m.width, m.height = termW, 40
	m.state = stateListPreview

	row := m.renderRow(s, false)
	rowW := lipgloss.Width(row)
	if rowW > lw {
		t.Errorf("renderRow in preview mode: width=%d exceeds lw=%d; row will word-wrap", rowW, lw)
	}
}

func TestUpdatePreviewHeights_ClampDirToOne(t *testing.T) {
	// height so small that dir available <= 0 → clamp to 1
	// infoTotalHeight=6, sectionHeaderLines=2; height=8 → available=0 → clamp
	m := newModel(makeSessions(), false)
	m.width = 100
	m.height = 8
	m.hasMsgs = false
	m.updatePreviewHeights()

	if m.vpDir.Height < 1 {
		t.Errorf("vpDir.Height = %d, want >= 1 (clamp)", m.vpDir.Height)
	}
}
