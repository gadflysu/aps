# bubbletea Picker — Replace fzf Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the fzf subprocess picker with a self-contained bubbletea TUI, eliminating the last external runtime dependency.

**Architecture:** Add bubbletea + lipgloss + sahilm/fuzzy as pure-Go deps; delete `picker/fzf.go`, `display/color.go`, `display/columns.go`, and all fzf-specific code in `main.go` / `cmd/root.go`; implement `picker/model.go` (full TUI model) + `picker/styles.go` (lipgloss styles); rewrite `display/format.go` list-mode rendering with lipgloss; add `preview.RenderClaude` / `preview.RenderOpencode` (io.Writer variants) to support the in-process preview panel.

**Tech Stack:** `charmbracelet/bubbletea` v1, `charmbracelet/lipgloss` v1, `charmbracelet/bubbles` (viewport + textinput), `sahilm/fuzzy`, Go stdlib.

---

## File Map

| Action | File | Responsibility |
|--------|------|----------------|
| Create | `picker/styles.go` | All lipgloss style vars + color constants |
| Create | `picker/model.go` | bubbletea Model, Update, View, Run() |
| Rewrite | `display/format.go` | FormatListRow + Header using lipgloss; AdaptiveTitleWidth |
| Create | `display/format_test.go` | AdaptiveTitleWidth tests (migrated from columns_test.go) |
| Delete | `display/columns_test.go` | Tests for functions that no longer exist |
| Delete | `display/columns.go` | Width/Pad/Truncate/consts → replaced by lipgloss |
| Delete | `display/color.go` | ANSI constants → replaced by lipgloss Color() |
| Modify | `preview/shared.go` | listDir(w io.Writer, dir) |
| Modify | `preview/claude.go` | Add RenderClaude(w, id, proj, cwd); remove Claude() |
| Modify | `preview/opencode.go` | Add RenderOpencode(w, id, dir); remove Opencode() |
| Modify | `cmd/root.go` | Remove PreviewMode, PreviewArgs, --_preview-* dispatch |
| Modify | `cmd/root_test.go` | Remove TestParse_Preview* tests |
| Rewrite | `main.go` | Remove runPreview; simplify runInteractive to call picker.Run |
| Delete | `picker/fzf.go` | Entire fzf subprocess layer |
| Modify | `go.mod` | Add bubbletea, lipgloss, bubbles, sahilm/fuzzy |

---

## Task 1: Add Go Dependencies

**Files:**
- Modify: `go.mod`
- Modify: `go.sum` (auto-generated)

- [ ] **Step 1: Add dependencies**

```bash
cd /Users/dsu/projects.local/aps
go get github.com/charmbracelet/bubbletea@latest
go get github.com/charmbracelet/lipgloss@latest
go get github.com/charmbracelet/bubbles@latest
go get github.com/sahilm/fuzzy@latest
go mod tidy
```

- [ ] **Step 2: Verify build still compiles**

```bash
go build ./...
```

Expected: no errors (existing code unaffected by new deps).

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "chore: add bubbletea/lipgloss/bubbles/fuzzy dependencies"
```

---

## Task 2: Refactor Preview Package to io.Writer

**Files:**
- Modify: `preview/shared.go`
- Modify: `preview/claude.go`
- Modify: `preview/opencode.go`

`runPreview` in main.go will be removed in Task 9, so `Claude()` and `Opencode()` (stdout wrappers) are dead code after this change. We replace them with `RenderClaude` / `RenderOpencode` that write to any `io.Writer`, enabling the bubbletea preview panel to capture output into a `strings.Builder`.

- [ ] **Step 1: Rewrite preview/shared.go — listDir to io.Writer**

```go
package preview

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
)

