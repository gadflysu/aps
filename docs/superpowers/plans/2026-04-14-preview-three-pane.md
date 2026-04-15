# Preview Three-Pane Layout — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

> **Status: DONE** — Implemented in v0.2.0.

**Goal:** Split the single preview viewport into three independent scrollable sections (SESSION INFO / RECENT MESSAGES / DIRECTORY LIST), each rendered as its own `viewport.Model` with a labeled header.

**Architecture:** Add section-level string-returning functions to the `preview` package; refactor `picker/model.go` to hold three viewports (`vpInfo`, `vpMsgs`, `vpDir`) composed vertically with lipgloss. Section headers use `BorderBottom(true)` instead of hand-drawn `━━━` lines. `j/k` scroll the focused section; `↑/↓` navigate the list cursor; `Tab` cycles between sections.

**Tech Stack:** `charmbracelet/bubbles v1.0.0` (viewport.Model), `charmbracelet/lipgloss v1.1.0` (JoinVertical, BorderBottom), Go stdlib.

---

## Context: Layout sketch

```
┌─ left (60%) ──────────────────┬─ right (40%) ──────────────────────────┐
│ > search                      │ SESSION INFO                           │
│                               │ ──────────────────────────────────     │
│ ▶ 2026-04-14 Fix login bug …  │ Title:    Fix login bug                │
│   2026-04-13 Refactor DB …    │ Time:     2026-04-14 15:04:05          │
│   …                           │ Messages: 7                            │
│                               │ Directory: ~/projects/auth             │
│                               │ RECENT MESSAGES                        │
│                               │ ──────────────────────────────────     │
│                               │ • fix the test                         │
│                               │ • refactor the auth module             │
│                               │ DIRECTORY                              │
│                               │ ──────────────────────────────────     │
│                               │ drwxr-x  src/                          │
│                               │ -rw-r--  main.go                       │
└───────────────────────────────┴────────────────────────────────────────┘
```

**Key bindings in preview mode:**
- `↑/↓`: navigate list cursor (same as before)
- `j/k`: scroll the focused section (vpMsgs or vpDir)
- `Tab`: cycle focused section between RECENT MESSAGES ↔ DIRECTORY
- `Space`: toggle preview panel on/off
- `Enter/Esc/q/Ctrl+C`: unchanged

**Section titles:** rendered via `lipgloss.NewStyle().BorderBottom(true)...Render(title)` — no hand-drawn `━━━` required.

---

## File Map

| Action | File | Change |
|--------|------|--------|
| Modify | `preview/claude.go` | Add `ClaudeInfo()`, `ClaudeMsgs()` |
| Modify | `preview/opencode.go` | Add `OpencodeInfo()` |
| Modify | `preview/shared.go` | Add `DirListing()` |
| Modify | `picker/model.go` | Three viewports, previewFocus, new key routing |
| Modify | `preview/preview_test.go` | Tests for the four new section functions |
| Modify | `picker/model_test.go` | Tests for `updatePreviewHeights` |

---

## Task 1: Add section render functions to preview package

**Files:**
- Modify: `preview/claude.go` (append after existing functions)
- Modify: `preview/opencode.go` (append after existing functions)
- Modify: `preview/shared.go` (append after existing function)
- Modify: `preview/preview_test.go` (append tests)

### Step 1: Write failing tests for section functions

Append to `preview/preview_test.go`:

