# Plan: Issue #50 — Codex rollout scanner drops messages when a line exceeds 64 KiB

## Goal

Fix all three Codex rollout parsers to use a 4 MiB scanner buffer so lines larger than
the default 64 KiB token limit do not silently truncate the parse and drop user messages.

## Root Cause

`bufio.NewScanner` has a default max token size of `bufio.MaxScanTokenSize` (64 KiB).
Codex rollout files contain lines that exceed this: tool call results, file content inlined
into a turn, and long model responses can all exceed 64 KiB. When the scanner hits such a
line it stops and returns `bufio.Scanner: token too long`; all subsequent lines, including
later `event_msg/user_message` entries, are never read.

Diagnostic evidence (`cmd/diagnose_codex_preview`):

| Session | Default | 4 MiB buffer | Lost |
|---------|---------|--------------|------|
| `019e6808` | 39 | 61 | 22 |
| `019e8252` | 89 | 114 | 25 |

## Non-Goals

- Changing how `session_meta` is parsed (first line only, always small).
- Supporting compressed `.jsonl.zst` rollout files (tracked separately).
- Any refactor of the parsing logic beyond the buffer fix.

## Target Files

| File | Function | Change |
|------|----------|--------|
| `preview/codex.go` | `parseCodexRolloutPreview` | add `scanner.Buffer(...)` after `bufio.NewScanner(f)` |
| `source/codex.go` | `countRolloutUserMessages` | same |
| `source/codex.go` | `parseRolloutFile` | same |

Buffer size: `4 * 1024 * 1024` bytes (4 MiB). This covers the largest realistic single-line
payload without inflating memory per session.

## Tests

Add one test in `source/codex_test.go` or `preview/preview_test.go` that creates a synthetic
rollout file with a line exceeding 64 KiB before a `user_message` event and asserts the
message is counted. This is the failing test that must pass after the fix.

The existing `countRolloutUserMessages` has no test at all; the new test covers it directly.

## Verification

After the fix, `cmd/diagnose_codex_preview` must report zero lost messages for every rollout
file in `~/.codex/sessions/`.

Run: `go test ./source/... ./preview/...`
