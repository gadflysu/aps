# Custom Command Flag Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `--claude-cmd`, `--opencode-cmd`, and `--cmd` flags to let users override the binary/command used to launch sessions (supports shell aliases, functions, and versioned commands like `npx claude@2.1`).

**Architecture:** Parse the new flags in `cmd/root.go`, propagate them through `launcher.Options`, and branch in `launcher/launch.go`: no custom cmd → existing LookPath+syscall.Exec path; custom cmd → `$SHELL -i -c "exec <cmd> <session-args>"`.

**Tech Stack:** Go stdlib (`flag`, `os`, `os/exec`, `syscall`), existing bubbletea TUI stack.

---

## File Map

| File | Action | Responsibility |
|------|--------|----------------|
| `cmd/root.go` | Modify | Add `ClaudeCmd`/`OpencodeCmd` to `Config`; parse three new flags; validate conflicts; update `usage()` |
| `launcher/launch.go` | Modify | Add `ClaudeCmd`/`OpencodeCmd` to `Options`; add `shellExec` helper; branch on non-empty custom cmd |
| `main.go` | Modify | Pass new cfg fields to `launchOpts` |
| `cmd/root_test.go` | Modify | Flag parsing + conflict validation tests |
| `launcher/launch_test.go` | Create | Custom cmd branch unit tests |

---

### Task 1: Add fields to `cmd.Config` and parse new flags

**Files:**
- Modify: `cmd/root.go`

- [ ] **Step 1: Write the failing tests**

Add to `cmd/root_test.go`:

```go
func TestParse_ClaudeCmdFlag(t *testing.T) {
	cfg := Parse([]string{"--claude-cmd", "cc"})
	if cfg.ClaudeCmd != "cc" {
		t.Errorf("ClaudeCmd = %q, want \"cc\"", cfg.ClaudeCmd)
	}
}

func TestParse_OpencodeCmdFlag(t *testing.T) {
	cfg := Parse([]string{"-o", "--opencode-cmd", "npx opencode@1.0"})
	if cfg.OpencodeCmd != "npx opencode@1.0" {
		t.Errorf("OpencodeCmd = %q, want \"npx opencode@1.0\"", cfg.OpencodeCmd)
	}
}

func TestParse_CmdFlagSingleClaudeDefault(t *testing.T) {
	cfg := Parse([]string{"--cmd", "cc"})
	if cfg.ClaudeCmd != "cc" {
		t.Errorf("ClaudeCmd = %q, want \"cc\" (default client)", cfg.ClaudeCmd)
	}
}

func TestParse_CmdFlagSingleExplicitClaude(t *testing.T) {
	cfg := Parse([]string{"-c", "--cmd", "cc"})
	if cfg.ClaudeCmd != "cc" {
		t.Errorf("ClaudeCmd = %q, want \"cc\"", cfg.ClaudeCmd)
	}
}

func TestParse_CmdFlagSingleOpencode(t *testing.T) {
	cfg := Parse([]string{"-o", "--cmd", "npx opencode@1.0"})
	if cfg.OpencodeCmd != "npx opencode@1.0" {
		t.Errorf("OpencodeCmd = %q, want \"npx opencode@1.0\"", cfg.OpencodeCmd)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./cmd/... -run "TestParse_ClaudeCmdFlag|TestParse_OpencodeCmdFlag|TestParse_CmdFlag" -v
```

Expected: FAIL — `cfg.ClaudeCmd` undefined.

- [ ] **Step 3: Add fields to `Config` and wire up flag parsing**

In `cmd/root.go`, add to `Config` struct:

```go
type Config struct {
	NoLaunch    bool
	Verbose     bool
	ListOnly    bool
	Claude      bool
	Opencode    bool
	All         bool
	DangerMode  bool
	Recursive   bool
	PathFilter  string
	ClaudeCmd   string
	OpencodeCmd string
}
```

In `Parse`, after the existing `fs.BoolVar` block and before `expanded := expandShortFlags(args)`, add:

```go
var rawCmd, rawClaudeCmd, rawOpencodeCmd string
fs.StringVar(&rawClaudeCmd, "claude-cmd", "", "")
fs.StringVar(&rawOpencodeCmd, "opencode-cmd", "", "")
fs.StringVar(&rawCmd, "cmd", "", "")
```

