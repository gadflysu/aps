# Preview Package Lipgloss Rewrite — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace all raw `\033[...m` ANSI escape sequences in the `preview` package with lipgloss style renders, eliminating the last hand-coded ANSI strings in the codebase.

**Architecture:** Add `preview/styles.go` with package-level lipgloss style vars; rewrite the 8 `fmt.Fprintf` calls in `claude.go` and the 7 in `opencode.go` that embed escape codes inline; fix the single error message in `shared.go`. The `listDir()` subprocess output (eza/ls `--color=always`) is forwarded as-is and is intentionally excluded. Function signatures are unchanged.

**Tech Stack:** `charmbracelet/lipgloss v1.1.0` (already in go.mod), Go stdlib.

---

## Context: What Changes and Why

### Current raw ANSI patterns → lipgloss equivalents

| Raw escape | Visible effect | New style var |
|-----------|---------------|---------------|
| `\033[1;36m` | Bold cyan | `previewHeader` |
| `\033[1;33m` | Bold yellow | `previewLabelTitle` |
| `\033[1;32m` | Bold green | `previewLabelTime` |
| `\033[1;35m` | Bold magenta | `previewLabelMsg` |
| `\033[1;90m` | Bold dark grey | `previewLabelDir`, `previewBullet` |
| `\033[0m` | Reset | handled by lipgloss automatically |

### Exact lines being replaced

**preview/claude.go** (8 lines in `RenderClaude`):
```go
// BEFORE — lines 24–37
fmt.Fprintf(w, "\033[1;36m━━━ SESSION INFO ━━━\033[0m\n")
fmt.Fprintf(w, "\033[1;33mTitle:\033[0m     %s\n", title)
fmt.Fprintf(w, "\033[1;32mTime:\033[0m      %s\n", timeStr)
fmt.Fprintf(w, "\033[1;35mMessages:\033[0m  %d\n", msgCount)
fmt.Fprintf(w, "\033[1;90mDirectory:\033[0m %s\n", workingDir)
fmt.Fprintf(w, "\033[1;36m━━━ RECENT MESSAGES ━━━\033[0m\n")   // inside if block
fmt.Fprintf(w, "\033[1;90m•\033[0m %s\n", msg)                  // inside range loop
fmt.Fprintf(w, "\033[1;36m━━━ DIRECTORY LIST ━━━\033[0m\n\n")
```

**preview/opencode.go** (6 lines in `printOpencodeInfo` + 1 in `RenderOpencode`):
```go
// BEFORE — lines 22, 51–56
fmt.Fprintf(w, "\033[1;36m━━━ DIRECTORY LIST ━━━\033[0m\n\n")              // RenderOpencode
fmt.Fprintf(w, "\033[1;36m━━━━━━━━━━━━━━━ SESSION INFO ━━━━━━━━━━━━━━━\033[0m\n")  // printOpencodeInfo
fmt.Fprintf(w, "\033[1;33mTitle:\033[0m     %s\n", title)
fmt.Fprintf(w, "\033[1;32mTime:\033[0m      %s\n", timeStr)
fmt.Fprintf(w, "\033[1;35mMessages:\033[0m  %d\n", msgCount)
fmt.Fprintf(w, "\033[1;90mDirectory:\033[0m %s\n", directory)
fmt.Fprintf(w, "\033[1;36m━━━━━━━━━━━━━━ DIRECTORY LIST ━━━━━━━━━━━━━━\033[0m\n\n")
```

**preview/shared.go** (1 line in `listDir`):
```go
// BEFORE — line 10
fmt.Fprintf(w, "(directory not found: %s)\n", dir)
```

---

## File Map

| Action | File | Responsibility |
|--------|------|----------------|
| Create | `preview/styles.go` | All lipgloss style vars for preview rendering |
| Modify | `preview/claude.go` | Replace 8 raw ANSI calls with lipgloss renders |
| Modify | `preview/opencode.go` | Replace 7 raw ANSI calls with lipgloss renders |
| Modify | `preview/shared.go` | Replace 1 error message with lipgloss render |
| Create | `preview/preview_test.go` | Regression tests: visible text preserved after rewrite |

---

## Task 1: Create preview/styles.go

**Files:**
- Create: `preview/styles.go`

- [ ] **Step 1: Write preview/styles.go**

