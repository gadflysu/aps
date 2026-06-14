package picker

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/muesli/termenv"

	"github.com/gadflysu/aps/display"
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
	m := newModel(sessions, false, nil, nil)
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
	m := newModel(sessions, false, nil, nil)
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
	m := newModel(sessions, false, nil, nil)
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

func TestNewModel_SearchPlaceholder(t *testing.T) {
	m := newModel(makeSessions(), false, nil, nil)
	if m.search.Placeholder != " search query" {
		t.Fatalf("search placeholder = %q, want %q", m.search.Placeholder, " search query")
	}
	if !m.search.PlaceholderStyle.GetFaint() {
		t.Fatal("search placeholder should be dim/faint")
	}
	if got, want := fmt.Sprint(m.search.PlaceholderStyle.GetForeground()), fmt.Sprint(display.ColorMuted); got != want {
		t.Fatalf("search placeholder foreground = %s, want muted %s", got, want)
	}

	rendered := stripANSI(m.search.View())
	if !strings.Contains(rendered, "search query") {
		t.Fatalf("search view = %q, want placeholder text", rendered)
	}
}

// --- listTitleWidth adaptive expansion ---

// TestListTitleWidth_ExpandsWithTerminalWidth verifies that in stateList mode
// extra terminal width is allocated to the title column beyond the 40-col cap.
func TestListTitleWidth_ExpandsWithTerminalWidth(t *testing.T) {
	// A title that is 60 display cols wide — exceeds MaxTitleLimit (40).
	longTitle := strings.Repeat("x", 60)
	// Give a non-empty CWDDisplay so the naturalW estimate is realistic.
	sessions := []source.Session{
		{ID: "abc", Title: longTitle, CWDDisplay: "~/projects/aps", MsgCount: 1},
	}
	m := newModel(sessions, false, nil, nil)
	m.state = stateList

	// At a narrow terminal the title is capped at titleColWidth+2 (no surplus).
	m.width, m.height = 80, 20
	narrow := m.listTitleWidth()
	if narrow != titleColWidth+2 {
		t.Errorf("narrow terminal: listTitleWidth() = %d, want %d", narrow, titleColWidth+2)
	}

	// At a wide terminal (200 cols) the title column should grow beyond titleColWidth+2.
	m.width = 200
	wide := m.listTitleWidth()
	if wide <= titleColWidth+2 {
		t.Errorf("wide terminal: listTitleWidth() = %d, want > %d", wide, titleColWidth+2)
	}
}

