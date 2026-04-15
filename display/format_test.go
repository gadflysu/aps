package display

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"local/aps/source"
)

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
	// nil sessions: width should equal len("MSG") = 3
	got := AdaptiveMsgWidth(nil)
	if got != len("MSG") {
		t.Errorf("AdaptiveMsgWidth(nil) = %d, want %d", got, len("MSG"))
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
	sessions := []source.Session{
		{Title: "hi", ID: "1ab683ce-f9fc-4799-a67e-48211866f4de", MsgCount: 1, CWDDisplay: "~"},
	}
	// naturalW = TIME(19) + titleW(2) + ID(36) + MSG(3) + DIR(1) + seps(4) = 65
	// bonus = 300 - 65 = 235 → titleW = 2 + 235 = 237
	w := ComputeListWidths(sessions, false, 300)
	if w.Title <= MaxTitleLimit {
		t.Errorf("Title should exceed MaxTitleLimit(%d) with bonus, got %d", MaxTitleLimit, w.Title)
	}
	if w.Source != 0 {
		t.Errorf("Source should be 0 when not combined, got %d", w.Source)
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
