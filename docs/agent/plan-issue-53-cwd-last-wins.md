## Goal

Fix `parseJSONL` so the session CWD reflects the **last** `cwd` value seen in the JSONL file, not the first.

## Problem

`attachment` records (SessionStart hook events) appear near the top of every JSONL and carry the launcher's working directory, not the project directory. The old `if cwd == ""` guard locked in this wrong value before the correct `cwd` appeared on later `user`/`assistant`/`system` records.

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

Add a test in `source/claude_test.go` that constructs a JSONL with:
1. An `attachment` record with `cwd=/wrong`
2. A `user` record with `cwd=/correct`

Assert `parseJSONL` returns `cwd=/correct`.

## Verification

```bash
aps -lc | grep 41507ada-3fa6-41a0-9802-9463fef4528f
# CWD column must show ~/projects.local/skill-store, not ~
```