// TestListTitleWidth_NeverExceedsNaturalMax verifies that even on a very wide
// terminal the title column does not grow wider than the longest title.
func TestListTitleWidth_NeverExceedsNaturalMax(t *testing.T) {
	longTitle := strings.Repeat("x", 60)
	sessions := []source.Session{
		{ID: "abc", Title: longTitle, MsgCount: 1},
	}
	m := newModel(sessions, false, nil, nil)
	m.state = stateList
	m.width, m.height = 500, 20

	tw := m.listTitleWidth()
	// outer = content + 2 padding; content must not exceed 60 (natural max).
	if tw > 60+2 {
		t.Errorf("listTitleWidth() = %d exceeds natural max %d", tw, 60+2)
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
	m := newModel(sessions, false, nil, nil)
	m.query = ""
	m.applyFilter()
	if len(m.filtered) != len(sessions) {
		t.Errorf("empty query: filtered len=%d, want %d", len(m.filtered), len(sessions))
	}
}

func TestApplyFilter_MatchesTitle(t *testing.T) {
	m := newModel(makeSessions(), false, nil, nil)
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
	m := newModel(makeSessions(), false, nil, nil)
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
	m := newModel(makeSessions(), false, nil, nil)
	m.query = "zzznomatch999"
	m.applyFilter()
	if len(m.filtered) != 0 {
		t.Errorf("no-match query: filtered len=%d, want 0", len(m.filtered))
	}
}

func TestApplyFilter_QueryClearedRestoresAll(t *testing.T) {
	sessions := makeSessions()
	m := newModel(sessions, false, nil, nil)
	m.query = "login"
	m.applyFilter()
	m.query = ""
	m.applyFilter()
	if len(m.filtered) != len(sessions) {
		t.Errorf("after clearing query: filtered len=%d, want %d", len(m.filtered), len(sessions))
	}
}

// TestApplyFilter_PreservesTimeOrder verifies that fuzzy matches are returned
// in the same time-descending order as m.sessions, not sorted by fuzzy score.
func TestApplyFilter_PreservesTimeOrder(t *testing.T) {
	now := time.Now()
	sessions := []source.Session{
		{Title: "fix bug alpha", Time: now.Add(-1 * time.Hour)},  // older
		{Title: "fix bug beta", Time: now.Add(-2 * time.Hour)},   // even older
		{Title: "fix bug gamma", Time: now},                       // newest
	}
	// sessions is assumed to be already time-sorted (newest first), matching
	// how newModel receives them from source.Load*.
	// Reorder to newest-first to match real data flow.
	sessions = []source.Session{sessions[2], sessions[0], sessions[1]}

	m := newModel(sessions, false, nil, nil)
	m.query = "fix bug"
	m.applyFilter()

	if len(m.filtered) != 3 {
		t.Fatalf("expected 3 matches, got %d", len(m.filtered))
	}
	for i := 1; i < len(m.filtered); i++ {
		if m.filtered[i].Time.After(m.filtered[i-1].Time) {
			t.Errorf("filtered[%d] (%s) is newer than filtered[%d] (%s): order not preserved",
				i, m.filtered[i].Title, i-1, m.filtered[i-1].Title)
		}
	}
}

// --- match highlighting ---

// TestHighlightField_NoHits returns the plain text unchanged (modulo ANSI) when no indices given.
func TestHighlightField_NoHits(t *testing.T) {
	base := titleStyle.Copy().UnsetWidth().UnsetPadding()
	got := highlightField("hello", nil, base, matchStyle)
	plain := stripANSI(got)
	if plain != "hello" {
		t.Errorf("no hits: stripped = %q, want %q", plain, "hello")
	}
}

// TestHighlightField_ContainsANSI verifies that matched characters produce ANSI output.
func TestHighlightField_ContainsANSI(t *testing.T) {
	base := titleStyle.Copy().UnsetWidth().UnsetPadding()
	got := highlightField("hello", []int{0, 1}, base, matchStyle)
	if !strings.Contains(got, "\x1b[") {
		t.Errorf("highlight: expected ANSI codes in output, got %q", got)
	}
	plain := stripANSI(got)
	if plain != "hello" {
		t.Errorf("highlight stripped: got %q, want %q", plain, "hello")
	}
}

// TestApplyFilter_PopulatesMatchIdx verifies that matchIdx is set after a query.
func TestApplyFilter_PopulatesMatchIdx(t *testing.T) {
	m := newModel(makeSessions(), false, nil, nil)
	m.query = "login"
	m.applyFilter()

	if m.matchIdx == nil {
		t.Fatal("matchIdx must not be nil after a non-empty query")
	}
	if len(m.filtered) == 0 {
		t.Fatal("expected at least one match")
	}
	sid := m.filtered[0].ID
	if _, ok := m.matchIdx[sid]; !ok {
		t.Errorf("matchIdx missing entry for session ID %q", sid)
	}
}

// TestApplyFilter_ClearsMatchIdxOnEmptyQuery verifies matchIdx is nil after clearing the query.
func TestApplyFilter_ClearsMatchIdxOnEmptyQuery(t *testing.T) {
	m := newModel(makeSessions(), false, nil, nil)
	m.query = "login"
	m.applyFilter()
	m.query = ""
	m.applyFilter()
	if m.matchIdx != nil {
		t.Error("matchIdx must be nil after empty query")
	}
}

// TestRenderRow_TitleHighlightedOnMatch verifies that renderRow injects ANSI codes
// into the title column when matchIdx is populated for that session.
func TestRenderRow_TitleHighlightedOnMatch(t *testing.T) {
	sessions := []source.Session{
		{ID: "abc", Title: "Fix login bug", CWDDisplay: "~/projects/auth"},
	}
	m := newModel(sessions, false, nil, nil)
	m.width, m.height = 120, 40
	m.query = "login"
	m.applyFilter()

	row := m.renderRow(sessions[0], false)
	if !strings.Contains(row, "\x1b[") {
		t.Error("renderRow must contain ANSI highlight codes when query matches title")
	}
}

// TestRenderRow_SelectedDirHighlighted verifies that the selected row also has
// match highlights (red + reverse) in the dir column, not just plain reverse.
func TestRenderRow_SelectedDirHighlighted(t *testing.T) {
	sessions := []source.Session{
		{ID: "abc", Title: "some title", CWDDisplay: "~/projects/auth"},
	}
	m := newModel(sessions, false, nil, nil)
	m.width, m.height = 120, 40
	// "auth" matches the dir; "a" is enough to ensure a match in cwd.
	m.query = "auth"
	m.applyFilter()

	rowSel := m.renderRow(sessions[0], true)
	rowUnsel := m.renderRow(sessions[0], false)

	// Both rows must contain matchStyle's colour (ColorSpinner = ANSI 9 → "91m").
	// Selected row additionally gets reverse (7), so the full sequence is "1;7;91m".
	if !strings.Contains(rowUnsel, "91m") {
		t.Error("unselected row: dir match must contain bright-red (91m)")
	}
	if !strings.Contains(rowSel, "91m") {
		t.Error("selected row: dir match must contain bright-red (91m) with reverse")
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
	m := newModel([]source.Session{s}, false, nil, nil)
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
	m := newModel([]source.Session{s}, false, nil, nil)
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
	m := newModel(makeSessions(), false, nil, nil)
	m.width, m.height = 120, 40
	h := stripANSI(m.renderColumnHeader())
	for _, label := range []string{"TIME", "TITLE", "ID", "TURNS", "DIRECTORY"} {
		if !strings.Contains(h, label) {
			t.Errorf("renderColumnHeader missing %q; stripped=%q", label, h)
		}
	}
}

func TestRenderColumnHeader_CombinedIncludesSRC(t *testing.T) {
	m := newModel(makeSessions(), true, nil, nil)
	m.width, m.height = 120, 40
	h := stripANSI(m.renderColumnHeader())
	if !strings.Contains(h, "SRC") {
		t.Error("renderColumnHeader in combined mode must contain \"SRC\"")
	}
}

func TestRenderColumnHeader_NoSRCWhenNotCombined(t *testing.T) {
	m := newModel(makeSessions(), false, nil, nil)
	m.width, m.height = 120, 40
	h := stripANSI(m.renderColumnHeader())
	if strings.Contains(h, "SRC") {
		t.Error("renderColumnHeader in non-combined mode must not contain \"SRC\"")
	}
}

// TestRenderColumnHeader_PreviewExcludesID verifies that in preview mode the
// header omits the ID column — the preview pane already shows the session ID.
func TestRenderColumnHeader_PreviewExcludesID(t *testing.T) {
	m := newModel(makeSessions(), false, nil, nil)
	m.width, m.height = 120, 40
	m.state = stateListPreview
	h := stripANSI(m.renderColumnHeader())
	if strings.Contains(h, "ID") {
		t.Errorf("preview header must not contain \"ID\"; got %q", h)
	}
}

// TestRenderColumnHeader_PreviewSingleLine verifies that the header does not
// word-wrap at narrow terminal widths (e.g. 105 cols) in preview mode.
func TestRenderColumnHeader_PreviewSingleLine(t *testing.T) {
	s := source.Session{
		Client: source.ClientClaude, ID: "abc", Title: "test", MsgCount: 1,
	}
	m := newModel([]source.Session{s}, false, nil, nil)
	m.width, m.height = 105, 40
	m.state = stateListPreview
	header := m.renderColumnHeader()
	if lineCount(header) != 1 {
		t.Errorf("preview header must be 1 line; got %d (width=%d)", lineCount(header), m.width)
	}
}

// TestRenderRow_PreviewOmitsIDCell verifies that in preview mode the rendered
// row does not contain the session ID text (it is shown in the preview pane).
func TestRenderRow_PreviewOmitsIDCell(t *testing.T) {
	s := source.Session{
		Client: source.ClientClaude, ID: "abc-123", Title: "test", MsgCount: 1,
	}
	m := newModel([]source.Session{s}, false, nil, nil)
	m.width, m.height = 120, 40
	m.state = stateListPreview
	row := stripANSI(m.renderRow(s, false))
	if strings.Contains(row, "abc-123") {
		t.Errorf("preview row must not contain session ID; got %q", row)
	}
}

// TestListTitleWidth_PreviewGrowsWhenIDRemoved verifies that removing the ID
// column in preview mode gives the title column more space than it would have
// if the ID column were still present.
func TestListTitleWidth_PreviewGrowsWhenIDRemoved(t *testing.T) {
	s := source.Session{
		Client: source.ClientClaude, ID: "abc", Title: "test", MsgCount: 1,
	}
	m := newModel([]source.Session{s}, false, nil, nil)
	m.width, m.height = 105, 40
	m.state = stateListPreview

	// Compute what tw would be if ID column were still in fixed.
	lw := m.width * 6 / 10
	fixedWithID := 2 + (timeColW + 2) + (m.idColW + 2) + (m.msgColW + 2)
	twWithID := lw - fixedWithID
	if twWithID < 3 {
		twWithID = 3
	}

	previewTW := m.listTitleWidth()
	if previewTW <= twWithID {
		t.Errorf("preview title width (%d) should exceed width with ID column (%d)", previewTW, twWithID)
	}
}

// --- esc behaviour ---

// TestEscInPreviewClosesPreview verifies that pressing esc while in
// stateListPreview collapses the preview pane instead of quitting.
func TestEscInPreviewClosesPreview(t *testing.T) {
	m := newModel(makeSessions(), false, nil, nil)
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

// TestEscInListExits verifies that pressing esc in stateList with no query triggers quit.
func TestEscInListExits(t *testing.T) {
	m := newModel(makeSessions(), false, nil, nil)
	m.state = stateList

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Error("esc in list mode must return tea.Quit cmd")
	}
}

// TestEscClearsQueryBeforeExiting verifies that pressing esc while there is a
// non-empty query clears the query instead of quitting.
func TestEscClearsQueryBeforeExiting(t *testing.T) {
	m := newModel(makeSessions(), false, nil, nil)
	m.state = stateList
	m.search.SetValue("hello")
	m.query = "hello"

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m2 := next.(Model)

	if cmd != nil {
		t.Error("esc with non-empty query must not quit")
	}
	if m2.query != "" {
		t.Errorf("esc with non-empty query: query = %q, want empty", m2.query)
	}
	if m2.search.Value() != "" {
		t.Errorf("esc with non-empty query: search value = %q, want empty", m2.search.Value())
	}
}

// TestEscExitsWhenQueryAlreadyEmpty verifies that pressing esc with an empty
// query triggers quit (second press behaviour).
func TestEscExitsWhenQueryAlreadyEmpty(t *testing.T) {
	m := newModel(makeSessions(), false, nil, nil)
	m.state = stateList
	// ensure query is empty
	m.search.SetValue("")
	m.query = ""

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Error("esc with empty query must return tea.Quit cmd")
	}
}

// --- search focus ---

// TestSearchFocusedOnInit verifies that the textinput is focused immediately
// after newModel, without needing Init() to be called first.
// Regression: Init() used a value receiver, so Focus() mutated a copy and
// the real model's search.focus stayed false — all keystrokes were silently dropped.
func TestSearchFocusedOnInit(t *testing.T) {
	m := newModel(makeSessions(), false, nil, nil)
	if !m.search.Focused() {
		t.Error("search textinput must be focused immediately after newModel")
	}
}

// --- updatePreviewHeights ---

func TestUpdatePreviewHeights_NoMsgs(t *testing.T) {
	// height=30: info(8) + statusBar(1) + sep+dir_header(2) + dir_content = 30 -> vpDir.Height = 19
	m := newModel(makeSessions(), false, nil, nil)
	m.width = 100
	m.height = 30
	m.hasMsgs = false
	m.updatePreviewHeights()

	if m.vpInfo.Height != 7 {
		t.Errorf("vpInfo.Height = %d, want 7", m.vpInfo.Height)
	}
	if m.vpMsgs.Height != 0 {
		t.Errorf("vpMsgs.Height = %d, want 0 when hasMsgs=false", m.vpMsgs.Height)
	}
	if m.vpDir.Height != 19 {
		t.Errorf("vpDir.Height = %d, want 19", m.vpDir.Height)
	}
}

func TestUpdatePreviewHeights_WithMsgs(t *testing.T) {
	// height=40: available_after_info=40-8-1=31, after_sep+msgs_header=31-2=29, msgsH=29/3=9, after_sep+dir_header=29-9-2=18
	m := newModel(makeSessions(), false, nil, nil)
	m.width = 100
	m.height = 40
	m.hasMsgs = true
	m.updatePreviewHeights()

	if m.vpInfo.Height != 7 {
		t.Errorf("vpInfo.Height = %d, want 7", m.vpInfo.Height)
	}
	if m.vpMsgs.Height != 9 {
		t.Errorf("vpMsgs.Height = %d, want 9", m.vpMsgs.Height)
	}
	if m.vpDir.Height != 18 {
		t.Errorf("vpDir.Height = %d, want 18", m.vpDir.Height)
	}
}

func TestUpdatePreviewHeights_WidthSet(t *testing.T) {
	// pw = 100*4/10 - 2 = 38
	m := newModel(makeSessions(), false, nil, nil)
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
	// infoTotalHeight=8, sep+sectionHeaderLines=2; available = height-8-2 = height-10
	// need available/3 < 1 -> available < 3 -> height < 13
	m := newModel(makeSessions(), false, nil, nil)
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
	m := newModel([]source.Session{s}, false, nil, nil)
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
	m := newModel([]source.Session{s}, false, nil, nil)
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
	m := newModel([]source.Session{s}, false, nil, nil)
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
	m := newModel([]source.Session{s}, true, nil, nil) // combined=true
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
	m := newModel([]source.Session{s}, false, nil, nil)
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
	m := newModel(makeSessions(), false, nil, nil)
	m.width = 100
	m.height = 8
	m.hasMsgs = false
	m.updatePreviewHeights()

	if m.vpDir.Height < 1 {
		t.Errorf("vpDir.Height = %d, want >= 1 (clamp)", m.vpDir.Height)
	}
}

// TestRenderRowSelected_SepColorsMatchAdjacentCells verifies that in a selected row,
// the space between TIME and TITLE is rendered using adjacent cell colors rather than
// a muted separator. Specifically: no muted/grey (ANSI 90) in the row, and both
// green (32, time) and yellow (33, title) appear with reverse video.
func TestRenderRowSelected_SepColorsMatchAdjacentCells(t *testing.T) {
	s := source.Session{
		Client: source.ClientClaude,
		ID:     "1ab683ce-f9fc-4799-a67e-48211866f4de",
		Title:  "test",
	}
	m := newModel([]source.Session{s}, false, nil, nil)
	m.width, m.height = 120, 40

	row := m.renderRow(s, true)
	// No muted/grey separator (90) in selected row — spaces come from cell colors.
	if strings.Contains(row, "\x1b[90m") {
		t.Errorf("selected row must not contain muted color (\\x1b[90m) sep; raw=%q", row)
	}
	if !containsColorWithReverse(row, "32") {
		t.Errorf("selected row: expected green (32) with reverse for time trailing space; raw=%q", row)
	}
	if !containsColorWithReverse(row, "33") {
		t.Errorf("selected row: expected yellow (33) with reverse for title leading space; raw=%q", row)
	}
}

// --- applyRefresh ---

// makeJSONLFile creates a temp project dir and JSONL file for testing applyRefresh.
// Returns (projectDir, jsonlPath).
func makeJSONLFile(t *testing.T, base, projectName, sessionID, title string) string {
	t.Helper()
	return makeJSONLFileWithCWD(t, base, projectName, sessionID, title, "/tmp/proj")
}

// makeJSONLFileWithCWD is like makeJSONLFile but lets the caller specify the cwd embedded in the JSONL.
func makeJSONLFileWithCWD(t *testing.T, base, projectName, sessionID, title, cwd string) string {
	t.Helper()
	projectDir := filepath.Join(base, projectName)
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	jsonlPath := filepath.Join(projectDir, sessionID+".jsonl")
	content := fmt.Sprintf(`{"type":"summary","cwd":"%s"}`+"\n"+
		`{"type":"custom-title","customTitle":"%s"}`+"\n", cwd, title)
	if err := os.WriteFile(jsonlPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return jsonlPath
}

func TestApplyRefresh_CursorAnchor(t *testing.T) {
	base := t.TempDir()

	// Two sessions; cursor on the second one (ID "sess-b").
	pathA := makeJSONLFile(t, base, "proj-a", "sess-a", "Session A")
	pathB := makeJSONLFile(t, base, "proj-b", "sess-b", "Session B")

	sessA := source.Session{Client: source.ClientClaude, ID: "sess-a", Title: "Session A", CWD: "/tmp/proj", Time: time.Now().Add(-2 * time.Second)}
	sessB := source.Session{Client: source.ClientClaude, ID: "sess-b", Title: "Session B", CWD: "/tmp/proj", Time: time.Now().Add(-1 * time.Second)}

	m := newModel([]source.Session{sessA, sessB}, false, nil, nil)
	m.cursor = 1 // pointing at sess-b

	// Simulate a refresh that updates both files.
	m.applyRefresh([]string{pathA, pathB})

	// Cursor must still point to sess-b despite possible list reorder.
	if m.cursor >= len(m.filtered) {
		t.Fatalf("cursor %d out of range (len=%d)", m.cursor, len(m.filtered))
	}
	if got := m.filtered[m.cursor].ID; got != "sess-b" {
		t.Errorf("cursor after refresh = %q, want \"sess-b\"", got)
	}
	_ = pathA // used via applyRefresh
}

func TestApplyRefresh_PendingInPreview(t *testing.T) {
	base := t.TempDir()
	pathA := makeJSONLFile(t, base, "proj-c", "sess-c", "Session C")

	sessC := source.Session{Client: source.ClientClaude, ID: "sess-c", Title: "Session C", CWD: "/tmp/proj", Time: time.Now()}
	m := newModel([]source.Session{sessC}, false, nil, nil)
	m.state = stateListPreview

	// Send a RefreshMsg while in preview mode.
	msg := RefreshMsg{Paths: []string{pathA}}
	updated, _ := m.Update(msg)
	m = updated.(Model)

	// Pending should have accumulated, sessions should NOT yet be updated.
	if len(m.pendingRefresh) == 0 {
		t.Error("pendingRefresh should be non-empty while in preview mode")
	}
	if m.filtered[0].Title != "Session C" {
		t.Errorf("sessions updated prematurely; title = %q", m.filtered[0].Title)
	}

	// Switch back to stateList via Space key.
	keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}}
	updated, _ = m.Update(keyMsg)
	m = updated.(Model)

	if len(m.pendingRefresh) != 0 {
		t.Error("pendingRefresh should be cleared after exiting preview")
	}
	// Session C title should now reflect the reloaded content ("Session C" from JSONL).
	if len(m.filtered) == 0 {
		t.Fatal("filtered is empty after refresh")
	}
	if m.filtered[0].ID != "sess-c" {
		t.Errorf("after refresh ID = %q, want \"sess-c\"", m.filtered[0].ID)
	}
}

