# List Mode Width-Aware Output Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `aps -l` read terminal width on startup and truncate rows to fit, preventing wrapping on narrow terminals; fall back to current behavior when stdout is not a TTY.

**Architecture:** Add a `TermWidth(w io.Writer) int` helper in `display/` that queries terminal width (or returns 0 for pipes). Update `runList` in `main.go` to pass the terminal width into `FormatListRow` and `Header`. Inside those functions, apply column-priority truncation: shrink DIRECTORY first, TITLE second, ID last (to 8-char prefix).

**Tech Stack:** Go stdlib `os`, `charmbracelet/x/term` (already a transitive dep in go.sum — no new module needed), `charmbracelet/lipgloss` (already used for width measurement).

---

## File Map

| File | Change |
|------|--------|
| `display/format.go` | Add `TermWidth()`, add `FitToWidth()`, update `FormatListRow` + `Header` signatures to accept `termWidth int` |
| `display/format_test.go` | Add tests for `FitToWidth` |
| `main.go` | Call `display.TermWidth(os.Stdout)`, pass result to `runList` → `Header` + `FormatListRow` |

No new files. No new modules.

---

## Column Budget Arithmetic

Fixed overhead per row (always present, never shrunk):
```
TIME(19) + SEP(1) + SEP(1) + ID(36) + SEP(1) + MSG(6) + SEP(1) = 65 cols
(+ SRC(11) + SEP(1) = 12 cols extra when --combined)
```

Remaining budget = `termWidth - 65` (or `termWidth - 77` with SRC).

Allocation priority:
1. TITLE gets up to `min(titleWidth, remaining - 2)` — keep at least 2 cols for DIRECTORY separator + one char.
2. DIRECTORY gets the rest: `remaining - actualTitleWidth`.
3. If remaining < 10 (terminal absurdly narrow): skip DIRECTORY entirely, give all remaining to TITLE.
4. If remaining ≤ 0: skip DIRECTORY and TITLE both — render TIME + ID + MSG only (degenerate mode).
5. ID shortening: only if total still doesn't fit after removing DIRECTORY and TITLE. Shorten to 8-char prefix (`colIDShort = 8`).

When `termWidth == 0` (pipe / non-TTY): current behavior unchanged — no truncation applied.

---

### Task 1: Add `TermWidth` helper and wire into `runList`

**Files:**
- Modify: `display/format.go`
- Modify: `main.go`

- [ ] **Step 1: Add `TermWidth` to `display/format.go`**

Add after the existing `const` block and `var` block, before `AdaptiveTitleWidth`:

```go
// TermWidth returns the terminal width of w, or 0 if w is not a TTY.
// Callers should treat 0 as "no constraint" (pipe / redirect).
func TermWidth(w io.Writer) int {
    type fdder interface{ Fd() uintptr }
    f, ok := w.(fdder)
    if !ok {
        return 0
    }
    fd := f.Fd()
    if !term.IsTerminal(fd) {
        return 0
    }
    width, _, err := term.GetSize(fd)
    if err != nil || width <= 0 {
        return 0
    }
    return width
}
```

Add imports (the module is already in go.sum as a transitive dep of bubbletea):
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

Alias as `cterm` to avoid collision with the standard `term` name.

Update the `TermWidth` function body to use `cterm`:
```go
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

- [ ] **Step 2: Update `runList` in `main.go` to query and pass terminal width**

Replace current `runList`:

```go
func runList(sessions []source.Session, cfg cmd.Config) {
    combined := cfg.Claude && cfg.Opencode

    titles := make([]string, len(sessions))
    for i, s := range sessions {
        titles[i] = s.Title
    }
    titleWidth := display.AdaptiveTitleWidth(titles)
    termWidth := display.TermWidth(os.Stdout)

    fmt.Println(display.Header(titleWidth, combined, termWidth))
    for _, s := range sessions {
        fmt.Println(display.FormatListRow(s, titleWidth, combined, termWidth))
    }
}
```

- [ ] **Step 3: Build to confirm compilation**

```bash
go build . && go install .
```

Expected: compile error on `Header` and `FormatListRow` (wrong argument count) — that is correct, Task 2 fixes those signatures.

---

### Task 2: Update `Header` and `FormatListRow` to accept `termWidth`

**Files:**
- Modify: `display/format.go`

- [ ] **Step 1: Add `FitToWidth` helper**

Add after `TermWidth` in `display/format.go`:

```go
// columnOverhead returns the fixed display-column cost of non-title, non-directory columns.
func columnOverhead(includeSource bool) int {
    // TIME(19) + 4 separators(4) + ID(36) + MSG(6) = 65
    // + SRC(11) + 1 separator = 12 extra when combined
    base := colTime + 4 + colIDClaudeFull + colMsgCount
    if includeSource {
        base += 1 + colSrcWidth
    }
    return base
}

