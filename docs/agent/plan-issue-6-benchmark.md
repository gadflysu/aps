# Plan: Issue #6 — BenchmarkLoadClaude

## Goal

Add `BenchmarkLoadClaude` in `source/claude_test.go` alongside the existing `BenchmarkCollectProcs`. Establishes a repeatable baseline for verifying future parse performance improvements.

## Branch

`feat/6-benchmark-load-claude`

## Files to change

| File | Change |
|------|--------|
| `source/claude_test.go` | Add `BenchmarkLoadClaude` function |

## Implementation

```go
func BenchmarkLoadClaude(b *testing.B) {
    for b.Loop() {
        LoadClaude("", false, false)
    }
}
```

Place it after `BenchmarkCollectProcs`. No helpers or fixtures needed — reads live `~/.claude/projects/` like the existing benchmark.

## Acceptance criteria

- `go test -bench=BenchmarkLoadClaude -benchtime=5s ./source/` runs and reports ns/op
- All existing tests still pass (`go test ./...`)

## GitHub workflow

- Branch: `feat/6-benchmark-load-claude`
- Commit body: `Closes #6`
- PR: `gh pr create` with `Closes #6`
- Merge: `gh pr merge --rebase`