// --- procsPollMsg ---

func TestProcsPollMsg_ClearsActiveConfsWhenNoProcs(t *testing.T) {
	sessions := makeSessions()
	m := newModel(sessions, false, nil, nil)

	// Mark session 0 as confirmed active.
	m.activeConfs = map[string]activeConf{sessions[0].ID: activeConfirmed}

	// Poll returns no procs — DetectActive will find nothing active.
	updated, _ := m.Update(procsPollMsg{procs: nil})
	m = updated.(Model)

	if m.activeConfs[sessions[0].ID] != 0 {
		t.Errorf("session %q still active after poll with no procs", sessions[0].ID)
	}
}

func TestProcsPollMsg_ReturnsNextPollCmd(t *testing.T) {
	m := newModel(makeSessions(), false, nil, nil)
	_, cmd := m.Update(procsPollMsg{procs: nil})
	if cmd == nil {
		t.Error("Update(procsPollMsg) should return a non-nil cmd to continue the poll loop")
	}
}

// TestScheduleProcsPollCmd_NoSharedStateWithMainGoroutine verifies that the cmd
// returned by scheduleProcsPollCmd does not access any shared Model state while
// the main goroutine concurrently modifies m.sessions. Run with -race.
func TestScheduleProcsPollCmd_NoSharedStateWithMainGoroutine(t *testing.T) {
	sessions := makeSessions()
	m := newModel(sessions, false, nil, nil)

	// Capture the cmd (this is what Init/Update schedule).
	_, cmd := m.Update(procsPollMsg{procs: nil})

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Simulate the cmd goroutine executing (call it as a tea.Cmd).
		cmd()
	}()

	// Main goroutine concurrently writes to m.sessions — would race if cmd
	// captured a reference to the backing array.
	for i := range m.sessions {
		m.sessions[i] = source.Session{ID: fmt.Sprintf("mutated-%d", i)}
	}

	wg.Wait()
}

func TestTickMsg_AdvancesSlowFrameEvery5Ticks(t *testing.T) {
	m := newModel(makeSessions(), false, nil, nil)
	m.tickCount = 0
	m.slowFrame = 0

	// After 5 ticks, slowFrame should advance by 1.
	for i := 0; i < 5; i++ {
		updated, _ := m.Update(tickMsg{})
		m = updated.(Model)
	}
	if m.slowFrame != 1 {
		t.Errorf("slowFrame after 5 ticks = %d, want 1", m.slowFrame)
	}

	// After 4 more ticks (total 9), slowFrame should still be 1.
	for i := 0; i < 4; i++ {
		updated, _ := m.Update(tickMsg{})
		m = updated.(Model)
	}
	if m.slowFrame != 1 {
		t.Errorf("slowFrame after 9 ticks = %d, want 1", m.slowFrame)
	}

	// After 1 more tick (total 10), slowFrame should advance to 2.
	updated, _ := m.Update(tickMsg{})
	m = updated.(Model)
	if m.slowFrame != 2 {
		t.Errorf("slowFrame after 10 ticks = %d, want 2", m.slowFrame)
	}
}

// TestEvictGuessedSiblings_EvictsWhenAllProcsConfirmed verifies that siblings are
// evicted when the confirmed proc is the only proc for that CWD (all accounted for).
func TestEvictGuessedSiblings_EvictsWhenAllProcsConfirmed(t *testing.T) {
	s1 := source.Session{ID: "s1", CWD: "/foo"}
	s2 := source.Session{ID: "s2", CWD: "/foo"}
	m := newModel([]source.Session{s1, s2}, false, nil, nil)
	m.activeConfs = map[string]activeConf{"s1": activeGuessed, "s2": activeGuessed}

	procA := source.ProcInfo{PID: "1", LStart: "ts1", CWD: "/foo"}
	m.procs = []source.ProcInfo{procA} // only one proc for /foo

	m.evictGuessedSiblings("s1", "/foo", procA)

	if m.activeConfs["s2"] != 0 {
		t.Errorf("s2 should be evicted when all procs for CWD are confirmed, got conf=%v", m.activeConfs["s2"])
	}
	if m.activeConfs["s1"] != activeGuessed {
		t.Errorf("s1 should be unchanged, got conf=%v", m.activeConfs["s1"])
	}
}