```go
// --- Section render functions ---

func TestClaudeInfo_ContainsAllFields(t *testing.T) {
	dir := t.TempDir()
	writeJSONL(t, dir, "s1", "hello")

	plain := stripANSI(ClaudeInfo("s1", dir, "/work/path"))

	for _, want := range []string{"Title:", "Time:", "Messages:", "Directory:", "/work/path"} {
		if !strings.Contains(plain, want) {
			t.Errorf("ClaudeInfo missing %q\noutput:\n%s", want, plain)
		}
	}
}

func TestClaudeMsgs_ReturnsMessages(t *testing.T) {
	dir := t.TempDir()
	writeJSONL(t, dir, "s2", "recent message text")

	plain := stripANSI(ClaudeMsgs("s2", dir))

	if !strings.Contains(plain, "recent message text") {
		t.Errorf("ClaudeMsgs missing message text\noutput:\n%s", plain)
	}
}

func TestClaudeMsgs_EmptyWhenNoJSONL(t *testing.T) {
	result := ClaudeMsgs("nonexistent", t.TempDir())
	if result != "" {
		t.Errorf("ClaudeMsgs expected empty string for missing JSONL, got %q", result)
	}
}

func TestOpencodeInfo_EmptyWhenNoDB(t *testing.T) {
	t.Setenv("OPENCODE_DATA_DIR", t.TempDir())
	result := OpencodeInfo("any-id", "/some/dir")
	if result != "" {
		t.Errorf("OpencodeInfo expected empty string when no DB, got %q", result)
	}
}

func TestDirListing_ExistingDir_ReturnsContent(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "testfile.txt"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	result := DirListing(dir)
	if result == "" {
		t.Error("DirListing returned empty for existing directory")
	}
}

func TestDirListing_NonExistentDir_ReturnsErrorMessage(t *testing.T) {
	plain := stripANSI(DirListing("/no/such/path/ever"))
	if !strings.Contains(plain, "directory not found") {
		t.Errorf("DirListing missing error message\noutput:\n%s", plain)
	}
}
```

### Step 2: Run tests — verify they fail

```bash
cd /Users/dsu/projects.local/aps && go test ./preview/... -v -run "TestClaudeInfo|TestClaudeMsgs|TestOpencodeInfo|TestDirListing" 2>&1
```

Expected: FAIL — `ClaudeInfo`, `ClaudeMsgs`, `OpencodeInfo`, `DirListing` undefined.

### Step 3: Add ClaudeInfo and ClaudeMsgs to preview/claude.go

Append after `filterPreviewMsg` at the end of the file:

```go
// ClaudeInfo returns the session info fields (Title/Time/Messages/Directory)
// as a styled string suitable for the info viewport section.
// No section header is included; the caller provides the header via lipgloss.
func ClaudeInfo(sessionID, projectPath, workingDir string) string {
	jsonlFile := filepath.Join(projectPath, sessionID+".jsonl")

	var timeStr string
	if info, err := os.Stat(jsonlFile); err == nil {
		timeStr = info.ModTime().Format("2006-01-02 15:04:05")
	}

	title, msgCount, _ := parseJSONLPreview(jsonlFile)

	var sb strings.Builder
	fmt.Fprintf(&sb, "%s     %s\n", previewLabelTitle.Render("Title:"), title)
	fmt.Fprintf(&sb, "%s      %s\n", previewLabelTime.Render("Time:"), timeStr)
	fmt.Fprintf(&sb, "%s  %d\n", previewLabelMsg.Render("Messages:"), msgCount)
	fmt.Fprintf(&sb, "%s %s\n", previewLabelDir.Render("Directory:"), workingDir)
	return sb.String()
}

// ClaudeMsgs returns the recent user messages as a styled bullet list.
// Returns empty string when the JSONL file is missing or has no user messages.
func ClaudeMsgs(sessionID, projectPath string) string {
	jsonlFile := filepath.Join(projectPath, sessionID+".jsonl")
	_, _, recentMsgs := parseJSONLPreview(jsonlFile)
	if len(recentMsgs) == 0 {
		return ""
	}
	var sb strings.Builder
	for _, msg := range recentMsgs {
		fmt.Fprintf(&sb, "%s %s\n", previewBullet.Render("•"), msg)
	}
	return sb.String()
}
```

### Step 4: Add OpencodeInfo to preview/opencode.go

Add to imports if needed: `"strings"` (already present? check — if not, add it).

Append after `opencodeDBPath` at the end of the file:

