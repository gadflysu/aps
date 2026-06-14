# Plan: Issue #51 — filter path match uses file instead of directory

## Goal

Fix `filter.Matches` so that a path filter token like `"aps"` that resolves to a
**file** (e.g. the compiled binary `./aps`) is not treated as an existing path.
Only a resolved path that is a **directory** should set `pathExists = true`.

## Root cause

`filter/path.go: fileExists` calls `os.Stat` and returns true for any filesystem
entry — files included. When `aps -la aps` is run from `~/projects.local/aps`,
`filepath.EvalSymlinks("aps")` resolves to `./aps` (the binary). This makes
`pathExists = true`, which combined with `strictMatch = true` blocks the raw
substring fallback (`!strictMatch || !pathExists` == false), so sessions with
`cwd = /Users/dsu/projects.local/aps` are silently dropped.

## Target files

| File | Change |
|------|--------|
| `filter/path.go` | `fileExists` → `dirExists`: use `stat.IsDir()` check |
| `filter/path_test.go` | add regression test for file-resolves-to-binary case |

## Non-goals

- No changes to substring matching logic or strictMatch semantics
- No changes to symlink resolution order
- No other packages touched

## Fix

In `filter/path.go`, change `fileExists` to check `IsDir()`:

```go
// before
func fileExists(p string) bool {
    _, err := os.Stat(p)
    return err == nil
}

// after
func pathIsDir(p string) bool {
    info, err := os.Stat(p)
    return err == nil && info.IsDir()
}
```

Update the single call site in `Matches`:

```go
pathExists := resolved != "" && pathIsDir(resolved)
```

## Tests

Add to `filter/path_test.go`:

```
TestMatches/file-named-same-as-project: pathFilter="aps" pointing to a regular
file should fall through to raw substring and match cwd containing "aps"
```

The test creates a temp file (not a directory) at a known path, passes that
filename as `pathFilter`, and asserts `Matches` returns true for a `cwd` that
contains it as a substring.

## Verification

```bash
go test ./filter/...
go build . && go install .
aps -la aps   # must return sessions from ~/projects.local/aps
```
