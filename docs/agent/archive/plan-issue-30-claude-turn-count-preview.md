# Plan: Claude Turn Count And Preview Unification

## Goal

Make Claude session turn counts and preview recent-message rendering use the same definition:
one countable turn is one user-submitted TUI input that should also appear as a recent user message
in preview.

This fixes cases where the list row, preview `Turns`, and preview bullet list disagree.

## Ground Truth

Use Claude Code session JSONL as the source of truth, with these local/source-code findings:

- Claude Code writes many `type:"user"` rows that are not human prompts, especially
  `tool_result` blocks and synthetic XML/status messages.
- Claude Code `countVisibleMessages()` excludes `isMeta` user rows and tool-result-only user rows,
  and treats user rows with visible text/image/document blocks as visible content.
- Session `33acf421-6fec-4ff6-a090-987c0cec924a` demonstrates the current aps mismatch:
  - current source/list count: 8
  - current preview `Turns`: 7
  - current visible preview bullets: 6
  - recommended unified count: 6
- In that session, line 92 is array text containing `[Request interrupted by user for tool use]`.
  It is currently counted by `array has text`, but should not count or display.

## Target Predicate

Create a single reusable helper for Claude user-turn classification and display extraction.

Count a row when all are true:

- `type == "user"`
- `isMeta != true`
- no `toolUseResult`
- no `sourceToolAssistantUUID`
- content is displayable user-submitted input

Displayable input rules:

| content shape | count | preview text |
|---------------|-------|--------------|
| plain string | yes | first line, trimmed |
| `<command-message>` / `<command-name>` string | yes | formatted `/command args` |
| `<bash-input>` string | yes | formatted `! command` |
| array containing visible text/image/document blocks | yes, once | last meaningful text block, else `[image]` / `[document]` |
| `tool_result` array | no | none |
| `isMeta:true` hidden prompt | no | none |
| `<local-command-caveat>` | no | none |
| `<local-command-stdout>` / `<local-command-stderr>` | no | none |
| `<bash-stdout>` / `<bash-stderr>` | no | none |
| `<task-notification>` | no | none |
| `<system-reminder>` | no | none |
| `[Request interrupted...]` | no | none |
| unknown XML-like generated content | no | none |

For mixed future rows containing `tool_result` plus text/image/document, default to not counting
when tool metadata is present. Add a targeted test so behavior is explicit.

## Scope

### Modify

| File | Change |
|------|--------|
| `source/claude.go` | Replace `IsRealUserMsg` with or wrap it around a stricter helper that skips `isMeta`, tool metadata, generated prefixes, and request-interrupt markers. Preserve exported compatibility if callers/tests already use it. |
| `preview/claude.go` | Use the same helper/predicate for preview `Turns` and recent-message inclusion. Keep preview-specific truncation and styling local. |
| `source/claude_test.go` | Add failing tests for meta hidden prompt, interrupt array text, command XML, bash input, generated XML/output exclusions, and tool metadata exclusions. |
| `preview/preview_test.go` | Add failing tests proving `Turns` equals displayed message count for relevant Claude rows, including the interrupt array text case. |
| `docs/agent/notes-claude-code-references.md` | Add a short note documenting the unified turn predicate and the session `33acf421...` example after implementation is accepted. |

### Do Not Change

- Do not change Opencode turn logic.
- Do not include subagent JSONL files in main Claude session counts.
- Do not alter title extraction priority in this task.
- Do not change list formatting labels beyond existing `Turns`.
- Do not modify `~/.claude/statusline.sh` in this repo task unless explicitly requested.

## TDD Plan

Write/update tests first and run them to observe failures:

```bash
go test ./source/... -run 'TestIsRealUserMsg|TestParseJSONL'
go test ./preview/... -run 'TestParseJSONLPreview'
```

Target source tests:

- plain string user prompt counts.
- command XML counts and formats separately through preview.
- bash input counts as TUI submission.
- `isMeta:true` array text does not count.
- request-interrupt string and array text do not count.
- tool-result-only array does not count.
- rows with `toolUseResult` or `sourceToolAssistantUUID` do not count.
- local-command caveat/stdout/stderr and bash stdout/stderr do not count.
- task notification and system reminder do not count.
- image/document array counts once if not tool metadata.

Target preview tests:

- preview `Turns` equals number of recent messages for a synthetic fixture with mixed countable and
  skipped user rows.
- session-like fixture for `33acf421...` yields 6 turns and bullets:
  `/review #29`, `direct fix...`, `why string...`, `reset HEAD^...`, `can`, `ok`.

## Implementation Plan

1. Add a small Claude user-turn helper in `source/claude.go`.
   - Return structured data, for example `{Countable bool, Text string}`, or split into
     `ClaudeUserTurnText(rec) (string, bool)` and keep `IsRealUserMsg` as a wrapper.
   - Normalize command XML and bash input in one place so preview does not need a second parser.

2. Update `parseJSONL()` in `source/claude.go`.
   - Increment `msgCount` only when the helper returns `Countable`.
   - Use the returned display text as the first-user-title candidate when title metadata is absent.

3. Update `parseJSONLPreview()` in `preview/claude.go`.
   - Count and append recent messages from the same helper result.
   - Remove or reduce duplicated `previewSkipPrefixes` logic if the shared helper fully covers it.

4. Keep title-specific cleanup separate from turn display cleanup.
   - `applyTitleRules` remains for titles.
   - Turn preview text should preserve useful command/bash formatting and then truncate for display.

5. Update documentation note after code/tests pass.

## Verification

Run:

```bash
go test ./source/... ./preview/...
go test ./...
go build .
go install .
```

Manual checks:

```bash
go run . -l -c --color never
```

For session `33acf421-6fec-4ff6-a090-987c0cec924a`, verify:

- list row count becomes 6.
- preview `Turns` becomes 6.
- preview recent messages show exactly 6 bullets.
- line 92 `[Request interrupted by user for tool use]` is absent.

## Risks

- Counting `<bash-input>` as a TUI turn is correct for "user submitted in TUI", but not for a stricter
  "model chat turns" metric. If product semantics change, make this a named policy rather than
  silently changing the helper.
- Mixed future rows with tool results and text are ambiguous. The safe default is to skip rows with
  tool metadata and add diagnostic coverage before changing behavior.
- Existing metadata cache may preserve old counts until JSONL mtime/size changes. If this blocks
  validation, add a cache schema version or explicit invalidation in a separate task.
