# Plan: Issue #9 — Streaming UI

## Goal

Render the interactive picker before all sessions have finished loading. The TUI should appear with a loading state, accept the first available session batch, remain interactive, and continue receiving sessions until loading completes.

This is the remaining open subtask for #4 after #5, #6, #7, and #8.

## Current State

- `main.go` calls `loadSessions(cfg, from, until)` before `picker.Run(...)`.
- `loadSessions` already loads Claude and Opencode concurrently, then sorts and date-filters the combined slice.
- `source.LoadClaude` already uses a bounded worker pool plus `MetaCache`, but it still returns a complete `[]Session` synchronously.
- `picker.Model` assumes an initial complete session slice and computes column widths, fuzzy matches, active-session state, and watcher state from that slice.
- `picker.View()` logs `first View()`, which can be used to prove whether the first frame happens before loading completes.
- Picker mode does not yet have a dedicated status surface; #38 adds a bottom status bar for loading progress and concise non-fatal errors.

## Approved Direction

Keep package boundaries intact:

- `source` owns discovery and parsing.
- `main` owns CLI-driven source selection, date filtering, source orchestration, and error reporting policy.
- `picker` owns interactive state only; it consumes session batches and renders loading/done/error states.

Do not make `picker.Run` directly call `source.LoadClaude`; that would make the TUI package responsible for CLI loading policy.

## Design

### 1. Preserve Blocking List Mode

Keep `loadSessions(cfg, from, until)` for list mode and any tests that expect a complete sorted slice. The `-l` path should remain deterministic and should continue to print nothing until the full table is ready.

### 2. Add a Streaming Loader for Interactive Mode

Add a `main`-level streaming orchestration path for interactive mode, for example:

```go
type sessionLoadEvent struct {
    Sessions []source.Session
    Err      error
}
```

The loader should:

- start selected sources according to `cfg.Claude`, `cfg.Opencode`, and `cfg.All`,
- apply `strictMatch` and date filters before emitting sessions,
- send Claude sessions incrementally as they are parsed,
- send Opencode sessions as one batch unless a concrete Opencode bottleneck appears,
- send non-fatal source errors as events so picker can show concise status,
- write detailed error diagnostics to `dbg.Log` only when `--debug-log` is enabled,
- close the event channel only after every selected source has completed.

Channel ownership contract: the `main`-level loader is the only producer and is responsible for closing the channel. Picker code must never close it. `streamCmd` maps a closed channel to a picker message with `Done=true`.

### 3. Add a Claude Streaming Source API Without Breaking `LoadClaude`

Do not add an "optional" parameter to `LoadClaude`; Go has no true optional parameters, and changing the signature would churn existing tests and callers.

Prefer an internal shared implementation:

```go
func LoadClaude(pathFilter string, strictMatch bool, verbose bool) ([]Session, error) {
    return loadClaude(pathFilter, strictMatch, verbose, nil)
}

func LoadClaudeStream(pathFilter string, strictMatch bool, verbose bool, emit func(Session)) error {
    _, err := loadClaude(pathFilter, strictMatch, verbose, emit)
    return err
}
```

`loadClaude` should still save `MetaCache` after workers complete. When `emit != nil`, each accepted session can be emitted from the worker path as soon as it is parsed or cache-hit. Guard shared cache and slice writes with the existing synchronization discipline.

### 4. Let Picker Consume Batches

Add an exported picker input type if needed so `main` can pass a stream without importing Bubble Tea internals into `main`, for example:

```go
type SessionBatch struct {
    Sessions []source.Session
    Err      error
    Done     bool
}
```

Add `picker.RunStreaming(stream <-chan SessionBatch, combined bool, cache *source.PIDCache)` or extend `Run` carefully with a clearly named variant. The model should start with an empty session slice and a `loading` flag.

Inside picker:

- `Init()` should issue a `streamCmd` that drains up to a small batch size or deadline.
- `Update()` should upsert each batch into `m.sessions`, sort by `Time` descending, recompute `filtered`, clamp cursor, recompute adaptive ID/message widths, update max horizontal offset, and refresh active-session confidence from the existing proc snapshot.
- When `Done` arrives, set `loading = false`.
- If loading is done and no sessions exist, show a no-sessions state rather than quitting before the user sees the picker.
- Keep Enter disabled when `m.filtered` is empty.
- Use #38's bottom status bar for loading progress and concise non-fatal error messages. Fatal errors that exit picker mode can still print to stderr after Bubble Tea exits alt screen.

