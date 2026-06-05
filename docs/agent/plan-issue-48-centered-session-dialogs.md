# Plan: Issue #48 - Centered Session Action Dialogs

## Goal

Add a centered session action dialog to interactive picker mode.

The first user-visible behavior is a resume confirmation dialog: when the user presses Enter on a
resumable session, aps shows a centered dialog with the selected session metadata and only returns the
chosen session after confirmation.

This creates the dialog foundation needed by #45, where history-only or cleaned Claude sessions can
open a non-resumable explanation dialog instead of launching.

## Current State

- `picker.Model` owns filtering, cursor movement, preview toggling, active-session refresh, and final
  selection.
- Pressing Enter sets `m.chosen` and quits immediately.
- `launcher` prints launch/resume text and then uses `syscall.Exec` to replace the current process.
- #38 adds a bottom status bar that must remain the last terminal row.
- Preview info already shows rich metadata, but `source.Session` does not expose a unified `DataPath`
  for picker-owned UI. Claude keeps `jsonlPath` unexported, while Opencode/Codex data paths are passed
  directly to preview renderers.
- There is no reusable picker dialog state or centered modal renderer.

## Design

Use a small picker-owned dialog abstraction rendered with the existing Bubble Tea + Lipgloss string
pipeline.

Do not introduce `huh`, `ultraviolet`, Bubble Tea v2, or Lipgloss v2 in this task.

### Dialog Model

Add explicit dialog state to `picker.Model`, for example:

```go
type dialogKind int

const (
    dialogNone dialogKind = iota
    dialogResume
    dialogCannotResume
)

type dialogAction int

const (
    dialogNoAction dialogAction = iota
    dialogConfirm
    dialogDismiss
)
```

The exact names may change during implementation, but the invariant should stay:

- picker root owns dialog state;
- dialog active state routes keys before the underlying picker;
- dialog returns typed intent such as confirm/dismiss;
- only picker root mutates `m.chosen` or exits Bubble Tea;
- `launcher` remains outside the UI path.

### Rendering Strategy

First implementation should render a modal dialog view, not a true transparent overlay.

Reasoning:

- aps currently renders `View()` as styled strings, not a cell buffer.
- True overlay composition over existing ANSI-styled list rows is fragile with ANSI escape sequences,
  CJK width, truncation, and line replacement.
- The user experience target is a centered confirmation popup, not preservation of every background
  list cell behind the dialog.

Implementation shape:

1. Render the normal picker status bar separately.
2. When dialog is active, render the dialog in the content area above the status bar.
3. Use `lipgloss.Place(width, contentHeight, lipgloss.Center, lipgloss.Center, dialog)` or an equivalent
   helper.
4. Append the status bar as the final row.
5. Keep terminal height accounting explicit: `contentHeight = m.height - statusBarHeight`.

This keeps #38's bottom-row invariant intact.

### Styling

Add styles in `picker/styles.go` using the existing ANSI 16-color palette only:

- dialog border;
- title;
- labels;
- values;
- muted help text;
- optional error/not-resumable text for the future #45 dialog.

Do not add RGB/hex colors.

### Session Metadata

The resume dialog should display the metadata aps knows for the selected session:

| Field | Source |
|-------|--------|
| Agent | `session.Client.String()` |
| Title | `session.Title` |
| Session ID | `session.ID` |
| Time | `session.Time` formatted consistently with picker/list conventions |
| Turns/Messages | `session.MsgCount` with the same label policy as existing UI |
| Directory | `session.CWD` or `session.CWDDisplay` depending on width |
| Data | new `source.Session.DataPath` or equivalent exported accessor |

Add a unified source-level data path if needed:

- Claude: set to JSONL path.
- Opencode SQL: set to the DB path or storage path used for discovery.
- Opencode JSON: set to the session JSON/message storage path when available.
- Codex: set to the resolved rollout path or SQLite metadata path, matching existing preview behavior.

Keep any Claude-only internal reload path if it is still needed for active-session refresh. If
`jsonlPath` remains unexported, it may duplicate `DataPath` for Claude.

### Width Safety

- Use `display.TruncateWidth` for CJK-safe field values before passing them into Lipgloss styles.
- Cap dialog width to terminal width with side padding.
- Preserve stable dimensions on narrow terminals:
  - minimum useful width around 44 columns when available;
  - maximum around 76-84 columns;
  - never render wider than `m.width`.
- If terminal height is very small, show a compact subset plus help text rather than corrupting the
  status bar row.

### Key Routing

When a dialog is active:

| Key | Behavior |
|-----|----------|
| `enter` | confirm resume, set `m.chosen`, quit |
| `esc` | dismiss dialog, stay in picker |
| `ctrl+c` | quit as existing global behavior |
| other keys | ignored by picker/list/search |

The underlying search input, cursor, preview pane, and filters must not change while a dialog is active.

### Relationship To #45

This issue should make #45 simple:

- #45 can add `dialogCannotResume`.
- Enter on a non-resumable history row opens that dialog.
- Confirm/dismiss closes the dialog without setting `m.chosen`.
- The same session-info renderer can be reused, with a not-resumable message added above the help row.

Do not implement #45 session discovery or dimmed history rows in this issue.

## What To Learn From Crush

Use Crush as an architectural reference, not as a dependency source.

Borrow:

- a root-owned dialog/overlay state;
- typed actions returned from dialog handling;
- drawing dialog content last;
- a shared render context for title/body/help/frame;
- viewport/dynamic sizing ideas for future complex dialogs.