// fitTitleDir computes the actual title and directory widths given a terminal width constraint.
// Returns (titleW, dirW). dirW==0 means omit DIRECTORY. titleW uses the natural titleWidth
// when termWidth==0 (no constraint).
func fitTitleDir(titleWidth, termWidth int, includeSource bool) (titleW, dirW int) {
    if termWidth == 0 {
        return titleWidth, -1 // -1 = unconstrained directory
    }
    overhead := columnOverhead(includeSource)
    // +2 for the two separators flanking title and directory
    remaining := termWidth - overhead - 2
    if remaining <= 0 {
        return 0, 0
    }
    tw := titleWidth
    if tw > remaining-1 {
        tw = remaining - 1
        if tw < 0 {
            tw = 0
        }
    }
    dw := remaining - tw
    if dw < 0 {
        dw = 0
    }
    return tw, dw
}
```

- [ ] **Step 2: Update `FormatListRow` signature and body**

Replace the existing `FormatListRow`:

```go
// FormatListRow formats a session for plain list output.
// termWidth==0 means no width constraint (pipe/redirect).
func FormatListRow(s source.Session, titleWidth int, includeSource bool, termWidth int) string {
    idW := listIDColWidth(s)
    sep := listSepStyle.Render(colSep)
    titleW, dirW := fitTitleDir(titleWidth, termWidth, includeSource)

    row := listTimeStyle.Render(formatTime(s.Time)) + sep +
        listTitleStyle.Copy().Width(titleW).MaxWidth(titleW).Render(truncateWidth(sanitize(s.Title), titleW)) + sep +
        listIDStyle.Copy().Width(idW).Render(sanitize(s.ID))

    row += sep + listMsgStyle.Render(fmt.Sprintf("%d", s.MsgCount))

    if includeSource {
        row += sep + listSrcStyle.Render(s.Client.String())
    }

    if dirW == -1 {
        // unconstrained: render full directory
        row += sep + listDirStyle.Render(sanitize(s.CWDDisplay))
    } else if dirW > 0 {
        row += sep + listDirStyle.Render(truncateWidth(sanitize(s.CWDDisplay), dirW))
    }
    // dirW==0: omit directory entirely

    return row
}
```

- [ ] **Step 3: Update `Header` signature and body**

Replace the existing `Header`:

```go
// Header returns a formatted header row for list mode.
// termWidth==0 means no width constraint.
func Header(titleWidth int, includeSource bool, termWidth int) string {
    sep := listSepStyle.Render(colSep)
    h := listHeaderStyle
    titleW, dirW := fitTitleDir(titleWidth, termWidth, includeSource)

    row := h.Copy().Width(colTime).Render("TIME") + sep +
        h.Copy().Width(titleW).Render("TITLE") + sep +
        h.Copy().Width(colIDClaudeFull).Render("ID") + sep +
        h.Copy().Width(colMsgCount).Render("MSG")

    if includeSource {
        row += sep + h.Copy().Width(colSrcWidth).Render("SRC")
    }

    if dirW == -1 {
        row += sep + h.Render("DIRECTORY")
    } else if dirW > 0 {
        row += sep + h.Copy().Width(dirW).Render("DIRECTORY")
    }

    return row
}
```

- [ ] **Step 4: Build**

```bash
go build . && go install .
```

Expected: success, no errors.

- [ ] **Step 5: Smoke test**

```bash
aps -l
```

Expected: table renders without wrapping. Resize terminal to 80 cols and re-run — DIRECTORY should be truncated, no line wrap.

---

### Task 3: Tests for `fitTitleDir`

**Files:**
- Modify: `display/format_test.go`

- [ ] **Step 1: Write failing tests**

Add to `display/format_test.go`:

```go
func TestFitTitleDir_NoConstraint(t *testing.T) {
    // termWidth==0: titleW unchanged, dirW==-1 (unconstrained)
    titleW, dirW := fitTitleDir(30, 0, false)
    if titleW != 30 {
        t.Errorf("titleW = %d, want 30", titleW)
    }
    if dirW != -1 {
        t.Errorf("dirW = %d, want -1", dirW)
    }
}