```go
package preview

import "github.com/charmbracelet/lipgloss"

// ANSI 16-color palette — matches picker/styles.go for consistent theming.
var (
	// previewHeader styles section dividers, e.g. "━━━ SESSION INFO ━━━".
	previewHeader = lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Bold(true)

	// previewLabel* styles each field name.
	previewLabelTitle = lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Bold(true) // yellow
	previewLabelTime  = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Bold(true) // green
	previewLabelMsg   = lipgloss.NewStyle().Foreground(lipgloss.Color("5")).Bold(true) // magenta
	previewLabelDir   = lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Bold(true) // dark grey

	// previewBullet styles the "•" preceding each recent message.
	previewBullet = lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Bold(true)

	// previewMissing styles the "directory not found" error in listDir.
	previewMissing = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
)
```

- [ ] **Step 2: Verify styles.go compiles**

```bash
go build ./preview/...
```

Expected: `EXIT: 0` (no errors; existing files unchanged, new styles file added).

- [ ] **Step 3: Commit**

```bash
git add preview/styles.go
git commit -m "feat(preview): add lipgloss style definitions"
```

---

## Task 2: Write Regression Tests

**Files:**
- Create: `preview/preview_test.go`

These tests establish a visible-text baseline. They pass with the *current* implementation and must continue to pass after the rewrite — any failure after Task 3/4/5 indicates a regression.

`stripANSI` removes ANSI sequences so assertions work regardless of terminal color support.

- [ ] **Step 1: Write preview/preview_test.go**

```go
package preview

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stripANSI removes ANSI escape sequences so plain text can be asserted.
func stripANSI(s string) string {
	var out strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '\033' && i+1 < len(s) && s[i+1] == '[' {
			for i < len(s) && s[i] != 'm' {
				i++
			}
			i++ // skip 'm'
		} else {
			out.WriteByte(s[i])
			i++
		}
	}
	return out.String()
}

// writeJSONL creates a minimal JSONL session file for testing.
func writeJSONL(t *testing.T, dir, sessionID, userMsg string) {
	t.Helper()
	line := `{"type":"user","message":{"content":"` + userMsg + `"}}` + "\n"
	p := filepath.Join(dir, sessionID+".jsonl")
	if err := os.WriteFile(p, []byte(line), 0600); err != nil {
		t.Fatal(err)
	}
}

// --- RenderClaude ---

func TestRenderClaude_SectionHeaders(t *testing.T) {
	dir := t.TempDir()
	writeJSONL(t, dir, "ses1", "hello world")

	var buf bytes.Buffer
	RenderClaude(&buf, "ses1", dir, "/work/dir")
	plain := stripANSI(buf.String())

	for _, want := range []string{"SESSION INFO", "DIRECTORY LIST"} {
		if !strings.Contains(plain, want) {
			t.Errorf("output missing section header %q\noutput:\n%s", want, plain)
		}
	}
}

func TestRenderClaude_FieldLabels(t *testing.T) {
	dir := t.TempDir()
	writeJSONL(t, dir, "ses2", "test message")

	var buf bytes.Buffer
	RenderClaude(&buf, "ses2", dir, "/some/path")
	plain := stripANSI(buf.String())

	for _, want := range []string{"Title:", "Time:", "Messages:", "Directory:"} {
		if !strings.Contains(plain, want) {
			t.Errorf("output missing field label %q\noutput:\n%s", want, plain)
		}
	}
}

func TestRenderClaude_WorkingDirInOutput(t *testing.T) {
	dir := t.TempDir()
	writeJSONL(t, dir, "ses3", "test")

	var buf bytes.Buffer
	RenderClaude(&buf, "ses3", dir, "/expected/workdir")
	plain := stripANSI(buf.String())

	if !strings.Contains(plain, "/expected/workdir") {
		t.Errorf("output missing working directory\noutput:\n%s", plain)
	}
}

func TestRenderClaude_RecentMessagesSection(t *testing.T) {
	dir := t.TempDir()
	writeJSONL(t, dir, "ses4", "recent message content")

	var buf bytes.Buffer
	RenderClaude(&buf, "ses4", dir, "/tmp")
	plain := stripANSI(buf.String())

	if !strings.Contains(plain, "RECENT MESSAGES") {
		t.Errorf("output missing RECENT MESSAGES section\noutput:\n%s", plain)
	}
	if !strings.Contains(plain, "recent message content") {
		t.Errorf("output missing message text\noutput:\n%s", plain)
	}
}

func TestRenderClaude_MissingJSONL_NoSessionInfo(t *testing.T) {
	// When JSONL file doesn't exist, SESSION INFO block still renders (with "Untitled").
	dir := t.TempDir()

	var buf bytes.Buffer
	RenderClaude(&buf, "nonexistent", dir, "/tmp")
	plain := stripANSI(buf.String())

	if !strings.Contains(plain, "SESSION INFO") {
		t.Errorf("output missing SESSION INFO even for missing JSONL\noutput:\n%s", plain)
	}
	if !strings.Contains(plain, "Untitled") {
		t.Errorf("output missing Untitled fallback title\noutput:\n%s", plain)
	}
}

// --- RenderOpencode ---

func TestRenderOpencode_NoDB_WritesDirectoryListHeader(t *testing.T) {
	// With no opencode DB, should still write the DIRECTORY LIST header.
	t.Setenv("OPENCODE_DATA_DIR", t.TempDir()) // empty dir — no opencode.db

	var buf bytes.Buffer
	RenderOpencode(&buf, "any-id", t.TempDir())
	plain := stripANSI(buf.String())

	if !strings.Contains(plain, "DIRECTORY LIST") {
		t.Errorf("expected DIRECTORY LIST header even without DB\noutput:\n%s", plain)
	}
}

// --- listDir ---

func TestListDir_NonExistentDir_WritesErrorMessage(t *testing.T) {
	var buf bytes.Buffer
	listDir(&buf, "/this/path/does/not/exist/ever")
	plain := stripANSI(buf.String())

	if !strings.Contains(plain, "directory not found") {
		t.Errorf("expected 'directory not found' message\noutput:\n%s", plain)
	}
}
```

