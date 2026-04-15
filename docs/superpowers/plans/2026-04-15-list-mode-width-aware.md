# List Mode Width-Aware Output Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `aps -l` columns adapt to content and terminal width. Each column is sized to its widest value (adaptive). If the natural table width fits within the terminal, any surplus space is given to the TITLE column. If the table is wider than the terminal, output as-is — no truncation, user can use `less -S`.

**Architecture:** Add adaptive-width helpers for MSG, ID, and DIR columns in `display/format.go` (mirroring the existing `AdaptiveTitleWidth`). Add `TermWidth()` to query the terminal. A new `ListWidths` struct carries all column widths through `Header` and `FormatListRow`. Compute widths once in `runList` (`main.go`) and pass the struct down. No truncation logic — columns are only ever padded, never shrunk.

**Tech Stack:** Go stdlib `os`, `charmbracelet/x/term` (already in go.sum as transitive dep of bubbletea — no new module), `charmbracelet/lipgloss` (already used).

---

## Column Width Arithmetic

```
separators = number_of_columns + 1  (one ｜ between each col and at boundaries)
```

Columns and their natural widths:

| Column | Width source |
|--------|-------------|
| TIME | fixed 19 |
| TITLE | `min(max of all title display widths, 40)` → then bonus below |
| ID | `max of all session ID display widths` (36 for Claude, 30 for Opencode, 36 for mixed) |
| MSG | `max(len("MSG"), max of all fmt.Sprintf("%d", s.MsgCount) widths)` |
| SRC | fixed 11 (only when `--combined`) |
| DIRECTORY | `min(max of all CWDDisplay display widths, termWidth)` — 0 when termWidth==0 means unconstrained |

**naturalTableW** = sum of all column widths + separators

**bonus** = `max(0, termWidth - naturalTableW)` — extra cols distributed entirely to TITLE.

Final `titleW = min(max_title_width, 40) + bonus`

When `termWidth == 0` (pipe / non-TTY): no bonus, DIRECTORY width is its natural max with no upper bound. Behaviour is identical to current code except MSG and ID columns now expand to fit content.

---

## File Map

| File | Change |
|------|--------|
| `display/format.go` | Add `TermWidth()`, `AdaptiveMsgWidth()`, `AdaptiveDirWidth()`, `AdaptiveIDWidth()`, `ListWidths` struct, `ComputeListWidths()`; update `Header` + `FormatListRow` to take `ListWidths` |
| `display/format_test.go` | Tests for new adaptive helpers and `ComputeListWidths` |
| `main.go` | Call `display.TermWidth(os.Stdout)` + `display.ComputeListWidths(sessions, combined, termWidth)`, pass `ListWidths` to `Header` + `FormatListRow` |

No new files. No new modules.

---

### Task 1: Add `TermWidth` and adaptive column helpers

**Files:**
- Modify: `display/format.go`

- [ ] **Step 1: Add `io` import and `TermWidth`**

Add `"io"` to the import block. Add after the existing `var` block:

```go
// TermWidth returns the terminal width of w, or 0 if w is not a TTY.
// Callers treat 0 as "unconstrained" (pipe / redirect).
func TermWidth(w io.Writer) int {
	type fdder interface{ Fd() uintptr }
	f, ok := w.(fdder)
	if !ok {
		return 0
	}
	fd := f.Fd()
	if !cterm.IsTerminal(fd) {
		return 0
	}
	width, _, err := cterm.GetSize(fd)
	if err != nil || width <= 0 {
		return 0
	}
	return width
}
```

Add import alias at the top of the import block:

```go
import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	cterm "github.com/charmbracelet/x/term"

	"local/aps/source"
)
```

- [ ] **Step 2: Add `AdaptiveMsgWidth`**

```go
// AdaptiveMsgWidth returns the column width needed to display the widest message count.
// The minimum is len("MSG") so the header always fits.
func AdaptiveMsgWidth(sessions []source.Session) int {
	max := len("MSG")
	for _, s := range sessions {
		if w := len(fmt.Sprintf("%d", s.MsgCount)); w > max {
			max = w
		}
	}
	return max
}
```

- [ ] **Step 3: Add `AdaptiveIDWidth`**

```go
// AdaptiveIDWidth returns the column width needed to display the widest session ID.
func AdaptiveIDWidth(sessions []source.Session) int {
	max := 0
	for _, s := range sessions {
		if w := lipgloss.Width(s.ID); w > max {
			max = w
		}
	}
	return max
}
```

- [ ] **Step 4: Add `AdaptiveDirWidth`**

```go
// AdaptiveDirWidth returns the column width needed to display the widest CWDDisplay.
// When termWidth > 0, the result is capped at termWidth to prevent absurdly wide columns.
// When termWidth == 0, result is the natural maximum (no cap).
func AdaptiveDirWidth(sessions []source.Session, termWidth int) int {
	max := 0
	for _, s := range sessions {
		if w := lipgloss.Width(s.CWDDisplay); w > max {
			max = w
		}
	}
	if termWidth > 0 && max > termWidth {
		return termWidth
	}
	return max
}
```