After the `if cfg.All { ... }` block, add conflict validation and `--cmd` resolution:

```go
// conflict: --cmd with --claude-cmd or --opencode-cmd
if rawCmd != "" && rawClaudeCmd != "" {
	fmt.Fprintln(os.Stderr, "error: --cmd conflicts with --claude-cmd")
	os.Exit(1)
}
if rawCmd != "" && rawOpencodeCmd != "" {
	fmt.Fprintln(os.Stderr, "error: --cmd conflicts with --opencode-cmd")
	os.Exit(1)
}
// conflict: --cmd with multiple clients
if rawCmd != "" && cfg.Claude && cfg.Opencode {
	fmt.Fprintln(os.Stderr, "error: --cmd is ambiguous when multiple clients are selected; use --claude-cmd or --opencode-cmd")
	os.Exit(1)
}
// resolve --cmd into the active client's field
if rawCmd != "" {
	if cfg.Claude {
		cfg.ClaudeCmd = rawCmd
	} else {
		cfg.OpencodeCmd = rawCmd
	}
}
cfg.ClaudeCmd = firstNonEmpty(cfg.ClaudeCmd, rawClaudeCmd)
cfg.OpencodeCmd = firstNonEmpty(cfg.OpencodeCmd, rawOpencodeCmd)
```

Add the helper at the bottom of `cmd/root.go`:

```go
func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./cmd/... -run "TestParse_ClaudeCmdFlag|TestParse_OpencodeCmdFlag|TestParse_CmdFlag" -v
```

Expected: PASS.

- [ ] **Step 5: Run all cmd tests**

```bash
go test ./cmd/... -v
```

Expected: all PASS.

- [ ] **Step 6: Commit**

```bash
git add cmd/root.go cmd/root_test.go
git commit -m "feat(cmd): add --claude-cmd, --opencode-cmd, --cmd flags"
```

---

### Task 2: Add conflict-validation edge-case tests

**Files:**
- Modify: `cmd/root_test.go`

These tests call `Parse` with bad combinations and expect `os.Exit(1)`. The standard pattern is to re-invoke the test binary as a subprocess.

- [ ] **Step 1: Write the subprocess-exit tests**

Add to `cmd/root_test.go`:

```go
import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// runParseExpectExit re-invokes the test binary with TEST_PARSE_ARGS set,
// expects a non-zero exit and the given stderr substring.
func runParseExpectExit(t *testing.T, args []string, wantStderr string) {
	t.Helper()
	if os.Getenv("TEST_PARSE_CRASH") == "1" {
		Parse(args)
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestParseExitHelper")
	cmd.Env = append(os.Environ(),
		"TEST_PARSE_CRASH=1",
		"TEST_PARSE_ARGS="+strings.Join(args, "\x00"),
	)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected non-zero exit, got nil; output: %s", out)
	}
	if !strings.Contains(string(out), wantStderr) {
		t.Errorf("stderr %q does not contain %q", string(out), wantStderr)
	}
}

// TestParseExitHelper is the subprocess entry-point (never called directly).
func TestParseExitHelper(t *testing.T) {
	raw := os.Getenv("TEST_PARSE_ARGS")
	if raw == "" {
		return
	}
	Parse(strings.Split(raw, "\x00"))
}

func TestParse_CmdConflictsWithClaudeCmd(t *testing.T) {
	runParseExpectExit(t, []string{"--cmd", "cc", "--claude-cmd", "cc"},
		"--cmd conflicts with --claude-cmd")
}

func TestParse_CmdConflictsWithOpencodeCmd(t *testing.T) {
	runParseExpectExit(t, []string{"-o", "--cmd", "oc", "--opencode-cmd", "oc"},
		"--cmd conflicts with --opencode-cmd")
}

func TestParse_CmdAmbiguousMultipleClients(t *testing.T) {
	runParseExpectExit(t, []string{"-a", "--cmd", "cc"},
		"--cmd is ambiguous when multiple clients are selected")
}
```

- [ ] **Step 2: Run tests to verify they pass**

```bash
go test ./cmd/... -run "TestParse_CmdConflicts|TestParse_CmdAmbiguous" -v
```

Expected: all PASS.

