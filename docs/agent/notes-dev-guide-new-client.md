# Developer Guide: Adding a New Client

Checklist for adding support for a new AI coding agent client to aps.

## Steps

1. **Locate storage path** — check the client's config/docs or reference
   `notes-coding-agent-references.md` for known paths.

2. **Identify format** — SQLite table, JSONL, or JSON files.

3. **Extract fields** — `session_id`, `project_path`, `title/summary`, `timestamp`.

4. **Normalize timestamp** — convert to `time.Time`; use `parseTimestamp` in
   `source/shared.go` for the >9,999,999,999 auto-detect heuristic (seconds vs
   milliseconds). Prefer explicit unit once confirmed.

5. **Title strategy** — prefer an explicit title field; fall back to first user
   message text. Apply skip-prefix filtering if needed (see `applyTitleRules`
   in `source/claude.go`).

6. **Add path filter** — pass `cwd` through `filter.Matches` (reuse existing
   logic in `source/shared.go`).

7. **Add a `Client` constant** in `source/session.go` and a `Load<Client>`
   function in a new `source/<client>.go` file following the Claude/Opencode
   pattern.

8. **Wire flags** — add a `-x` short flag and `--<client>` long flag in
   `cmd/flags.go`; include in the combined `-a` / `--all` expansion.

9. **Add launcher** — add `launcher.<Client>` in `launcher/` following the
   `Claude`/`Opencode` pattern (use `syscall.Exec`).

10. **Tests** — add unit tests in `source/<client>_test.go` covering at least:
    - normal parse path
    - empty / missing storage directory
    - timestamp normalization