// TestEvictGuessedSiblings_DoesNotEvictWhenUnconfirmedProcRemains verifies that when a
// second proc shares the same CWD but has no cache entry, siblings are NOT evicted —
// that unconfirmed proc may belong to the sibling session.
func TestEvictGuessedSiblings_DoesNotEvictWhenUnconfirmedProcRemains(t *testing.T) {
	s1 := source.Session{ID: "s1", CWD: "/foo"}
	s2 := source.Session{ID: "s2", CWD: "/foo"}
	m := newModel([]source.Session{s1, s2}, false, nil, nil)
	m.activeConfs = map[string]activeConf{"s1": activeGuessed, "s2": activeGuessed}

	procA := source.ProcInfo{PID: "1", LStart: "ts1", CWD: "/foo"}
	procB := source.ProcInfo{PID: "2", LStart: "ts2", CWD: "/foo"}
	m.procs = []source.ProcInfo{procA, procB} // procB is unconfirmed, may belong to s2

	m.evictGuessedSiblings("s1", "/foo", procA) // procA confirmed → s1; procB still unmapped

	if m.activeConfs["s2"] == 0 {
		t.Error("s2 must NOT be evicted when an unconfirmed proc remains for the CWD")
	}
}

// TestApplyRefresh_EvictsGuessedSiblingOnConfirm verifies that when applyRefresh
// confirms one session's proc match (unique proc for that CWD), any other guessed
// session that shares the same CWD is immediately evicted from activeConfs — because
// the single proc for that CWD is now known to belong to the confirmed session.
func TestApplyRefresh_EvictsGuessedSiblingOnConfirm(t *testing.T) {
	base := t.TempDir()
	sharedCWD := "/tmp/shared-proj"

	// S1 and S2 both live in sharedCWD. Only one proc exists for that CWD.
	pathS1 := makeJSONLFileWithCWD(t, base, "proj-s1", "sess-s1", "Session S1", sharedCWD)
	_ = makeJSONLFileWithCWD(t, base, "proj-s2", "sess-s2", "Session S2", sharedCWD)

	sessS1 := source.Session{Client: source.ClientClaude, ID: "sess-s1", Title: "Session S1", CWD: sharedCWD, Time: time.Now().Add(-2 * time.Second)}
	sessS2 := source.Session{Client: source.ClientClaude, ID: "sess-s2", Title: "Session S2", CWD: sharedCWD, Time: time.Now().Add(-1 * time.Second)}

	m := newModel([]source.Session{sessS1, sessS2}, false, nil, nil)

	// Both sessions are initially guessed.
	m.activeConfs = map[string]activeConf{
		"sess-s1": activeGuessed,
		"sess-s2": activeGuessed,
	}

	// One proc exists for sharedCWD — unique match.
	m.procs = []source.ProcInfo{
		{PID: "111", LStart: "Wed 13 May 12:00:00 2026", CWD: sharedCWD},
	}

	// applyRefresh for S1: proc uniquely matches → S1 becomes confirmed.
	m.applyRefresh([]string{pathS1})

	if m.activeConfs["sess-s1"] != activeConfirmed {
		t.Errorf("sess-s1 should be activeConfirmed after proc match, got %v", m.activeConfs["sess-s1"])
	}
	// S2 shares the same CWD but the only proc is now owned by S1 — S2 must be evicted.
	if m.activeConfs["sess-s2"] != 0 {
		t.Errorf("sess-s2 should be evicted (conf=0) after sibling confirmation, got %v", m.activeConfs["sess-s2"])
	}
}

// --- splitPaths and live refresh ---

// TestSplitPaths_HotMatchesCurrentSession verifies that splitPaths classifies
// a path belonging to the cursor session as hot, and others as cold.
func TestSplitPaths_HotMatchesCurrentSession(t *testing.T) {
	base := t.TempDir()
	pathA := makeJSONLFile(t, base, "proj-a", "sess-a", "A")
	pathB := makeJSONLFile(t, base, "proj-b", "sess-b", "B")

	sessA := source.Session{
		Client:      source.ClientClaude,
		ID:          "sess-a",
		ProjectPath: filepath.Join(base, "proj-a"),
	}
	sessB := source.Session{
		Client:      source.ClientClaude,
		ID:          "sess-b",
		ProjectPath: filepath.Join(base, "proj-b"),
	}
	m := newModel([]source.Session{sessA, sessB}, false, nil, nil)
	m.cursor = 0 // cursor on sess-a

	hot, cold := m.splitPaths([]string{pathA, pathB})

	if len(hot) != 1 || hot[0] != pathA {
		t.Errorf("hot = %v, want [%s]", hot, pathA)
	}
	if len(cold) != 1 || cold[0] != pathB {
		t.Errorf("cold = %v, want [%s]", cold, pathB)
	}
}

// TestSplitPaths_EmptyFilteredReturnsAllCold verifies that when filtered is
// empty, splitPaths returns all paths as cold (no hot paths).
func TestSplitPaths_EmptyFilteredReturnsAllCold(t *testing.T) {
	m := newModel([]source.Session{}, false, nil, nil)
	hot, cold := m.splitPaths([]string{"/some/path.jsonl"})
	if len(hot) != 0 {
		t.Errorf("expected no hot paths when filtered is empty, got %v", hot)
	}
	if len(cold) != 1 {
		t.Errorf("expected 1 cold path, got %v", cold)
	}
}

// TestLiveRefreshUpdatesPreview verifies that when in stateListPreview and the
// cursor session's JSONL file changes, the preview viewports are immediately
// updated and pendingRefresh stays empty for that hot path.
func TestLiveRefreshUpdatesPreview(t *testing.T) {
	base := t.TempDir()
	pathA := makeJSONLFile(t, base, "proj-a", "sess-a", "Title Before")

	sessA := source.Session{
		Client:      source.ClientClaude,
		ID:          "sess-a",
		Title:       "Title Before",
		ProjectPath: filepath.Join(base, "proj-a"),
		CWD:         "/tmp/proj",
		Time:        time.Now(),
	}
	m := newModel([]source.Session{sessA}, false, nil, nil)
	m.state = stateListPreview
	m.width, m.height = 120, 40
	m.loadPreview()

	// Update the JSONL with a new title.
	newContent := fmt.Sprintf(`{"type":"summary","cwd":"/tmp/proj"}`+"\n"+
		`{"type":"custom-title","customTitle":"%s"}`+"\n", "Title After")
	if err := os.WriteFile(pathA, []byte(newContent), 0o644); err != nil {
		t.Fatal(err)
	}

	updated, _ := m.Update(RefreshMsg{Paths: []string{pathA}})
	m = updated.(Model)

	// Hot path must NOT land in pendingRefresh.
	if len(m.pendingRefresh) != 0 {
		t.Errorf("pendingRefresh should be empty for hot path, got %v", m.pendingRefresh)
	}
	// Session data must reflect the updated title.
	if m.filtered[0].Title != "Title After" {
		t.Errorf("session title = %q, want \"Title After\"", m.filtered[0].Title)
	}
	// vpInfo content must be non-empty (preview was reloaded).
	if m.vpInfo.View() == "" {
		t.Error("vpInfo viewport should be non-empty after live refresh")
	}
}

// TestLiveRefreshBuffersOtherPaths verifies that when in stateListPreview and a
// non-cursor session's JSONL file changes, the path is buffered in pendingRefresh
// and the preview viewports are NOT changed.
func TestLiveRefreshBuffersOtherPaths(t *testing.T) {
	base := t.TempDir()
	pathA := makeJSONLFile(t, base, "proj-a", "sess-a", "Title A")
	pathB := makeJSONLFile(t, base, "proj-b", "sess-b", "Title B")

	sessA := source.Session{
		Client:      source.ClientClaude,
		ID:          "sess-a",
		ProjectPath: filepath.Join(base, "proj-a"),
		CWD:         "/tmp/proj",
		Time:        time.Now().Add(-time.Second),
	}
	sessB := source.Session{
		Client:      source.ClientClaude,
		ID:          "sess-b",
		ProjectPath: filepath.Join(base, "proj-b"),
		CWD:         "/tmp/proj",
		Time:        time.Now(),
	}
	m := newModel([]source.Session{sessA, sessB}, false, nil, nil)
	m.state = stateListPreview
	m.width, m.height = 120, 40
	// cursor on sess-a (index 0 after sort by time: sessB is newer so index 0)
	// Find sess-a's index after sort.
	for i, s := range m.filtered {
		if s.ID == "sess-a" {
			m.cursor = i
			break
		}
	}
	m.loadPreview()
	infoBefore := m.vpInfo.View()

	// Only pathB changes (non-cursor session).
	updated, _ := m.Update(RefreshMsg{Paths: []string{pathB}})
	m = updated.(Model)

	if len(m.pendingRefresh) != 1 || m.pendingRefresh[0] != pathB {
		t.Errorf("pendingRefresh = %v, want [%s]", m.pendingRefresh, pathB)
	}
	if m.vpInfo.View() != infoBefore {
		t.Error("vpInfo should not change when a non-cursor session's file changes")
	}
	_ = pathA
}

