package picker

import "testing"

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