```go
// OpencodeInfo returns the session info fields from the Opencode SQLite DB
// as a styled string suitable for the info viewport section.
// Returns empty string when the DB is absent or the session is not found.
func OpencodeInfo(sessionID, directory string) string {
	dbPath := opencodeDBPath()
	if dbPath == "" {
		return ""
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return ""
	}
	defer db.Close()

	var (
		title       string
		timeUpdated sql.NullFloat64
		msgCount    int
	)
	err = db.QueryRow(`
		SELECT s.title, s.time_updated, COUNT(m.id)
		FROM session s
		LEFT JOIN message m ON s.id = m.session_id
		WHERE s.id = ?
		GROUP BY s.id
	`, sessionID).Scan(&title, &timeUpdated, &msgCount)
	if err != nil {
		return ""
	}

	timeStr := formatTimestamp(timeUpdated)

	var sb strings.Builder
	fmt.Fprintf(&sb, "%s     %s\n", previewLabelTitle.Render("Title:"), title)
	fmt.Fprintf(&sb, "%s      %s\n", previewLabelTime.Render("Time:"), timeStr)
	fmt.Fprintf(&sb, "%s  %d\n", previewLabelMsg.Render("Messages:"), msgCount)
	fmt.Fprintf(&sb, "%s %s\n", previewLabelDir.Render("Directory:"), directory)
	return sb.String()
}
```

Check if `strings` is already imported in `opencode.go` — it's not in the current file. Add it to the import block.

### Step 5: Add DirListing to preview/shared.go

Append after `listDir` at the end of the file:

```go
// DirListing returns the directory listing output as a string.
// Delegates to listDir; the caller is responsible for providing the section header.
func DirListing(dir string) string {
	var sb strings.Builder
	listDir(&sb, dir)
	return sb.String()
}
```

`strings` is not currently imported in `shared.go` — add it to the import block.

### Step 6: Run tests — verify they pass

```bash
go test ./preview/... -v -run "TestClaudeInfo|TestClaudeMsgs|TestOpencodeInfo|TestDirListing" 2>&1
```

Expected: all 6 PASS.

### Step 7: Run full preview test suite

```bash
go test ./preview/... -v 2>&1
```

Expected: all 13 PASS.

### Step 8: Commit

```bash
git add preview/claude.go preview/opencode.go preview/shared.go preview/preview_test.go
git commit -m "feat(preview): add section render functions ClaudeInfo/ClaudeMsgs/OpencodeInfo/DirListing"
```

---

## Task 2: Refactor picker/model.go to use three viewports

**Files:**
- Modify: `picker/model.go` (full structural refactor)
- Modify: `picker/model_test.go` (add updatePreviewHeights tests)

### Step 1: Write failing tests for updatePreviewHeights

Append to `picker/model_test.go`:

```go
// --- updatePreviewHeights ---

func TestUpdatePreviewHeights_NoMsgs(t *testing.T) {
	// height=30: info(6) + dir_header(2) + dir_content = 30
	// vpDir.Height = 30 - 6 - 2 = 22
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
	// height=40: info(6) + msgs_header(2) + msgs(available/3) + dir_header(2) + dir_content
	// available_after_info = 34
	// available_after_msgs_header = 32
	// msgsH = 32/3 = 10
	// available_after_msgs = 22
	// dirH = 22 - 2 = 20
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
	// pw = width*4/10 - 2 = 100*4/10 - 2 = 38
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
```

### Step 2: Run tests — verify they fail

```bash
go test ./picker/... -v -run "TestUpdatePreviewHeights" 2>&1
```

Expected: FAIL — `hasMsgs`, `vpInfo`, `vpMsgs`, `vpDir`, `updatePreviewHeights` undefined.

### Step 3: Rewrite picker/model.go

Replace the entire file with:

```go
package picker

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sahilm/fuzzy"

	"local/aps/preview"
	"local/aps/source"
)

type state int

const (
	stateList        state = iota
	stateListPreview
)

type previewFocus int

const (
	focusMsgs previewFocus = iota
	focusDir
)

// headerHeight is the number of terminal rows consumed by the search bar:
// one input line + two blank lines ("> query\n\n").
const headerHeight = 3

const minWidth, minHeight = 80, 10

// sectionHeaderLines: one title text line + one bottom-border line = 2 rows.
// infoContentLines: Title / Time / Messages / Directory = 4 rows.
// infoTotalHeight: total rows consumed by the SESSION INFO section.
const (
	sectionHeaderLines = 2
	infoContentLines   = 4
	infoTotalHeight    = sectionHeaderLines + infoContentLines // 6
)

// Model is the bubbletea model for the interactive session picker.
type Model struct {
	sessions     []source.Session
	filtered     []source.Session // subset after fuzzy filter; equals sessions when query=""
	cursor       int              // index into filtered
	query        string           // current search string
	state        state
	vpInfo       viewport.Model  // SESSION INFO section
	vpMsgs       viewport.Model  // RECENT MESSAGES section
	vpDir        viewport.Model  // DIRECTORY section
	previewFocus previewFocus    // which section receives j/k scroll
	hasMsgs      bool            // whether current session has recent messages
	search       textinput.Model
	width        int // terminal columns (from WindowSizeMsg)
	height       int // terminal rows   (from WindowSizeMsg)
	combined     bool
	chosen       *source.Session // non-nil after Enter; signals tea.Quit
}

func newModel(sessions []source.Session, combined bool) Model {
	ti := textinput.New()
	ti.Prompt = ""
	ti.CharLimit = 200

	return Model{
		sessions:     sessions,
		filtered:     sessions,
		search:       ti,
		vpInfo:       viewport.New(0, 0),
		vpMsgs:       viewport.New(0, 0),
		vpDir:        viewport.New(0, 0),
		previewFocus: focusDir,
		combined:     combined,
	}
}

func (m Model) Init() tea.Cmd {
	return m.search.Focus()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.updatePreviewHeights()
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

		case "up":
			if m.cursor > 0 {
				m.cursor--
			}
			if m.state == stateListPreview {
				m.loadPreview()
			}

		case "down":
			if m.cursor < len(m.filtered)-1 {
				m.cursor++
			}
			if m.state == stateListPreview {
				m.loadPreview()
			}

		case "k":
			if m.state == stateListPreview {
				// scroll focused section up
				switch m.previewFocus {
				case focusMsgs:
					m.vpMsgs.LineUp(1)
				case focusDir:
					m.vpDir.LineUp(1)
				}
			} else {
				if m.cursor > 0 {
					m.cursor--
				}
			}

		case "j":
			if m.state == stateListPreview {
				// scroll focused section down
				switch m.previewFocus {
				case focusMsgs:
					m.vpMsgs.LineDown(1)
				case focusDir:
					m.vpDir.LineDown(1)
				}
			} else {
				if m.cursor < len(m.filtered)-1 {
					m.cursor++
				}
			}

		case "tab":
			if m.state == stateListPreview && m.hasMsgs {
				if m.previewFocus == focusMsgs {
					m.previewFocus = focusDir
				} else {
					m.previewFocus = focusMsgs
				}
			}

		case " ":
			if m.state == stateList {
				m.state = stateListPreview
				m.loadPreview()
			} else {
				m.state = stateList
			}

		default:
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

// updatePreviewHeights recomputes all three viewport dimensions from m.width,
// m.height, and m.hasMsgs. Call after WindowSizeMsg or after loadPreview changes hasMsgs.
func (m *Model) updatePreviewHeights() {
	pw := m.width*4/10 - 2 // usable content width inside previewBorder chrome
	m.vpInfo.Width = pw
	m.vpMsgs.Width = pw
	m.vpDir.Width = pw

	m.vpInfo.Height = infoContentLines

	available := m.height - infoTotalHeight

	if m.hasMsgs {
		available -= sectionHeaderLines // account for msgs title row
		msgsH := available / 3
		if msgsH < 1 {
			msgsH = 1
		}
		m.vpMsgs.Height = msgsH
		available -= msgsH
	} else {
		m.vpMsgs.Height = 0
	}

	available -= sectionHeaderLines // account for dir title row
	if available < 1 {
		available = 1
	}
	m.vpDir.Height = available
}

// applyFilter re-computes m.filtered from m.sessions using sahilm/fuzzy.
// Performance assumption: < 5 000 sessions → no debounce needed.
//
// Unicode/CJK: sahilm/fuzzy iterates rune indices (not bytes), so CJK
// characters in titles and paths are matched correctly as individual runes.
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

// loadPreview populates the three viewports for the currently selected session.
// Safe to call when m.filtered is empty.
func (m *Model) loadPreview() {
	if len(m.filtered) == 0 {
		m.vpInfo.SetContent("No sessions.")
		m.vpMsgs.SetContent("")
		m.vpDir.SetContent("")
		m.hasMsgs = false
		m.updatePreviewHeights()
		return
	}

	s := m.filtered[m.cursor]

	if s.Client == source.ClientClaude {
		m.vpInfo.SetContent(preview.ClaudeInfo(s.ID, s.ProjectPath, s.CWD))
		msgsContent := preview.ClaudeMsgs(s.ID, s.ProjectPath)
		m.hasMsgs = msgsContent != ""
		m.vpMsgs.SetContent(msgsContent)
	} else {
		infoContent := preview.OpencodeInfo(s.ID, s.CWD)
		m.hasMsgs = false // opencode has no separate messages section
		m.vpInfo.SetContent(infoContent)
		m.vpMsgs.SetContent("")
	}

	m.vpDir.SetContent(preview.DirListing(s.CWD))

	if !m.hasMsgs {
		m.previewFocus = focusDir
	}

	m.updatePreviewHeights()
	m.vpInfo.GotoTop()
	m.vpMsgs.GotoTop()
	m.vpDir.GotoTop()
}

// visibleRange returns the [start, end) slice indices of sessions to render
// given cursor position, total count, and available row height.
// Extracted as a pure function for easier boundary-condition testing.
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
		return dirStyle.Render("No matches.")
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

func (m Model) renderRow(s source.Session, selected bool) string {
	id := s.ID
	if s.Client == source.ClientClaude && len(id) > 12 {
		id = id[:12]
	}
	tSty, dSty, prefix := titleStyle, dirStyle, "  "
	if selected {
		tSty, dSty, prefix = titleStyleSel, dirStyleSel, "▶ "
	}

	sep := sepStyle.Render("｜")
	row := timeStyle.Render(s.Time.Format("2006-01-02 15:04:05")) + sep +
		tSty.Render(s.Title) + sep +
		idStyle.Render(id) + sep +
		msgStyle.Render(fmt.Sprintf("%d", s.MsgCount))
	if m.combined {
		row += sep + srcStyle.Render(s.Client.String())
	}
	row += sep + dSty.Render(s.CWDDisplay)
	return prefix + row
}

// renderSectionPanel renders a section as: [title line + bottom border] + content.
// focused=true uses cyan (colorID) instead of dark grey (colorBorder) for the title.
func renderSectionPanel(title, content string, width int, focused bool) string {
	fg := lipgloss.Color(colorBorder)
	if focused {
		fg = lipgloss.Color(colorID)
	}
	header := lipgloss.NewStyle().
		Bold(true).
		Foreground(fg).
		BorderBottom(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderBottomForeground(fg).
		Width(width).
		Render(title)
	return lipgloss.JoinVertical(lipgloss.Top, header, content)
}

// renderPreviewPane composes the three section panels vertically.
func (m Model) renderPreviewPane() string {
	pw := m.width*4/10 - 2

	infoPanel := renderSectionPanel("SESSION INFO", m.vpInfo.View(), pw, false)

	var sections []string
	sections = append(sections, infoPanel)

	if m.hasMsgs {
		focused := m.previewFocus == focusMsgs
		msgsPanel := renderSectionPanel("RECENT MESSAGES", m.vpMsgs.View(), pw, focused)
		sections = append(sections, msgsPanel)
	}

	dirFocused := m.previewFocus == focusDir
	dirPanel := renderSectionPanel("DIRECTORY", m.vpDir.View(), pw, dirFocused)
	sections = append(sections, dirPanel)

	return lipgloss.JoinVertical(lipgloss.Top, sections...)
}

func (m Model) View() string {
	if m.width < minWidth || m.height < minHeight {
		return fmt.Sprintf("Terminal too small (need %dx%d, got %dx%d)",
			minWidth, minHeight, m.width, m.height)
	}

	header := "> " + m.search.View() + "\n\n" // headerHeight rows
	list := m.renderList()

	if m.state == stateListPreview {
		lw := m.width * 6 / 10
		pw := m.width - lw
		left := lipgloss.NewStyle().Width(lw).Render(header + list)
		right := previewBorder.Width(pw).Height(m.height).Render(m.renderPreviewPane())
		return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	}
	return header + list
}

// Run starts the interactive session picker and returns the chosen session,
// or nil if the user cancelled. combined=true shows the SRC column.
func Run(sessions []source.Session, combined bool) (*source.Session, error) {
	m := newModel(sessions, combined)
	p := tea.NewProgram(m, tea.WithAltScreen())
	final, err := p.Run()
	if err != nil {
		return nil, err
	}
	result, ok := final.(Model)
	if !ok {
		return nil, fmt.Errorf("unexpected model type")
	}
	return result.chosen, nil
}
```