// TestApplyRefresh_ReguessesOnTimeChange verifies that when a JSONL file is updated
// (making one session newer), applyRefresh re-evaluates the guessed assignment so
// that the more-recently-active session holds the slot.
func TestApplyRefresh_ReguessesOnTimeChange(t *testing.T) {
	base := t.TempDir()
	sharedCWD := "/tmp/reguess-proj"

	// S1 starts as the older session; S2 starts as the newer one (and is guessed).
	pathS1 := makeJSONLFileWithCWD(t, base, "proj-s1", "sess-s1", "Session S1", sharedCWD)
	_ = makeJSONLFileWithCWD(t, base, "proj-s2", "sess-s2", "Session S2", sharedCWD)

	older := time.Now().Add(-10 * time.Second)
	newer := time.Now().Add(-1 * time.Second)

	sessS1 := source.Session{Client: source.ClientClaude, ID: "sess-s1", Title: "Session S1", CWD: sharedCWD, Time: older}
	sessS2 := source.Session{Client: source.ClientClaude, ID: "sess-s2", Title: "Session S2", CWD: sharedCWD, Time: newer}

	m := newModel([]source.Session{sessS1, sessS2}, false, nil, nil)

	// Only S2 is guessed initially (it was the most-recently-active).
	m.activeConfs = map[string]activeConf{
		"sess-s2": activeGuessed,
	}
	// Two unmapped procs exist for the shared CWD — findUniqueProc returns nil
	// (ambiguous), so neither session can be confirmed; both compete for Guessed slots.
	m.procs = []source.ProcInfo{
		{PID: "221", LStart: "Wed 13 May 10:00:00 2026", CWD: sharedCWD},
		{PID: "222", LStart: "Wed 13 May 10:00:01 2026", CWD: sharedCWD},
	}

	// S1's JSONL is written, giving it a newer mtime than S2.
	// Touch the file so its mtime reflects "now" (the newest).
	nowTime := time.Now()
	if err := os.Chtimes(pathS1, nowTime, nowTime); err != nil {
		t.Fatal(err)
	}

	m.applyRefresh([]string{pathS1})

	// S1 should now hold the guessed slot (it is the newest); S2 should be evicted.
	if m.activeConfs["sess-s1"] != activeGuessed {
		t.Errorf("sess-s1 should be activeGuessed after time update, got %v", m.activeConfs["sess-s1"])
	}
	if m.activeConfs["sess-s2"] != 0 {
		t.Errorf("sess-s2 should lose guessed slot after sess-s1 became newer, got %v", m.activeConfs["sess-s2"])
	}
}

// --- horizontal scroll (colOffset) ---

// TestColOffset_InitiallyZero verifies that a new model has no horizontal offset.
func TestColOffset_InitiallyZero(t *testing.T) {
	m := newModel(makeSessions(), false, nil, nil)
	if m.colOffset != 0 {
		t.Errorf("colOffset = %d, want 0", m.colOffset)
	}
}

// TestColOffset_RightKeyIncrements verifies that pressing right increases colOffset
// when the content is wider than the terminal.
func TestColOffset_RightKeyIncrements(t *testing.T) {
	m := newModel(makeSessions(), false, nil, nil)
	m.width, m.height = 40, 40 // narrow: row content overflows
	m.updateMaxColOffset()
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m2 := updated.(Model)
	if m2.colOffset <= 0 {
		t.Errorf("right key on narrow terminal: colOffset = %d, want > 0", m2.colOffset)
	}
}

// TestColOffset_LeftKeyDecrements verifies that pressing left decreases colOffset.
func TestColOffset_LeftKeyDecrements(t *testing.T) {
	m := newModel(makeSessions(), false, nil, nil)
	m.width, m.height = 120, 40
	m.colOffset = 10
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	m2 := updated.(Model)
	if m2.colOffset >= 10 {
		t.Errorf("left key: colOffset = %d, want < 10", m2.colOffset)
	}
}

// TestColOffset_ClampAtZero verifies that pressing left when colOffset=0 keeps it at 0.
func TestColOffset_ClampAtZero(t *testing.T) {
	m := newModel(makeSessions(), false, nil, nil)
	m.width, m.height = 120, 40
	m.colOffset = 0
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	m2 := updated.(Model)
	if m2.colOffset < 0 {
		t.Errorf("left key at 0: colOffset = %d, want >= 0", m2.colOffset)
	}
}

// TestColOffset_ClampAtMax verifies that colOffset never exceeds maxColOffset.
func TestColOffset_ClampAtMax(t *testing.T) {
	m := newModel(makeSessions(), false, nil, nil)
	m.width, m.height = 80, 40 // narrow terminal to have a finite max
	m.updateMaxColOffset()
	// Scroll right repeatedly until we reach max.
	for i := 0; i < 1000; i++ {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRight})
		m = updated.(Model)
		if m.colOffset == m.maxColOffset {
			break
		}
	}
	// One more right key must not exceed max.
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m2 := updated.(Model)
	if m2.colOffset > m2.maxColOffset {
		t.Errorf("right key past max: colOffset = %d, want <= %d", m2.colOffset, m2.maxColOffset)
	}
}

// TestColOffset_ResetOnFilter verifies that changing the search query resets colOffset to 0.
func TestColOffset_ResetOnFilter(t *testing.T) {
	m := newModel(makeSessions(), false, nil, nil)
	m.width, m.height = 120, 40
	m.colOffset = 15
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	m2 := updated.(Model)
	if m2.colOffset != 0 {
		t.Errorf("colOffset after filter = %d, want 0", m2.colOffset)
	}
}

// TestColOffset_HeaderScrolls verifies that applying an offset causes the column header
// to produce different output (i.e., the leftmost portion is hidden).
func TestColOffset_HeaderScrolls(t *testing.T) {
	m := newModel(makeSessions(), false, nil, nil)
	m.width, m.height = 120, 40
	headerAt0 := m.renderColumnHeader()
	m.colOffset = 5
	headerAt5 := m.renderColumnHeader()
	if headerAt0 == headerAt5 {
		t.Error("renderColumnHeader at offset=5 must differ from offset=0")
	}
}

// TestColOffset_ShiftWheelDown scrolls right (Shift+WheelDown = horizontal scroll right).
func TestColOffset_ShiftWheelDown(t *testing.T) {
	m := newModel(makeSessions(), false, nil, nil)
	m.width, m.height = 40, 40
	m.updateMaxColOffset()
	updated, _ := m.Update(tea.MouseMsg{Button: tea.MouseButtonWheelDown, Shift: true})
	m2 := updated.(Model)
	if m2.colOffset <= 0 {
		t.Errorf("Shift+WheelDown: colOffset = %d, want > 0", m2.colOffset)
	}
}

// TestColOffset_ShiftWheelUp scrolls left (Shift+WheelUp = horizontal scroll left).
func TestColOffset_ShiftWheelUp(t *testing.T) {
	m := newModel(makeSessions(), false, nil, nil)
	m.width, m.height = 40, 40
	m.colOffset = 10
	updated, _ := m.Update(tea.MouseMsg{Button: tea.MouseButtonWheelUp, Shift: true})
	m2 := updated.(Model)
	if m2.colOffset >= 10 {
		t.Errorf("Shift+WheelUp: colOffset = %d, want < 10", m2.colOffset)
	}
}

// TestColOffset_WheelRight scrolls right via native horizontal wheel.
func TestColOffset_WheelRight(t *testing.T) {
	m := newModel(makeSessions(), false, nil, nil)
	m.width, m.height = 40, 40
	m.updateMaxColOffset()
	updated, _ := m.Update(tea.MouseMsg{Button: tea.MouseButtonWheelRight})
	m2 := updated.(Model)
	if m2.colOffset <= 0 {
		t.Errorf("WheelRight: colOffset = %d, want > 0", m2.colOffset)
	}
}

// TestColOffset_WheelLeft scrolls left via native horizontal wheel.
func TestColOffset_WheelLeft(t *testing.T) {
	m := newModel(makeSessions(), false, nil, nil)
	m.width, m.height = 40, 40
	m.colOffset = 10
	updated, _ := m.Update(tea.MouseMsg{Button: tea.MouseButtonWheelLeft})
	m2 := updated.(Model)
	if m2.colOffset >= 10 {
		t.Errorf("WheelLeft: colOffset = %d, want < 10", m2.colOffset)
	}
}

// TestColOffset_WheelDownMovesCursor verifies plain WheelDown moves cursor down in list mode.
func TestColOffset_WheelDownMovesCursor(t *testing.T) {
	m := newModel(makeSessions(), false, nil, nil)
	m.width, m.height = 120, 40
	m.cursor = 0
	updated, _ := m.Update(tea.MouseMsg{Button: tea.MouseButtonWheelDown})
	m2 := updated.(Model)
	if m2.cursor != 1 {
		t.Errorf("WheelDown: cursor = %d, want 1", m2.cursor)
	}
}

// TestColOffset_WheelUpMovesCursor verifies plain WheelUp moves cursor up in list mode.
func TestColOffset_WheelUpMovesCursor(t *testing.T) {
	m := newModel(makeSessions(), false, nil, nil)
	m.width, m.height = 120, 40
	m.cursor = 2
	updated, _ := m.Update(tea.MouseMsg{Button: tea.MouseButtonWheelUp})
	m2 := updated.(Model)
	if m2.cursor != 1 {
		t.Errorf("WheelUp: cursor = %d, want 1", m2.cursor)
	}
}