- [ ] **Step 5: Build to confirm no syntax errors**

```bash
go build . && go install .
```

Expected: success (existing callers of `Header` and `FormatListRow` unchanged so far).

---

### Task 2: Add `ListWidths` struct and `ComputeListWidths`

**Files:**
- Modify: `display/format.go`

- [ ] **Step 1: Add `ListWidths` struct**

```go
// ListWidths holds pre-computed column widths for list-mode rendering.
type ListWidths struct {
	Title  int
	ID     int
	Msg    int
	Dir    int // 0 means unconstrained (pipe mode — Dir rendered without Width padding)
	Source int // 0 when not combined
}
```

- [ ] **Step 2: Add `ComputeListWidths`**

```go
// ComputeListWidths computes adaptive column widths for all sessions.
// termWidth==0 means stdout is not a TTY; no bonus space is allocated.
func ComputeListWidths(sessions []source.Session, includeSource bool, termWidth int) ListWidths {
	titleW := AdaptiveTitleWidth(extractTitles(sessions)) // min(max, 40)
	idW    := AdaptiveIDWidth(sessions)
	msgW   := AdaptiveMsgWidth(sessions)
	dirW   := AdaptiveDirWidth(sessions, termWidth)

	srcW := 0
	if includeSource {
		srcW = colSrcWidth
	}

	// Count separators: one between each adjacent column pair.
	// Columns present: TIME, TITLE, ID, MSG, [SRC,] DIR
	numCols := 5
	if includeSource {
		numCols = 6
	}
	seps := numCols - 1 // separators between columns

	naturalW := colTime + titleW + idW + msgW + srcW + dirW + seps

	// Bonus: surplus terminal width goes entirely to TITLE.
	if termWidth > 0 && naturalW < termWidth {
		titleW += termWidth - naturalW
	}

	return ListWidths{
		Title:  titleW,
		ID:     idW,
		Msg:    msgW,
		Dir:    dirW,
		Source: srcW,
	}
}

// extractTitles returns the Title field of each session.
func extractTitles(sessions []source.Session) []string {
	titles := make([]string, len(sessions))
	for i, s := range sessions {
		titles[i] = s.Title
	}
	return titles
}
```

- [ ] **Step 3: Build**

```bash
go build . && go install .
```

Expected: success.

---

### Task 3: Update `FormatListRow` and `Header` to use `ListWidths`

**Files:**
- Modify: `display/format.go`

- [ ] **Step 1: Update `FormatListRow`**

Replace the existing `FormatListRow`:

```go
// FormatListRow formats a session for plain list output.
func FormatListRow(s source.Session, w ListWidths) string {
	sep := listSepStyle.Render(colSep)

	row := listTimeStyle.Render(formatTime(s.Time)) + sep +
		listTitleStyle.Copy().Width(w.Title).Render(truncateWidth(sanitize(s.Title), w.Title)) + sep +
		listIDStyle.Copy().Width(w.ID).Render(sanitize(s.ID)) + sep +
		listMsgStyle.Copy().Width(w.Msg).Render(fmt.Sprintf("%d", s.MsgCount))

	if w.Source > 0 {
		row += sep + listSrcStyle.Render(s.Client.String())
	}

	if w.Dir > 0 {
		row += sep + listDirStyle.Copy().Width(w.Dir).Render(sanitize(s.CWDDisplay))
	} else {
		// pipe / unconstrained: render without width padding
		row += sep + listDirStyle.Render(sanitize(s.CWDDisplay))
	}

	return row
}
```

Note: `MaxWidth` is removed from the title style — `truncateWidth` pre-truncates before lipgloss sees the string, avoiding the lipgloss CJK Width+MaxWidth bug documented in `docs/superpowers/specs/lipgloss-bug-width-maxwidth-cjk.md`.

- [ ] **Step 2: Update `Header`**

Replace the existing `Header`:

```go
// Header returns a formatted header row for list mode.
func Header(w ListWidths) string {
	sep := listSepStyle.Render(colSep)
	h := listHeaderStyle

	row := h.Copy().Width(colTime).Render("TIME") + sep +
		h.Copy().Width(w.Title).Render("TITLE") + sep +
		h.Copy().Width(w.ID).Render("ID") + sep +
		h.Copy().Width(w.Msg).Render("MSG")

	if w.Source > 0 {
		row += sep + h.Copy().Width(colSrcWidth).Render("SRC")
	}

	if w.Dir > 0 {
		row += sep + h.Copy().Width(w.Dir).Render("DIRECTORY")
	} else {
		row += sep + h.Render("DIRECTORY")
	}

	return row
}
```

- [ ] **Step 3: Remove now-unused helpers**

Remove `listIDColWidth` (replaced by `AdaptiveIDWidth`) and the `colIDClaudeFull`/`colIDOpencode` constants if no longer referenced elsewhere. Check first:

```bash
grep -rn "listIDColWidth\|colIDClaudeFull\|colIDOpencode" /Users/dsu/projects.local/aps/
```

Remove only what is unreferenced.

- [ ] **Step 4: Build — expect errors in `main.go`**

```bash
go build .
```