- [ ] **Step 2: Run tests — verify all pass against current code**

```bash
go test ./preview/... -v -run "TestRenderClaude|TestRenderOpencode|TestListDir"
```

Expected: all PASS (tests verify visible content which both old and new code produce).

- [ ] **Step 3: Commit**

```bash
git add preview/preview_test.go
git commit -m "test(preview): add regression tests for RenderClaude/RenderOpencode/listDir"
```

---

## Task 3: Rewrite preview/claude.go

**Files:**
- Modify: `preview/claude.go:24–38`

Replace the 8 raw ANSI `fmt.Fprintf` calls in `RenderClaude` with lipgloss renders. All other code in the file (JSONL parsing, `extractUserText`, `filterPreviewMsg`) is unchanged.

- [ ] **Step 1: Replace ANSI calls in RenderClaude**

The complete new body of `RenderClaude` (lines 14–39 in the original, replacing only the Fprintf calls):

```go
// RenderClaude writes a preview of a Claude Code session to w.
func RenderClaude(w io.Writer, sessionID, projectPath, workingDir string) {
	jsonlFile := filepath.Join(projectPath, sessionID+".jsonl")

	var timeStr string
	if info, err := os.Stat(jsonlFile); err == nil {
		timeStr = info.ModTime().Format("2006-01-02 15:04:05")
	}

	title, msgCount, recentMsgs := parseJSONLPreview(jsonlFile)

	fmt.Fprintf(w, "%s\n", previewHeader.Render("━━━ SESSION INFO ━━━"))
	fmt.Fprintf(w, "%s     %s\n", previewLabelTitle.Render("Title:"), title)
	fmt.Fprintf(w, "%s      %s\n", previewLabelTime.Render("Time:"), timeStr)
	fmt.Fprintf(w, "%s  %d\n", previewLabelMsg.Render("Messages:"), msgCount)
	fmt.Fprintf(w, "%s %s\n", previewLabelDir.Render("Directory:"), workingDir)

	if len(recentMsgs) > 0 {
		fmt.Fprintf(w, "%s\n", previewHeader.Render("━━━ RECENT MESSAGES ━━━"))
		for _, msg := range recentMsgs {
			fmt.Fprintf(w, "%s %s\n", previewBullet.Render("•"), msg)
		}
	}

	fmt.Fprintf(w, "%s\n\n", previewHeader.Render("━━━ DIRECTORY LIST ━━━"))
	listDir(w, workingDir)
}
```

Note on spacing: the spaces between label and value are plain-text padding that matches the original layout:
- `"Title:"` → 5 spaces → value
- `"Time:"` → 6 spaces → value
- `"Messages:"` → 2 spaces → value
- `"Directory:"` → 1 space → value

- [ ] **Step 2: Build**

```bash
go build ./preview/...
```

Expected: `EXIT: 0`.

- [ ] **Step 3: Run regression tests**

```bash
go test ./preview/... -v -run "TestRenderClaude"
```

Expected: all PASS.

- [ ] **Step 4: Commit**

```bash
git add preview/claude.go
git commit -m "refactor(preview): replace raw ANSI in RenderClaude with lipgloss"
```

