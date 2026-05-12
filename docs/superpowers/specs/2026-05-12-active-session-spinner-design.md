# Active Session Spinner — Design Spec

**Date:** 2026-05-12
**Status:** Approved

## Problem

The picker list shows sessions sorted by recency but gives no signal about which
sessions are currently in use. Users with multiple simultaneous claude sessions
cannot tell at a glance which ones are live.

## Solution

Display an animated spinner glyph (palindrome character sequence, matching Claude
Code's own spinner) in a fixed 2-character column to the left of the TIME column,
for sessions that are detected as active. Sessions with no active process show blank
space in that column. The spinner always runs while the picker is open.

## Detection Logic (`source/active.go`)

Function signature:

```go
func DetectActive(sessions []Session) map[string]bool
```

Steps:

1. **Collect process CWDs** — run `ps aux`, for each process whose command contains
   `claude` or `opencode`, call `lsof -p <pid>` and extract the `cwd` entry.
   Result: a `map[string]bool` of absolute CWD paths.

2. **Compute today's midnight** — `time.Now()` truncated to calendar day in local time.

3. **Per-session check:**
   - **Claude:** `Session.CWD` must be in the process CWD set AND the session's
     `.jsonl` mtime must be >= today's midnight. (Note: `Session.CWD` is the
     working directory stored in the JSONL, which may be a worktree path; this
     matches the process CWD directly without going through `ProjectPath`.)
   - **Opencode:** `Session.CWD` must be in the process CWD set AND
     `Session.Time` (time_updated) must be >= today's midnight.

4. Return `map[string]bool` keyed by `session.ID`. True = active.

**JSONL mtime access:** `Session` gains an unexported `jsonlPath string` field
(set by `LoadClaude`, invisible outside `source` package). `DetectActive` uses it
to `os.Stat` the file without re-scanning the directory.

**Failure handling:** if `ps` or `lsof` fails, return empty map silently. The picker
starts normally with no active markers rather than failing.

## Picker Changes

**`picker.Model` new field:**
```go
activeIDs map[string]bool
```

**`newModel()` — compute once at startup:**
```go
activeIDs: source.DetectActive(sessions),
```

**`renderRowFull` — replace demo index check with map lookup:**
```go
// before: idx >= 0 && idx < 3
// after:
if m.activeIDs[s.ID] {
    // render spinner glyph
}
```

**`spinTick`** — unchanged, always runs at 150ms regardless of activeIDs contents.

**`visibleIndex`** — removed once activeIDs replaces the demo logic.

## Unchanged

- `source.Session` exported fields — no new exported fields
- List mode (`-l`) — no spinner, plain text output
- `spinnerFrames` palindrome sequence — already in place from demo

## Constraints

- Active state is computed once at picker startup and does not update while picker
  is open. This is intentional: live refresh (which will update activeIDs on each
  reload cycle) is the next feature after this one.
- macOS only: `lsof` CWD detection. Linux `lsof` syntax is compatible; Windows
  is not supported (no `lsof`).

## Future: Live Refresh Integration

When live refresh is implemented, `DetectActive` will be called as part of the
periodic session reload (target interval ~10s), replacing the static startup call.
The `checkTick` pattern is intentionally avoided to prevent duplicate polling.
