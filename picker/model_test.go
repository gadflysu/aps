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
