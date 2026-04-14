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

We only use a small subset of fzf's capabilities; the features we need are fully
covered by this stack.

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

Preview content is loaded **synchronously** on `Space` press. For typical JSONL
sizes (< 1 MB) this completes in < 10 ms and does not block the UI noticeably.
Async loading (via `tea.Cmd`) is a future optimization, not required for v1.

No animation is needed for the `stateList → stateListPreview` transition;
bubbletea does not provide built-in transitions and a CLI tool does not need them.

### 2.5 Minimum Terminal Size

Below 80 columns or 10 rows the layout is undefined. The `Init` function checks
`tea.WindowSizeMsg` and renders an error message if the terminal is too small,
rather than attempting to draw a broken layout.

### 2.6 Separator Character

The column separator `｜` (U+FF5C FULLWIDTH VERTICAL LINE) has east-asian-width
`F` (fullwidth = 2 columns). lipgloss uses go-runewidth internally and handles
this correctly; no special treatment is required.

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

// ANSI 16-color palette constants — respect the user's terminal color theme.
const (
    colorTime   = lipgloss.Color("2")  // ANSI green
    colorTitle  = lipgloss.Color("3")  // ANSI yellow
    colorID     = lipgloss.Color("6")  // ANSI cyan
    colorMsg    = lipgloss.Color("5")  // ANSI magenta
    colorDir    = lipgloss.Color("8")  // ANSI dark grey
    colorDirSel = lipgloss.Color("7")  // ANSI white (selected state)
    colorBorder = lipgloss.Color("8")  // ANSI dark grey
)

var (
    timeStyle  = lipgloss.NewStyle().Foreground(colorTime).Width(19)
    titleStyle = lipgloss.NewStyle().Foreground(colorTitle).
                     Width(titleColWidth).MaxWidth(titleColWidth)
    idStyle    = lipgloss.NewStyle().Foreground(colorID).Width(12)
    msgStyle   = lipgloss.NewStyle().Foreground(colorMsg).Width(6)
    srcStyle   = lipgloss.NewStyle().Foreground(colorMsg).Width(11)
    dirStyle   = lipgloss.NewStyle().Foreground(colorDir)
    sepStyle   = lipgloss.NewStyle().Foreground(colorDir)

    // Selected-state variants: title bold, directory brightens to white
    titleStyleSel = titleStyle.Copy().Bold(true)
    dirStyleSel   = lipgloss.NewStyle().Foreground(colorDirSel)

    previewBorder = lipgloss.NewStyle().
                        BorderLeft(true).
                        BorderStyle(lipgloss.NormalBorder()).
                        BorderForeground(colorBorder).
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
    filtered  []source.Session  // subset after fuzzy filter; equals sessions when query=""
    cursor    int               // index into filtered
    query     string            // current search string
    state     state
    preview   viewport.Model   // right-panel scroll state
    search    textinput.Model  // search input widget
    width     int              // terminal columns (from WindowSizeMsg)
    height    int              // terminal rows   (from WindowSizeMsg)
    combined  bool
    chosen    *source.Session  // non-nil after Enter; signals tea.Quit
}
```

### Update — key handling, filter, window resize

```go
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {

    case tea.WindowSizeMsg:
        m.width, m.height = msg.Width, msg.Height
        // Preview column allocation: m.width * 4/10 total.
        // previewBorder adds BorderLeft(1) + PaddingLeft(1) = 2 cols of chrome.
        // Viewport content width must be 2 less so the styled output fills
        // exactly m.width*4/10 columns.
        m.preview.Width  = msg.Width*4/10 - 2
        // No top/bottom border in previewBorder, so no vertical chrome.
        // Left pane: headerHeight(3) + list(m.height-3) = m.height.
        // Right pane: m.height. Both sides match → JoinHorizontal fits terminal.
        m.preview.Height = msg.Height
        return m, nil

    case tea.KeyMsg:
        switch msg.String() {
        case "ctrl+c", "esc", "q":
            return m, tea.Quit
        case "enter":
            if len(m.filtered) > 0 {
                s := m.filtered[m.cursor]
                m.chosen = &s
            }
            return m, tea.Quit
        case "up", "k":
            if m.cursor > 0 {
                m.cursor--
            }
            if m.state == stateListPreview {
                m.loadPreview()
            }
        case "down", "j":
            if m.cursor < len(m.filtered)-1 {
                m.cursor++
            }
            if m.state == stateListPreview {
                m.loadPreview()
            }
        case " ":
            if m.state == stateList {
                m.state = stateListPreview
                m.loadPreview()
            } else {
                m.state = stateList
            }
        default:
            // All other keys go to the search input
            var cmd tea.Cmd
            m.search, cmd = m.search.Update(msg)
            newQuery := m.search.Value()
            if newQuery != m.query {
                m.query = newQuery
                m.applyFilter()
                m.cursor = 0
                if m.state == stateListPreview {
                    m.loadPreview()
                }
            }
            return m, cmd
        }
    }
    return m, nil
}

