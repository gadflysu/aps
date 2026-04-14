# Design: Replace fzf with bubbletea TUI Picker

**Date:** 2026-04-14  
**Branch:** go-rewrite  
**Status:** Draft — pending user approval

---

## 1. Goal

Eliminate all external runtime dependencies (bash, python, fzf). Deliver a single
Go binary with zero required external tools.

The current stack has three uncontrolled factors:
- `bash` — shell portability, version differences
- `python3` — SQLite, JSONL, Unicode logic embedded in heredocs
- `fzf` — interactive picker, preview subprocess, version-gated features

The Go rewrite eliminated bash and python. This change eliminates fzf.

---

## 2. Design Decisions

### 2.1 Framework Choice

**bubbletea** (Elm architecture TUI) + **lipgloss** (styling) + **sahilm/fuzzy**
(fuzzy matching).

Why not reimplementing fzf's full feature set: we only use a small subset of
fzf's capabilities. The features we need are fully covered by this stack.

### 2.2 Rendering Principle

> **Do not manually concatenate ANSI escape codes or manually pad strings.**
> Pass plain text to lipgloss styles; let the framework own all terminal rendering.

This eliminates `color.go` (ANSI constants) and `columns.go` (manual Pad/Truncate)
entirely. The equivalent is provided by lipgloss `Width()`, `MaxWidth()`, and
`Foreground()` style properties, which are CJK-aware internally via go-runewidth.

### 2.3 Interaction Model

- **Search**: always active — type to fuzzy-filter, no mode switch required
- **Preview**: hidden by default; `Space` toggles a right-side panel (60/40 split)
- **Navigation**: arrow keys or `j`/`k`
- **Launch**: `Enter`
- **Quit**: `Esc` / `q` / `Ctrl-C`

### 2.4 Preview Architecture

fzf invokes preview by forking a subprocess (`aps --_preview-claude ...`) on
every focus change. With bubbletea, `Space` triggers a direct Go function call
to `preview.Claude()` or `preview.Opencode()` — no fork, no IPC, no internal
`--_preview-*` subcommands.

---

## 3. Package Changes

### Deleted

| File | Reason |
|------|--------|
| `picker/fzf.go` | Entire fzf subprocess layer replaced |
| `display/color.go` | ANSI constants → lipgloss styles |
| `display/columns.go` | Manual Pad/Truncate → lipgloss Width/MaxWidth |
| `display/FormatInteractive*` | fzf TAB-field protocol, no longer needed |
| `display/buildLineWith*` | Internal helpers for the above |
| `cmd/` — PreviewMode, PreviewArgs | Internal subcommand dispatch, gone |
| `cmd/` — `--_preview-*` handling | No subprocess preview |

### Modified

| File | Change |
|------|--------|
| `display/format.go` | `FormatListRow` + `Header` rewritten using lipgloss |
| `main.go` | `runInteractive` simplified: no line formatting, no field parsing |

`AdaptiveTitleWidth` calculation logic moves to `main.go` (application-layer
decision, not a rendering primitive). The `display/` package no longer owns it.

### New

| File | Purpose |
|------|---------|
| `picker/model.go` | bubbletea Model: state machine, fuzzy filter, key handling |
| `picker/styles.go` | All lipgloss style definitions, selected/normal variants |

---

## 4. Core Structures

### picker/styles.go

```go
const titleColWidth = 40  // TUI mode: fixed; list mode: adaptive from main.go

var (
    timeStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Width(19)
    titleStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("3")).
                     Width(titleColWidth).MaxWidth(titleColWidth)
    idStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Width(12)
    msgStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("5")).Width(6)
    srcStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("5")).Width(11)
    dirStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
    sepStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))

    // Selected-state variants
    titleStyleSel = titleStyle.Copy().Bold(true)
    dirStyleSel   = lipgloss.NewStyle().Foreground(lipgloss.Color("7"))

    previewBorder = lipgloss.NewStyle().
                        BorderLeft(true).
                        BorderStyle(lipgloss.NormalBorder()).
                        BorderForeground(lipgloss.Color("8")).
                        PaddingLeft(1)
)
```

### picker/model.go — Model

```go
type state int
const (
    stateList        state = iota
    stateListPreview
)

type Model struct {
    sessions  []source.Session
    filtered  []source.Session
    cursor    int
    query     string
    state     state
    preview   viewport.Model
    search    textinput.Model
    width     int
    height    int
    combined  bool
    chosen    *source.Session   // non-nil when user pressed Enter
}
```