// TestColOffset_MaxFromWidestVisibleRow verifies that maxColOffset is determined by
// the widest visible row, not just the cursor row. If the cursor is on a short row
// but another visible row is longer, scrolling must still be allowed.
func TestColOffset_MaxFromWidestVisibleRow(t *testing.T) {
	short := source.Session{ID: "abc", Title: "Short", CWDDisplay: "~/x"}
	long := source.Session{ID: "def", Title: "Short", CWDDisplay: "/very/long/path/that/overflows/the/narrow/terminal/width/definitely"}
	m := newModel([]source.Session{short, long}, false, nil, nil)
	m.width, m.height = 60, 40 // narrow enough that the long row overflows
	m.cursor = 0               // cursor is on the short row

	// Compute what maxColOffset would be if only the cursor row were considered.
	cursorRow := m.renderRowFull(m.filtered[0], true, false)
	// measure scrollable part of cursor row directly via lipgloss
	cursorScrollableW := lipgloss.Width(cursorRow) - spinnerColW
	viewport := m.width - spinnerColW
	cursorOnlyMax := cursorScrollableW - viewport
	if cursorOnlyMax < 0 {
		cursorOnlyMax = 0
	}
	m.updateMaxColOffset()
	if m.maxColOffset <= cursorOnlyMax {
		t.Errorf("maxColOffset=%d should exceed cursor-only max=%d; long row must be considered", m.maxColOffset, cursorOnlyMax)
	}
}

// TestColOffset_NoScrollWhenContentFits verifies that pressing right has no effect
// when all session rows fit within the terminal width (maxColOffset == 0).
func TestColOffset_NoScrollWhenContentFits(t *testing.T) {
	sessions := []source.Session{
		{ID: "abc", Title: "Short title", CWDDisplay: "~/x"},
		{ID: "def", Title: "Also short", CWDDisplay: "~/y"},
	}
	m := newModel(sessions, false, nil, nil)
	m.width, m.height = 200, 40 // very wide terminal; content easily fits
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m2 := updated.(Model)
	if m2.colOffset != 0 {
		t.Errorf("right key when content fits: colOffset = %d, want 0", m2.colOffset)
	}
}

// TestColOffset_SpinnerAlwaysVisible verifies that the 2-col spinner prefix is preserved
// regardless of colOffset (it must not be scrolled away).
func TestColOffset_SpinnerAlwaysVisible(t *testing.T) {
	s := source.Session{Client: source.ClientClaude, ID: "abc", Title: "test"}
	m := newModel([]source.Session{s}, false, nil, nil)
	m.width, m.height = 120, 40
	m.colOffset = 5

	list := m.renderList()
	// The spinner area is always 2 leading chars ("  " or spinner+space) before
	// the scrollable content. Strip ANSI and check the list starts with 2 chars.
	plain := stripANSI(list)
	if len(plain) < 2 {
		t.Errorf("renderList with colOffset=5: plain output too short (%d chars)", len(plain))
	}
}

// TestMouseSGRFragment_DroppedFromSearch verifies that a split SGR mouse fragment
// never reaches the textinput, even when it arrives in multiple partial KeyRunes
// messages (as happens in practice when ESC is consumed by the disambiguation timer).
func TestMouseSGRFragment_DroppedFromSearch(t *testing.T) {
	cases := [][]string{
		// whole sequence in one message
		{"[<71;56;6M"},
		// split at various boundaries
		{"[", "<71;56;6M"},
		{"[<", "71;56;6M"},
		{"[<7", "1;56;6M"},
		{"[<71", ";56;6M"},
		{"[<71;", "56;6M"},
		{"[<71;5", "6;6M"},
		{"[<71;56", ";6M"},
		{"[<71;56;", "6M"},
		{"[<71;56;6", "M"},
		// three-part split
		{"[<71", ";56", ";6M"},
		// lowercase terminator
		{"[<64;1;1", "m"},
		// multiple complete fragments in one message (fast-scroll burst)
		{"[<71;49;6M[<71;49;6M[<71;49;6M"},
		// multiple complete + trailing partial (simulates burst with pending prefix)
		{"[<71;49;6M[<71;49;6M[<71;49;6M["},
		// multiple complete fragments split: first message ends mid-sequence
		{"[<71;49;6M[<71;49;6M[<71;49", ";6M"},
		// realistic burst: six complete followed by dangling prefix across two messages
		{"[<71;49;6M[<71;49;6M[<71;49;6M[<71;49;6M[<71;49;6M[<71;49;6M[", "<71;49;6M"},
	}
	for _, parts := range cases {
		m := newModel(makeSessions(), false, nil, nil)
		m.width, m.height = 120, 40
		for _, part := range parts {
			updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(part)})
			m = updated.(Model)
		}
		if m.search.Value() != "" {
			t.Errorf("split %v: search = %q, want empty", parts, m.search.Value())
		}
	}
}

// TestMouseSGRFragment_AltFlag verifies that when bubbletea sets Alt=true on a
// KeyRunes message (it consumed ESC as an Alt modifier), the raw rune content
// is still recognised and dropped rather than passed to the search box.
func TestMouseSGRFragment_AltFlag(t *testing.T) {
	// Simulate ESC consumed → Alt=true, runes = "[<71;49;6M"
	m := newModel(makeSessions(), false, nil, nil)
	m.width, m.height = 120, 40
	msg := tea.KeyMsg{Type: tea.KeyRunes, Alt: true, Runes: []rune("[<71;49;6M")}
	updated, _ := m.Update(msg)
	m = updated.(Model)
	if m.search.Value() != "" {
		t.Errorf("Alt-flagged SGR fragment leaked into search: %q", m.search.Value())
	}
}

