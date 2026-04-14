# AI Client Session Storage Formats

Reference for adding new client support to aps.
Derived from agf source: https://github.com/subinium/agf (read 2026-04-14)

---

## Data Model (agf Session struct)

```
session_id    string   — unique ID
project_path  string   — absolute path to working directory
project_name  string   — basename of project_path
summaries     []string — display strings (title / first user message), newest-first
timestamp     i64      — Unix milliseconds
git_branch    ?string
worktree      ?string  — set when session ran inside a git worktree
```

---

## Concurrency Model

agf achieves fast startup via two-level parallelism:

1. **Per-client threads** (`scan_all` in `scanner/mod.rs`):
   spawns one `thread::spawn` per client (7 total), collects results, sorts by timestamp.
   All 7 clients are scanned simultaneously regardless of which are installed.

2. **Intra-client rayon** (Claude scanner only):
   after listing all JSONL paths, uses `rayon::into_par_iter()` to scan worktree metadata
   across all files in parallel using the thread pool.

For aps (Python): the equivalent would be `concurrent.futures.ThreadPoolExecutor` for
the per-client level, and per-file parallelism inside the Claude scanner.

---

## Clients

### Claude Code

| Field | Value |
|-------|-------|
| Base dir | `~/.claude/` |
| **Primary source** | `~/.claude/history.jsonl` |
| Format | JSONL |

**Key insight:** agf reads `~/.claude/history.jsonl`, **not** `~/.claude/projects/*/session.jsonl`.

`history.jsonl` line schema:
```json
{ "display": "user message text", "timestamp": 1234567890.0,
  "project": "/abs/path/to/project", "sessionId": "uuid" }
```

- Groups lines by `sessionId`; keeps latest `timestamp` and `project`
- `summaries` = all `display` values sorted newest-first
- Worktree detection: reads first 3 lines of each `~/.claude/projects/*/<id>.jsonl`,
  checks if `cwd` contains `/.claude/worktrees/<name>`.
  File list is built first, then scanned with `rayon::into_par_iter()` — all JSONL files
  read concurrently across CPU cores.
- Git branch: reads `.git/HEAD` of project root (100 ms timeout)

**aps current approach:** scans `~/.claude/projects/*/*.jsonl`, extracts `custom-title`
or first non-system user message. Richer title logic but misses `history.jsonl` efficiency.

---

### Opencode

| Field | Value |
|-------|-------|
| Base dir | `~/.local/share/opencode/` |
| **Primary source** | `opencode.db` (SQLite) |

Query:
```sql
SELECT s.id, s.title, s.directory, s.time_updated,
       GROUP_CONCAT(sub.title, '|||')
FROM session s
LEFT JOIN session sub ON sub.parent_id = s.id
WHERE s.time_archived IS NULL AND s.parent_id IS NULL
GROUP BY s.id
ORDER BY s.time_updated DESC
```

- Filters: `time_archived IS NULL`, `parent_id IS NULL` (top-level only)
- Subagent titles concatenated as additional summaries (agf) — aps does not read subagent titles

**aps current approach:** similar, but no `time_archived` filter and no subagent title JOIN.

---

### Codex (OpenAI)

| Field | Value |
|-------|-------|
| Base dir | `~/.codex/` |
| **Primary source** | `~/.codex/state_<N>.sqlite` (latest by filename) |
| **Summary source** | `~/.codex/history.jsonl` |
| **Fallback** | `~/.codex/sessions/**/*.jsonl` |

SQLite (`threads` table):
```sql
SELECT id, cwd, title, updated_at, git_branch, first_user_message
FROM threads
WHERE archived = 0 AND cwd != ''
ORDER BY updated_at DESC
```
`updated_at` is Unix **seconds** (multiply × 1000 for ms).

`history.jsonl` line schema:
```json
{ "session_id": "uuid", "ts": 1234567890.0, "text": "display string" }
```

JSONL fallback (legacy): first line must be `{type: "session_meta", payload: {id, cwd, timestamp, git: {branch}}}`.

