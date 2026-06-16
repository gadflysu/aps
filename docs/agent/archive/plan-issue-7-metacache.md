# Plan: Issue #7 — MetaCache

## Goal

New `source/metacache.go` that persists per-file parse results to `~/.cache/aps/session-meta.gob`. Pure new file — zero changes to `LoadClaude` or any existing file. API is consumed by Issue #8.

## Branch

`feat/7-metacache`

## Files to create / change

| File | Change |
|------|--------|
| `source/metacache.go` | New file — `MetaCache` type and its methods |
| `source/metacache_test.go` | New file — unit tests |

## API design

```go
// MetaEntry holds the parsed metadata for one JSONL file.
type MetaEntry struct {
    Mtime    time.Time
    Size     int64
    Title    string
    CWD      string
    MsgCount int
}

// MetaCache is an in-process cache backed by a gob file on disk.
type MetaCache struct { ... }

// LoadMetaCache reads ~/.cache/aps/session-meta.gob.
// Returns an empty cache (not an error) if the file is missing or corrupt.
func LoadMetaCache() *MetaCache

// Lookup returns the cached entry for path if mtime and size both match.
// Returns (entry, true) on hit, (zero, false) on miss.
func (c *MetaCache) Lookup(path string, mtime time.Time, size int64) (MetaEntry, bool)

// Store records an entry for path.
func (c *MetaCache) Store(path string, e MetaEntry)

// Save writes the cache to disk. Called once after all files are parsed.
func (c *MetaCache) Save() error
```

## Validation rules

- `Lookup` returns miss if either `mtime` or `size` differs (guards atomic-rename editors)
- `LoadMetaCache` never returns nil and never returns error — corrupt/missing = empty cache
- `Save` creates `~/.cache/aps/` if it does not exist

## Tests (TDD — write tests first)

| Test | What it checks |
|------|---------------|
| `TestMetaCache_HitOnExactMatch` | Lookup returns entry when mtime+size match |
| `TestMetaCache_MissOnMtimeChange` | Lookup misses when mtime differs |
| `TestMetaCache_MissOnSizeChange` | Lookup misses when size differs |
| `TestMetaCache_RoundTrip` | Save then LoadMetaCache returns same entries |
| `TestMetaCache_CorruptFile` | LoadMetaCache on corrupt gob returns empty cache, no panic |
| `TestMetaCache_MissingFile` | LoadMetaCache when file absent returns empty cache |

## Acceptance criteria

- All 6 tests pass
- `go test ./source/...` green
- `source/claude.go` unchanged (diff must show 0 lines changed in that file)

## GitHub workflow

- Branch: `feat/7-metacache`
- Commit body: `Closes #7`
- PR: `gh pr create` with `Closes #7`
- Merge before #8: `gh pr merge --rebase`
