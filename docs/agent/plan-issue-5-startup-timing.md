# Plan: Issue #5 — Startup Timing Checkpoints

## Goal

Add per-phase `dbg.Log` timestamps behind `--debug-log` so startup latency can be measured precisely. No user-visible behaviour change.

## Branch

`feat/5-startup-timing`

## Files to change

| File | Change |
|------|--------|
| `main.go` | Record `t0 := time.Now()` before `loadSessions`; log `"loadSessions: %v (%d sessions)"` after |
| `picker/model.go` | In `Run()`, log `"picker.Run start"` before `tea.NewProgram`; in `View()`, log `"first View()"` on first call only (guard with a bool field on `Model`) |

## Implementation steps

1. `main.go`: add `time` import; wrap `loadSessions` with timing; emit via `dbg.Log`
2. `picker/model.go`: add `firstViewDone bool` field to `Model`; log in `Run()` and `View()`
3. Tests: no new tests needed — `dbg.Log` is a no-op when logger is nil; existing tests are unaffected

## Acceptance criteria

- `aps --debug-log /tmp/a.log` then `grep -c "first View" /tmp/a.log` outputs `1`
- All existing tests pass (`go test ./...`)
- No change in behaviour when `--debug-log` is not set

## GitHub workflow

- Branch: `feat/5-startup-timing`
- Commit body: `Closes #5`
- PR: `gh pr create` with `Closes #5`
- Merge: `gh pr merge --rebase`
