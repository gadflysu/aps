# Active Session Spinner Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Display an animated spinner glyph next to sessions whose claude/opencode process is currently running and has been active today.

**Architecture:** Add an unexported `jsonlPath` field to `source.Session` so `LoadClaude` can record the JSONL file path at parse time. A new `source/active.go` file implements `DetectActive` which collects running process CWDs via `ps`+`lsof`, then cross-checks each session's CWD and today-mtime condition. The picker calls `DetectActive` once at startup and stores the result in `activeIDs map[string]bool`, replacing the demo hard-coded index check.

**Tech Stack:** Go standard library (`os/exec`, `os`, `strings`, `time`), existing `source` and `picker` packages.

---

### Task 1: Add `jsonlPath` to `Session` and set it in `LoadClaude`

**Files:**
- Modify: `source/session.go`
- Modify: `source/claude.go:82-91`

- [ ] **Step 1: Add unexported field to Session**

In `source/session.go`, add `jsonlPath` after `MsgCount`:

```go
type Session struct {
	Client      Client
	ID          string
	Title       string
	CWD         string
	CWDDisplay  string
	ProjectPath string
	Time        time.Time
	MsgCount    int
	jsonlPath   string // unexported: path to the .jsonl file, Claude only
}
```

- [ ] **Step 2: Set jsonlPath in LoadClaude**

In `source/claude.go`, inside the `sessions = append(sessions, Session{...})` block, add `jsonlPath: jsonlFile`:

```go
sessions = append(sessions, Session{
    Client:      ClientClaude,
    ID:          sessionID,
    Title:       title,
    CWD:         cwd,
    CWDDisplay:  cwdDisplay,
    ProjectPath: projectPath,
    Time:        mtime,
    MsgCount:    msgCount,
    jsonlPath:   jsonlFile,
})
```

- [ ] **Step 3: Build to verify no compile errors**

```bash
go build ./...
```

Expected: no output (clean build).

- [ ] **Step 4: Commit**

```bash
git add source/session.go source/claude.go
git commit -m "feat(source): add unexported jsonlPath field to Session"
```

---

### Task 2: Implement `DetectActive` in `source/active.go`

**Files:**
- Create: `source/active.go`

- [ ] **Step 1: Write the failing test first**

Create `source/active_test.go`:

```go
package source

import (
	"testing"
	"time"
)

func TestDetectActive_emptySessionsReturnsEmptyMap(t *testing.T) {
	result := DetectActive(nil)
	if result == nil {
		t.Fatal("expected non-nil map, got nil")
	}
	if len(result) != 0 {
		t.Fatalf("expected empty map, got %v", result)
	}
}

func TestDetectActive_sessionWithNoMatchingCWDIsNotActive(t *testing.T) {
	s := Session{
		Client:    ClientClaude,
		ID:        "test-id-1",
		CWD:       "/nonexistent/path/that/no/process/uses",
		Time:      time.Now(),
		jsonlPath: "", // no file
	}
	result := DetectActive([]Session{s})
	if result["test-id-1"] {
		t.Fatal("session with unmatched CWD should not be active")
	}
}

func TestDetectActive_opencodeSessionNotTodayIsNotActive(t *testing.T) {
	// Session with yesterday's time should never be active regardless of CWD
	yesterday := time.Now().AddDate(0, 0, -1)
	s := Session{
		Client: ClientOpencode,
		ID:     "oc-old",
		CWD:    "/some/path",
		Time:   yesterday,
	}
	result := DetectActive([]Session{s})
	if result["oc-old"] {
		t.Fatal("opencode session from yesterday should not be active")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./source/... -run TestDetectActive -v
```

Expected: FAIL — `DetectActive` undefined.

- [ ] **Step 3: Implement `source/active.go`**