func listDir(w io.Writer, dir string) {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		fmt.Fprintf(w, "(directory not found: %s)\n", dir)
		return
	}

	if path, err := exec.LookPath("eza"); err == nil {
		cmd := exec.Command(path,
			"-lF", "--time-style=+%Y-%m-%d %H:%M:%S",
			"--group-directories-first", "--binary",
			"--color=always", "--no-permissions", "--no-user", "-M", dir)
		if out, err := cmd.Output(); err == nil {
			fmt.Fprint(w, string(out))
			return
		}
	}

	lsPath, err := exec.LookPath("ls")
	if err != nil {
		return
	}
	if runtime.GOOS == "darwin" {
		cmd := exec.Command(lsPath, "-lF", "-D", "%Y-%m-%d %H:%M:%S",
			"-h", "--color=always", "-o", "-g", dir)
		if out, err := cmd.Output(); err == nil {
			fmt.Fprint(w, string(out))
		}
	} else {
		cmd := exec.Command(lsPath, "-lF",
			"--time-style=+%Y-%m-%d %H:%M:%S",
			"--group-directories-first", "-h",
			"--color=always", "-o", "-g", dir)
		if out, err := cmd.Output(); err == nil {
			fmt.Fprint(w, string(out))
		}
	}
}
```

- [ ] **Step 2: Rewrite preview/claude.go — RenderClaude(w io.Writer, ...)**

```go
package preview

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// RenderClaude writes a preview of a Claude Code session to w.
func RenderClaude(w io.Writer, sessionID, projectPath, workingDir string) {
	jsonlFile := filepath.Join(projectPath, sessionID+".jsonl")

	var timeStr string
	if info, err := os.Stat(jsonlFile); err == nil {
		timeStr = info.ModTime().Format("2006-01-02 15:04:05")
	}

	title, msgCount, recentMsgs := parseJSONLPreview(jsonlFile)

	fmt.Fprintf(w, "\033[1;36m━━━ SESSION INFO ━━━\033[0m\n")
	fmt.Fprintf(w, "\033[1;33mTitle:\033[0m     %s\n", title)
	fmt.Fprintf(w, "\033[1;32mTime:\033[0m      %s\n", timeStr)
	fmt.Fprintf(w, "\033[1;35mMessages:\033[0m  %d\n", msgCount)
	fmt.Fprintf(w, "\033[1;90mDirectory:\033[0m %s\n", workingDir)

	if len(recentMsgs) > 0 {
		fmt.Fprintf(w, "\033[1;36m━━━ RECENT MESSAGES ━━━\033[0m\n")
		for _, msg := range recentMsgs {
			fmt.Fprintf(w, "\033[1;90m•\033[0m %s\n", msg)
		}
	}

	fmt.Fprintf(w, "\033[1;36m━━━ DIRECTORY LIST ━━━\033[0m\n\n")
	listDir(w, workingDir)
}

var previewSkipPrefixes = []string{
	"<local-command-caveat>",
	"<command-message>",
	"<command-name>",
	"<local-command-stdout>",
	"<bash-input>",
	"<bash-stdout>",
	"<task-notification>",
	"[Request interrupted",
	"[{'type': 'tool_result'",
}

func parseJSONLPreview(path string) (title string, msgCount int, recent []string) {
	f, err := os.Open(path)
	if err != nil {
		return "Untitled", 0, nil
	}
	defer f.Close()

	var (
		lastCustomTitle string
		firstUserTitle  string
		allUserMsgs     []string
	)

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		var rec map[string]json.RawMessage
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			continue
		}

		var recType string
		if raw, ok := rec["type"]; ok {
			json.Unmarshal(raw, &recType)
		}

		switch recType {
		case "custom-title":
			if raw, ok := rec["customTitle"]; ok {
				var ct string
				if json.Unmarshal(raw, &ct) == nil && ct != "" {
					lastCustomTitle = strings.TrimSpace(ct)
				}
			}

		case "user":
			msgCount++
			text := extractUserText(rec)
			if text != "" {
				if firstUserTitle == "" {
					firstUserTitle = text
				}
				allUserMsgs = append(allUserMsgs, text)
			}
		}
	}

	if lastCustomTitle != "" {
		title = lastCustomTitle
	} else if firstUserTitle != "" {
		title = firstUserTitle
	} else {
		title = "Untitled"
	}

	if len(allUserMsgs) > 10 {
		allUserMsgs = allUserMsgs[len(allUserMsgs)-10:]
	}
	for i, j := 0, len(allUserMsgs)-1; i < j; i, j = i+1, j-1 {
		allUserMsgs[i], allUserMsgs[j] = allUserMsgs[j], allUserMsgs[i]
	}
	for _, m := range allUserMsgs {
		if len([]rune(m)) > 80 {
			m = string([]rune(m)[:80])
		}
		recent = append(recent, m)
	}

	return title, msgCount, recent
}

func extractUserText(rec map[string]json.RawMessage) string {
	msgRaw, ok := rec["message"]
	if !ok {
		return ""
	}
	var msg map[string]json.RawMessage
	if err := json.Unmarshal(msgRaw, &msg); err != nil {
		return ""
	}
	contentRaw, ok := msg["content"]
	if !ok {
		return ""
	}

	var s string
	if json.Unmarshal(contentRaw, &s) == nil {
		return filterPreviewMsg(s)
	}
	var items []map[string]json.RawMessage
	if json.Unmarshal(contentRaw, &items) == nil {
		for _, item := range items {
			var t string
			if typeRaw, ok := item["type"]; ok {
				json.Unmarshal(typeRaw, &t)
			}
			if t != "text" {
				continue
			}
			var text string
			if textRaw, ok := item["text"]; ok {
				if json.Unmarshal(textRaw, &text) == nil {
					return filterPreviewMsg(strings.TrimSpace(text))
				}
			}
		}
	}
	return ""
}

