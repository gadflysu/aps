# Plan: Issue #38 — Picker Bottom Status Bar

## Goal

Add one compact bottom status bar to picker mode so the TUI can show transient operational state without writing directly to stderr while Bubble Tea owns the alt screen.

## Current State

- Picker mode renders search input, column header, list rows, and optional preview pane.
- There is no dedicated status line for loading progress, non-fatal loading errors, or no-session state.
- `--debug-log` already exists for detailed diagnostics when explicitly enabled by the user.

## Design

- Reserve one terminal row at the bottom of picker mode for status text in both list and preview states.
- Keep the status bar non-modal; it must not block typing, navigation, preview toggling, or selection.
- Render the status row after the main picker body, not inside the left list pane, so preview mode has one full-width bottom row.
- Use concise text only. Examples:
  - `Loading sessions... 42 loaded`
  - `Claude load failed; showing Opencode sessions`
  - `No sessions found.`
- Treat #38 as status-bar infrastructure. Streaming loading progress can remain a future #9 integration unless #9 is implemented in the same branch.
- Keep detailed diagnostics out of the status bar. Write details to `--debug-log` only when the user enabled it.
- Do not create a default OS log path or hidden temp log file.
- Fatal errors that exit picker mode may continue to print to stderr after Bubble Tea exits alt screen.

## Proposed Model

- Add a small picker-owned status state, for example:

  ```go
  type statusLevel int

  const (
      statusMuted statusLevel = iota
      statusError
  )

  type statusLine struct {
      text  string
      level statusLevel
  }
  ```

- Add `status statusLine` to `Model`.
- Keep the initial status empty for normal non-streaming startup with sessions loaded.
- Set an empty-result status when `len(m.sessions) == 0`, e.g. `No sessions found.`
- Allow future loading code to update the same field with progress/error text; do not introduce a default log file or background loader in #38.
- Prefer methods over exported fields:
  - `func (m Model) renderStatusBar() string`
  - `func (m Model) listHeight() int`
  - `func (m Model) bodyHeight() int`

## Render Flow

- Add `const statusBarHeight = 1`.
- Use one shared helper for available list rows:

  ```go
  func (m Model) listHeight() int {
      h := m.height - headerHeight - statusBarHeight
      if h < 0 {
          return 0
      }
      return h
  }
  ```

- Replace direct `m.height - headerHeight` calculations in `renderList()` and `scrollableWidth()` with `m.listHeight()`.
- In `View()`:
  - Build `mainBody` as the current list-only or list+preview layout.
  - Return `lipgloss.JoinVertical(lipgloss.Top, mainBody, m.renderStatusBar())`.
  - Keep the `Terminal too small` early return unchanged; do not append a status row to the too-small message.
- In preview mode, keep `lipgloss.JoinHorizontal` for the main body and append the status row below it.
- In `updatePreviewHeights()`, subtract `statusBarHeight` from the available preview height before assigning message/directory viewport heights.

## Files To Change

| File | Change |
|------|--------|
| `picker/model.go` | Add status state, shared height helpers, status rendering, and preview-height accounting |
| `picker/model_test.go` | Cover status rendering, list/preview height accounting, empty-state text, and width truncation |
| `picker/styles.go` | Add or reuse ANSI 16-color styles for muted/status/error text |

## Layout Constraints

- Always subtract one row from picker body height; do not let status visibility change list height.
- Avoid shifting the UI height when status text appears; reserve the row consistently in picker mode.
- Keep status text width-safe by using `display.TruncateWidth(text, m.width, "")` before styling.
- Preserve preview layout and horizontal scrolling behavior.
- Preserve existing `minHeight` handling unless implementation proves the minimum must increase.
- Preserve existing ANSI 16-color palette; do not add hex/RGB colors.
- Do not route status rendering through `preview` package; status belongs to picker layout state.

## TDD Tests

Write or update tests before implementation:

| Test | What it proves |
|------|----------------|
| `TestRenderStatusBar_Empty` | Empty status still reserves exactly one row without noisy text |
| `TestRenderStatusBar_Error` | Non-fatal error status renders concise text with error styling |
| `TestNewModel_NoSessionsStatus` | Empty session input initializes a no-session status |
| `TestRenderList_ReservesStatusRow` | `renderList()` uses `m.height - headerHeight - statusBarHeight` |
| `TestScrollableWidth_UsesStatusAdjustedListHeight` | horizontal-scroll width calculation uses the same visible range as `renderList()` |
| `TestRenderPreview_ReservesStatusRow` | preview viewport heights subtract the status row |
| `TestView_AppendsStatusBelowPreview` | preview mode renders one full-width status row below the joined list/preview body |
| `TestRenderStatusBar_TruncatesToWidth` | long status text does not exceed terminal width |

## Implementation Order

1. Add failing tests for the status renderer and height helpers.
2. Add `statusBarHeight`, status types, `Model.status`, `renderStatusBar()`, and height helpers.
3. Replace duplicated height math in list rendering, scroll width calculation, and preview viewport sizing.
4. Update `View()` to append the status row below the main body.
5. Add empty-session status initialization in `newModel()` or a small helper called by `newModel()`.
6. Run focused picker tests, then full verification.

## Acceptance Criteria

- `go test ./...` passes.
- `go vet ./...` passes.
- `go build .` passes, then `go install .` is run immediately.
- Manual smoke test: picker shows a stable bottom status row in list and preview modes without corrupting alt-screen rendering.
- Manual smoke test: toggling preview with `Space` does not move the status row into the left pane.
- Manual smoke test: a long status message truncates within terminal width.

## Non-Goals

- Do not implement streaming session loading in #38.
- Do not add CLI flags for status behavior.
- Do not write status messages to stderr while Bubble Tea owns the alt screen.
- Do not add a default `--debug-log` path, temp log, or persistent status history.

## Relationship To #9

#9 should use this status bar for streaming loading progress and concise non-fatal loading errors. Detailed error content remains in `--debug-log` when enabled.