---

## Task 4: Rewrite preview/opencode.go

**Files:**
- Modify: `preview/opencode.go:22` and `preview/opencode.go:51–56`

Replace ANSI calls in `RenderOpencode` (1 call) and `printOpencodeInfo` (6 calls).

- [ ] **Step 1: Replace ANSI calls in RenderOpencode and printOpencodeInfo**

The complete new bodies of both functions:

```go
// RenderOpencode writes a preview of an Opencode session to w.
func RenderOpencode(w io.Writer, sessionID, directory string) {
	dbPath := opencodeDBPath()

	if dbPath != "" {
		printOpencodeInfo(w, dbPath, sessionID, directory)
	}

	fmt.Fprintf(w, "%s\n\n", previewHeader.Render("━━━ DIRECTORY LIST ━━━"))
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

	fmt.Fprintf(w, "%s\n", previewHeader.Render("━━━━━━━━━━━━━━━ SESSION INFO ━━━━━━━━━━━━━━━"))
	fmt.Fprintf(w, "%s     %s\n", previewLabelTitle.Render("Title:"), title)
	fmt.Fprintf(w, "%s      %s\n", previewLabelTime.Render("Time:"), timeStr)
	fmt.Fprintf(w, "%s  %d\n", previewLabelMsg.Render("Messages:"), msgCount)
	fmt.Fprintf(w, "%s %s\n", previewLabelDir.Render("Directory:"), directory)
	fmt.Fprintf(w, "%s\n\n", previewHeader.Render("━━━━━━━━━━━━━━ DIRECTORY LIST ━━━━━━━━━━━━━━"))
}
```

Everything else in `opencode.go` (`formatTimestamp`, `opencodeDBPath`) is unchanged.

- [ ] **Step 2: Build**

```bash
go build ./preview/...
```

Expected: `EXIT: 0`.

- [ ] **Step 3: Run regression tests**

```bash
go test ./preview/... -v -run "TestRenderOpencode"
```

Expected: `TestRenderOpencode_NoDB_WritesDirectoryListHeader` PASS.

- [ ] **Step 4: Commit**

```bash
git add preview/opencode.go
git commit -m "refactor(preview): replace raw ANSI in RenderOpencode/printOpencodeInfo with lipgloss"
```

---

## Task 5: Rewrite preview/shared.go

**Files:**
- Modify: `preview/shared.go:10`

One line change: the `"(directory not found: ...)"` error message.

- [ ] **Step 1: Replace the error message line in listDir**

Change line 10 of `preview/shared.go` from:

```go
fmt.Fprintf(w, "(directory not found: %s)\n", dir)
```

to:

```go
fmt.Fprintf(w, "%s\n", previewMissing.Render(fmt.Sprintf("(directory not found: %s)", dir)))
```

The full `listDir` function after the change:

```go
func listDir(w io.Writer, dir string) {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		fmt.Fprintf(w, "%s\n", previewMissing.Render(fmt.Sprintf("(directory not found: %s)", dir)))
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

- [ ] **Step 2: Full build**

```bash
go build ./...
```

Expected: `EXIT: 0`.

- [ ] **Step 3: Run all tests**

```bash
go test ./... -v
```

Expected: all existing tests PASS; `TestListDir_NonExistentDir_WritesErrorMessage` PASS.

- [ ] **Step 4: Commit**

```bash
git add preview/shared.go
git commit -m "refactor(preview): replace raw ANSI in listDir error message with lipgloss"
```

---

## Self-Review

### 1. Spec coverage

| Requirement | Task |
|------------|------|
| preview/claude.go — 8 raw ANSI calls replaced | Task 3 |
| preview/opencode.go — 7 raw ANSI calls replaced | Task 4 |
| preview/shared.go — 1 error message replaced | Task 5 |
| preview/styles.go created with 7 style vars | Task 1 |
| ANSI 16-color palette, matches picker/styles.go | Task 1 |
| Function signatures unchanged | Tasks 3–5 |
| listDir subprocess output not modified | Task 5 (only error path changed) |
| Regression tests added | Task 2 |

No gaps.

### 2. Placeholder scan

No TBD / TODO / "similar to" patterns present. All code blocks are complete.

### 3. Type consistency

`previewHeader`, `previewLabelTitle`, `previewLabelTime`, `previewLabelMsg`, `previewLabelDir`, `previewBullet`, `previewMissing` — defined in Task 1 (`styles.go`), used in Tasks 3, 4, 5. Names are consistent across all tasks.
