# Plan: Debug-Load Performance Hook

GitHub issue: #59

## Goal

Add one minimal way to run aps from process start through the load lifecycle under test and then
exit. Timing stays outside aps with tools such as `hyperfine`. Test data stays external and can
continue to be redirected with `HOME=/path`.

## Issue Relationship

This implementation issue depends on #61. That refactor should make the current load paths easier to
call from a debug hook, but this plan must not assume specific function names or package shapes from
the refactor.

At implementation time, inspect the current code and call the appropriate load boundary exposed by
the refactor. Today the app has two relevant load paths:

- a complete path used by list mode;
- a streaming path used by interactive mode.

`--debug-load` should expose both paths for external measurement.

## Decision

Do not add `aps perf fixture`, `aps perf load --json`, or `aps perf stream --json`.

Add one hidden/advanced CLI flag with two explicit modes:

```bash
HOME=.worktrees/perf-home aps --debug-load=complete -c
HOME=.worktrees/perf-home aps --debug-load=stream -c
```

Behavior for both modes:

1. Parse normal aps flags.
2. Apply the same source selection, path filter, date filter, and cache behavior as normal startup.
3. Use the load path selected by `--debug-load`.
4. Exit `0` when that load path reaches its natural “all selected sessions loaded” boundary.
5. Exit non-zero on fatal load/config errors.
6. Do not render list output, start the TUI, or launch an agent.

The command should not print per-session rows. It may print a concise summary only when an explicit
flag is provided, but the default should be quiet so `hyperfine` measures the load path with minimal
output noise.

Mode semantics:

- `--debug-load=complete`: measure the complete/list-style load path through the point where all
  selected sessions have been loaded and normalized for complete consumption.
- `--debug-load=stream`: measure the streaming/interactive-style load path by draining it until its
  final completion signal, without starting Bubble Tea.

## Why This Is Enough

For total startup/load acceptance, the external benchmark only needs a stable process boundary:

```bash
hyperfine --warmup 3 --runs 20 \
  'HOME=.worktrees/perf-home ./aps --debug-load=complete -c'

hyperfine --warmup 3 --runs 20 \
  'HOME=.worktrees/perf-home ./aps --debug-load=stream -c'
```

`--debug-load` should not know implementation details such as concrete function names or package
layout. The benchmark questions are:

- complete: “How long does aps take to start, use the complete load path, load every selected session,
  and exit?”
- stream: “How long does aps take to start, use the stream load path, drain it to completion, and
  exit?”

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
| `cmd/root.go` | Add hidden/advanced `--debug-load` string mode to `Config` and parser |
| load lifecycle code from refactor | Use the appropriate complete or stream load boundary present at implementation time |
| `main.go` | Dispatch debug-load before list/TUI/launch behavior |
| `cmd/root_test.go` | Cover parsing and interactions with source flags |
| main-level test if practical | Cover debug-load complete and stream modes without entering picker/list rendering |
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
--debug-load=complete
--debug-load=stream
```

Rationale: it marks the flag as an operational/debug hook rather than a normal user-facing mode,
while still describing the lifecycle in the issue and docs.

Allowed combinations:

```bash
aps --debug-load=complete                 # default source selection
aps --debug-load=complete -c              # Claude only
aps --debug-load=complete -a              # all sources
aps --debug-load=complete --from today
aps --debug-load=complete /some/path
HOME=.worktrees/perf-home aps --debug-load=complete -c

aps --debug-load=stream                   # default source selection
aps --debug-load=stream -c                # Claude only
HOME=.worktrees/perf-home aps --debug-load=stream -c
```

Invalid or discouraged combinations:

- `--debug-load` without a value: reject with a clear error listing `complete` and `stream`.
- `--debug-load=<unknown>`: reject with a clear error listing `complete` and `stream`.
- `--debug-load=complete -l` or `--debug-load=stream -l`: reject with a clear error, because list mode
  has its own output contract.
- `--debug-load=complete -n` or `--debug-load=stream -n`: reject with a clear error, because no
  session is selected or launched.
- `--debug-load=complete --cmd ...` or `--debug-load=stream --cmd ...`: reject with a clear error,
  because launch commands are irrelevant.

If rejecting combinations creates too much parser churn, a smaller first implementation may ignore
launch-only flags in debug-load mode, but tests must lock the chosen behavior.

## TDD Plan

Write or update tests before implementation:

| Test | Expected Proof |
|---|---|
| `TestParse_DebugLoadComplete` | `--debug-load=complete` sets the complete mode |
| `TestParse_DebugLoadStream` | `--debug-load=stream` sets the stream mode |
| `TestParse_DebugLoadRejectsMissingMode` | `--debug-load` without a value exits with a clear error |
| `TestParse_DebugLoadRejectsUnknownMode` | unknown mode exits with a clear error |
| `TestParse_DebugLoadWithClaude` | source flags still apply |
| `TestParse_DebugLoadRejectsListMode` | `--debug-load=<mode> -l` exits with a clear error, if rejection is chosen |
| `TestParse_DebugLoadRejectsNoLaunch` | `--debug-load=<mode> -n` exits with a clear error, if rejection is chosen |
| `TestRunDebugLoadComplete_LoadsAndExits` | complete mode reaches all-loaded boundary and does not enter picker |
| `TestRunDebugLoadStream_DrainsAndExits` | stream mode drains to completion and does not enter picker |

Keep tests focused. Do not build benchmark assertions into unit tests.

## Implementation Notes

Do not hard-code assumptions from this plan about the final load API. At implementation time, inspect
the load lifecycle code introduced by the prerequisite refactor and wire `--debug-load=complete` and
`--debug-load=stream` to the corresponding boundaries.

Both modes must exit after the selected load boundary completes. Neither mode should start PID cache
GC for interactive UI, watcher, Bubble Tea, launcher, or list rendering.

Suggested debug log keys:

- `debugLoad complete: ...`
- `debugLoad stream: ...`

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
HOME=$PWD/.worktrees/perf-home ./aps --debug-load=complete -c
HOME=$PWD/.worktrees/perf-home ./aps --debug-load=stream -c
HOME=$PWD/.worktrees/perf-home ./aps --debug-load=complete -c --debug-log .worktrees/debug-load.log
HOME=$PWD/.worktrees/perf-home ./aps --debug-load=stream -c --debug-log .worktrees/debug-load.log
rg 'debugLoad (complete|stream):' .worktrees/debug-load.log
```

Benchmark smoke:

```bash
hyperfine --warmup 3 --runs 20 \
  'HOME=$PWD/.worktrees/perf-home ./aps --debug-load=complete -c'

hyperfine --warmup 3 --runs 20 \
  'HOME=$PWD/.worktrees/perf-home ./aps --debug-load=stream -c'
```

## Follow-Up Acceptance Flow

Future performance acceptance should use external tooling:

1. Generate or reuse a fake `HOME` fixture outside aps.
2. Verify `HOME=<fixture> aps --debug-load=complete -c` exits zero.
3. Verify `HOME=<fixture> aps --debug-load=stream -c` exits zero.
4. Use `hyperfine` to compare baseline/current binaries for both modes.
5. Keep summary/report generation outside aps.

The product provides only the stable process boundary; external tools own data construction, repeated
runs, statistics, and reporting.

## Engineering Philosophy

Expose the smallest stable boundary that answers the acceptance question. Let external benchmark
tools own repetition and statistics; keep the product out of test-data generation and report writing.
