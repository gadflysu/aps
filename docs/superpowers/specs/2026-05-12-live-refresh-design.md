# Live Refresh Design

**Date:** 2026-05-12
**Scope:** `watcher` package (new), `source` package (new `ReloadSession`), `picker` package (state machine + bubbletea integration)

---

## Problem

`source.LoadClaude` runs once at startup. While Claude Code is running, the corresponding `.jsonl` file is continuously written: `MsgCount`, `Time` (mtime), and `custom-title` all change. The TUI has no awareness of these changes.

---

## Goals

- Session list (turns, title, sort order) reflects live state while the TUI is open.
- At most one refresh per second (rate-limit, not debounce).
- No disruption when the user is in preview mode (`stateListPreview`).
- Cursor stays anchored to the same session by ID across refreshes.
- Zero impact on `main.go` public API.

---

## Architecture

### New package: `watcher`

Single responsibility: watch `~/.claude/projects/` for JSONL changes, emit batches of changed paths.

```
watcher/
  watcher.go
  watcher_test.go
```

**Two detection paths share one downstream channel:**

1. **FSNotify (primary):** registers root dir + all existing project subdirs (depth-1 only). On root CREATE event for a new project dir, dynamically calls `watcher.Add()` on it. Filters events to `*.jsonl` files only.
2. **Stat-only fallback poll (5s interval):** scans all known JSONL paths with `os.Stat`, emits paths whose mtime changed. Compensates for any fsnotify misses.

**Rate-limit (1s):** first event in a quiet period fires immediately; subsequent events within the 1s cooldown accumulate into a pending set; when the cooldown timer fires, the pending set is emitted and the timer resets. Guarantees ≤ 1 refresh/second.

### `source` package addition

```go
// ReloadSession re-parses a single JSONL file.
// Does not change LoadClaude signature.
func ReloadSession(jsonlPath string) (Session, error)
```

### `picker` package changes

**New fields on `Model`:**

```go
pendingRefresh []string           // paths buffered while in preview mode
mtimes         map[string]time.Time // path → last known mtime for fallback poll
watcher        *watcher.Watcher
```

**New message type:**

```go
type RefreshMsg struct{ Paths []string }
```

**`Update()` new branch:**

- `state == stateListPreview` → append paths to `pendingRefresh`, no UI change
- `state == stateList` → call `applyRefresh(paths)`

**`applyRefresh(paths)`:**
1. For each path call `source.ReloadSession`; update matching session by ID; append if new.
2. Re-sort by `Time` descending.
3. Locate original cursor session by ID; if gone, cursor = 0.

**Exiting preview:** on `Space` key switching to `stateList`, if `pendingRefresh` is non-empty, call `applyRefresh(pendingRefresh)` and clear it.

**bubbletea integration:**

```go
func waitForRefresh(ch <-chan []string) tea.Cmd {
    return func() tea.Msg { return RefreshMsg{Paths: <-ch} }
}
```

`Init()` returns `waitForRefresh(w.C())`. Each `Update()` handling a `RefreshMsg` returns it again to keep the loop alive.

**Lifecycle:**

```
picker.Run()
  watcher.New(baseDir)     // start watching
  newModel(sessions, w)    // model holds watcher
  tea.NewProgram(m).Run()
  w.Stop()                 // cleanup after TUI exits
```

---

## Error Handling

| Scenario | Behavior |
|---|---|
| `watcher.New()` fails | degrade to 5s poll-only; TUI starts normally |
| `ReloadSession()` parse error | keep existing session data, skip silently |
| JSONL deleted before refresh | stat error → skip path, keep session in list |
| `watcher.Add()` fails for new dir | log to stderr (`-v`), fallback poll compensates |
| Channel backpressure | buffered channel size=1; rate-limit prevents sender blocking |

---

## Testing

**`watcher` package**

- `TestRateLimit`: fire 10 rapid events, assert only 1 emission within 1s, then 1 more after cooldown (merged).
- `TestNewProjectDir`: create new project subdir, assert it gets auto-registered and subsequent JSONL events are received.
- `TestFallbackPoll`: disable fsnotify path, mutate file mtime, assert 5s poll detects change.

**`source` package**

- `TestReloadSession`: write new content to a temp JSONL, call `ReloadSession`, assert `Title`, `MsgCount`, `Time` updated.

**`picker` package**

- `TestApplyRefresh_CursorAnchor`: prepend new session to list, assert cursor still points to original session by ID.
- `TestApplyRefresh_PendingInPreview`: send `RefreshMsg` while `state == stateListPreview`, assert `pendingRefresh` accumulates; switch to `stateList`, assert refresh applied.
