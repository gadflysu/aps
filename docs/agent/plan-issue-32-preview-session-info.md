# Plan: Preview Session Info Metadata

## Goal

Make the preview session info panel expose the metadata aps already knows for each selected session,
with a layout that stays easy to scan across Claude Code and Opencode sessions.

## User-Visible Behavior

The preview info section should show:

- Agent
- Title
- Session ID
- Time
- Turns or Messages
- Directory
- Data

All values should begin at the same visual column so long labels such as `Session ID:` do not make the
panel look uneven. The info viewport should reserve enough rows for every field.

## Scope

### Modify

| File | Change |
|------|--------|
| `preview/claude.go` | Add Agent, Session ID, and Data rows to Claude preview info and full preview output. |
| `preview/opencode.go` | Add Agent, Session ID, and Data rows to Opencode preview info and full preview output. |
| `preview/shared.go` | Add a shared formatter for aligned preview info rows. |
| `preview/styles.go` | Add label styles for Agent, Session ID, and Data using the existing 16-color palette. |
| `picker/model.go` | Increase the info viewport row allocation to fit all metadata fields. |
| `preview/preview_test.go` | Cover field presence and aligned output for Claude Code and Opencode. |
| `picker/model_test.go` | Cover updated preview height allocation. |

### Do Not Change

- Do not change session discovery.
- Do not change Claude or Opencode title extraction semantics.
- Do not change turn-count semantics.
- Do not introduce new colors outside the existing ANSI palette.

## TDD Plan

Add failing tests before implementation:

```bash
go test ./preview/... -run 'Test(RenderClaude_(FieldLabels|IncludesSessionMetadata)|ClaudeInfo_ContainsAllFields|OpencodeInfo_IncludesSessionID)'
go test ./preview/... -run 'Test(ClaudeInfo|OpencodeInfo)_FieldsAreAligned'
go test ./picker/... -run 'TestUpdatePreviewHeights_(NoMsgs|WithMsgs|ClampMsgsToOne)'
```

Expected failures before implementation:

- Claude preview output lacks Agent, Session ID, and Data fields.
- Opencode preview output lacks Agent, Session ID, and Data fields.
- Info values do not share a single value column.
- Picker info viewport height is too small for seven metadata rows.

## Implementation Plan

1. Add a shared `writePreviewInfoRow` helper that pads labels to the width of `Session ID:`.
2. Use the helper in Claude preview info and full preview rendering.
3. Use the helper in Opencode preview info and full preview rendering.
4. Add label styles for the new fields using existing display color constants.
5. Increase `infoContentLines` from four to seven rows and update picker height tests.

## Verification

Run:

```bash
go test ./preview/... -run 'Test(RenderClaude_(FieldLabels|IncludesSessionMetadata)|ClaudeInfo_ContainsAllFields|OpencodeInfo_IncludesSessionID)'
go test ./preview/... -run 'Test(ClaudeInfo|OpencodeInfo)_FieldsAreAligned'
go test ./picker/... -run 'TestUpdatePreviewHeights_(NoMsgs|WithMsgs|ClampMsgsToOne)'
go test ./...
go build .
go install .
```

Manual smoke:

```bash
aps
```

Verify the preview info panel shows seven fields and that field values align for both Claude Code and
Opencode sessions.

## Risks

- Longer data paths can still wrap on narrow terminals. This task only aligns the start column; path
  truncation or wrapping policy should be handled separately if needed.
- Adding rows reduces space for recent messages and directory preview in short terminals. Existing
  viewport height tests should cover the intended redistribution.