### 5. Use ID-Based Upsert For Stream And Refresh

Initial streaming and watcher refresh can both update the same session while the picker is running. Avoid duplicate rows and stale overwrite bugs by adding one merge helper that upserts by `(Client, ID)`:

- if the key exists, replace the existing session;
- if the key does not exist, insert the new session;
- after every upsert batch, sort by `Time` descending, reapply the current query, clamp cursor, and update column widths.

Use this helper for both streaming batches and watcher refresh updates.

### 6. Add Debug Checkpoints For Verification

Add debug checkpoints that make the streaming acceptance criteria measurable:

- `interactiveLoad start`
- `interactiveLoad batch: <n> sessions`
- `interactiveLoad done: <n> sessions`

With `--debug-log`, `first View()` must appear before `interactiveLoad done`.

## Files To Change

| File | Change |
|------|--------|
| `main.go` | Split list-mode blocking load from interactive streaming load; pass stream into picker; move interactive empty-result handling into picker |
| `source/claude.go` | Add shared `loadClaude` helper and `LoadClaudeStream` entry point; emit accepted sessions incrementally |
| `picker/model.go` | Add stream batch message/command, loading/done/error state, ID-based upsert helper, width/filter/active-state refresh after batches |
| `picker/model_test.go` | Cover stream command batching, upsert behavior, cursor clamping, loading/no-session/status rendering |
| `source/claude_test.go` | Cover `LoadClaude` unchanged behavior and streaming emission equivalence |
| `main` tests, if present/added | Cover list mode remains blocking and interactive mode uses streaming path |
| `docs/agent/plan-issue-38-picker-status-bar.md` | Coordinate status rendering expectations if #38 is not merged before #9 starts |

## TDD Tests

Write or update tests before implementation:

| Test | What it proves |
|------|----------------|
| `TestLoadClaudeStream_EmitsSameSessionsAsLoadClaude` | Streaming and blocking Claude loaders produce the same set of sessions |
| `TestLoadClaude_BlockingAPIUnchanged` | Existing `LoadClaude` behavior and sort order remain intact |
| `TestStreamCmd_DrainsByBatchSize` | Picker stream command returns after the configured batch size |
| `TestStreamCmd_DrainsByDeadline` | Picker stream command returns partial batches without waiting for stream completion |
| `TestStreamCmd_ClosedChannelReturnsDone` | Picker maps producer channel close to `Done=true` |
| `TestApplySessionBatch_UpsertsByClientID` | Stream and refresh updates replace existing `(Client, ID)` rows instead of appending duplicates |
| `TestApplySessionBatch_MergeSortsAndClampsCursor` | Batch merge keeps newest-first order and valid cursor |
| `TestApplySessionBatch_RecomputesWidthsAndFilter` | New sessions update adaptive columns and current query results |
| `TestLoadingEmptyState` | Empty picker shows loading while pending and no-sessions only after done |
| `TestNonFatalLoadError_StatusAndLog` | Non-fatal load errors update picker status and detailed debug log only when enabled |

## Acceptance Criteria

- `go test ./...` passes.
- `go vet ./...` passes.
- `go build .` passes, then `go install .` is run immediately.
- `aps --debug-log <safe-workspace-path>` shows `first View()` before `interactiveLoad done`.
- Manual smoke test: interactive TUI appears with partial results, accepts typing/navigation while more sessions arrive, then reaches a stable sorted list.
- Manual combined-mode smoke test: `-a` shows both Claude and Opencode sessions through the same streaming UI path.
- Manual error smoke test: a non-fatal source error appears as concise picker status and detailed content appears only in `--debug-log` when enabled.

## Non-Goals

- Do not change list-mode output behavior.
- Do not add preview caching or async preview loading.
- Do not add new dependencies unless implementation proves the standard library path is inadequate.
- Do not change direct launch behavior after a session is selected.
- Do not add default OS logging or hidden temp log files.

## Branch And GitHub Workflow

- Branch: `feat/9-streaming-ui` from current `master`.
- First commit: docs plan, if not already committed on the issue branch.
- Implementation commits should include `Closes #9` in the body.
- PR body should include `Closes #9`.
- Prefer merging #38 first so #9 can reuse the bottom status bar for loading and non-fatal error status.
- After merge, verify whether #4 can be closed because #9 is the last remaining sub-issue.