Expected: compile errors in `main.go` because `Header` and `FormatListRow` signatures changed. That is correct — Task 4 fixes `main.go`.

---

### Task 4: Update `runList` in `main.go`

**Files:**
- Modify: `main.go`

- [ ] **Step 1: Replace `runList`**

```go
func runList(sessions []source.Session, cfg cmd.Config) {
	combined := cfg.Claude && cfg.Opencode
	termWidth := display.TermWidth(os.Stdout)
	w := display.ComputeListWidths(sessions, combined, termWidth)

	fmt.Println(display.Header(w))
	for _, s := range sessions {
		fmt.Println(display.FormatListRow(s, w))
	}
}
```

- [ ] **Step 2: Build and install**

```bash
go build . && go install .
```

Expected: success, no errors.

- [ ] **Step 3: Smoke test — TTY**

```bash
aps -l
```

Expected: all columns padded to their widest value. TITLE may be wider than 40 if terminal has surplus space. No wrapping.

- [ ] **Step 4: Smoke test — pipe**

```bash
aps -l | cat
```

Expected: clean plain-text output, no ANSI width control (DIRECTORY rendered without padding). Suitable for `grep`.

---

### Task 5: Tests for new helpers and `ComputeListWidths`

**Files:**
- Modify: `display/format_test.go`

- [ ] **Step 1: Write tests**

```go
func TestAdaptiveMsgWidth_MinIsHeaderLen(t *testing.T) {
	// No sessions: width should equal len("MSG") = 3
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
	if got != 5 { // len("12345") == 5
		t.Errorf("AdaptiveMsgWidth = %d, want 5", got)
	}
}

func TestAdaptiveIDWidth_Claude(t *testing.T) {
	sessions := []source.Session{
		{ID: "1ab683ce-f9fc-4799-a67e-48211866f4de"},
	}
	got := AdaptiveIDWidth(sessions)
	if got != 36 {
		t.Errorf("AdaptiveIDWidth Claude = %d, want 36", got)
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
	// With a wide terminal, surplus goes to title
	termWidth := 300
	w := ComputeListWidths(sessions, false, termWidth)
	// naturalW = 19 + 2 + 36 + 3 + 1 + 4(seps) = 65
	// titleW base = min(2, 40) = 2
	// bonus = 300 - 65 = 235, titleW = 2 + 235 = 237
	if w.Title <= 40 {
		t.Errorf("Title should exceed 40 with bonus, got %d", w.Title)
	}
	if w.Source != 0 {
		t.Errorf("Source should be 0 when not combined, got %d", w.Source)
	}
}

func TestComputeListWidths_NoBonus(t *testing.T) {
	sessions := []source.Session{
		{Title: "hi", ID: "1ab683ce-f9fc-4799-a67e-48211866f4de", MsgCount: 1, CWDDisplay: "~"},
	}
	// termWidth==0 (pipe): no bonus, title stays at adaptive min
	w := ComputeListWidths(sessions, false, 0)
	if w.Title != 2 { // len("hi") == 2 < 40
		t.Errorf("Title without bonus = %d, want 2", w.Title)
	}
}
```

- [ ] **Step 2: Run tests**

```bash
go test ./display/... -v
```

Expected: all PASS.

- [ ] **Step 3: Run full test suite**

```bash
go test ./...
```

Expected: all PASS.

- [ ] **Step 4: Commit**

```bash
git add display/format.go display/format_test.go main.go
git commit -m "feat(display): adaptive column widths in list mode

All columns now size to their widest content value. Surplus terminal
width is given to the TITLE column (may exceed prior 40-col cap).
No truncation: output wider than terminal is left to less -S.
Uses charmbracelet/x/term (already a transitive dep) for TTY detection."
```

---

## Self-Review

**Spec coverage:**
- ✅ Adaptive MSG width → `AdaptiveMsgWidth`
- ✅ Adaptive ID width (multi-client aware) → `AdaptiveIDWidth`
- ✅ Adaptive DIR width, capped at termWidth → `AdaptiveDirWidth`
- ✅ `AdaptiveTitleWidth` unchanged: min(max, 40) baseline
- ✅ Surplus terminal width → bonus to TITLE → `ComputeListWidths`
- ✅ No truncation of any column
- ✅ Pipe-safe: termWidth==0 skips TTY query, no bonus, DIR unconstrained
- ✅ No new modules

**Placeholder scan:** None found.

**Type consistency:**
- `ListWidths` defined in Task 2 Step 1; used in Task 3 Steps 1-2 and Task 4 Step 1 — field names `Title`, `ID`, `Msg`, `Dir`, `Source` consistent throughout.
- `ComputeListWidths` defined in Task 2 Step 2; called in Task 4 Step 1 — signature `([]source.Session, bool, int) ListWidths` consistent.
- `AdaptiveMsgWidth`, `AdaptiveIDWidth`, `AdaptiveDirWidth` defined in Task 1; called in Task 2 — signatures consistent.
- `Header(w ListWidths)` and `FormatListRow(s source.Session, w ListWidths)` defined in Task 3; called in Task 4 — consistent.