### Step 4: Run updatePreviewHeights tests — verify they pass

```bash
go test ./picker/... -v -run "TestUpdatePreviewHeights" 2>&1
```

Expected: all 3 PASS.

### Step 5: Run all picker tests

```bash
go test ./picker/... -v 2>&1
```

Expected: all 13 PASS (10 existing + 3 new).

### Step 6: Full build

```bash
go build ./... 2>&1
```

Expected: EXIT 0.

### Step 7: Run all tests

```bash
go test ./... 2>&1
```

Expected: all PASS.

### Step 8: Commit

```bash
git add picker/model.go picker/model_test.go
git commit -m "feat(picker): split preview into three independent scrollable sections"
```

---

## Task 3: Clean up — remove unused ━━━ headers from preview render functions

Now that `RenderClaude` and `RenderOpencode` are no longer called from the picker (the section functions are used instead), their `━━━ HEADER ━━━` lines are still present but only affect non-picker uses. Since `RenderClaude`/`RenderOpencode` are public functions with existing tests, leave them unchanged.

**No action needed in this task** — the existing functions are tested, pass, and serve as backwards-compatible API.

---

## Self-Review

### 1. Spec coverage

| Requirement | Task |
|------------|------|
| Three independent viewports: vpInfo, vpMsgs, vpDir | Task 2 |
| Vertical layout with JoinVertical | Task 2 (renderPreviewPane) |
| Section headers without hand-drawn ━━━ | Task 2 (renderSectionPanel with BorderBottom) |
| ↑/↓ navigate list cursor | Task 2 (split from j/k) |
| j/k scroll focused section | Task 2 (new key routing) |
| Tab cycles focusMsgs ↔ focusDir | Task 2 |
| Space toggles preview as before | Task 2 (unchanged) |
| vpMsgs height=0 when no messages | Task 2 (updatePreviewHeights + hasMsgs=false) |
| Height allocation: info=6, msgs=dynamic, dir=remaining | Task 2 (updatePreviewHeights) |
| Section render functions for preview package | Task 1 |
| Tests for section functions | Task 1 |
| Tests for updatePreviewHeights | Task 2 |

