# Plan: issue #62 — Claude resume launch directory for worktree sessions

## Goal

Resume Claude sessions correctly when the transcript's tail `cwd` points to a
`.claude/worktrees/` path, while preserving existing behavior for non-worktree
sessions.

## Root cause

`parseJSONL` uses last-wins semantics for `cwd`. When a session starts in the
project root and then work moves into a worktree, the last `cwd` is the worktree
path. `launcher.Claude` does `os.Chdir(cwd)` before `claude --resume <id>`.
Claude Code resolves transcript storage from the launch directory namespace, so
it searches a namespace that contains no transcript.

## Solution

Separate two concepts in `source.Session`:

- `CWD` — last non-empty `cwd` from the transcript (display/filter, unchanged)
- `LaunchCWD` — first non-empty `cwd` from the transcript; this is the cwd used
  by `cd <LaunchCWD> && claude --resume <session-id>`, not the
  `~/.claude/projects/<sanitized-cwd>` storage directory itself

For sessions that never changed directory, `LaunchCWD == CWD`.

## Files changed

| File | Change |
|------|--------|
| `source/session.go` | Add `LaunchCWD string` field |
| `source/metacache.go` | Add `LaunchCWD string` to `MetaEntry`; make `Lookup` reject incomplete entries so old cache records reparse |
| `source/claude.go` | `parseJSONL` returns `jsonlMeta` with separate `CWD` and `LaunchCWD`; `parseOne` and `ReloadSession` populate `Session.LaunchCWD`; cache misses and incomplete older entries refresh cache |
| `source/claude_test.go` | New tests: `TestParseJSONL_LaunchCWDFirstCWD`, `TestParseJSONL_LaunchCWDEqualsCWDForSingleProject`, `TestLoadClaude_SessionLaunchCWDFromFirstCWD`, `TestLoadClaude_OldCacheEntryWithoutLaunchCWDReparses` |
| `main.go` | Use `session.LaunchCWD` (fallback `session.CWD`) as the `dir` arg to launcher; report missing launch directories without silently falling back to the last cwd |

## Non-goals

- Changing path-filter logic (still uses `CWD`)
- Changing `CWDDisplay` (still uses `CWD`)
- Handling Opencode or Codex worktree patterns (not observed)
- Treating Claude's sanitized project directory name as a reversible path encoding
- Silently falling back from a missing Claude launch directory to the last cwd; that can resume from the wrong namespace

## Verification

`go test ./...` passes. The new tests exercise first-cwd parsing, session population,
old cache migration, and launch-directory diagnostics.
