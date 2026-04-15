package picker

import (
	"testing"

	"local/aps/source"
)

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
	// height=30: info(6) + dir_header(2) + dir_content = 30 → vpDir.Height = 22
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
	if m.vpDir.Height != 22 {
		t.Errorf("vpDir.Height = %d, want 22", m.vpDir.Height)
	}
}

func TestUpdatePreviewHeights_WithMsgs(t *testing.T) {
	// height=40: available_after_info=34, after_msgs_header=32, msgsH=32/3=10, dirH=22-2=20
	m := newModel(makeSessions(), false)
	m.width = 100
	m.height = 40
	m.hasMsgs = true
	m.updatePreviewHeights()

	if m.vpInfo.Height != 4 {
		t.Errorf("vpInfo.Height = %d, want 4", m.vpInfo.Height)
	}
	if m.vpMsgs.Height != 10 {
		t.Errorf("vpMsgs.Height = %d, want 10", m.vpMsgs.Height)
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
	// infoTotalHeight=6, sectionHeaderLines=2; available = height-6-2 = height-8
	// need available/3 < 1 → available < 3 → height < 11
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