No gaps.

### 2. Placeholder scan

No TBD/TODO/placeholder patterns. All code blocks are complete.

### 3. Type consistency

- `previewFocus`, `focusMsgs`, `focusDir`: defined in model.go preamble, used in Update/loadPreview/renderPreviewPane ✓
- `vpInfo`, `vpMsgs`, `vpDir`: defined in Model struct, used in newModel/updatePreviewHeights/loadPreview/renderPreviewPane ✓
- `updatePreviewHeights()`: defined as `func (m *Model)`, called from WindowSizeMsg and loadPreview ✓
- `renderSectionPanel(title, content string, width int, focused bool) string`: package-level function, called from renderPreviewPane ✓
- `ClaudeInfo`, `ClaudeMsgs`, `OpencodeInfo`, `DirListing`: defined in Task 1, called in loadPreview Task 2 ✓
- `sectionHeaderLines=2`, `infoContentLines=4`, `infoTotalHeight=6`: defined as constants in model.go, used in updatePreviewHeights and test expectations ✓
- `colorBorder`, `colorID`: defined in `picker/styles.go` as `lipgloss.Color` constants (type `lipgloss.Color`). In `renderSectionPanel`, used as `lipgloss.Color(colorBorder)` — note: since `colorBorder` is already `lipgloss.Color("8")`, the cast `lipgloss.Color(colorBorder)` is a no-op identity cast. Can simplify to just `colorBorder`. Fix: use `colorBorder` and `colorID` directly without cast ✓ (they're already the right type)
