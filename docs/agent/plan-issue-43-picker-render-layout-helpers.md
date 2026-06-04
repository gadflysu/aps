# Plan: Issue #43 — Picker Render Layout Helpers

## Goal

Reduce picker layout coupling by moving search/header/list/preview/status assembly out of the monolithic `Model.View()` body into small picker-internal helpers.

## Current State

- `picker.Model.View()` directly builds the search row, column header, list body, preview body, and status row.
- Recent #38 work changed search spacing, status bar placement, terminal-height accounting, and textinput placeholder behavior in the same local region.
- Charm/Bubble Tea already supplies reusable widgets for `textinput` and `viewport`; this task should improve local composition, not introduce a new UI framework.

## Dependency

- Execute after #38 lands, or start from a branch that already contains #38.
- Keep this task behavior-preserving. If a visible layout bug is found, fix it in a separate commit with a targeted test.

## Files To Change

| File | Change |
|------|--------|
| `picker/model.go` | Extract render/layout helpers from `View()` and replace duplicated height math with named helpers |
| `picker/model_test.go` | Add/adjust tests around helper behavior and full-view line accounting |

## Proposed Helpers

- `func (m Model) renderSearchBar() string`
  - Owns the `> ` prefix, textinput view, placeholder behavior, and trailing newline.
- `func (m Model) renderHeader() string`
  - Wraps `renderColumnHeader()` and owns its trailing newline.
- `func (m Model) bodyHeight() int`
  - Returns terminal height minus header/search rows and status row.
- `func (m Model) listHeight() int`
  - Returns the number of list rows available for `renderList()` and `scrollableWidth()`.
- `func (m Model) renderListBody() string`
  - Pads/trims list content consistently before status rendering.
- `func (m Model) renderMainBody() string`
  - Owns list-only versus preview-mode horizontal composition.
- Keep `renderStatusBar()` focused on status text/hints only.

## Implementation Notes

- Preserve the current output exactly for list mode and preview mode.
- Keep `Terminal too small` as an early return without appending status UI.
- Do not move preview section rendering into `picker`; `preview` still owns section content, while `picker` owns layout assembly.
- Do not introduce a generic component interface unless repeated helper signatures prove it necessary during implementation.
- Keep ANSI 16-color style constraints and existing CJK truncation helpers.

## TDD Tests

Write or update tests before refactoring:

| Test | What it proves |
|------|----------------|
| `TestRenderSearchBar_Placeholder` | Empty query renders the existing placeholder text and one trailing newline |
| `TestRenderHeader_OneLine` | Header helper renders exactly one header row plus one trailing newline |
| `TestView_ListModeLineCount` | List mode still renders exactly terminal height when enough rows exist |
| `TestView_PreviewModeStatusPlacement` | Preview mode keeps status below the joined main body |
| `TestListHeight_SharedByRenderAndScroll` | `renderList()` and `scrollableWidth()` use the same height helper |

## Acceptance Criteria

- `go test ./picker -count=1` passes.
- `go test ./...` passes.
- `go vet ./...` passes.
- `go build .` passes, then `go install .` is run immediately.
- Manual smoke test in a fixed-size tmux pane confirms:
  - list mode line count fills the terminal
  - preview mode keeps search/header/list/preview/status aligned
  - search placeholder and status bar behavior from #38 remain unchanged

## Non-Goals

- Do not replace Bubble Tea with a different component framework.
- Do not adopt `bubbles/list` for the session table.
- Do not change key bindings, filtering behavior, active-session indicators, or preview content.
- Do not broaden this into streaming startup work.
