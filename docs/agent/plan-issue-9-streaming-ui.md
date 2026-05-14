# Plan: Issue #9 — Streaming UI

## Goal

Deliver the first batch of parsed sessions to bubbletea immediately as workers finish, rather than blocking until all files are parsed. The TUI appears and is interactive with partial data; remaining sessions stream in as `tea.Msg` updates.

**Merge Issue #8 before starting this branch.**

## Branch

`feat/9-streaming-ui` (branch from master after #8 is merged)

## Design

`LoadClaude` grows an optional `results chan<- Session` parameter. When non-nil, each worker sends the parsed session to the channel immediately instead of appending to the slice. The caller (`picker.Run`) owns the channel and forwards batches to bubbletea via `tea.Cmd`.

New bubbletea message type:

```go
// SessionBatchMsg carries a batch of newly loaded sessions.
type SessionBatchMsg struct {
    Sessions []source.Session
}
```

`picker.Run` spawns a `tea.Cmd` goroutine that reads from the channel in small batches (drain up to N or until 50 ms elapses, whichever comes first) and returns a `SessionBatchMsg`. `Update` handles the message: merges new sessions into `m.sessions`, re-sorts, and issues another read `tea.Cmd` until the channel is closed.

## Files to change

| File | Change |
|------|--------|
| `source/claude.go` | Add optional `results chan<- Session` param to `LoadClaude`; send to channel in worker goroutine when non-nil |
| `main.go` | Pass `nil` channel (list mode and `-l` path unchanged) |
| `picker/model.go` | Add `SessionBatchMsg`; add `streamCmd` that reads a batch from channel; handle `SessionBatchMsg` in `Update`; pass channel into `LoadClaude` from `Run` |

## Key constraints

- `LoadClaude` signature change must remain backward-compatible: `nil` channel = current blocking behaviour (used by list mode and tests)
- Sort order (`Time` descending) must be maintained after each batch merge in `Update`
- Existing tests pass `nil` — no test changes required for `LoadClaude` itself
- First render must not block: `Init()` issues the first `streamCmd` immediately

## Batch drain logic

```go
func streamCmd(ch <-chan source.Session) tea.Cmd {
    return func() tea.Msg {
        var batch []source.Session
        deadline := time.After(50 * time.Millisecond)
        for {
            select {
            case s, ok := <-ch:
                if !ok {
                    return SessionBatchMsg{Sessions: batch, Done: true}
                }
                batch = append(batch, s)
                if len(batch) >= 20 {
                    return SessionBatchMsg{Sessions: batch}
                }
            case <-deadline:
                return SessionBatchMsg{Sessions: batch}
            }
        }
    }
}
```

## Tests (TDD — write tests first)

| Test | What it checks |
|------|---------------|
| `TestSessionBatchMsg_MergeSort` | After receiving a batch, `m.sessions` is sorted by `Time` descending |
| `TestStreamCmd_DrainsBatch` | `streamCmd` returns after 20 items without waiting for deadline |
| `TestStreamCmd_DrainsByDeadline` | `streamCmd` returns after 50 ms with partial batch |
| `TestLoadClaude_NilChannel` | `nil` channel = blocking return, result identical to current behaviour |

## Acceptance criteria

- All new and existing tests pass (`go test ./...`)
- `aps --debug-log /tmp/a.log`: `"first View()"` timestamp appears before `"loadSessions"` completes
- `go vet ./...` clean
- Manual smoke test: TUI appears with partial list, remaining sessions populate within ~200 ms

## GitHub workflow

- Branch: `feat/9-streaming-ui`
- Commit body: `Closes #9`
- PR: `gh pr create` with `Closes #9`
- Merge after #8: `gh pr merge --rebase`
