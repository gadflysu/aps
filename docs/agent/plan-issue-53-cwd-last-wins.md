## Goal

Fix `parseJSONL` so the session CWD reflects the **last** `cwd` value seen in the JSONL file, not the first.

## Problem

`attachment` records (SessionStart hook events) appear near the top of every JSONL and carry the launcher's working directory, not the project directory. The old `if cwd == ""` guard locked in this wrong value before a later record carried the correct project directory. The `cwd` field is unrelated to record type — any record can carry it; correctness is determined solely by position in the file.

## Target Files

- `source/claude.go` — `parseJSONL`, line ~241

## Change

Remove the `if cwd == ""` guard. Always overwrite `cwd` when a non-empty value is found:

```go
// Extract cwd — last value wins (early records may carry launcher dir)
if raw, ok := rec["cwd"]; ok {
    var s string
    if json.Unmarshal(raw, &s) == nil && s != "" {
        cwd = s
    }
}
```

## Non-Goals

- No change to title extraction logic
- No change to `ReloadSession` (already calls `parseJSONL`)
- No performance work — `parseJSONL` already scans the full file for other fields

## Tests

Add two tests in `source/claude_test.go`:
1. `TestParseJSONL_CWDLastWins`: two records of the same type, second cwd overwrites first — proves last-wins is purely positional, not type-dependent
2. `TestParseJSONL_CWDEmptyNotOverwrite`: valid cwd followed by empty cwd — proves `s != ""` guard prevents erasure

## Verification

```bash
aps -lc | grep 41507ada-3fa6-41a0-9802-9463fef4528f
# CWD column must show ~/projects.local/skill-store, not ~
```