- [ ] **Step 3: Commit**

```bash
git add cmd/root_test.go
git commit -m "test(cmd): add conflict-validation tests for --cmd flag"
```

---

### Task 3: Add custom cmd support to `launcher/launch.go`

**Files:**
- Modify: `launcher/launch.go`
- Create: `launcher/launch_test.go`

- [ ] **Step 1: Write the failing tests**

Create `launcher/launch_test.go`:

```go
package launcher

import (
	"os"
	"strings"
	"testing"
)

// captureShellExecArgs calls shellExec in dry-run mode by temporarily
// replacing syscall.Exec — but since we can't mock syscall.Exec easily,
// we test buildShellCmd directly.

func TestBuildShellCmd_Claude(t *testing.T) {
	shell := "/bin/zsh"
	got := buildShellCmd(shell, "cc", "--resume", "abc123")
	want := shell + " -i -c exec cc --resume abc123"
	// argv[2] is the -c argument
	if len(got) != 4 {
		t.Fatalf("buildShellCmd: len=%d, want 4; got %v", len(got), got)
	}
	if got[0] != shell {
		t.Errorf("argv[0] = %q, want %q", got[0], shell)
	}
	if got[1] != "-i" {
		t.Errorf("argv[1] = %q, want \"-i\"", got[1])
	}
	if got[2] != "-c" {
		t.Errorf("argv[2] = %q, want \"-c\"", got[2])
	}
	wantScript := "exec cc --resume abc123"
	if got[3] != wantScript {
		t.Errorf("argv[3] = %q, want %q", got[3], wantScript)
	}
	_ = want
}

func TestBuildShellCmd_Opencode(t *testing.T) {
	shell := "/bin/bash"
	got := buildShellCmd(shell, "npx opencode@1.0", "-s", "sess-xyz")
	if got[3] != "exec npx opencode@1.0 -s sess-xyz" {
		t.Errorf("argv[3] = %q", got[3])
	}
}

func TestBuildShellCmd_DangerMode(t *testing.T) {
	shell := "/bin/zsh"
	got := buildShellCmd(shell, "cc", "--dangerously-skip-permissions --resume", "abc123")
	if !strings.Contains(got[3], "--dangerously-skip-permissions") {
		t.Errorf("DangerMode not in script: %q", got[3])
	}
}

func TestResolveShell_EnvSet(t *testing.T) {
	t.Setenv("SHELL", "/usr/local/bin/zsh")
	if got := resolveShell(); got != "/usr/local/bin/zsh" {
		t.Errorf("resolveShell = %q, want /usr/local/bin/zsh", got)
	}
}

func TestResolveShell_Fallback(t *testing.T) {
	t.Setenv("SHELL", "")
	if got := resolveShell(); got != "/bin/sh" {
		t.Errorf("resolveShell fallback = %q, want /bin/sh", got)
	}
}

func TestVerboseOutput_CustomCmd(t *testing.T) {
	got := verboseClaudeCmd("cc", "/my/dir", "abc123", false)
	want := `cd "/my/dir" && cc --resume abc123`
	if got != want {
		t.Errorf("verboseClaudeCmd = %q, want %q", got, want)
	}
}

func TestVerboseOutput_CustomCmdDanger(t *testing.T) {
	got := verboseClaudeCmd("cc", "/my/dir", "abc123", true)
	want := `cd "/my/dir" && cc --dangerously-skip-permissions --resume abc123`
	if got != want {
		t.Errorf("verboseClaudeCmd danger = %q, want %q", got, want)
	}
}

func TestVerboseOutput_OpencodeCustomCmd(t *testing.T) {
	got := verboseOpencodeCmd("mycode", "/my/dir", "sess-1")
	want := `cd "/my/dir" && mycode -s sess-1`
	if got != want {
		t.Errorf("verboseOpencodeCmd = %q, want %q", got, want)
	}
}

// Ensure os package is used (for t.Setenv compile check)
var _ = os.Getenv
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./launcher/... -v
```

Expected: FAIL — `buildShellCmd`, `resolveShell`, `verboseClaudeCmd`, `verboseOpencodeCmd` undefined.

- [ ] **Step 3: Add `ClaudeCmd`/`OpencodeCmd` to `Options` and implement helpers**