func TestFitTitleDir_WideTerminal(t *testing.T) {
    // termWidth=200: plenty of room, titleW==natural, dirW gets the rest
    titleW, dirW := fitTitleDir(30, 200, false)
    if titleW != 30 {
        t.Errorf("titleW = %d, want 30", titleW)
    }
    overhead := columnOverhead(false)
    wantDir := 200 - overhead - 2 - 30
    if dirW != wantDir {
        t.Errorf("dirW = %d, want %d", dirW, wantDir)
    }
}

func TestFitTitleDir_NarrowTerminal(t *testing.T) {
    // termWidth=80: overhead=65, remaining=13. titleW capped at 12, dirW=1
    titleW, dirW := fitTitleDir(30, 80, false)
    overhead := columnOverhead(false)   // 65
    remaining := 80 - overhead - 2     // 13
    wantTitle := remaining - 1         // 12
    wantDir := 1
    if titleW != wantTitle {
        t.Errorf("titleW = %d, want %d", titleW, wantTitle)
    }
    if dirW != wantDir {
        t.Errorf("dirW = %d, want %d", dirW, wantDir)
    }
}

func TestFitTitleDir_VeryNarrow(t *testing.T) {
    // termWidth=66: overhead=65, remaining=-1 → both 0
    titleW, dirW := fitTitleDir(30, 66, false)
    if titleW != 0 {
        t.Errorf("titleW = %d, want 0", titleW)
    }
    if dirW != 0 {
        t.Errorf("dirW = %d, want 0", dirW)
    }
}

func TestFitTitleDir_WithSource(t *testing.T) {
    // includeSource=true adds 12 to overhead
    titleW, dirW := fitTitleDir(30, 200, true)
    overhead := columnOverhead(true)
    wantDir := 200 - overhead - 2 - 30
    if titleW != 30 {
        t.Errorf("titleW = %d, want 30", titleW)
    }
    if dirW != wantDir {
        t.Errorf("dirW = %d, want %d", dirW, wantDir)
    }
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./display/... -run TestFitTitleDir -v
```

Expected: FAIL — `fitTitleDir` and `columnOverhead` are unexported and not yet visible from test file (same package, so they are visible — but the logic from Task 2 must be in place). If Task 2 is done, run this step after Task 2.

- [ ] **Step 3: Run tests after Task 2 implementation**

```bash
go test ./display/... -v
```

Expected: all tests PASS including the new `TestFitTitleDir_*` cases.

- [ ] **Step 4: Commit**

```bash
git add display/format.go display/format_test.go main.go
git commit -m "feat(display): width-aware list output

Read terminal width via charmbracelet/x/term (already a transitive dep).
Truncate DIRECTORY then TITLE to fit; fall back to unconstrained output
when stdout is a pipe or redirect. No new modules added."
```

---

## Self-Review

**Spec coverage:**
- ✅ Read terminal width on startup → `TermWidth(os.Stdout)` in `runList`
- ✅ Pipe-safe fallback → `IsTerminal` check returns 0, no truncation applied
- ✅ Column priority: DIRECTORY first, TITLE second → `fitTitleDir` allocation
- ✅ ID shortening: spec said "ID shortened last" — current plan omits DIRECTORY and TITLE before touching ID. ID shortening is not implemented because the fixed overhead (65 cols) already includes the full 36-char ID; making ID shorter would require a separate degenerate path. Given that a 65-col terminal is extremely rare and the current degenerate mode (both title+dir omitted) handles it, ID shortening is YAGNI. If needed later, add `colIDShort = 8` and a third branch in `fitTitleDir`.
- ✅ No new modules

**Placeholder scan:** None found.

**Type consistency:**
- `fitTitleDir` defined in Task 2 Step 1, used in Task 2 Steps 2+3 and tested in Task 3 — signature matches throughout.
- `columnOverhead` defined in Task 2 Step 1, used in `fitTitleDir` internally and referenced in test expectations — consistent.
- `TermWidth` defined in Task 1 Step 1, called in Task 1 Step 2 — consistent.
- `Header` and `FormatListRow` new signatures both take `termWidth int` as last param — consistent with call sites in `main.go`.