func filterPreviewMsg(s string) string {
	s = strings.TrimSpace(s)
	for _, prefix := range previewSkipPrefixes {
		if strings.HasPrefix(s, prefix) {
			return ""
		}
	}
	if idx := strings.Index(s, "\n"); idx >= 0 {
		s = s[:idx]
	}
	return strings.TrimSpace(s)
}
```

- [ ] **Step 3: Rewrite preview/opencode.go — RenderOpencode(w io.Writer, ...)**

```go
package preview

import (
	"database/sql"
	"fmt"
	"io"
	"math"
	"os"
	"time"

	_ "modernc.org/sqlite"
)

// RenderOpencode writes a preview of an Opencode session to w.
func RenderOpencode(w io.Writer, sessionID, directory string) {
	dbPath := opencodeDBPath()

	if dbPath != "" {
		printOpencodeInfo(w, dbPath, sessionID, directory)
	}

	fmt.Fprintf(w, "\033[1;36m━━━ DIRECTORY LIST ━━━\033[0m\n\n")
	listDir(w, directory)
}

func printOpencodeInfo(w io.Writer, dbPath, sessionID, directory string) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return
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
		return
	}

	timeStr := formatTimestamp(timeUpdated)

	fmt.Fprintf(w, "\033[1;36m━━━━━━━━━━━━━━━ SESSION INFO ━━━━━━━━━━━━━━━\033[0m\n")
	fmt.Fprintf(w, "\033[1;33mTitle:\033[0m     %s\n", title)
	fmt.Fprintf(w, "\033[1;32mTime:\033[0m      %s\n", timeStr)
	fmt.Fprintf(w, "\033[1;35mMessages:\033[0m  %d\n", msgCount)
	fmt.Fprintf(w, "\033[1;90mDirectory:\033[0m %s\n", directory)
	fmt.Fprintf(w, "\033[1;36m━━━━━━━━━━━━━━ DIRECTORY LIST ━━━━━━━━━━━━━━\033[0m\n\n")
}

func formatTimestamp(v sql.NullFloat64) string {
	if !v.Valid {
		return "Unknown"
	}
	ts := v.Float64
	if ts > 9_999_999_999 {
		ts /= 1000.0
	}
	sec := int64(math.Floor(ts))
	nsec := int64((ts - float64(sec)) * 1e9)
	return time.Unix(sec, nsec).Format("2006-01-02 15:04:05")
}

func opencodeDBPath() string {
	dataDir := os.Getenv("OPENCODE_DATA_DIR")
	if dataDir == "" {
		home, _ := os.UserHomeDir()
		dataDir = home + "/.local/share/opencode"
	}
	p := dataDir + "/opencode.db"
	if _, err := os.Stat(p); err == nil {
		return p
	}
	return ""
}
```

- [ ] **Step 4: Verify build**

```bash
go build ./...
```

Expected: no errors.

- [ ] **Step 5: Commit**

```bash
git add preview/shared.go preview/claude.go preview/opencode.go
git commit -m "refactor(preview): accept io.Writer instead of writing to stdout"
```

---

## Task 3: Create picker/styles.go

**Files:**
- Create: `picker/styles.go`

- [ ] **Step 1: Write picker/styles.go**

```go
package picker

import "github.com/charmbracelet/lipgloss"

// titleColWidth is the fixed title column width in TUI (interactive) mode.
// In list mode the width is adaptive; see display.AdaptiveTitleWidth.
const titleColWidth = 40

// ANSI 16-color palette — respect the user's terminal color theme.
const (
	colorTime   = lipgloss.Color("2") // ANSI green
	colorTitle  = lipgloss.Color("3") // ANSI yellow
	colorID     = lipgloss.Color("6") // ANSI cyan
	colorMsg    = lipgloss.Color("5") // ANSI magenta
	colorDir    = lipgloss.Color("8") // ANSI dark grey (normal row)
	colorDirSel = lipgloss.Color("7") // ANSI white    (selected row)
	colorBorder = lipgloss.Color("8") // ANSI dark grey
)