In `launcher/launch.go`, update `Options`:

```go
type Options struct {
	NoLaunch    bool
	Verbose     bool
	DangerMode  bool // Claude only
	ClaudeCmd   string
	OpencodeCmd string
}
```

Add helpers after `joinArgs`:

```go
func resolveShell() string {
	if s := os.Getenv("SHELL"); s != "" {
		return s
	}
	return "/bin/sh"
}

// buildShellCmd returns argv for: $SHELL -i -c "exec <customCmd> <sessionFlag> <sessionID>"
func buildShellCmd(shell, customCmd, sessionFlag, sessionID string) []string {
	script := "exec " + customCmd + " " + sessionFlag + " " + sessionID
	return []string{shell, "-i", "-c", script}
}

func verboseClaudeCmd(customCmd, dir, sessionID string, danger bool) string {
	args := ""
	if danger {
		args = "--dangerously-skip-permissions --resume " + sessionID
	} else {
		args = "--resume " + sessionID
	}
	return fmt.Sprintf("cd %q && %s %s", dir, customCmd, args)
}

func verboseOpencodeCmd(customCmd, dir, sessionID string) string {
	return fmt.Sprintf("cd %q && %s -s %s", dir, customCmd, sessionID)
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./launcher/... -v
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add launcher/launch.go launcher/launch_test.go
git commit -m "feat(launcher): add ClaudeCmd/OpencodeCmd to Options with helpers"
```

---

### Task 4: Wire custom cmd into `Claude()` and `Opencode()` launch functions

**Files:**
- Modify: `launcher/launch.go`

- [ ] **Step 1: Update `Claude()` to branch on `opts.ClaudeCmd`**

Replace the body of `Claude` in `launcher/launch.go`:

```go
func Claude(sessionID, dir string, opts Options) error {
	if opts.NoLaunch {
		if opts.Verbose {
			if opts.ClaudeCmd != "" {
				fmt.Println(verboseClaudeCmd(opts.ClaudeCmd, dir, sessionID, opts.DangerMode))
			} else {
				args := []string{"--resume", sessionID}
				if opts.DangerMode {
					args = []string{"--dangerously-skip-permissions", "--resume", sessionID}
				}
				fmt.Printf("cd %q && claude %s\n", dir, joinArgs(args))
			}
		} else {
			fmt.Println(dir)
		}
		return nil
	}

	fmt.Printf("Resuming Claude Code session: %s\n", sessionID)
	fmt.Printf("Directory: %s\n", dir)
	if opts.DangerMode {
		fmt.Fprintf(os.Stderr, "\033[31mWARNING: DANGER MODE: Skipping all permissions checks\033[0m\n")
	}

	if err := os.Chdir(dir); err != nil {
		return fmt.Errorf("chdir %s: %w", dir, err)
	}

	if opts.ClaudeCmd != "" {
		sessionFlag := "--resume"
		if opts.DangerMode {
			sessionFlag = "--dangerously-skip-permissions --resume"
		}
		shell := resolveShell()
		argv := buildShellCmd(shell, opts.ClaudeCmd, sessionFlag, sessionID)
		return syscall.Exec(shell, argv, os.Environ())
	}

	claudePath, err := exec.LookPath("claude")
	if err != nil {
		return fallbackShell()
	}
	args := []string{"--resume", sessionID}
	if opts.DangerMode {
		args = []string{"--dangerously-skip-permissions", "--resume", sessionID}
	}
	return syscall.Exec(claudePath, append([]string{"claude"}, args...), os.Environ())
}
```

- [ ] **Step 2: Update `Opencode()` to branch on `opts.OpencodeCmd`**

Replace the body of `Opencode` in `launcher/launch.go`:

