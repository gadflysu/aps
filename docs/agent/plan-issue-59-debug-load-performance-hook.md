# Plan: Debug-Load Performance Hook

GitHub issue: #59

## Goal

Add one minimal way to run aps from process start through “all selected sessions loaded” and then exit.
Timing stays outside aps with tools such as `hyperfine`. Test data stays external and can continue to
be redirected with `HOME=/path`.

## Issue Relationship

Create this as a new implementation issue for the debug-load performance hook. It depends on a
separate refactor issue that first introduces a complete-load interface for the “all selected
sessions are loaded” lifecycle boundary.

Do not implement `--debug-load` directly against today's ad hoc `main.go` helpers. The hook should be
small because the refactor has already made the load lifecycle explicit.

Recommended split:

1. Refactor issue: define and implement the complete-load interface.
2. Debug-load issue: add the CLI flag and call that interface, then exit.

## Decision

Do not add `aps perf fixture`, `aps perf load --json`, or `aps perf stream --json`.

Add a single hidden/advanced CLI mode:

```bash
HOME=.worktrees/perf-home aps --debug-load -c
```

Behavior:

1. Parse normal aps flags.
2. Apply the same source selection, path filter, date filter, and cache behavior as normal startup.
3. Call the project’s complete-load interface for the selected sources.
4. Exit `0` when that interface has loaded all selected sessions, without rendering list output or
   starting the TUI.
5. Exit non-zero on fatal load/config errors.

The command should not print per-session rows. It may print a concise summary only when an explicit
flag is provided, but the default should be quiet so `hyperfine` measures the load path with minimal
output noise.

## Why This Is Enough

For total startup/load acceptance, the external benchmark only needs a stable process boundary:

```bash
hyperfine --warmup 3 --runs 20 \
  'HOME=.worktrees/perf-home ./aps --debug-load -c'
```

`--debug-load` should not care whether the implementation underneath is streaming or blocking. The
benchmark question is: “How long does aps take to start, call the complete-load interface, load every
selected session, and exit?” Streaming first-paint benefits remain development-time/internal metrics;
the total performance acceptance uses only the all-loaded boundary.

This removes the unstable parts of the previous approach:

- no Bubble Tea startup;
- no PTY or keystroke injection;
- no parsing `first View()` logs;
- no product-owned fixture generator;
- no product-owned statistics/reporting command.

## Scope

### Add

| File | Change |
|---|---|
| `cmd/root.go` | Add hidden/advanced `--debug-load` bool to `Config` and parser |
| complete-load interface from refactor | Reuse the refactored complete-load interface |
| `main.go` | Call the complete-load interface and exit before list/TUI/launch behavior |
| `cmd/root_test.go` | Cover parsing and interactions with source flags |
| main-level test if practical | Cover debug-load loads all selected sessions and does not enter picker/list rendering |
| `docs/agent/notes-performance.md` | Document the benchmark flow |

### Do Not Add

- Do not add `APS_HOME`, `--home`, or `--claude-projects-dir`.
- Do not add `aps perf ...`.
- Do not add JSON metrics output.
- Do not add fixture generation.
- Do not add dependencies.
- Do not change normal interactive, list, or launch behavior.

## CLI Semantics

Recommended flag:

```text
--debug-load
```

Rationale: it marks the flag as an operational/debug hook rather than a normal user-facing mode,
while still describing the lifecycle in the issue and docs.

Allowed combinations:

```bash
aps --debug-load                 # default source selection
aps --debug-load -c              # Claude only
aps --debug-load -a              # all sources
aps --debug-load --from today
aps --debug-load /some/path
HOME=.worktrees/perf-home aps --debug-load -c
```

Invalid or discouraged combinations:

- `--debug-load -l`: reject with a clear error, because list mode has its own output contract.
- `--debug-load -n`: reject with a clear error, because no session is selected or launched.
- `--debug-load --cmd ...`: reject with a clear error, because launch commands are irrelevant.

If rejecting combinations creates too much parser churn, a smaller first implementation may ignore
launch-only flags in debug-load mode, but tests must lock the chosen behavior.

## TDD Plan

Write or update tests before implementation:

| Test | Expected Proof |
|---|---|
| `TestParse_DebugLoadFlag` | `--debug-load` sets `Config.DebugLoad` |
| `TestParse_DebugLoadWithClaude` | source flags still apply |
| `TestParse_DebugLoadRejectsListMode` | `--debug-load -l` exits with a clear error, if rejection is chosen |
| `TestParse_DebugLoadRejectsNoLaunch` | `--debug-load -n` exits with a clear error, if rejection is chosen |
| `TestRunDebugLoad_LoadsAllSessionsAndExits` | debug-load calls the complete-load path and does not enter picker |

Keep tests focused. Do not build benchmark assertions into unit tests.

## Implementation Notes

In `main.go`, route debug-load through the complete-load interface introduced by the prerequisite
refactor. Do not bypass that interface from the new debug-load branch.

Suggested shape:

```go
if cfg.DebugLoad {
    t0 := time.Now()
    result, err := loadComplete(cfg, from, until)
    if err != nil {
        fmt.Fprintf(os.Stderr, "Error loading sessions: %v\n", err)
        os.Exit(1)
    }
    dbg.Log("debugLoad: %v (%d sessions)", time.Since(t0), len(result.Sessions))
    return
}
```

This intentionally measures the complete-load boundary without starting Bubble Tea. The implementation
must not branch on whether the underlying loader currently happens to be streaming or blocking.

Do not start PID cache GC, watcher, Bubble Tea, launcher, or list rendering in debug-load mode.

## Verification

Run:

```bash
go test ./cmd ./...
go vet ./...
go build .
go install .
```

Manual smoke:

```bash
HOME=$PWD/.worktrees/perf-home ./aps --debug-load -c
HOME=$PWD/.worktrees/perf-home ./aps --debug-load -c --debug-log .worktrees/debug-load.log
rg 'debugLoad:' .worktrees/debug-load.log
```

Benchmark smoke:

```bash
hyperfine --warmup 3 --runs 20 \
  'HOME=$PWD/.worktrees/perf-home ./aps --debug-load -c'
```

## Follow-Up Acceptance Flow

Future performance acceptance should use external tooling:

1. Generate or reuse a fake `HOME` fixture outside aps.
2. Verify `HOME=<fixture> aps --debug-load -c` exits zero.
3. Use `hyperfine` to compare baseline/current binaries.
4. Keep summary/report generation outside aps.

The product provides only the stable process boundary; external tools own data construction, repeated
runs, statistics, and reporting.

## Engineering Philosophy

Expose the smallest stable boundary that answers the acceptance question. Let external benchmark
tools own repetition and statistics; keep the product out of test-data generation and report writing.
