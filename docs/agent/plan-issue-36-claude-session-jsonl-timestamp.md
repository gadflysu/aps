# Plan: Claude Session Time From JSONL Timestamp

## Goal

Use Claude session JSONL data as the authority for `Session.Time` instead of the transcript file's
filesystem mtime.

## Problem

`source.parseOne()` and `source.ReloadSession()` currently set Claude `Session.Time` from
`os.Stat(jsonlFile).ModTime()`. File mtime can drift when a transcript is touched, copied, restored,
or rewritten for metadata after the last real session message. This can make list sorting, picker
ordering, preview time, and date filters disagree with Claude Code's recorded conversation data.

## Target Behavior

For Claude sessions:

1. Parse the timestamp from the last usable timestamped JSONL record in the session transcript.
2. Use that parsed timestamp for `Session.Time`.
3. Fall back to file mtime only when the JSONL has no valid timestamp.
4. Apply the same rule in initial load and watcher reload.

## Scope

### Modify

| File | Change |
|------|--------|
| `source/claude.go` | Extend JSONL parsing to return session timestamp; use it in `parseOne()` and `ReloadSession()`. |
| `source/metacache.go` | Add cached `SessionTime` or equivalent field so cache hits do not lose JSONL-derived time. |
| `source/claude_test.go` | Add tests for timestamp extraction, mtime fallback, initial load, reload, and cache hits. |
| `source/metacache_test.go` | Add/adjust round-trip tests for the new cached time field. |
| `docs/agent/notes-claude-code-references.md` | Document the chosen Claude session-time rule after implementation. |

### Do Not Change

- Do not change title extraction priority.
- Do not change user-turn counting.
- Do not change Opencode time semantics.
- Do not change active-session detection.

## Timestamp Rule

Preferred rule:

- Scan all JSONL lines in order.
- For each valid JSON object, read top-level `timestamp` if present.
- Accept RFC3339/RFC3339Nano string timestamps used by Claude session records.
- Keep the latest valid timestamp seen in file order.
- Return that timestamp as the session time.

If future local data shows numeric timestamps in Claude transcript rows, add a parser fallback with
tests. Do not infer time from `uuid`, `parentUuid`, or filesystem paths.

Open implementation detail:

- If the last timestamped row is non-conversation metadata, decide whether it should count. The user
  expectation is "last message timestamp", so tests should include a metadata row after the last
  assistant/user/system message and make the intended behavior explicit before implementation.

## TDD Plan

Write failing tests first:

```bash
go test ./source/... -run 'TestParseJSONL.*Timestamp|TestLoadClaude.*Timestamp|TestReloadSession.*Timestamp|TestMetaCache'
```

Target cases:

- JSONL last assistant/user timestamp wins over file mtime.
- Rows without timestamp are ignored.
- Invalid timestamp falls back to the last valid timestamp.
- No valid timestamp falls back to file mtime.
- Cache hit returns the cached JSONL-derived time, not the current mtime.
- `ReloadSession()` uses the same parsed timestamp rule.
- Date filtering uses the JSONL-derived `Session.Time`.

## Implementation Plan

1. Add a timestamp return value to `parseJSONL()` or split metadata parsing into a struct result.
2. Parse top-level `timestamp` while scanning rows.
3. Store parsed session time in `MetaEntry`.
4. On cache hit, use cached `SessionTime` when non-zero; otherwise fall back to mtime for old cache
   compatibility.
5. Update `parseOne()` and `ReloadSession()` to use the parsed/cached session time.
6. Update docs after tests pass.

## Verification

Run:

```bash
go test ./source/...
go test ./...
go build .
go install .
```

Manual smoke:

```bash
aps -c -l --color never
aps -c --from today -l --color never
```

Verify sessions sort by the JSONL-derived time and date filters include/exclude sessions based on
that same time.

## Risks

- Old gob cache entries will not have the new field. Keep backward compatibility by treating zero
  `SessionTime` as "not cached".
- Some Claude metadata rows may have timestamps. Tests must lock down whether metadata after the last
  conversation row should affect session time.
- Large JSONL parsing already happens for title/count extraction; adding timestamp extraction should
  not add an additional full-file pass.