```go
func Opencode(sessionID, dir string, opts Options) error {
	if opts.NoLaunch {
		if opts.Verbose {
			if opts.OpencodeCmd != "" {
				fmt.Println(verboseOpencodeCmd(opts.OpencodeCmd, dir, sessionID))
			} else {
				fmt.Printf("cd %q && opencode -s %q\n", dir, sessionID)
			}
		} else {
			fmt.Println(dir)
		}
		return nil
	}

	fmt.Printf("Resuming Opencode session: %s\n", sessionID)
	fmt.Printf("Directory: %s\n", dir)

	if err := os.Chdir(dir); err != nil {
		return fmt.Errorf("chdir %s: %w", dir, err)
	}

	if opts.OpencodeCmd != "" {
		shell := resolveShell()
		argv := buildShellCmd(shell, opts.OpencodeCmd, "-s", sessionID)
		return syscall.Exec(shell, argv, os.Environ())
	}

	opPath, err := exec.LookPath("opencode")
	if err != nil {
		return fallbackShell()
	}
	return syscall.Exec(opPath, []string{"opencode", "-s", sessionID}, os.Environ())
}
```

- [ ] **Step 3: Build and verify**

```bash
go build . && go install .
```

Expected: no errors.

- [ ] **Step 4: Run all tests**

```bash
go test ./...
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add launcher/launch.go
git commit -m "feat(launcher): branch on ClaudeCmd/OpencodeCmd in launch functions"
```

---

### Task 5: Wire new Config fields into `main.go` and update `usage()`

**Files:**
- Modify: `main.go`
- Modify: `cmd/root.go`

- [ ] **Step 1: Pass new fields from `cfg` to `launchOpts` in `main.go`**

In `runInteractive`, update the `launchOpts` initialization:

```go
launchOpts := launcher.Options{
	NoLaunch:    cfg.NoLaunch,
	Verbose:     cfg.Verbose,
	DangerMode:  cfg.DangerMode,
	ClaudeCmd:   cfg.ClaudeCmd,
	OpencodeCmd: cfg.OpencodeCmd,
}
```

- [ ] **Step 2: Update `usage()` in `cmd/root.go`**

Replace the usage string to add the three new flags:

```go
func usage() {
	fmt.Fprintf(os.Stderr, `Usage: aps [OPTIONS] [PATH_FILTER]

Interactive session picker for Claude Code and Opencode.

Options:
  -n, --no-launch       Print target directory instead of launching client
  -v, --verbose         With -n: print full launch command
  -l, --list            Non-interactive table output and exit
  -c, --claude          Include Claude Code sessions (default if no client flag)
  -o, --opencode        Include Opencode sessions
  -a, --all             Include both clients
  -d, --danger          Claude: launch with --dangerously-skip-permissions
  -r, --recursive       Looser path filter (substring match)
      --claude-cmd STR  Override command used to launch Claude Code
      --opencode-cmd STR Override command used to launch Opencode
      --cmd STR         Override command for the single active client
  -h, --help            Show this help

Arguments:
  PATH_FILTER           Filter sessions by directory path. Use '.' for cwd.

Examples:
  aps                         Interactive pick (Claude sessions, cwd filter default)
  aps -l .                    List mode, current directory
  aps -d                      Danger mode (--dangerously-skip-permissions)
  aps --claude-cmd "npx claude@2.1"   Use specific Claude version
  aps --cmd cc                Use 'cc' alias (single client active)
  aps -o --cmd "npx opencode@1.0"  Use specific Opencode version
`)
}
```

- [ ] **Step 3: Build, install, and run all tests**

```bash
go build . && go install . && go test ./...
```

Expected: no errors, all tests PASS.

- [ ] **Step 4: Smoke test**

```bash
aps --cmd "echo" -n -v .
```

Expected: prints something like `cd "/some/path" && echo --resume <id>` (no-launch verbose with custom cmd).

- [ ] **Step 5: Commit**

```bash
git add main.go cmd/root.go
git commit -m "feat(cmd): wire --claude-cmd/--opencode-cmd/--cmd into launcher and update usage"
```

---

### Task 6: Handle DangerMode shell-cmd edge case in `buildShellCmd`

The current `buildShellCmd` receives a `sessionFlag` that may be a multi-word string like `"--dangerously-skip-permissions --resume"`. This works because it is embedded in a shell script string (`exec cc --dangerously-skip-permissions --resume abc123`) and the shell parses it correctly. No code change needed — but verify with a direct test.

- [ ] **Step 1: Verify the DangerMode test already covers this**

```bash
go test ./launcher/... -run TestBuildShellCmd_DangerMode -v
```

Expected: PASS.

- [ ] **Step 2: Final full test run**

```bash
go test ./...
```

Expected: all PASS.
