# Preview Live Refresh — Design Spec

**Date:** 2026-05-13  
**Status:** Approved

## Goal

When the user has the preview pane open (`stateListPreview`) and the JSONL file
for the currently selected session changes on disk, the SESSION INFO and RECENT
MESSAGES viewports refresh immediately — without requiring the user to close and
reopen the preview pane.

## Scope

Minimum-change approach (Option C): only the currently selected session's path
triggers an immediate preview reload. All other changed paths continue to be
buffered in `pendingRefresh` as before, and are applied when the user closes the
preview pane.

## Architecture

Single file changed: `picker/model.go`.

### New helper: `splitPaths`

```go
func (m *Model) splitPaths(paths []string) (hot, cold []string)
```

Iterates `paths`. A path is **hot** (current session) if:
- The current session is a Claude session and
  `filepath.Base(path) == currentSession.ID + ".jsonl"`, OR
- The path equals the session's `ProjectPath` joined with the ID (same check,
  full path comparison for safety).

All other paths are **cold**.

Returns early with all-cold if `m.filtered` is empty or `m.cursor` is out of range.

### Modified: `RefreshMsg` handler

```
Before:
  if stateListPreview → buffer all paths in pendingRefresh

After:
  if stateListPreview:
    hot, cold = splitPaths(paths)
    if len(hot) > 0:
      applyRefresh(hot)   ← updates m.sessions + m.activeConfs
      loadPreview()       ← re-renders the three viewports
    pendingRefresh += cold
  else:
    applyRefresh(paths)   ← unchanged
```

`applyRefresh` already re-anchors the cursor by ID, so the selected row stays
stable after the data update.

## Data Flow

```
fsnotify/poll → watcher.C() → waitForRefresh() → RefreshMsg{Paths}
                                                        ↓
                                              splitPaths(paths)
                                             /              \
                                           hot             cold
                                            ↓                ↓
                                      applyRefresh    pendingRefresh
                                      loadPreview()
                                            ↓
                                    viewports updated
                                    (user sees new content)
```

## Error Handling

No new error surface. `applyRefresh` already silently skips paths that fail
`source.ReloadSession`. `loadPreview` is already safe to call on an empty
filtered list.

## Testing

New unit test in `picker/model_test.go`:

- **`TestLiveRefreshUpdatesPreview`**: construct a Model in `stateListPreview`
  with a known cursor session; send a `RefreshMsg` whose path matches the cursor
  session's JSONL; assert that `vpMsgs` content changed and `pendingRefresh` is
  empty.
- **`TestLiveRefreshBuffersOtherPaths`**: send a `RefreshMsg` with a path for a
  *different* session; assert that `pendingRefresh` has one entry and viewports
  are unchanged.
- **`TestSplitPaths`**: pure unit test for the helper covering hot/cold/empty cases.

## Non-Goals

- Refreshing the list column order or other sessions' rows while preview is open
  (that is Option A, explicitly deferred).
- Opencode live refresh (Opencode sessions have no JSONL; `splitPaths` returns
  all-cold for them, preserving existing behaviour).
