# Load Lifecycle Refactor Plan

GitHub issue: #61

**Goal:** Introduce a shared load lifecycle boundary so list mode, interactive mode, and future
`--debug-load` can reuse source orchestration without copying loader logic.

**Architecture:** Keep complete loading and streaming loading as separate caller-facing APIs because
they have different lifecycle semantics. Add a new `loader` package that owns source orchestration
and the stream batch contract. `picker` consumes `loader.Batch` directly; `main.go` does not adapt
between two batch types.

**Tech Stack:** Go, existing `cmd.Config`, `source.Session`, new `loader` package,
`sync.WaitGroup`, standard `testing`.

---

## Current Problem

`main.go` currently owns two load paths:

```text
list mode
  -> loadSessions(...)
  -> waits for all selected sources
  -> merges, sorts, date-filters
  -> runList(...)

interactive mode
  -> runInteractiveStreaming(...)
  -> starts picker immediately
  -> producer goroutine loads selected sources
  -> sends picker.SessionBatch values
  -> picker consumes batches through Bubble Tea commands
```

Existing reuse is partial:

- Claude blocking and streaming paths both reuse `source.loadClaude`.
- Opencode and Codex use the same blocking source loaders in both modes.
- Source orchestration, partial-failure aggregation, stream completion, and complete-load result
  construction are split across `main.go`.
- The streaming contract currently lives in `picker`, even though batches are produced by loader
  orchestration.

## Package Boundary

Create a new top-level `loader` package:

```text
loader
  -> cmd
  -> dbg
  -> filter
  -> source

picker
  -> loader
  -> source/display/preview/watcher/dbg

main
  -> loader
  -> picker
  -> cmd/source/display/filter/launcher/dbg
```

Rules:

- `loader` must not import `picker`.
- `picker` should consume `loader.Batch` directly.
- `main.go` should not adapt between `loader.Batch` and another batch type.
- `source` remains responsible for persistence, parsing, metadata extraction, and source-specific
  caches.
- `loader` owns source selection, source concurrency, date filtering, partial-failure aggregation,
  complete-load results, and stream completion.

This keeps dependency direction valid:

```text
source/filter/dbg/cmd  <-  loader  <-  picker
                                  \ <-  main
```

## Public Shape

```go
package loader

import (
    "time"

    "github.com/gadflysu/aps/cmd"
    "github.com/gadflysu/aps/source"
)

type Bounds struct {
    From  *time.Time
    Until *time.Time
}

type Result struct {
    Sessions    []source.Session
    StatusText  string
    StatusIsErr bool
}

type Batch struct {
    Sessions []source.Session
    Err      error
    Done     bool
}

func Complete(cfg cmd.Config, bounds Bounds) (Result, error)
func Stream(cfg cmd.Config, bounds Bounds) <-chan Batch
```

Naming note: use `loader.Complete` and `loader.Stream` rather than `CompleteLoad` and `StreamLoad`.
The package name already supplies the noun.

## Target Flow

```text
main.go
  |
  +-- list mode
  |     |
  |     v
  |   loader.Complete(cfg, bounds)
  |     |
  |     v
  |   runList(result.Sessions, cfg)
  |
  +-- future debug-load mode
  |     |
  |     v
  |   loader.Complete(cfg, bounds)
  |     |
  |     v
  |   dbg.Log("debugLoad: ...")
  |
  +-- interactive mode
        |
        v
      loader.Stream(cfg, bounds)
        |
        v
      picker.RunStreaming(stream, ...)
```

`picker` changes from owning the stream message type to consuming the loader-owned type:

```go
func RunStreaming(stream <-chan loader.Batch, combined bool, cache *source.PIDCache) (*source.Session, error)
```

## Design Decisions

- Keep `loader.Complete` synchronous. It returns a full sorted result and is the stable boundary for
  list mode and benchmark/debug hooks.
- Keep `loader.Stream` incremental. It emits batches and a final `Done` batch for interactive
  startup.
- Do not implement `loader.Complete` by draining `loader.Stream` in the first version. That can be
  revisited if internal helpers make it obviously simpler.
- Keep picker-specific burst coalescing in `picker.streamCmd`; that is consumer behavior, not loader
  behavior.
- Keep watcher setup, PID cache GC, Bubble Tea program creation, launching, and list rendering outside
  `loader`.
- Do not introduce a second batch type.
- Do not introduce `--debug-load` in this refactor; that belongs to issue #59.

## Non-Goals

- Do not change source parsing behavior.
- Do not change list output formatting.
- Do not change interactive key handling or picker rendering.
- Do not change CLI flags.
- Do not add benchmark tooling.
- Do not move watcher, launcher, or preview code.

## Implementation Risks

No blocking issue remains. Watch these risks:

1. Import direction: if `loader` imports `picker`, stop and revise the design.
2. Error behavior drift: preserve today's non-fatal source failure behavior and status text.
3. Date filtering drift: preserve current date filtering for both complete and streaming paths.
4. Debug-log drift: preserve useful existing `interactiveLoad ...` checkpoints unless there is a
   deliberate replacement.

## Implementation Tasks

### Task 1: Create `loader` Types And Move Complete Load

**Files:**
- Create: `loader/loader.go`
- Create: `loader/loader_test.go` if practical
- Modify: `main.go`

Steps:

1. Create `loader/loader.go` with `Bounds`, `Result`, `Batch`, and `Complete`.
2. Move the body of `loadSessions(cfg, from, until)` into `loader.Complete`.
3. Move helper logic needed only by complete loading into `loader`, including source failure summary,
   selected-source loading, date filtering, and final sort.
4. Keep `main.loadSessions` temporarily as a wrapper to reduce routing churn:

```go
func loadSessions(cfg cmd.Config, from, until *time.Time) ([]source.Session, string, bool, error) {
    result, err := loader.Complete(cfg, loader.Bounds{From: from, Until: until})
    return result.Sessions, result.StatusText, result.StatusIsErr, err
}
```

5. Preserve list mode output and error behavior exactly.
6. Run:

```bash
go list ./...
go test ./...
go vet ./...
go build .
go install .
```

### Task 2: Move Batch Ownership From `picker` To `loader`

**Files:**
- Modify: `loader/loader.go`
- Modify: `picker/model.go`

Steps:

1. Remove `type SessionBatch` from `picker/model.go`.
2. Use `loader.Batch` in picker stream fields and functions:

```go
streamCh <-chan loader.Batch
func streamCmd(ch <-chan loader.Batch) tea.Cmd
func RunStreaming(stream <-chan loader.Batch, combined bool, cache *source.PIDCache) (*source.Session, error)
func (m *Model) applySessionBatch(batch loader.Batch)
```

3. Update the `Update` type switch:

```go
case loader.Batch:
    m.applySessionBatch(msg)
```

4. Preserve `streamCmd` behavior: blocking read of the first batch, non-blocking drain of queued
   batches, and return `loader.Batch{Done: true}` when the channel closes.
5. Run:

```bash
go list ./...
go test ./picker ./...
go vet ./...
go build .
go install .
```

### Task 3: Move Interactive Producer Into `loader.Stream`

**Files:**
- Modify: `loader/loader.go`
- Modify: `main.go`

Steps:

1. Move the producer goroutine from `runInteractiveStreaming` into `loader.Stream`.
2. Preserve current source behavior:
   - Claude uses `source.LoadClaudeStream` and can emit one batch per accepted session.
   - Opencode uses `source.LoadOpencode` and emits one batch after that source finishes.
   - Codex uses `source.LoadCodex` and emits one batch after that source finishes.
3. Preserve final completion behavior:

```go
stream <- loader.Batch{Done: true}
```

or, for non-fatal source failures:

```go
stream <- loader.Batch{Err: errors.New(msg), Done: true}
```

4. Keep PID cache GC and `picker.RunStreaming` setup in `main.go`.
5. Keep watcher creation inside `picker.RunStreaming`.
6. Run:

```bash
go list ./...
go test ./...
go vet ./...
go build .
go install .
```

### Task 4: Simplify `main.go` Routing

**Files:**
- Modify: `main.go`

Steps:

1. Replace list mode's temporary `loadSessions` wrapper with direct `loader.Complete`.
2. Delete `loadSessions` from `main.go`.
3. Make mode routing read as:

```text
if list mode:
    loader.Complete -> runList
else:
    loader.Stream -> picker.RunStreaming -> launch
```

4. Keep date parsing, debug-log setup, no-session handling, launch validation, and launch dispatch in
   `main.go`.
5. Run:

```bash
go list ./...
go test ./...
go vet ./...
go build .
go install .
```

## Verification

Manual smoke after automated checks:

```bash
go list ./...
./aps -l --color=never
./aps -c -l --color=never
./aps --debug-log .worktrees/load-refactor.log
rg 'interactiveLoad start|interactiveLoad done|interactiveLoad batch' .worktrees/load-refactor.log
```

Expected:

- `go list ./...` reports no import cycle.
- List mode still prints the same rows.
- Interactive mode still starts before all sessions finish loading.
- Debug log still contains interactive streaming checkpoints.
- No new CLI flags are introduced by this refactor.

## Relationship To Issue #59

This refactor should land before issue #59. After this refactor exists, issue #59 should inspect the
current load lifecycle code and wire `--debug-load=complete` and `--debug-load=stream` to the
corresponding boundaries, without depending on ad hoc `main.go` helpers.

## Engineering Philosophy

Unify the lifecycle boundary, not the caller shape. Synchronous callers should receive a complete
result; streaming callers should receive batches. Shared orchestration belongs in `loader`, while
UI-specific consumption stays in `picker`.
