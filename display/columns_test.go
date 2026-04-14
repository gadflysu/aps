package display

import (
	"strings"
	"testing"
)

// --- Width ---

func TestWidth_ASCII(t *testing.T) {
	if got := Width("hello"); got != 5 {
		t.Errorf("Width(\"hello\") = %d, want 5", got)
	}
}

func TestWidth_Empty(t *testing.T) {
	if got := Width(""); got != 0 {
		t.Errorf("Width(\"\") = %d, want 0", got)
	}
}

func TestWidth_CJK(t *testing.T) {
	// Each CJK character counts as 2 columns
	if got := Width("你好"); got != 4 {
		t.Errorf("Width(\"你好\") = %d, want 4", got)
	}
}

func TestWidth_Mixed(t *testing.T) {
	// "A" (1) + "中" (2) + "B" (1) = 4
	if got := Width("A中B"); got != 4 {
		t.Errorf("Width(\"A中B\") = %d, want 4", got)
	}
}

// --- Pad ---

func TestPad_ShorterThanWidth(t *testing.T) {
	got := Pad("hi", 5)
	if len(got) != 5 {
		t.Errorf("Pad(\"hi\", 5) length = %d, want 5", len(got))
	}
	if !strings.HasPrefix(got, "hi") {
		t.Errorf("Pad(\"hi\", 5) = %q, should start with \"hi\"", got)
	}
}

func TestPad_ExactWidth(t *testing.T) {
	got := Pad("hello", 5)
	if got != "hello" {
		t.Errorf("Pad(\"hello\", 5) = %q, want \"hello\"", got)
	}
}

func TestPad_WiderThanWidth(t *testing.T) {
	// When string is already wider, return unchanged
	got := Pad("toolong", 3)
	if got != "toolong" {
		t.Errorf("Pad(\"toolong\", 3) = %q, want \"toolong\" (unchanged)", got)
	}
}

func TestPad_CJKWidth(t *testing.T) {
	// "中" has width 2, padding to 4 should add 2 spaces
	got := Pad("中", 4)
	if Width(got) != 4 {
		t.Errorf("Pad(\"中\", 4) display width = %d, want 4", Width(got))
	}
}

// --- Truncate ---

func TestTruncate_NoTruncationNeeded(t *testing.T) {
	got := Truncate("hello", 10)
	if got != "hello" {
		t.Errorf("Truncate(\"hello\", 10) = %q, want \"hello\"", got)
	}
}

func TestTruncate_ExactFit(t *testing.T) {
	// "hello" width=5, maxWidth=5; suffix "..." width=3; target=2; "he" is 2 but "hel" is 3 which exceeds 2.
	// So "he..." which is 5 wide. But actually: target = 5-3=2, so "he" + "..." = "he..."
	got := Truncate("hello", 5)
	if got != "he..." {
		t.Errorf("Truncate(\"hello\", 5) = %q, want \"he...\"", got)
	}
}

func TestTruncate_TruncatesLongString(t *testing.T) {
	long := strings.Repeat("a", 60)
	got := Truncate(long, 20)
	if Width(got) > 20 {
		t.Errorf("Truncate result width %d exceeds maxWidth 20", Width(got))
	}
	if !strings.HasSuffix(got, "...") {
		t.Errorf("Truncate(long, 20) = %q, should end with \"...\"", got)
	}
}

func TestTruncate_VerySmallMaxWidth(t *testing.T) {
	// maxWidth <= suffixWidth (3), target <= 0 → return "..."
	got := Truncate("hello", 3)
	if got != "..." {
		t.Errorf("Truncate(\"hello\", 3) = %q, want \"...\"", got)
	}
	got2 := Truncate("hello", 2)
	if got2 != "..." {
		t.Errorf("Truncate(\"hello\", 2) = %q, want \"...\"", got2)
	}
}

func TestTruncate_CJK(t *testing.T) {
	// "你好世界" = 8 cols; maxWidth=6, target=3; "你"=2,"好"=2 → cur=4>3 at "好"? No:
	// i=0 "你" cw=2; cur+2=2 <= 3 → cur=2
	// i=3 "好" cw=2; cur+2=4 > 3 → truncate at i=3, return "你..."
	got := Truncate("你好世界", 6)
	if got != "你..." {
		t.Errorf("Truncate(\"你好世界\", 6) = %q, want \"你...\"", got)
	}
}

func TestTruncate_Empty(t *testing.T) {
	got := Truncate("", 10)
	if got != "" {
		t.Errorf("Truncate(\"\", 10) = %q, want \"\"", got)
	}
}

// --- AdaptiveTitleWidth ---

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
	want := 7 // "world!!" = 7
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