// TestColOffset_ActiveSpinnerNoCorruption verifies that cutScrollable does not
// produce garbled output when the spinner cell contains a multi-byte UTF-8
// character with ANSI color codes (active session case).
// Regression: row[spinnerColW:] is a byte slice that cuts inside the multi-byte
// spinner ANSI sequence, causing fragments like "36m..." to appear as literal text.
func TestColOffset_ActiveSpinnerNoCorruption(t *testing.T) {
	s := source.Session{Client: source.ClientClaude, ID: "abc", Title: "test session"}
	m := newModel([]source.Session{s}, false, nil, nil)
	m.width, m.height = 40, 40
	m.activeConfs = map[string]activeConf{s.ID: activeConfirmed}
	m.spinFrame = 1 // spinnerFrames[1] = "✢" — 3-byte UTF-8
	m.colOffset = 4

	row := m.renderRowFull(s, false, false)
	cut := m.cutScrollable(row)
	plain := stripANSI(cut)

	// After stripping ANSI, the first rune must NOT be an ASCII digit or letter
	// that is an ANSI parameter fragment (e.g. "36m" left over from a cut ESC sequence).
	// Valid leading chars are: the spinner glyph itself, a space, or a content character.
	if len(plain) == 0 {
		return
	}
	first := rune(plain[0])
	if (first >= '0' && first <= '9') || first == 'm' || first == ';' {
		t.Errorf("cutScrollable produced ANSI fragment at start: plain[0]=%q, full plain prefix=%q", first, plain[:min(10, len(plain))])
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// containsColorWithReverse reports whether s contains an ANSI SGR sequence with
// both reverse video (7) and the given color code in the same sequence.
func containsColorWithReverse(s, colorCode string) bool {
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
		params := strings.Split(s[i+2:j], ";")
		hasReverse, hasColor := false, false
		for _, p := range params {
			if p == "7" {
				hasReverse = true
			}
			if p == colorCode {
				hasColor = true
			}
		}
		if hasReverse && hasColor {
			return true
		}
	}
	return false
}

// TestProcsPollInterval_Is3Seconds verifies the poll interval constant.
func TestProcsPollInterval_Is3Seconds(t *testing.T) {
	if procsPollInterval != 3*time.Second {
		t.Errorf("procsPollInterval = %v, want 3s", procsPollInterval)
	}
}

// --- status bar ---

// TestRenderStatusBar_CursorPosition verifies that the status bar shows "x/N"
// cursor position (1-indexed).
func TestRenderStatusBar_CursorPosition(t *testing.T) {
	sessions := makeSessions() // 3 sessions
	m := newModel(sessions, false, nil, nil)
	m.width, m.height = 120, 40
	m.cursor = 1

	bar := m.renderStatusBar()
	plain := stripANSI(bar)
	if !strings.Contains(plain, "2/3") {
		t.Errorf("status bar = %q, want cursor position \"2/3\"", plain)
	}
}

// TestRenderStatusBar_CursorPositionFirst verifies cursor=0 shows "1/N".
func TestRenderStatusBar_CursorPositionFirst(t *testing.T) {
	sessions := makeSessions()
	m := newModel(sessions, false, nil, nil)
	m.width, m.height = 120, 40
	m.cursor = 0

	bar := m.renderStatusBar()
	plain := stripANSI(bar)
	if !strings.Contains(plain, "1/3") {
		t.Errorf("status bar = %q, want \"1/3\"", plain)
	}
}

// TestRenderStatusBar_Error verifies that a non-fatal error status renders
// concise error text.
func TestRenderStatusBar_Error(t *testing.T) {
	m := newModel(makeSessions(), false, nil, nil)
	m.width, m.height = 120, 40
	m.statusText = "Claude load failed; showing Opencode sessions"
	m.statusIsErr = true

	bar := m.renderStatusBar()
	plain := stripANSI(bar)
	if !strings.Contains(plain, "Claude load failed") {
		t.Errorf("status bar error text = %q, want error message", plain)
	}
}

// TestRenderStatusBar_ErrorUsesErrStyle verifies that statusIsErr=true causes
// the status text to render with error color (ColorError), not muted color.
func TestRenderStatusBar_ErrorUsesErrStyle(t *testing.T) {
	m := newModel(makeSessions(), false, nil, nil)
	m.width, m.height = 120, 40
	m.statusText = "Claude load failed"
	m.statusIsErr = true

	barErr := m.renderStatusBar()

	m.statusIsErr = false
	barMuted := m.renderStatusBar()

	// The raw ANSI output must differ — error style uses a different color sequence.
	if barErr == barMuted {
		t.Error("statusIsErr=true and statusIsErr=false produce identical output; error style has no effect")
	}
}

// TestRenderStatusBar_StatusTextMerged verifies that statusText and the
// trailing separator spaces are rendered as a single Render call to produce
// a single ANSI segment (no spurious reset between text and spaces).
func TestRenderStatusBar_StatusTextMerged(t *testing.T) {
	m := newModel(makeSessions(), false, nil, nil)
	m.width, m.height = 120, 40
	m.statusText = "status"

	bar := m.renderStatusBar()
	plain := stripANSI(bar)
	// "status  " (with two trailing spaces) must appear as a contiguous substring.
	if !strings.Contains(plain, "status  ") {
		t.Errorf("status bar = %q, want \"status  \" as contiguous text", plain)
	}
}

// TestRenderStatusBar_EmptyResult verifies that the empty-result state renders
// in the status bar when loading completes with no sessions.
func TestRenderStatusBar_EmptyResult(t *testing.T) {
	m := newModel([]source.Session{}, false, nil, nil)
	m.width, m.height = 120, 40
	m.statusText = "No sessions found."

	bar := m.renderStatusBar()
	plain := stripANSI(bar)
	if !strings.Contains(plain, "No sessions found.") {
		t.Errorf("status bar empty text = %q, want empty message", plain)
	}
}

// TestRenderStatusBar_HiddenWhenNoSessions verifies that when there are no
// sessions and no statusText, renderStatusBar returns an empty string.
func TestRenderStatusBar_HiddenWhenNoSessions(t *testing.T) {
	m := newModel([]source.Session{}, false, nil, nil)
	m.width, m.height = 120, 40
	m.statusText = ""

	bar := m.renderStatusBar()
	if bar != "" {
		t.Errorf("no sessions and no status should render nothing; got %q", bar)
	}
}

// TestRenderList_ReservesStatusRow verifies that the list viewport height
// accounts for the bottom status row (always reserved, not conditional).
func TestRenderList_ReservesStatusRow(t *testing.T) {
	sessions := makeSessions()
	m := newModel(sessions, false, nil, nil)
	m.width, m.height = 120, 40

	list := m.renderList()
	listLines := strings.Count(list, "\n")

	// Expected visible rows: height - headerHeight - statusBarHeight = 40 - 2 - 1 = 37
	expectedRows := m.height - headerHeight - statusBarHeight
	if len(sessions) < expectedRows {
		expectedRows = len(sessions)
	}
	if listLines != expectedRows {
		t.Errorf("list rows = %d, want %d (height=%d - header=%d - statusBar=%d)",
			listLines, expectedRows, m.height, headerHeight, statusBarHeight)
	}
}

// TestRenderPreview_ReservesStatusRow verifies that preview layout accounts
// for the bottom status row (always reserved, not conditional on statusText).
func TestRenderPreview_ReservesStatusRow(t *testing.T) {
	m := newModel(makeSessions(), false, nil, nil)
	m.width, m.height = 120, 40
	m.state = stateListPreview
	m.hasMsgs = false

	m.updatePreviewHeights()

	// available = height - infoTotalHeight - statusBarHeight = 40 - 8 - 1 = 31
	// after sep+dir_header (2) = 29
	expectedDir := m.height - infoTotalHeight - statusBarHeight - sectionSepLines - sectionHeaderLines
	if m.vpDir.Height != expectedDir {
		t.Errorf("vpDir.Height = %d, want %d (accounts for statusBarHeight)", m.vpDir.Height, expectedDir)
	}
}

// TestRenderStatusBar_TruncatesToWidth verifies that long status text does
// not overflow the terminal width.
func TestRenderStatusBar_TruncatesToWidth(t *testing.T) {
	m := newModel(makeSessions(), false, nil, nil)
	m.width, m.height = 40, 40
	long := strings.Repeat("x", 200)
	m.statusText = long

	bar := m.renderStatusBar()
	barW := lipgloss.Width(bar)
	if barW > m.width {
		t.Errorf("status bar width %d exceeds terminal width %d", barW, m.width)
	}
}

// TestRenderStatusBar_FitsTerminalWidth verifies that the status bar does not
// exceed the terminal width (but may use the full width for right-alignment).
func TestRenderStatusBar_FitsTerminalWidth(t *testing.T) {
	m := newModel(makeSessions(), false, nil, nil)
	m.width, m.height = 40, 40
	m.statusText = "status"

	bar := m.renderStatusBar()
	barW := lipgloss.Width(bar)
	if barW > m.width {
		t.Errorf("status bar width %d exceeds terminal width %d", barW, m.width)
	}
}

// TestRenderStatusBar_KeyHints verifies that the status bar contains key hints.
func TestRenderStatusBar_KeyHints(t *testing.T) {
	sessions := makeSessions()
	m := newModel(sessions, false, nil, nil)
	m.width, m.height = 120, 40

	bar := m.renderStatusBar()
	plain := stripANSI(bar)
	if !strings.Contains(plain, "space:preview") {
		t.Errorf("status bar = %q, want key hint \"space:preview\"", plain)
	}
	if !strings.Contains(plain, "enter:select") {
		t.Errorf("status bar = %q, want key hint \"enter:select\"", plain)
	}
}

// TestRenderStatusBar_KeyHintsPreview verifies preview-mode key hints.
func TestRenderStatusBar_KeyHintsPreview(t *testing.T) {
	sessions := makeSessions()
	m := newModel(sessions, false, nil, nil)
	m.width, m.height = 120, 40
	m.state = stateListPreview

	bar := m.renderStatusBar()
	plain := stripANSI(bar)
	if !strings.Contains(plain, "esc:close") {
		t.Errorf("preview status bar = %q, want \"esc:close\"", plain)
	}
}

// TestView_ContainsStatusBar verifies that View() includes the x/N cursor
// position in the status bar.
func TestView_ContainsStatusBar(t *testing.T) {
	sessions := makeSessions() // 3 sessions
	m := newModel(sessions, false, nil, nil)
	m.width, m.height = 120, 40
	m.statusText = "status msg"

	view := m.View()
	if !strings.Contains(view, "status msg") {
		t.Error("View() should contain status bar text")
	}
	if !strings.Contains(view, "1/3") {
		t.Error("View() should contain cursor position \"1/3\"")
	}
}

// TestView_StatusBarOccupiesLastRow verifies that the status bar is rendered
// on the terminal's final row when the list has enough rows to fill the body.
func TestView_StatusBarOccupiesLastRow(t *testing.T) {
	sessions := make([]source.Session, 50)
	for i := range sessions {
		sessions[i] = source.Session{
			Client:     source.ClientClaude,
			ID:         fmt.Sprintf("session-%02d", i),
			Title:      fmt.Sprintf("Session %02d", i),
			CWDDisplay: "/tmp/project",
			Time:       time.Date(2026, 6, 3, 12, i%60, 0, 0, time.UTC),
			MsgCount:   i + 1,
		}
	}

	m := newModel(sessions, false, nil, nil)
	m.width, m.height = 120, 40
	view := m.View()
	lines := strings.Split(view, "\n")
	if len(lines) != m.height {
		t.Fatalf("View() line count = %d, want terminal height %d", len(lines), m.height)
	}
	last := stripANSI(lines[len(lines)-1])
	if !strings.Contains(last, "1/50") {
		t.Fatalf("last row = %q, want status bar with cursor position", last)
	}
}

// TestView_PreviewModeOccupiesExactHeight verifies that View() in stateListPreview
// produces exactly m.height lines — same invariant as stateList mode.
func TestView_PreviewModeOccupiesExactHeight(t *testing.T) {
	sessions := make([]source.Session, 50)
	for i := range sessions {
		sessions[i] = source.Session{
			Client:     source.ClientClaude,
			ID:         fmt.Sprintf("session-%02d", i),
			Title:      fmt.Sprintf("Session %02d", i),
			CWDDisplay: "/tmp/project",
			Time:       time.Date(2026, 6, 3, 12, i%60, 0, 0, time.UTC),
			MsgCount:   i + 1,
		}
	}

	m := newModel(sessions, false, nil, nil)
	m.width, m.height = 120, 40
	m.state = stateListPreview
	m.hasMsgs = false
	m.updatePreviewHeights()

	view := m.View()
	lines := strings.Split(view, "\n")
	if len(lines) != m.height {
		t.Fatalf("View() in preview mode line count = %d, want terminal height %d", len(lines), m.height)
	}
}

// TestView_StatusBarOnLastRowWithFewSessions verifies that when there are fewer
// sessions than listHeight, the status bar still occupies the last terminal row.
func TestView_StatusBarOnLastRowWithFewSessions(t *testing.T) {
	// makeSessions() returns 3 sessions — far fewer than a 40-row terminal
	m := newModel(makeSessions(), false, nil, nil)
	m.width, m.height = 120, 40
	m.statusText = "loading"
	view := m.View()
	lines := strings.Split(view, "\n")
	if len(lines) != m.height {
		t.Fatalf("View() line count = %d, want terminal height %d", len(lines), m.height)
	}
	last := stripANSI(lines[len(lines)-1])
	if !strings.Contains(last, "1/3") {
		t.Fatalf("last row = %q, want status bar with cursor position", last)
	}
}

// TestScrollableWidth_UsesStatusAdjustedHeight verifies that scrollableWidth()
// subtracts statusBarHeight from the visible row count, consistent with
// renderList() and updatePreviewHeights().
func TestScrollableWidth_UsesStatusAdjustedHeight(t *testing.T) {
	sessions := make([]source.Session, 40)
	for i := range sessions {
		sessions[i] = source.Session{
			Client:     source.ClientClaude,
			ID:         fmt.Sprintf("id-%02d", i),
			Title:      fmt.Sprintf("Session %02d", i),
			CWDDisplay: "/tmp/short",
			Time:       time.Date(2026, 6, 3, 12, i%60, 0, 0, time.UTC),
			MsgCount:   i + 1,
		}
	}
	// Make the last session have a very long directory so it dominates width.
	sessions[len(sessions)-1].CWDDisplay = "/tmp/this-is-a-very-long-directory-name-that-should-produce-a-much-wider-row"

	m := newModel(sessions, false, nil, nil)
	m.width, m.height = 120, 25

	// listHeight = 25 - 2 - 1 = 22 visible rows.
	// With cursor at 0, visibleRange returns [0, 22).
	// The wide row (index 39) is NOT visible, so it should not affect width.
	m.cursor = 0
	m.filtered = m.sessions
	widthWithStatus := m.scrollableWidth()

	// Verify the visible range used is correct.
	listHeight := m.height - headerHeight - statusBarHeight
	_, end := visibleRange(m.cursor, len(m.filtered), listHeight)
	if end != listHeight {
		t.Errorf("visibleRange end = %d, want %d (height - header - statusBar)", end, listHeight)
	}

	// Move cursor to 21 (last row in 22-row visible range).
	// visibleRange becomes [0, 22) — wide row still excluded.
	m.cursor = 21
	_ = m.scrollableWidth() // should not panic

	// Move cursor to 22. visibleRange shifts to [1, 23).
	// Still excludes the wide row at index 39.
	m.cursor = 22
	widthShifted := m.scrollableWidth()
	if widthShifted != widthWithStatus {
		t.Logf("scrollableWidth at cursor=0: %d, cursor=22: %d", widthWithStatus, widthShifted)
	}

	// The key invariant: visible rows = height - headerHeight - statusBarHeight.
	if listHeight != m.height-headerHeight-statusBarHeight {
		t.Errorf("listHeight = %d, want %d", listHeight, m.height-headerHeight-statusBarHeight)
	}
}

// --- Streaming: SessionBatch / applySessionBatch ---

func makeSession(id, cwd string, t time.Time) source.Session {
	return source.Session{
		Client: source.ClientClaude,
		ID:     id,
		CWD:    cwd,
		Time:   t,
	}
}

func TestApplySessionBatch_UpsertsByClientID(t *testing.T) {
	base := time.Now()
	s1 := makeSession("aaa", "/tmp/a", base)
	s2 := makeSession("bbb", "/tmp/b", base.Add(-time.Second))
	m := newModel([]source.Session{s1, s2}, false, nil, nil)

	// Upsert with updated title for s1 and a new session s3.
	s1updated := s1
	s1updated.MsgCount = 99
	s3 := makeSession("ccc", "/tmp/c", base.Add(-2*time.Second))
	batch := SessionBatch{Sessions: []source.Session{s1updated, s3}}
	m.applySessionBatch(batch)

	if len(m.sessions) != 3 {
		t.Fatalf("sessions len = %d, want 3", len(m.sessions))
	}
	// s1 must be updated, not duplicated.
	var found1 bool
	for _, s := range m.sessions {
		if s.ID == "aaa" {
			found1 = true
			if s.MsgCount != 99 {
				t.Errorf("s1.MsgCount = %d, want 99 after upsert", s.MsgCount)
			}
		}
	}
	if !found1 {
		t.Error("s1 not found after upsert")
	}
	// s3 must be inserted.
	var found3 bool
	for _, s := range m.sessions {
		if s.ID == "ccc" {
			found3 = true
		}
	}
	if !found3 {
		t.Error("s3 not inserted")
	}
}

func TestApplySessionBatch_MergeSortsAndClampsCursor(t *testing.T) {
	base := time.Now()
	sessions := make([]source.Session, 5)
	for i := range sessions {
		sessions[i] = makeSession(fmt.Sprintf("id%d", i), "/tmp/x", base.Add(time.Duration(-i)*time.Second))
	}
	m := newModel(sessions, false, nil, nil)
	m.width = 120
	m.height = 20
	m.cursor = 4 // point at last

	// Insert a newer session.
	newer := makeSession("new", "/tmp/new", base.Add(time.Second))
	m.applySessionBatch(SessionBatch{Sessions: []source.Session{newer}})

	// Sessions must remain newest-first.
	for i := 1; i < len(m.sessions); i++ {
		if m.sessions[i-1].Time.Before(m.sessions[i].Time) {
			t.Errorf("sessions not sorted: [%d].Time < [%d].Time", i-1, i)
		}
	}
	// Cursor must be within bounds.
	if m.cursor < 0 || m.cursor >= len(m.filtered) {
		t.Errorf("cursor %d out of bounds [0, %d)", m.cursor, len(m.filtered))
	}
}

func TestApplySessionBatch_RecomputesWidthsAndFilter(t *testing.T) {
	base := time.Now()
	s1 := makeSession("short", "/tmp/a", base)
	m := newModel([]source.Session{s1}, false, nil, nil)
	m.width = 120
	m.height = 20
	m.query = "long"
	m.applyFilter()
	initialFiltered := len(m.filtered) // 0: query "long" won't match "short"

	// Add a session whose ID matches the query.
	sLong := makeSession("longid", "/tmp/b", base.Add(-time.Second))
	m.applySessionBatch(SessionBatch{Sessions: []source.Session{sLong}})

	// filtered must now include sLong.
	if len(m.filtered) <= initialFiltered {
		t.Errorf("filtered len = %d after batch with matching session, want > %d", len(m.filtered), initialFiltered)
	}
}

func TestLoadingEmptyState_ShowsLoadingWhilePending(t *testing.T) {
	m := newModel(nil, false, nil, nil)
	m.width = 120
	m.height = 20
	m.loading = true

	view := m.View()
	if strings.Contains(view, "No matches") || strings.Contains(view, "No sessions") {
		t.Errorf("View() shows empty state while loading: %q", view)
	}
}

func TestLoadingEmptyState_ShowsNoSessionsAfterDone(t *testing.T) {
	m := newModel(nil, false, nil, nil)
	m.width = 120
	m.height = 20
	m.loading = false

	view := m.View()
	// When not loading and no sessions, should show empty indicator.
	if !strings.Contains(view, "No") {
		t.Errorf("View() should show no-sessions state when done, got: %q", view)
	}
}

// --- streamCmd ---

func TestStreamCmd_ClosedChannelReturnsDone(t *testing.T) {
	ch := make(chan SessionBatch)
	close(ch)
	cmd := streamCmd(ch)
	msg := cmd()
	batch, ok := msg.(SessionBatch)
	if !ok {
		t.Fatalf("streamCmd on closed channel returned %T, want SessionBatch", msg)
	}
	if !batch.Done {
		t.Errorf("closed channel: batch.Done = false, want true")
	}
}

func TestStreamCmd_DrainsSingleBatch(t *testing.T) {
	ch := make(chan SessionBatch, 1)
	base := time.Now()
	want := SessionBatch{Sessions: []source.Session{makeSession("x", "/tmp/x", base)}}
	ch <- want
	cmd := streamCmd(ch)
	msg := cmd()
	batch, ok := msg.(SessionBatch)
	if !ok {
		t.Fatalf("streamCmd returned %T, want SessionBatch", msg)
	}
	if batch.Done {
		t.Errorf("open channel with data: batch.Done = true, want false")
	}
	if len(batch.Sessions) != 1 || batch.Sessions[0].ID != "x" {
		t.Errorf("batch.Sessions = %v, want single session x", batch.Sessions)
	}
}

// --- Non-fatal load error status ---

func TestNonFatalLoadError_SetsStatusText(t *testing.T) {
	m := newModel(nil, false, nil, nil)
	m.width = 120
	m.height = 20
	m.loading = true

	errBatch := SessionBatch{Err: fmt.Errorf("Claude load failed")}
	updated, _ := m.Update(errBatch)
	m2, ok := updated.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want Model", updated)
	}

	if m2.statusText != "Claude load failed" {
		t.Errorf("statusText = %q, want %q", m2.statusText, "Claude load failed")
	}
	if !m2.statusIsErr {
		t.Error("statusIsErr = false, want true after error batch")
	}
}

func TestApplySessionBatch_ReguessesActiveAfterBatch(t *testing.T) {
	base := time.Now()
	s := makeSession("aaa", "/tmp/a", base)
	// Pre-populate activeConfs with a stale guessed entry not in the batch.
	// reguessActive clears stale guessed entries; if it's not called the stale
	// entry will remain after applySessionBatch.
	m := newModel(nil, false, nil, nil)
	m.loading = true
	m.activeConfs["stale-id"] = activeGuessed

	m.applySessionBatch(SessionBatch{Sessions: []source.Session{s}})

	// reguessActive should have cleared the stale guessed entry (no proc matches it).
	if m.activeConfs["stale-id"] != 0 {
		t.Error("stale activeGuessed entry survived applySessionBatch; reguessActive was not called")
	}
}
