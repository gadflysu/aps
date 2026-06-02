# Plan: Codex Agent CLI Support

## Goal

Add aps support for OpenAI Codex CLI sessions while preserving the existing source/display/picker/
preview/launcher package boundaries.

## Ground Truth

Use `docs/agent/notes-codex-references.md` as the storage reference.

Key decisions from the research:

- Current Codex state DB filename is `state_5.sqlite`.
- `codex_home` and `sqlite_home` can differ.
- Rollout JSONL files are the durable transcript format; SQLite is the queryable metadata index.
- Stale DB rows can exist, so launchable sessions must have a verified rollout path.
- First release should list Codex CLI sessions (`source = "cli"`), not IDE/app/internal/subagent
  threads.
- `codex resume <session-id>` is the launcher command.

## Scope

### Modify

| File | Change |
|------|--------|
| `source/session.go` | Add `ClientCodex` and display name `Codex` or `OpenAI Codex`. Add an internal data path field if needed for preview. |
| `source/codex.go` | Add Codex home/sqlite home resolution, SQLite loading, rollout fallback scanning, title/preview extraction, and path filtering. |
| `source/codex_test.go` | Add storage parsing tests for DB rows, rollout fallback, stale DB rows, source filtering, timestamp handling, and missing storage. |
| `preview/codex.go` | Render Codex session info, recent user messages, and directory listing following existing preview patterns. |
| `preview/preview_test.go` | Cover Codex info fields and recent-message extraction. |
| `launcher/launch.go` | Add `Codex` launcher using `codex resume <session-id>`. |
| `launcher/launch_test.go` | Cover verbose/custom command rendering for Codex. |
| `cmd/root.go` | Add `--codex`, a short flag, `--codex-cmd`, and include Codex in `--all`. |
| `main.go` | Load Codex sessions in parallel and dispatch Codex launcher. |
| `display` / `picker` tests | Update combined-source expectations if source column/client handling changes. |
| `docs/agent/notes-coding-agent-references.md` | Update or cross-link the Codex section after implementation to prevent stale global reference data. |

### Do Not Change

- Do not add Gemini support in this issue.
- Do not list Codex archived sessions by default.
- Do not include Codex IDE/app/internal/subagent threads in the initial CLI support.
- Do not change Claude or Opencode behavior.
- Do not introduce compressed rollout support unless active Codex sessions require it.

## Data Loading Design

### Path Resolution

1. Resolve `codex_home` from `$CODEX_HOME`, else `~/.codex`.
2. Resolve `sqlite_home` from:
   - top-level `sqlite_home` in `<codex_home>/config.toml`, if cheaply parseable,
   - `$CODEX_SQLITE_HOME`, if set,
   - `codex_home`.
3. Use `<sqlite_home>/state_5.sqlite` as the current state DB path.

If parsing `config.toml` would require a new dependency, implement a narrow parser for top-level
quoted `sqlite_home = "..."` and add tests. Defer full TOML support.

### SQLite Query

When the DB exists, query active CLI sessions:

```sql
SELECT id, rollout_path, cwd, title, preview, first_user_message,
       COALESCE(updated_at_ms, updated_at * 1000) AS updated_ms,
       COALESCE(created_at_ms, created_at * 1000) AS created_ms,
       git_branch, source, model_provider, cli_version
FROM threads
WHERE archived = 0
  AND source = 'cli'
  AND cwd <> ''
ORDER BY updated_ms DESC
```

If `updated_at_ms` / `created_at_ms` columns are absent in old DBs, fall back to a seconds-based
query. If `preview` or `first_user_message` is absent, fall back to `title` and rollout parsing.

Verify every DB row's `rollout_path`:

- keep rows whose file exists;
- if missing, try to find a rollout filename containing the session id under `<codex_home>/sessions`;
- skip rows that cannot be matched to a readable rollout file.

### Rollout Fallback

Scan:

```text
<codex_home>/sessions/YYYY/MM/DD/rollout-*.jsonl
```

Parse the first useful records to extract:

- `session_meta.id`
- `session_meta.cwd`
- `session_meta.source`
- `session_meta.cli_version`
- `session_meta.model_provider`
- optional git branch
- first `event_msg` user message as preview/title fallback

Only include `source = "cli"`. De-duplicate by session id; DB metadata wins when both sources exist,
but the verified rollout path is retained.

### Title And Count

Title priority:

1. SQLite `title`
2. SQLite `preview`
3. SQLite `first_user_message`
4. `session_index.jsonl` latest thread name
5. first rollout user message
6. `Untitled`

Turn count:

- Count rollout `event_msg` records whose payload is a user message with non-empty text or images.
- Do not count `response_item`, `turn_context`, `compacted`, assistant/tool output, or metadata rows.

## TDD Plan

Add failing tests before implementation:

```bash
go test ./source/... -run 'TestLoadCodex|TestParseCodex'
go test ./preview/... -run 'TestCodex'
go test ./launcher/... -run 'Test.*Codex'
go test ./cmd/... -run 'Test.*Codex'
```

Target tests:

- Missing `codex_home` returns zero sessions without error.
- SQLite active CLI row loads with title, cwd, timestamp, branch, and data path.
- Archived rows and non-CLI rows are excluded.
- A DB row with missing rollout path is skipped unless filename fallback finds the rollout.
- Rollout-only session loads when DB is absent.
- `updated_at_ms` is preferred; old seconds columns still work.
- `session_index.jsonl` can provide title fallback when DB title/preview are empty.
- Turn count counts only rollout user messages.
- Path filter uses existing exact/symlink/substring matching behavior.
- `Codex` launcher renders and execs `codex resume <session-id>`.

## Implementation Plan

1. Add `ClientCodex` and command config fields.
2. Implement Codex storage helpers in `source/codex.go`.
3. Implement DB loader with schema-tolerant column handling.
4. Implement rollout parser and fallback scanner.
5. Add Codex preview renderer and route picker preview calls.
6. Add Codex launcher and CLI flags.
7. Wire Codex into `--all`, list mode, interactive mode, and no-launch verbose output.
8. Update global reference docs or cross-link the issue-specific note.

## Verification

Run:

```bash
go test ./source/... ./preview/... ./launcher/... ./cmd/...
go test ./...
go build .
go install .
```

Manual smoke:

```bash
aps --codex -l --color never
aps --codex -n -v
aps --all -l --color never
```

For a known Codex CLI session, verify:

- list row shows `Codex` source in combined mode;
- session id matches the rollout/session metadata id;
- title follows the planned priority;
- directory filter works with `.` and explicit paths;
- no-launch verbose prints `cd "<cwd>" && codex resume <id>`.

## Risks

- Codex DB rows can be stale while rollout files are missing. Always verify a resumable rollout
  before showing a session.
- Full rollout scans may be expensive for large histories. Start with correctness; add metadata cache
  or bounded parsing later if needed.
- Compressed `.jsonl.zst` files may require a new dependency. Keep active uncompressed support first
  unless tests or observed data prove active sessions are compressed.
- Custom `sqlite_home` in config may be missed if the narrow parser is too limited. Add tests and
  document unsupported TOML shapes before broadening parser support.

## Implementation Summary

**Status:** ✅ Completed

**Commits:**
- `473045a` feat(source): add ClientCodex constant
- `e9795a3` feat(source): add Codex session loader
- `7127b06` feat(preview): add Codex session preview
- `562fd91` feat(launcher): add Codex launcher
- `89c0fdd` feat(cmd): add Codex CLI flags and integration

**Files Created/Modified:**
- `source/session.go` - Added ClientCodex constant
- `source/codex.go` - Codex session loader with SQLite and rollout support
- `source/codex_test.go` - Comprehensive tests for all loading scenarios
- `preview/codex.go` - Codex preview rendering functions
- `launcher/launch.go` - Codex launcher with custom command support
- `launcher/launch_test.go` - Tests for Codex launcher
- `cmd/root.go` - Added --codex/-x and --codex-cmd flags
- `main.go` - Integrated Codex into session loading and launching

**Test Coverage:**
- All existing tests pass
- New tests cover: missing home, SQLite loading, archived/non-CLI filtering, stale DB rows, rollout-only sessions, timestamp handling, session index fallback, turn counting, path filtering
- `go test ./...` passes
- `go build .` and `go install .` succeeds

**Verification:**
```bash
aps --codex -l --color never    # List Codex sessions
aps --codex -n -v               # No-launch verbose
aps --all -l --color never      # Combined mode
```

**Known Limitations:**
- No compressed rollout support (.jsonl.zst)
- No archived sessions listing
- No IDE/app/internal/subagent threads (CLI only)
- Minimal TOML parser for sqlite_home (top-level quoted strings only)
