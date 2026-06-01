# Plan: Claude Title Extraction

## Goal

Align aps Claude session titles with Claude Code's `/resume` display semantics while keeping the
change surgical and compatible with the existing full-JSONL parser.

## Current State

- `source.parseJSONL()` resolves title as `custom-title > ai-title > first user message > Untitled`.
- `preview.parseJSONLPreview()` resolves title as `custom-title > first user message > Untitled`.
- `source.IsRealUserMsg()` counts only string `message.content` as a real user turn; Claude Code now
  has user-authored array `text` blocks too.
- Claude Code `/resume` fast path reads JSONL head+tail only and may omit `agentName`; aps currently
  parses the whole file, so it can match the full path more closely without adding a fast-path
  subsystem.

## Target Title Semantics

Use this priority for aps list rows and preview info:

1. Last valid `agent-name.agentName`
2. Last valid `custom-title.customTitle`
3. Last valid `ai-title.aiTitle`
4. Last valid `summary.summary`
5. Last valid `last-prompt.lastPrompt`
6. First meaningful real user text
7. `Untitled`

Filtering/cleanup:

- Pass every title candidate through `applyTitleRules`.
- Keep existing command/local-output skip prefixes and the `Implement the following plan:` special
  case.
- Treat array `message.content` with at least one `{"type":"text"}` block as real user text.
- Treat array `message.content` containing only `tool_result` blocks as tool feedback, not a user
  turn.

## Scope

### Modify

| File | Change |
|------|--------|
| `source/claude.go` | Track `lastAgentName`, `lastSummary`, and `lastPrompt`; update final title priority; update real-user detection for text arrays |
| `source/claude_test.go` | Add failing tests for title priority and array-text turn counting before implementation |
| `preview/claude.go` | Reuse the same title priority enough for preview: agent name, custom title, AI title, summary, last prompt, first user text |

### Do Not Change

- Do not introduce Claude Code's fast/progressive loader yet.
- Do not include nested `subagents/*.jsonl` in the main picker.
- Do not change `MetaEntry` unless tests show cache invalidation cannot be handled by mtime/size.
- Do not refactor Opencode title logic.

## TDD Checklist

Write/update tests first and run them to watch the expected failures:

```bash
go test ./source/... -run 'TestParseJSONL_(AgentName|Summary|LastPrompt|TitlePriority|ArrayText)'
```

Planned source tests:

- `TestParseJSONL_AgentNameWinsOverCustomTitle`: `agent-name` title beats `custom-title`.
- `TestParseJSONL_CustomTitleWinsOverAiSummaryPrompt`: user title beats AI title, summary, and prompt.
- `TestParseJSONL_AiTitleWinsOverSummaryAndPrompt`: AI title beats summary and prompt.
- `TestParseJSONL_SummaryWinsOverLastPromptAndFirstUser`: summary fallback works.
- `TestParseJSONL_LastPromptWinsOverFirstUser`: latest prompt fallback works.
- `TestParseJSONL_ArrayTextCountsAsRealUserMessage`: text block arrays count and can become first-user title.
- `TestParseJSONL_ToolResultArrayNotCounted`: keep existing tool-result exclusion.

Preview tests do not currently exist; if adding them is cheap, add focused tests for
`parseJSONLPreview()` title priority. If that would require broad preview test scaffolding, keep
preview implementation small and verify through existing source tests plus manual list/preview smoke.

## Implementation Notes

- Replace scattered title variables in `parseJSONL()` with explicit candidate variables:
  `lastAgentName`, `lastCustomTitle`, `lastAiTitle`, `lastSummary`, `lastPrompt`,
  `firstUserMsgTitle`.
- Keep `applyTitleRules()` as the single normalization function for list titles.
- Add a helper such as `isRealUserContent(raw json.RawMessage) bool` to avoid duplicating
  string-vs-array logic between `IsRealUserMsg()` and title extraction.
- For `last-prompt`, read field `lastPrompt`.
- For `summary`, read field `summary`; only use if `applyTitleRules()` returns non-empty.
- In preview, either share a small exported helper from `source` or mirror the priority locally.
  Prefer sharing only if it does not create an awkward dependency from `source` into preview.

## Verification

Run:

```bash
go test ./source/... ./preview/...
go test ./...
go build .
go install .
```

Manual smoke:

```bash
aps -l -c | head
```

Confirm a recently renamed Claude session displays the `agent-name` title when present, and a session
with only `ai-title` or `summary` still has a useful title.

## Risks

- `agent-name` may identify an agent persona rather than a user-facing title in some sessions. This
  still matches Claude Code full-path display priority.
- `summary` records may describe a compacted leaf rather than a user-chosen title. It is lower than
  explicit titles and only improves fallback quality.
- Existing cache entries remain valid until JSONL mtime/size changes; users with stale cached titles
  may need a cache miss to see new priority for old unchanged sessions. If this is unacceptable,
  add a cache schema version in a separate plan.