// applyFilter re-computes m.filtered from m.sessions using sahilm/fuzzy.
// Performance assumption: < 5 000 sessions → no debounce needed.
//
// Unicode/CJK: sahilm/fuzzy iterates rune indices (not bytes), so CJK
// characters in titles and paths are matched correctly as individual runes.
// Fuzzy matching on CJK is character-by-character (e.g. query "修" matches
// title "修复 bug"). Queries mixing ASCII and CJK are also supported.
// This is sufficient for session title search; no special handling needed.
func (m *Model) applyFilter() {
    if m.query == "" {
        m.filtered = m.sessions
        return
    }
    targets := make([]string, len(m.sessions))
    for i, s := range m.sessions {
        targets[i] = s.Title + " " + s.CWDDisplay
    }
    matches := fuzzy.Find(m.query, targets)
    m.filtered = make([]source.Session, len(matches))
    for i, match := range matches {
        m.filtered[i] = m.sessions[match.Index]
    }
}

// loadPreview calls the appropriate preview function synchronously and stores
// the result in m.preview. Safe to call when m.filtered is empty.
func (m *Model) loadPreview() {
    if len(m.filtered) == 0 {
        m.preview.SetContent("No sessions.")
        return
    }
    s := m.filtered[m.cursor]
    var buf strings.Builder
    if s.Client == source.ClientClaude {
        preview.RenderClaude(&buf, s.ID, s.ProjectPath, s.CWD)
    } else {
        preview.RenderOpencode(&buf, s.ID, s.CWD)
    }
    m.preview.SetContent(buf.String())
    m.preview.GotoTop()
}
```

### renderList — viewport window + empty state

```go
// headerHeight is the number of terminal rows consumed by the search bar:
// one input line + two blank lines ("> query\n\n").
// Used in both renderList (to compute visible rows) and View (header string).
const headerHeight = 3

// visibleRange returns the [start, end) slice indices of sessions to render
// given cursor position, total count, and available row height.
// Extracted as a pure function for easier boundary-condition testing
// (cursor=0, cursor=last, cursor outside viewport, total < height, etc.).
func visibleRange(cursor, total, height int) (start, end int) {
    start = 0
    if cursor >= height {
        start = cursor - height + 1
    }
    end = start + height
    if end > total {
        end = total
    }
    return
}

func (m Model) renderList() string {
    if len(m.filtered) == 0 {
        return lipgloss.NewStyle().Foreground(colorDir).Render("No matches.")
    }

    listHeight := m.height - headerHeight
    start, end := visibleRange(m.cursor, len(m.filtered), listHeight)

    var sb strings.Builder
    for i := start; i < end; i++ {
        sb.WriteString(m.renderRow(m.filtered[i], i == m.cursor))
        sb.WriteByte('\n')
    }
    return sb.String()
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
const minWidth, minHeight = 80, 10

func (m Model) View() string {
    if m.width < minWidth || m.height < minHeight {
        return fmt.Sprintf("Terminal too small (need %dx%d, got %dx%d)",
            minWidth, minHeight, m.width, m.height)
    }

    header := "> " + m.search.View() + "\n\n"  // headerHeight rows
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