var (
	timeStyle  = lipgloss.NewStyle().Foreground(colorTime).Width(19)
	titleStyle = lipgloss.NewStyle().Foreground(colorTitle).
			Width(titleColWidth).MaxWidth(titleColWidth)
	idStyle  = lipgloss.NewStyle().Foreground(colorID).Width(12)
	msgStyle = lipgloss.NewStyle().Foreground(colorMsg).Width(6)
	srcStyle = lipgloss.NewStyle().Foreground(colorMsg).Width(11)
	dirStyle = lipgloss.NewStyle().Foreground(colorDir)
	sepStyle = lipgloss.NewStyle().Foreground(colorDir)

	// Selected-state variants: title bold, directory brightens to white.
	titleStyleSel = titleStyle.Bold(true)
	dirStyleSel   = lipgloss.NewStyle().Foreground(colorDirSel)

	// previewBorder adds BorderLeft(1) + PaddingLeft(1) = 2 cols of chrome.
	// Viewport content width = panel width - 2.
	previewBorder = lipgloss.NewStyle().
			BorderLeft(true).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(colorBorder).
			PaddingLeft(1)
)
```

- [ ] **Step 2: Verify compilation**

```bash
go build ./picker/...
```

Expected: no errors (styles.go compiles; model.go doesn't exist yet so picker package is incomplete — this is expected at this stage if Go allows partial packages. If it errors, proceed to Task 4 immediately).

- [ ] **Step 3: Commit**

```bash
git add picker/styles.go
git commit -m "feat(picker): add lipgloss style definitions"
```

---

## Task 4: Create picker/model.go

**Files:**
- Create: `picker/model.go`

This file replaces `picker/fzf.go` entirely. The `Run()` function signature changes from `Run(lines []string, cfg Config) (string, error)` to `Run(sessions []source.Session, combined bool) (*source.Session, error)`.

- [ ] **Step 1: Write picker/model.go**

```go
package picker

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/sahilm/fuzzy"

	"local/aps/preview"
	"local/aps/source"
)

type state int

const (
	stateList        state = iota
	stateListPreview
)

// headerHeight is the number of terminal rows consumed by the search bar:
// one input line + two blank lines ("> query\n\n").
const headerHeight = 3

const minWidth, minHeight = 80, 10

// Model is the bubbletea model for the interactive session picker.
type Model struct {
	sessions []source.Session
	filtered []source.Session // subset after fuzzy filter; equals sessions when query=""
	cursor   int              // index into filtered
	query    string           // current search string
	state    state
	preview  viewport.Model
	search   textinput.Model
	width    int // terminal columns (from WindowSizeMsg)
	height   int // terminal rows   (from WindowSizeMsg)
	combined bool
	chosen   *source.Session // non-nil after Enter; signals tea.Quit
}

func newModel(sessions []source.Session, combined bool) Model {
	ti := textinput.New()
	ti.Focus()
	ti.Prompt = ""
	ti.CharLimit = 200

	return Model{
		sessions: sessions,
		filtered: sessions,
		search:   ti,
		combined: combined,
	}
}