Do not borrow:

- Ultraviolet renderer dependency;
- Bubble Tea v2 migration;
- Crush's full dialog stack;
- Crush's session list or filterable list implementation;
- Huh-style form semantics.

The useful translation for aps is:

```text
Crush Overlay stack + Ultraviolet compositor
=> aps dialog state + Lipgloss centered content view
```

If aps later needs true transparent overlays over ANSI-styled background rows, open a separate renderer
architecture issue. That should not be part of #48.

## Target Files

| File | Intended change |
|------|-----------------|
| `source/session.go` | Add exported session data path/metadata field if needed by picker UI. |
| `source/claude.go` | Populate data path from Claude JSONL path. |
| `source/opencode.go` | Populate data path from Opencode SQL DB or JSON storage paths. |
| `source/codex.go` | Populate data path from resolved rollout path or metadata source. |
| `picker/model.go` | Add dialog state, key routing, confirmation flow, and centered dialog rendering. |
| `picker/styles.go` | Add dialog styles using ANSI 16-color palette. |
| `picker/model_test.go` | Add dialog behavior, rendering, and status-bar invariant tests. |
| `preview/*` | Avoid changes unless data-path unification creates duplication worth removing. |

## Non-Goals

- Do not change list-mode output.
- Do not change session discovery semantics.
- Do not implement history-only sessions from #45.
- Do not add new UI dependencies.
- Do not change launcher process replacement behavior.
- Do not change title extraction, turn counting, or preview message extraction.
- Do not refactor picker rendering beyond what the dialog requires.

## TDD Plan

Write or update tests before implementation.

Recommended focused tests:

```bash
go test ./picker/... -run 'TestDialog'
go test ./source/... -run 'TestLoad.*DataPath|TestReloadSession'
```

Add tests covering:

- pressing Enter on a selected session opens the resume dialog instead of immediately choosing it;
- pressing Esc in the resume dialog dismisses it and leaves `m.chosen == nil`;
- pressing Enter in the resume dialog sets `m.chosen` and returns `tea.Quit`;
- typing/search/navigation keys do not mutate query/cursor/preview state while dialog is active;
- dialog view includes Agent, Title, Session ID, Time, Turns/Messages, Directory, and Data when present;
- long title/directory/data values are truncated to terminal width;
- CJK values are truncated with `display.TruncateWidth`;
- status bar remains the last rendered row when dialog is active;
- very narrow or short terminal dimensions do not produce rows wider than `m.width`;
- source loaders populate `DataPath` or equivalent for Claude, Opencode, and Codex.

## Implementation Plan

1. Add source metadata plumbing.
   - Add `DataPath string` to `source.Session`, or an equivalent exported accessor if that fits better.
   - Populate it in Claude, Opencode, and Codex loaders.
   - Keep existing unexported fields required for active-session refresh.

2. Add dialog state and actions in `picker/model.go`.
   - Introduce a small enum for dialog kind.
   - Store the target session by index or pointer.
   - Keep dialog state serializable/simple enough for tests.

3. Change Enter selection flow.
   - Current Enter behavior should become "open resume dialog" for resumable sessions.
   - A helper such as `openResumeDialog()` should capture the current selected session.
   - Confirm action should set `m.chosen` and quit.

4. Add dialog key routing.
   - At the top of `Update`, after global quit handling if appropriate, dispatch to dialog handling
     when active.
   - Ensure handled dialog keys do not fall through to search/list/preview behavior.

5. Add dialog rendering helpers.
   - Build field rows with aligned labels.
   - Use CJK-safe truncation for values.
   - Center the dialog in the content area above the status bar.
   - Keep status bar render/append logic explicit and last.

6. Add tests and iterate.
   - Start with failing picker tests.
   - Add source tests for data path if needed.
   - Implement in small steps and keep each behavior independently testable.

## Verification

Run:

```bash
go test ./source/... ./picker/...
go test ./...
go vet ./...
go build .
go install .
```

Manual smoke:

```bash
aps
aps -a
aps -c
aps -o
```

Check manually:

- Enter opens a centered resume confirmation instead of launching immediately.
- Esc returns to the picker with filter/cursor state preserved.
- Enter inside the dialog launches the selected session.
- The bottom status bar remains on the last terminal row.
- Preview mode and list mode both behave correctly around the dialog.
- Narrow terminal widths do not corrupt the dialog or status bar.

## Risks

- True overlay composition over ANSI-styled rows is easy to get wrong. Avoid it in #48.
- Adding `DataPath` may expose missing metadata for some source variants. Use empty string rather than
  inventing fake paths.
- Changing Enter semantics adds one confirmation step before launch. If this feels too heavy after
  manual testing, make the dialog conditional or add a future preference in a separate issue rather
  than weakening the dialog architecture.
- This touches picker `Update` and `View`, so it may conflict with #38 if implemented before #38 is
  merged. Prefer implementing after #38 lands, or rebase carefully on the status-bar branch.

## Acceptance Criteria

- Detailed tests pass for dialog key handling and render invariants.
- `go test ./...`, `go vet ./...`, `go build .`, and `go install .` pass.
- No new UI dependencies are added.
- `launcher` remains free of picker UI concerns.
- The issue body links this plan file.
- Manual smoke confirms the centered dialog, confirm/cancel behavior, and bottom status bar placement.
