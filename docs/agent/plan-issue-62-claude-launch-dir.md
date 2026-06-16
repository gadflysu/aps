## Goal

Fix Claude resume launch directory selection so sessions stored under the original project namespace
can still resume after later transcript records move into a Claude worktree.

## Problem

Claude Code resolves `claude --resume <session-id>` against the launch directory's storage
namespace. `aps` currently parses the last non-empty transcript `cwd` and uses it both as the
display directory and the launch directory. For worktree sessions, the transcript can live at:

```text
~/.claude/projects/-Users-sd-projects-aps/<session-id>.jsonl
```

while later records carry:

```text
/Users/sd/projects/aps/.claude/worktrees/fix+18-metacache-reload
```

Launching from the worktree makes Claude search a non-existent worktree namespace and fail with
`No conversation found with session ID`.

## Target Files

- `source/session.go` — add a Claude launch directory field or equivalent typed distinction.
- `source/claude.go` — parse and populate display/current cwd separately from launch cwd.
- `launcher/launch.go` or `main.go` — pass Claude launch cwd to `launcher.Claude`.
- `preview/claude.go` — keep preview data path and displayed directory semantics clear.
- `source/claude_test.go` — cover launch cwd extraction and worktree-state cases.
- `launcher/launch_test.go` or main-level tests if available — cover command directory selection.

## Intended Change

1. Represent two concepts explicitly:
   - `CWD`: latest/display working directory used for UI context and path filtering.
   - `LaunchCWD`: directory used to invoke Claude resume.
2. Populate `LaunchCWD` for Claude sessions from the transcript storage namespace or earliest
   project cwd, not from the tail worktree cwd.
3. Keep `CWD` as the last non-empty transcript cwd so the picker still shows where the session last
   operated.
4. Make `main.go` call `launcher.Claude(session.ID, session.LaunchCWD, launchOpts)` for Claude and
   preserve existing launch behavior for Opencode and Codex.
5. Keep old cache compatibility in mind. If `MetaCache` stores cwd-only metadata, either add a
   schema-safe cached launch cwd field or force a cache miss/version bump for Claude metadata.

## Non-Goals

- Do not change Claude title extraction.
- Do not change path filtering semantics unless tests prove current behavior conflicts with the
  launch fix.
- Do not alter Opencode or Codex launch behavior.

## Tests

Write failing tests before implementation:

1. `TestParseJSONL_LaunchCWDUsesOriginalProjectWhenWorktreeTailCWD`:
   - transcript begins in `/Users/sd/projects/aps`
   - later records and `worktree-state` point at `/Users/sd/projects/aps/.claude/worktrees/...`
   - parsed session has `CWD` equal to the worktree path and `LaunchCWD` equal to the original path
2. `TestParseJSONL_LaunchCWDFallsBackToCWDForNormalSession`:
   - non-worktree transcript keeps `LaunchCWD == CWD`
3. cache round-trip test:
   - cached Claude metadata preserves `LaunchCWD` or invalidates old cache safely
4. launcher/main selection test:
   - Claude uses `LaunchCWD`
   - Opencode/Codex still use their existing `CWD`

## Verification

```bash
go test ./source ./launcher ./cmd ./picker
go test ./...
go build .
go install .
```

Manual check with a known affected session:

```bash
aps -c
# select session a60e87e2-eb65-4b92-bd29-204dc165d47c
# expected: Claude resumes instead of reporting "No conversation found with session ID"
```

Use `aps -c -n -v` before the real launch if command output needs inspection.