func (m Model) Init() tea.Cmd {
	return textinput.Blink
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		// Preview column allocation: m.width * 4/10 total.
		// previewBorder adds BorderLeft(1) + PaddingLeft(1) = 2 cols of chrome.
		// Viewport content width must be 2 less so the styled output fills
		// exactly m.width*4/10 columns.
		m.preview.Width = msg.Width*4/10 - 2
		// No top/bottom border in previewBorder, so no vertical chrome.
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
		left := lipglossWidth(lw).Render(header + list)
		right := previewBorder.Width(pw).Height(m.height).Render(m.preview.View())
		return lipglossJoinH(left, right)
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

- [ ] **Step 2: Add lipgloss layout helpers at the bottom of model.go**

Append these two small helpers to `picker/model.go` (after the `Run` function). They wrap lipgloss functions with short local names to keep `View()` readable without an extra import alias:

```go
// lipglossWidth returns a style with a fixed width — used for left-panel sizing.
func lipglossWidth(w int) lipgloss.Style {
	return lipgloss.NewStyle().Width(w)
}

// lipglossJoinH joins two strings side-by-side (bubbletea top-aligned).
func lipglossJoinH(left, right string) string {
	return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
}
```

Also add `"github.com/charmbracelet/lipgloss"` to the import block in `model.go`.

- [ ] **Step 3: Verify compilation**

```bash
go build ./picker/...
```

Expected: no errors.

- [ ] **Step 4: Write visibleRange unit test**

Create `picker/model_test.go`:

```go
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
```

- [ ] **Step 5: Run the new tests**

```bash
go test ./picker/... -v -run TestVisibleRange
```

Expected: all 5 tests PASS.

- [ ] **Step 6: Commit**

```bash
git add picker/model.go picker/model_test.go
git commit -m "feat(picker): implement bubbletea TUI picker model"
```

---

## Task 5: Rewrite display/format.go with lipgloss

**Files:**
- Rewrite: `display/format.go`

Remove all fzf TAB-field functions (`FormatInteractiveClaude`, `FormatInteractiveOpencode`, `FormatInteractiveAll`) and internal ANSI helpers. Rewrite `FormatListRow` and `Header` using lipgloss. Move `AdaptiveTitleWidth` and column constants here (removing dependency on `columns.go`).

- [ ] **Step 1: Overwrite display/format.go**

```go
package display

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"local/aps/source"
)

// Column widths (display columns, not bytes).
const (
	MaxTitleLimit   = 40 // cap for adaptive title width
	colTime         = 19 // fixed: "2006-01-02 15:04:05"
	colMsgCount     = 6
	colSrcWidth     = 11 // len("Claude Code")
	colIDClaudeFull = 36 // full UUID, list mode
	colIDOpencode   = 30
	colSep          = "｜" // U+FF5C FULLWIDTH VERTICAL LINE
)

// List-mode lipgloss styles (directory = white, matching original ColorWhite).
var (
	listTimeStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Width(colTime)
	listTitleStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	listIDStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	listMsgStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("5")).Width(colMsgCount)
	listSrcStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("5")).Width(colSrcWidth)
	listDirStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("7")) // white for list
	listSepStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	listHeaderStyle = lipgloss.NewStyle().Underline(true).Foreground(lipgloss.Color("7"))
)

// AdaptiveTitleWidth returns min(MaxTitleLimit, maxActualDisplayWidth) across titles.
func AdaptiveTitleWidth(titles []string) int {
	max := 0
	for _, t := range titles {
		if w := lipgloss.Width(t); w > max {
			max = w
		}
	}
	if max > MaxTitleLimit {
		return MaxTitleLimit
	}
	return max
}

// FormatListRow formats a session for plain list output (no hidden TAB fields).
func FormatListRow(s source.Session, titleWidth int, includeSource bool) string {
	idW := listIDColWidth(s)
	sep := listSepStyle.Render(colSep)

	row := listTimeStyle.Render(formatTime(s.Time)) + sep +
		listTitleStyle.Copy().Width(titleWidth).MaxWidth(titleWidth).Render(sanitize(s.Title)) + sep +
		listIDStyle.Copy().Width(idW).Render(sanitize(s.ID)) + sep +
		listMsgStyle.Render(fmt.Sprintf("%d", s.MsgCount))
	if includeSource {
		row += sep + listSrcStyle.Render(s.Client.String())
	}
	row += sep + listDirStyle.Render(sanitize(s.CWDDisplay))
	return row
}

// Header returns a formatted header row for list mode.
func Header(titleWidth int, includeSource bool) string {
	sep := listSepStyle.Render(colSep)
	h := listHeaderStyle

	row := h.Copy().Width(colTime).Render("TIME") + sep +
		h.Copy().Width(titleWidth).Render("TITLE") + sep +
		h.Copy().Width(colIDClaudeFull).Render("ID") + sep +
		h.Copy().Width(colMsgCount).Render("MSG")
	if includeSource {
		row += sep + h.Copy().Width(colSrcWidth).Render("SRC")
	}
	row += sep + h.Render("DIRECTORY")
	return row
}

// --- internal helpers ---

func listIDColWidth(s source.Session) int {
	if s.Client == source.ClientClaude {
		return colIDClaudeFull
	}
	return colIDOpencode
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return "No time"
	}
	return t.Format("2006-01-02 15:04:05")
}

func sanitize(s string) string {
	s = strings.ReplaceAll(s, "\t", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}
```

- [ ] **Step 2: Verify compilation (display/ still has columns.go; that is fine at this stage)**

```bash
go build ./display/...
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add display/format.go
git commit -m "refactor(display): rewrite FormatListRow and Header with lipgloss"
```

---

## Task 6: Migrate display/columns_test.go → display/format_test.go

**Files:**
- Create: `display/format_test.go`
- Delete: `display/columns_test.go`

The `Width`, `Pad`, `Truncate` functions are being deleted (their logic moves inside lipgloss). Testing lipgloss internals is not our job. `AdaptiveTitleWidth` moves to `format.go` and its tests migrate to `format_test.go`.

- [ ] **Step 1: Create display/format_test.go**

```go
package display

import (
	"strings"
	"testing"
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
```

- [ ] **Step 2: Run the new test to confirm it passes**

```bash
go test ./display/... -v -run TestAdaptiveTitleWidth
```

Expected: 5 tests PASS (columns_test.go still present but tests dead symbols — it will fail; that's OK here, proceed to Step 3 to delete it).

- [ ] **Step 3: Delete display/columns_test.go**

```bash
rm display/columns_test.go
```

- [ ] **Step 4: Run display tests**

```bash
go test ./display/... -v
```

Expected: `format_test.go` tests PASS.

- [ ] **Step 5: Commit**

```bash
git add display/format_test.go
git rm display/columns_test.go
git commit -m "test(display): migrate AdaptiveTitleWidth tests to format_test.go"
```

---

## Task 7: Delete display/columns.go and display/color.go

**Files:**
- Delete: `display/columns.go`
- Delete: `display/color.go`

After this, any file importing `display.Width`, `display.Pad`, `display.Truncate`, `display.Color*`, `display.Col*` will fail to compile. The only remaining callers are `display/format.go` (already rewritten) and `picker/fzf.go` (will be deleted in Task 10 — it's OK to have a broken build there temporarily; we'll fix it in sequence).

- [ ] **Step 1: Delete the files**

```bash
rm display/columns.go display/color.go
```

- [ ] **Step 2: Attempt build to discover any lingering imports**

```bash
go build ./display/...
```

Expected: PASS (format.go no longer imports columns.go symbols).

```bash
go build ./...
```

Expected: FAIL on `picker/fzf.go` and `main.go` references — that is expected and will be fixed in Tasks 9–10.

- [ ] **Step 3: Run display tests**

```bash
go test ./display/... -v
```

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git rm display/columns.go display/color.go
git commit -m "refactor(display): delete columns.go and color.go (replaced by lipgloss)"
```

---

## Task 8: Clean Up cmd/root.go — Remove Preview Subcommands

**Files:**
- Modify: `cmd/root.go`
- Modify: `cmd/root_test.go`

`--_preview-*` were internal fzf subcommands. With bubbletea there are no subprocess forks, so these are dead code.

- [ ] **Step 1: Rewrite cmd/root.go — remove PreviewMode, PreviewArgs, --_preview-* dispatch**

```go
package cmd

import (
	"flag"
	"fmt"
	"os"
)

// Config holds all parsed CLI state.
type Config struct {
	NoLaunch   bool
	Verbose    bool
	ListOnly   bool
	Claude     bool
	Opencode   bool
	All        bool
	DangerMode bool
	Recursive  bool
	PathFilter string
}

func Parse(args []string) Config {
	fs := flag.NewFlagSet("aps", flag.ExitOnError)
	fs.Usage = usage

	var cfg Config
	var showHelp bool

	fs.BoolVar(&cfg.NoLaunch, "n", false, "")
	fs.BoolVar(&cfg.NoLaunch, "no-launch", false, "")
	fs.BoolVar(&cfg.Verbose, "v", false, "")
	fs.BoolVar(&cfg.Verbose, "verbose", false, "")
	fs.BoolVar(&cfg.ListOnly, "l", false, "")
	fs.BoolVar(&cfg.ListOnly, "list", false, "")
	fs.BoolVar(&cfg.Claude, "c", false, "")
	fs.BoolVar(&cfg.Claude, "claude", false, "")
	fs.BoolVar(&cfg.Opencode, "o", false, "")
	fs.BoolVar(&cfg.Opencode, "opencode", false, "")
	fs.BoolVar(&cfg.All, "a", false, "")
	fs.BoolVar(&cfg.All, "all", false, "")
	fs.BoolVar(&cfg.DangerMode, "d", false, "")
	fs.BoolVar(&cfg.DangerMode, "danger", false, "")
	fs.BoolVar(&cfg.Recursive, "r", false, "")
	fs.BoolVar(&cfg.Recursive, "recursive", false, "")
	fs.BoolVar(&showHelp, "h", false, "")
	fs.BoolVar(&showHelp, "help", false, "")

	expanded := expandShortFlags(args)
	_ = fs.Parse(expanded)

	if showHelp {
		usage()
		os.Exit(0)
	}

	if !cfg.Claude && !cfg.Opencode && !cfg.All {
		cfg.Claude = true
	}
	if cfg.All {
		cfg.Claude = true
		cfg.Opencode = true
	}

	if fs.NArg() > 0 {
		cfg.PathFilter = fs.Arg(0)
	}

	if cfg.PathFilter == "." {
		if cwd, err := os.Getwd(); err == nil {
			cfg.PathFilter = cwd
		}
	}

	return cfg
}

// expandShortFlags splits combined short flags like -nv into -n -v.
func expandShortFlags(args []string) []string {
	var out []string
	for _, a := range args {
		if len(a) > 2 && a[0] == '-' && a[1] != '-' {
			for _, c := range a[1:] {
				out = append(out, "-"+string(c))
			}
		} else {
			out = append(out, a)
		}
	}
	return out
}

func usage() {
	fmt.Fprintf(os.Stderr, `Usage: aps [OPTIONS] [PATH_FILTER]

Interactive session picker for Claude Code and Opencode.

Options:
  -n, --no-launch    Print target directory instead of launching client
  -v, --verbose      With -n: print full launch command
  -l, --list         Non-interactive table output and exit
  -c, --claude       Include Claude Code sessions (default if no client flag)
  -o, --opencode     Include Opencode sessions
  -a, --all          Include both clients
  -d, --danger       Claude: launch with --dangerously-skip-permissions
  -r, --recursive    Looser path filter (substring match)
  -h, --help         Show this help

Arguments:
  PATH_FILTER        Filter sessions by directory path. Use '.' for cwd.

Examples:
  aps               Interactive pick (Claude sessions, cwd filter default)
  aps -l .          List mode, current directory
  aps -l scripts    List mode, substring filter
  aps -r -l foo     Recursive substring match
  aps -c            Claude Code only
  aps -o            Opencode only
  aps -a            Both clients combined
  aps -n            No-launch: print target directory
  aps -nv           No-launch verbose: print full command
  aps -d            Danger mode (--dangerously-skip-permissions)
`)
}
```

- [ ] **Step 2: Remove preview tests from cmd/root_test.go**

Delete the three test functions `TestParse_PreviewClaude`, `TestParse_PreviewOpencode`, `TestParse_PreviewAll` (lines 163–188 in the original file). All other tests remain unchanged.

The file after deletion ends at `TestParse_CombinedFlags` (line ~162):

```go
func TestParse_CombinedFlags(t *testing.T) {
	cfg := Parse([]string{"-nv"})
	if !cfg.NoLaunch {
		t.Error("-nv should set NoLaunch=true")
	}
	if !cfg.Verbose {
		t.Error("-nv should set Verbose=true")
	}
}
```

- [ ] **Step 3: Run cmd tests**

```bash
go test ./cmd/... -v
```

Expected: all tests PASS (preview tests removed, remaining tests unaffected).

- [ ] **Step 4: Commit**

```bash
git add cmd/root.go cmd/root_test.go
git commit -m "refactor(cmd): remove --_preview-* subcommand dispatch (fzf-only feature)"
```

---

## Task 9: Rewrite main.go — Remove runPreview, Simplify runInteractive

**Files:**
- Rewrite: `main.go`

`runInteractive` currently builds fzf TAB-field lines, calls `picker.Run(lines, fzfCfg)`, then parses the TAB fields back out to get id/cwd. With bubbletea, `picker.Run` returns `*source.Session` directly — no line building, no field parsing.

- [ ] **Step 1: Overwrite main.go**

```go
package main

import (
	"fmt"
	"os"
	"sort"

	"local/aps/cmd"
	"local/aps/display"
	"local/aps/launcher"
	"local/aps/picker"
	"local/aps/source"
)

func main() {
	cfg := cmd.Parse(os.Args[1:])

	sessions, err := loadSessions(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading sessions: %v\n", err)
		os.Exit(1)
	}
	if len(sessions) == 0 {
		fmt.Fprintln(os.Stderr, "No sessions found.")
		os.Exit(0)
	}

	if cfg.ListOnly {
		runList(sessions, cfg)
		return
	}

	runInteractive(sessions, cfg)
}

func loadSessions(cfg cmd.Config) ([]source.Session, error) {
	strictMatch := !cfg.Recursive
	var all []source.Session

	if cfg.Claude {
		sessions, err := source.LoadClaude(cfg.PathFilter, strictMatch, cfg.Verbose)
		if err != nil && cfg.Verbose {
			fmt.Fprintf(os.Stderr, "claude: %v\n", err)
		}
		all = append(all, sessions...)
	}

	if cfg.Opencode {
		sessions, err := source.LoadOpencode(cfg.PathFilter, strictMatch, cfg.Verbose)
		if err != nil && cfg.Verbose {
			fmt.Fprintf(os.Stderr, "opencode: %v\n", err)
		}
		all = append(all, sessions...)
	}

	sort.Slice(all, func(i, j int) bool {
		return all[i].Time.After(all[j].Time)
	})

	return all, nil
}

func runList(sessions []source.Session, cfg cmd.Config) {
	combined := cfg.Claude && cfg.Opencode

	titles := make([]string, len(sessions))
	for i, s := range sessions {
		titles[i] = s.Title
	}
	titleWidth := display.AdaptiveTitleWidth(titles)

	fmt.Println(display.Header(titleWidth, combined))
	for _, s := range sessions {
		fmt.Println(display.FormatListRow(s, titleWidth, combined))
	}
}

func runInteractive(sessions []source.Session, cfg cmd.Config) {
	combined := cfg.Claude && cfg.Opencode

	session, err := picker.Run(sessions, combined)
	if err != nil {
		fmt.Fprintf(os.Stderr, "picker error: %v\n", err)
		os.Exit(1)
	}
	if session == nil {
		os.Exit(0) // user cancelled
	}

	if !dirExists(session.CWD) {
		fmt.Fprintf(os.Stderr, "Error: directory not found: %s\n", session.CWD)
		os.Exit(1)
	}

	launchOpts := launcher.Options{
		NoLaunch:   cfg.NoLaunch,
		Verbose:    cfg.Verbose,
		DangerMode: cfg.DangerMode,
	}

	switch session.Client {
	case source.ClientClaude:
		mustLaunch(launcher.Claude(session.ID, session.CWD, launchOpts))
	default:
		mustLaunch(launcher.Opencode(session.ID, session.CWD, launchOpts))
	}
}

func mustLaunch(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "launch error: %v\n", err)
		os.Exit(1)
	}
}

func dirExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}
```

- [ ] **Step 2: Verify build**

```bash
go build ./...
```

Expected: FAIL on `picker/fzf.go` references (DetectCapabilities, Config) no longer called — that is fine; fzf.go itself still compiles, it's just the references in main.go that are gone. Actually this should PASS since fzf.go is still present and compilable on its own, and main.go no longer imports its types. Verify:

```bash
go build .
```

Expected: PASS.

- [ ] **Step 3: Run all tests**

```bash
go test ./...
```

Expected: all tests PASS.

- [ ] **Step 4: Commit**

```bash
git add main.go
git commit -m "refactor(main): replace fzf picker with bubbletea picker.Run"
```

---

## Task 10: Delete picker/fzf.go and Final Verification

**Files:**
- Delete: `picker/fzf.go`

`picker/fzf.go` exports `DetectCapabilities`, `Capabilities`, `Config`, `Run`, `Parse`. After Task 9, none of these are imported by main.go or anywhere else.

- [ ] **Step 1: Verify fzf.go is no longer imported**

```bash
grep -r "picker\.DetectCapabilities\|picker\.Config\|picker\.Parse\|fzf" --include="*.go" . | grep -v "_test.go" | grep -v "picker/fzf.go"
```

Expected: no output (no other file imports fzf-specific symbols).

- [ ] **Step 2: Delete picker/fzf.go**

```bash
rm picker/fzf.go
```

- [ ] **Step 3: Full build**

```bash
go build ./...
```

Expected: PASS.

- [ ] **Step 4: Full test suite**

```bash
go test ./... -v
```

Expected: all tests PASS. Verify test count includes: `cmd` (expandShortFlags + Parse tests), `display` (AdaptiveTitleWidth tests), `filter` (path filter tests), `source` (claude + helpers tests), `picker` (visibleRange tests).

- [ ] **Step 5: Commit**

```bash
git rm picker/fzf.go
git commit -m "feat(picker): delete fzf.go — bubbletea picker complete, zero external deps"
```

---

## Self-Review Against Spec

Checked against `docs/superpowers/specs/2026-04-14-bubbletea-picker-design.md`:

| Spec Requirement | Covered by Task |
|------------------|-----------------|
| Delete picker/fzf.go | Task 10 |
| Delete display/color.go | Task 7 |
| Delete display/columns.go | Task 7 |
| Delete FormatInteractive* | Task 5 (rewrite removes them) |
| Delete --_preview-* handling | Task 8 |
| New picker/styles.go with named color constants | Task 3 |
| New picker/model.go with full bubbletea Model | Task 4 |
| FormatListRow + Header rewritten with lipgloss | Task 5 |
| AdaptiveTitleWidth: application-layer calculation | Task 5 (stays in display/) |
| preview.RenderClaude / RenderOpencode (io.Writer) | Task 2 |
| visibleRange pure function for testability | Task 4 |
| headerHeight constant (no magic 3) | Task 4 |
| Interaction: search, Space=preview, j/k/arrows, Enter, q/Esc | Task 4 |
| Min terminal size check 80×10 | Task 4 |
| go.mod: add new deps | Task 1 |
| All existing tests pass | Tasks 4, 6, 8, 10 |
| Combined mode shows SRC column | Task 4 renderRow |
| Preview panel 60/40 split | Task 4 View() |

No gaps found.
