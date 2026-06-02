# Plan: Issue #38 — Picker Bottom Status Bar

## Goal

Add one compact bottom status bar to picker mode so the TUI can show transient operational state without writing directly to stderr while Bubble Tea owns the alt screen.

## Current State

- Picker mode renders search input, column header, list rows, and optional preview pane.
- There is no dedicated status line for loading progress, non-fatal loading errors, or no-session state.
- `--debug-log` already exists for detailed diagnostics when explicitly enabled by the user.

## Design

- Reserve one terminal row at the bottom of picker mode for status text.
- Keep the status bar non-modal; it must not block typing, navigation, preview toggling, or selection.
- Use concise text only. Examples:
  - `Loading sessions... 42 loaded`
  - `Claude load failed; showing Opencode sessions`
  - `No sessions found.`
- Keep detailed diagnostics out of the status bar. Write details to `--debug-log` only when the user enabled it.
- Do not create a default OS log path or hidden temp log file.
- Fatal errors that exit picker mode may continue to print to stderr after Bubble Tea exits alt screen.

## Files To Change

| File | Change |
|------|--------|
| `picker/model.go` | Add status fields and render one bottom status row in both list and preview layouts |
| `picker/model_test.go` | Cover status rendering, height accounting, and empty/loading/error text |
| `picker/styles.go` | Add or reuse ANSI 16-color styles for muted/status/error text |

## Layout Constraints

- Subtract one row from the list viewport height when the status bar is visible or reserved.
- Avoid shifting the UI height when status text appears; reserve the row consistently in picker mode.
- Keep text width-safe by truncating to terminal width.
- Preserve preview layout and horizontal scrolling behavior.

## TDD Tests

Write or update tests before implementation:

| Test | What it proves |
|------|----------------|
| `TestRenderStatusBar_Loading` | Loading status renders compact progress text |
| `TestRenderStatusBar_Error` | Non-fatal error status renders concise text |
| `TestRenderList_ReservesStatusRow` | List height accounts for the bottom status row |
| `TestRenderPreview_ReservesStatusRow` | Preview layout accounts for the bottom status row |
| `TestRenderStatusBar_TruncatesToWidth` | Long status text does not overflow terminal width |

## Acceptance Criteria

- `go test ./...` passes.
- `go vet ./...` passes.
- `go build .` passes, then `go install .` is run immediately.
- Manual smoke test: picker shows a stable bottom status row in list and preview modes without corrupting alt-screen rendering.

## Relationship To #9

#9 should use this status bar for streaming loading progress and concise non-fatal loading errors. Detailed error content remains in `--debug-log` when enabled.