---

### Cursor Agent

| Field | Value |
|-------|-------|
| Base dir | `~/.cursor/` |
| **Primary source** | `~/.cursor/projects/<encoded_path>/agent-transcripts/<session_id>.txt` |
| **Metadata** | `~/.cursor/chats/<workspace_hash>/<session_id>/store.db` |

Path encoding: directory separators replaced with `-` (e.g. `Users-dsu-Desktop-foo`).
Decoding uses backtracking: splits on `-`, greedily joins segments checking `is_dir()`.

`store.db` has a `cursorDiskKV` table; key `composerData`, value = hex-encoded JSON:
```json
{ "name": "session title", "createdAt": 1234567890000 }
```
`createdAt` is Unix **milliseconds**.

Timestamp fallback: file mtime of the `.txt` transcript.

---

### Gemini

| Field | Value |
|-------|-------|
| Base dir | `~/.gemini/` |
| **Primary source** | `~/.gemini/tmp/<dir_name>/chats/session-*.json` |
| **Path map** | `~/.gemini/projects.json` |

`projects.json` maps project_path → dir_name (or SHA256(project_path) → dir_name for old hash dirs).

Session JSON schema:
```json
{
  "sessionId": "uuid",
  "lastUpdated": "2026-01-01T00:00:00Z",
  "messages": [
    { "type": "user", "content": "text or [{text: ...}]" }
  ]
}
```

Performance note: files can reach 28 MB+. agf reads at most 64 KB per file;
falls back to string-search extraction for truncated JSON.

---

### Kiro (AWS)

| Field | Value |
|-------|-------|
| Base dir | macOS: `~/Library/Application Support/kiro-cli/` |
| **Primary source** | `data.sqlite3` |

Table `conversations_v2`:
```sql
SELECT key, conversation_id, value, updated_at
FROM conversations_v2
ORDER BY updated_at DESC
```
- `key` = project directory path
- `conversation_id` = session UUID
- `value` = JSON blob: `{messages: [{role: "user"|"assistant", content: ...}]}`
- `updated_at` = Unix **milliseconds**

Summary: first `role == "user"` message from `value` JSON.

---

### Pi

| Field | Value |
|-------|-------|
| Base dir | `~/.pi/agent/sessions/` |
| **Primary source** | `**/*.jsonl` (walkdir) |

First line of each JSONL is the session header:
```json
{ "type": "session", "id": "uuid", "timestamp": "2026-01-01T00:00:00Z", "cwd": "/abs/path" }
```

No summaries. Deduplicates by `project_path`, keeping only the most recent session per directory
(because `pi --resume` always resumes the latest session in a directory).

---

## Architecture Comparison: agf vs aps

| Aspect | agf | aps |
|--------|-----|-----|
| Language | Rust | Bash + Python |
| Parallelism | `thread::spawn` per client (7 threads) + `rayon::par_iter` within Claude scanner | Sequential Python |
| Claude source | `history.jsonl` (fast, single file) | `projects/*/*.jsonl` (full parse, richer titles) |
| Opencode | SQLite + subagent JOIN | SQLite, no subagent titles |
| Title logic | `display` field from history | custom-title → first user message (whitelist filter) |
| CJK alignment | Not handled | `wcswidth` (correct) |
| Shell integration | TUI only | `-l` list, `-n` print-dir, `-nv` full cmd |

---

## Adding a New Client to aps

Checklist based on above patterns:

1. **Locate storage path** (check `config.rs` or client docs)
2. **Identify format**: SQLite table, JSONL, or JSON files
3. **Extract fields**: `session_id`, `project_path`, `title/summary`, `timestamp`
4. **Timestamp unit**: seconds vs milliseconds — normalize to seconds for display
5. **Title strategy**: prefer explicit title field; fall back to first user message
6. **Add path filter logic** (reuse existing `path_filter` + `path_exists` pattern)
7. **Add `-X` flag and `generate_X_*` functions** following existing Claude/Opencode pattern