### Row rendering (plain text → lipgloss, no ANSI hand-coding)

```go
func (m Model) renderRow(s source.Session, selected bool) string {
    id := s.ID
    if s.Client == source.ClientClaude && len(id) > 12 {
        id = id[:12]
    }
    tSty, dSty, prefix := titleStyle, dirStyle, "  "
    if selected {
        tSty, dSty, prefix = titleStyleSel, dirStyleSel, "▶ "
    }
    row := timeStyle.Render(s.Time.Format("2006-01-02 15:04:05")) +
        sepStyle.Render("｜") +
        tSty.Render(s.Title) +
        sepStyle.Render("｜") +
        idStyle.Render(id) +
        sepStyle.Render("｜") +
        msgStyle.Render(fmt.Sprintf("%d", s.MsgCount))
    if m.combined {
        row += sepStyle.Render("｜") + srcStyle.Render(s.Client.String())
    }
    row += sepStyle.Render("｜") + dSty.Render(s.CWDDisplay)
    return prefix + row
}
```

### Layout (View)

```go
func (m Model) View() string {
    header := "> " + m.search.View() + "\n\n"
    list   := m.renderList()

    if m.state == stateListPreview {
        lw := m.width * 6 / 10
        pw := m.width - lw
        left  := lipgloss.NewStyle().Width(lw).Render(header + list)
        right := previewBorder.Width(pw).Height(m.height).Render(m.preview.View())
        return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
    }
    return header + list
}
```

### Public interface (called from main.go)

```go
func Run(sessions []source.Session, combined bool) (*source.Session, error)
```

main.go after selection:
```go
session, err := picker.Run(sessions, combined)
if session == nil { os.Exit(0) }
mustLaunch(launcher.Claude(session.ID, session.CWD, launchOpts))
```

---

## 5. Performance Analysis

### 5.1 Startup latency

| Phase | Current (fzf) | This change |
|-------|--------------|-------------|
| Claude JSONL scan | sequential | unchanged |
| Opencode SQLite query | sequential, after Claude | unchanged |
| UI appears | after all data loaded | after all data loaded |

**No regression.** Both approaches wait for full data before showing UI.

The current implementation also had no streaming because the Go binary pipes all
lines to fzf's stdin in one shot — equivalent to waiting for completion.

### 5.2 Fuzzy filtering (per keypress)

sahilm/fuzzy on 1 000 sessions: < 1 ms. Not a concern at typical session counts.
At > 10 000 sessions, add a 50 ms debounce to avoid filtering on every keystroke.

### 5.3 Per-frame rendering

lipgloss renders ~30 visible rows per frame. String operations at this scale are
microsecond-level. bubbletea only triggers re-render on state changes, never
on a fixed tick.

### 5.4 Preview loading

| | fzf | This change |
|-|-----|------------|
| Trigger | every focus change (automatic) | Space keypress (manual) |
| Execution | fork subprocess, parse args, re-open file | direct Go function call |
| Fork overhead | ~5–20 ms per focus event | eliminated |
| File I/O | JSONL re-read on every focus | same I/O, no fork cost |
| Caching | none (subprocess has no memory) | can cache by session ID |

---

## 6. Future Performance Optimizations

These are deferred to a later iteration. The first version must be correct;
optimizations come after the baseline is validated.

### P1 — Parallel session loading with streaming UI

Load Claude and Opencode concurrently:

```go
go func() { ch <- loadClaude(...) }()
go func() { ch <- loadOpencode(...) }()
```

bubbletea receives partial results via `tea.Cmd` / `tea.Msg` and renders
immediately. UI appears with the first batch of sessions; remaining sessions
stream in as they load.

fzf cannot do this (stdin pipe must be written linearly).
**Expected impact: UI appears ~2× faster when both sources are enabled.**

### P2 — Preview content cache

```go
previewCache map[string]string  // session ID → rendered preview text
```

Once a session's preview is loaded (Space keypress), the result is stored.
Toggling preview off and back on for the same session is instant.

### P3 — Fuzzy match character highlighting

sahilm/fuzzy returns `MatchedIndexes []int` per match. Use these to bold or
highlight matched characters in the title column. Requires iterating title runes
and applying per-character lipgloss styles. Deferred because it increases
rendering complexity without affecting correctness.

### P4 — Async preview loading

Move preview loading into a `tea.Cmd` goroutine. The UI remains responsive
during file I/O; a spinner or placeholder text is shown until the content arrives.
For typical JSONL sizes this is unnecessary, but useful for very large session
files.