```go
package source

import (
	"os"
	"os/exec"
	"strings"
	"time"
)

// DetectActive returns a set of session IDs that are currently active.
// A session is active when its CWD matches a running claude/opencode process CWD
// AND its last-activity timestamp is today (>= local midnight).
// Errors from ps/lsof are silently ignored — callers get an empty map on failure.
func DetectActive(sessions []Session) map[string]bool {
	result := make(map[string]bool)
	if len(sessions) == 0 {
		return result
	}

	processCWDs := collectProcessCWDs()
	todayMidnight := todayMidnight()

	for _, s := range sessions {
		if !processCWDs[s.CWD] {
			continue
		}
		switch s.Client {
		case ClientClaude:
			if s.jsonlPath == "" {
				continue
			}
			info, err := os.Stat(s.jsonlPath)
			if err != nil {
				continue
			}
			if info.ModTime().Before(todayMidnight) {
				continue
			}
			result[s.ID] = true
		case ClientOpencode:
			if s.Time.Before(todayMidnight) {
				continue
			}
			result[s.ID] = true
		}
	}
	return result
}

// collectProcessCWDs returns the set of working directories of all running
// claude and opencode processes. Errors are silently ignored.
func collectProcessCWDs() map[string]bool {
	cwds := make(map[string]bool)

	out, err := exec.Command("ps", "aux").Output()
	if err != nil {
		return cwds
	}

	for _, line := range strings.Split(string(out), "\n") {
		if !strings.Contains(line, "claude") && !strings.Contains(line, "opencode") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		pid := fields[1]
		cwd := lsofCWD(pid)
		if cwd != "" {
			cwds[cwd] = true
		}
	}
	return cwds
}

// lsofCWD returns the working directory of the given PID via lsof, or "".
func lsofCWD(pid string) string {
	out, err := exec.Command("lsof", "-p", pid, "-Fn").Output()
	if err != nil {
		return ""
	}
	// lsof -Fn output: lines starting with 'n' for name; cwd entry has type 'cwd'
	// With -Fn the format is: pPID\nfFD\nnNAME — we need the 'n' line after 'fcwd'
	lines := strings.Split(string(out), "\n")
	for i, l := range lines {
		if l == "fcwd" && i+1 < len(lines) {
			name := lines[i+1]
			if strings.HasPrefix(name, "n") {
				return name[1:]
			}
		}
	}
	return ""
}

// todayMidnight returns 00:00:00 of today in local time.
func todayMidnight() time.Time {
	now := time.Now()
	y, m, d := now.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, now.Location())
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./source/... -run TestDetectActive -v
```

Expected: all three tests PASS.

- [ ] **Step 5: Build**

```bash
go build ./...
```

Expected: no output.

- [ ] **Step 6: Commit**

```bash
git add source/active.go source/active_test.go
git commit -m "feat(source): add DetectActive for running process detection"
```

---

### Task 3: Wire `DetectActive` into picker, remove demo code

**Files:**
- Modify: `picker/model.go`

- [ ] **Step 1: Add `activeIDs` field to Model**

In `picker/model.go`, in the `Model` struct, add after `spinFrame`:

```go
activeIDs    map[string]bool // sessions with a running process active today
```

- [ ] **Step 2: Populate `activeIDs` in `newModel()`**

In `newModel()`, add to the return struct literal:

```go
activeIDs:    source.DetectActive(sessions),
```

- [ ] **Step 3: Replace demo spinner logic in `renderRowFull`**

Replace the demo block:

```go
// Spinner cell: rendered for demo rows (first 3 visible); blank otherwise.
spinCell := "  "
if idx := m.visibleIndex(s); idx >= 0 && idx < 3 {
    glyph := spinnerFrames[m.spinFrame%len(spinnerFrames)]
    spinCell = lipgloss.NewStyle().Foreground(display.ColorTime).Render(glyph) + " "
}
```

With:

```go
spinCell := "  "
if m.activeIDs[s.ID] {
    glyph := spinnerFrames[m.spinFrame%len(spinnerFrames)]
    spinCell = lipgloss.NewStyle().Foreground(display.ColorTime).Render(glyph) + " "
}
```

- [ ] **Step 4: Remove `visibleIndex` method**

Delete the entire `visibleIndex` function from `picker/model.go`:

```go
// visibleIndex returns the position of s within m.filtered, or -1 if not found.
func (m Model) visibleIndex(s source.Session) int {
	for i, f := range m.filtered {
		if f.ID == s.ID {
			return i
		}
	}
	return -1
}
```

- [ ] **Step 5: Build and test**

```bash
go build ./... && go test ./...
```

Expected: clean build, all tests pass.

- [ ] **Step 6: Install and smoke-test**

```bash
go install .
aps
```

Expected: picker opens; sessions whose claude process is running today show a cycling spinner glyph (`·`→`✢`→`✳`→`✶`→`✻`→`✽`→`✽`→…) in the left column; others show two spaces.

- [ ] **Step 7: Commit**

```bash
git add picker/model.go
git commit -m "feat(picker): wire DetectActive into spinner column, remove demo code"
```

---

### Task 4: Clean up visual companion server and commit spec/plan docs

**Files:**
- Modify: none (cleanup + doc commit)

- [ ] **Step 1: Stop visual companion server**

```bash
bash /Users/dsu/.claude/plugins/cache/claude-plugins-official/superpowers/5.0.7/skills/brainstorming/scripts/stop-server.sh /Users/dsu/projects.local/aps/.superpowers/brainstorm/71646-1778573435
```

- [ ] **Step 2: Commit spec and plan docs**

```bash
git add docs/superpowers/specs/2026-05-12-active-session-spinner-design.md \
        docs/superpowers/plans/2026-05-12-active-session-spinner.md \
        .gitignore
git commit -m "docs: add active-session-spinner spec and implementation plan"
```
