# Plan: Issue #8 — Parallel LoadClaude + MetaCache Integration

## Goal

Two changes:
1. Run `LoadClaude` and `LoadOpencode` concurrently in `main.go`
2. Parse JSONL files inside `LoadClaude` with a bounded worker pool (cap: `NumCPU/2`)
3. Integrate `MetaCache` (from Issue #7) to skip parsing unchanged files

`LoadClaude` still returns `[]Session` synchronously — streaming UI is Issue #9.

**Merge Issue #7 before starting this branch.**

## Branch

`feat/8-parallel-load` (branch from master after #7 is merged)

## Files to change

| File | Change |
|------|--------|
| `main.go` | Run `LoadClaude` + `LoadOpencode` concurrently via goroutines |
| `source/claude.go` | Extract `parseOne`; replace serial loop with worker pool; integrate `MetaCache` |

## Implementation steps

### Step 1 — Concurrent client loading (`main.go`)

Replace the sequential calls in `loadSessions` with concurrent goroutines:

```go
var (
    claudeSessions   []source.Session
    opencodeSessions []source.Session
    claudeErr        error
    opencodeErr      error
    wg               sync.WaitGroup
)
if cfg.Claude {
    wg.Add(1)
    go func() { defer wg.Done(); claudeSessions, claudeErr = source.LoadClaude(...) }()
}
if cfg.Opencode {
    wg.Add(1)
    go func() { defer wg.Done(); opencodeSessions, opencodeErr = source.LoadOpencode(...) }()
}
wg.Wait()
```

### Step 2 — Per-file worker pool (`source/claude.go`)

Extract existing per-file logic into `parseOne(path string, verbose bool) (Session, bool)`.
Replace the serial loop with a bounded pool:

```go
workers := max(1, runtime.NumCPU()/2)
sem := make(chan struct{}, workers)
var mu sync.Mutex
var wg sync.WaitGroup
for _, jsonlFile := range allFiles {
    wg.Add(1)
    sem <- struct{}{}
    go func(path string) {
        defer wg.Done()
        defer func() { <-sem }()
        s, ok := parseOne(path, verbose)
        if ok {
            mu.Lock(); sessions = append(sessions, s); mu.Unlock()
        }
    }(jsonlFile)
}
wg.Wait()
```

### Step 3 — MetaCache integration (`source/claude.go`)

- Call `LoadMetaCache()` once at the top of `LoadClaude`
- In `parseOne`: stat the file; call `cache.Lookup(path, mtime, size)`; on hit return cached entry; on miss parse and call `cache.Store`
- After `wg.Wait()`, call `cache.Save()`

## Tests (TDD — write tests first)

| Test | What it checks |
|------|---------------|
| `TestLoadClaude_Concurrent` | Result set matches serial output (order-independent) |
| `TestLoadClaude_CacheHit` | When cache has valid entry, `parseJSONL` is not called for that file |
| `TestLoadClaude_CacheMiss` | When mtime/size changed, file is re-parsed and cache updated |

## Acceptance criteria

- All new and existing tests pass (`go test ./...`)
- `go test -bench=BenchmarkLoadClaude -benchtime=5s ./source/` shows improvement vs pre-#8 baseline
- `go vet ./...` clean
- Manual smoke test: `aps` lists sessions correctly; `aps --debug-log /tmp/a.log` shows reduced `loadSessions` time on second run

## GitHub workflow

- Branch: `feat/8-parallel-load`
- Commit body: `Closes #8`
- PR: `gh pr create` with `Closes #8`
- Merge after #7: `gh pr merge --rebase`
