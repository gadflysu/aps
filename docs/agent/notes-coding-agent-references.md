# Coding Agent References

Reference document recording how each AI coding agent client stores sessions
on disk: storage paths, schemas, field extraction, and timestamp units.
Implementation-agnostic — describes each client's storage format as a factual
record.

## Investigation Sources

| Source | Description | Date |
|--------|-------------|------|
| On-machine inspection | Files present on the investigating machine (macOS, Apple Silicon) | 2026-05-12 |
| agf source review | https://github.com/subinium/agf scanner source, v0.11.1 | 2026-04-14 |

---

## Clients

### Claude Code

**Version investigated:** 2.1.117
**Investigation method:** On-machine inspection + agf source review (v0.11.1)
**Investigation date:** 2026-05-12

#### Storage Locations

| Location | Format | Role |
|----------|--------|------|
| `~/.claude/history.jsonl` | JSONL | Index — one record per message, all sessions |
| `~/.claude/projects/<url-encoded-path>/` | Directory | Per-project session container |
| `~/.claude/projects/<url-encoded-path>/<session-id>.jsonl` | JSONL | Per-session full message log |

Project directory names are URL-percent-encoded absolute paths
(e.g. `/Users/dsu/foo` → `%2FUsers%2Fdsu%2Ffoo`).

#### Primary Index: `history.jsonl`

One JSON object per line:

```json
{
  "display": "user-visible message text",
  "pastedContents": {},
  "timestamp": 1770969849764,
  "project": "/Users/dsu/some/project",
  "sessionId": "550e8400-e29b-41d4-a716-446655440000"
}
```

| Field | Type | Notes |
|-------|------|-------|
| `display` | string | Collapsed display text for the message (whitespace-normalized) |
| `pastedContents` | object | Pasted content map (may be empty) |
| `timestamp` | integer | Unix **milliseconds** — confirmed on-machine (~1.77 trillion) |
| `project` | string | Absolute path to project root (always the real root, not a worktree path) |
| `sessionId` | string | UUID; groups lines into sessions |

Sessions are reconstructed by grouping lines on `sessionId`. The latest
`timestamp` and `project` among the group are the session's timestamp and
project path.

#### Per-Session JSONL: `<session-id>.jsonl`

Each line is a JSON record with a `type` field.

**Record types:**

| `type` | Purpose |
|--------|---------|
| `summary` | First record in the file; contains session metadata |
| `custom-title` | User-set custom title for the session; may appear multiple times |
| `user` | A user turn; contains the message body |
| `ai-title` | AI-generated session title (agf v0.11.1); field `aiTitle: string` |
| `system` + `subtype: "away_summary"` | Idle recap written by Claude Code on inactivity; field `content: string`, `timestamp: string` (RFC 3339) |

**Field reference:**

| JSON key | Type | Record type | Notes |
|----------|------|-------------|-------|
| `type` | string | all | Record type discriminator |
| `cwd` | string | `summary` | Session working directory (absolute path). For worktree sessions this is the worktree path, e.g. `<root>/.claude/worktrees/<name>` |
| `version` | int | `summary` | Format version number (observed value: 1) |
| `customTitle` | string | `custom-title` | User-defined session title; if multiple records exist, the last one wins |
| `message.content` | string \| array | `user` | Message body; array elements have the form `{type: "text", text: string}` |
| `aiTitle` | string | `ai-title` | AI-generated title, emitted early in the session |
| `content` | string | `system`/`away_summary` | Idle recap text (may be suffixed with `(disable recaps in /config)`) |
| `subtype` | string | `system` | Sub-classification; `"away_summary"` identifies recap records |

**`user` records containing non-user-input content:**

These prefixes appear in `user`-type records but are injected by Claude Code
internally, not typed by the user:

| Prefix | Origin |
|--------|--------|
| `<local-command-caveat>` | System note about local command restrictions |
| `<command-message>` | Invoked command message body |
| `<command-name>` | Command name record |
| `<local-command-stdout>` | Local command stdout |
| `<bash-input>` | Bash command input |
| `<bash-stdout>` | Bash command output |
| `<task-notification>` | Task status notification |
| `[Request interrupted` | Record produced when the user interrupts a request |
| `[{'type': 'tool_result'` | Tool call result content |

**Special message pattern:**

A `user` record whose content begins with `"Implement the following plan:"` is
produced by the Claude Code Plan workflow. The lines following the header
contain the actual plan steps.

#### Worktree Detection

Read the `cwd` field from the per-session JSONL. If the value contains
`/.claude/worktrees/`, the segment after that path component is the worktree
name. agf reads a head slice (16 KB) and tail slice (256 KB) per file;
`cwd` is reliably in the head.

#### Git Branch

Read `.git/HEAD` from the `project` path (the real project root, not the
worktree). Strip the `ref: refs/heads/` prefix to get the branch name.
Returns nothing for detached HEAD state. No timeout needed for local reads
(agf removed the earlier 100 ms timeout in v0.11.1).

---

### Opencode

**Version investigated:** 1.14.21
**Investigation method:** On-machine inspection + agf source review (v0.11.1)
**Investigation date:** 2026-05-12

#### Storage Locations

Base path: `$XDG_DATA_HOME/opencode/` (defaults to `~/.local/share/opencode/` when
`XDG_DATA_HOME` is unset — standard XDG Base Directory Specification).

| Location | Format | Role |
|----------|--------|------|
| `<base>/opencode.db` | SQLite | Primary (~63 MB observed on-machine) |
| `<base>/storage/session/global/ses_*.json` | JSON | Legacy (pre-SQLite format) |
| `<base>/storage/message/<session_id>/msg_*.json` | JSON | Legacy message files |

#### Schema: `session` Table (SQLite)

| Column | Type | Notes |
|--------|------|-------|
| `id` | string | Session identifier |
| `title` | string | Session title |
| `directory` | string | Absolute working directory path |
| `time_updated` | integer | Unix **milliseconds** — confirmed on-machine (~1.776 trillion) |
| `time_archived` | integer \| null | Non-null when session has been archived |
| `parent_id` | string \| null | Non-null for subagent sessions (child sessions spawned by the parent) |

**Extraction query:**

```sql
SELECT s.id, s.title, s.directory, s.time_updated,
       GROUP_CONCAT(sub.title, '|||')
FROM session s
LEFT JOIN session sub ON sub.parent_id = s.id
WHERE s.time_archived IS NULL
  AND s.parent_id IS NULL
GROUP BY s.id
ORDER BY s.time_updated DESC
```

- `time_archived IS NULL` — excludes archived sessions
- `parent_id IS NULL` — top-level sessions only; subagent sessions excluded from listing
- `GROUP_CONCAT(..., '|||')` — subagent session titles, `|||`-delimited, for richer summary display

#### Schema: `message` Table (SQLite)

| Column | Type | Notes |
|--------|------|-------|
| `id` | string | Message identifier |
| `session_id` | string | Foreign key to `session.id` |
| `role` | string | `"user"` or `"assistant"` |

Used via `LEFT JOIN message m ON s.id = m.session_id` + `COUNT(m.id)` to compute
per-session message count without reading individual message content.

#### Legacy JSON Format

Files at `storage/session/global/ses_*.json`:

```json
{
  "id": "ses_...",
  "title": "session title",
  "directory": "/abs/path",
  "time": { "updated": 1234567890000.0 }
}
```

`time.updated` is a float; same millisecond epoch as the SQLite column.

Message count: count files matching `storage/message/<session_id>/msg_*.json`.

---

### Codex (OpenAI)

**Version investigated:** Unknown
**Investigation method:** agf source review (v0.11.1)
**Investigation date:** 2026-04-14

#### Storage Locations

| Location | Format | Role |
|----------|--------|------|
| `~/.codex/state_<N>.sqlite` | SQLite | Primary — use file with highest numeric `N` suffix |
| `~/.codex/history.jsonl` | JSONL | Display-text summaries |
| `~/.codex/sessions/**/*.jsonl` | JSONL | Legacy fallback (rollout files) |

#### Primary Schema: SQLite `threads` Table

```sql
SELECT id, cwd, title, updated_at, git_branch, first_user_message
FROM threads
WHERE archived = 0 AND cwd != ''
ORDER BY updated_at DESC
```

| Column | Type | Notes |
|--------|------|-------|
| `id` | string | Session identifier |
| `cwd` | string | Absolute working directory |
| `title` | string | Session title |
| `updated_at` | integer | Unix **seconds** (multiply × 1000 for milliseconds) |
| `git_branch` | string \| null | Git branch name |
| `first_user_message` | string | First user message text |
| `archived` | integer | 0 = active, non-zero = archived |

Sessions whose rollout JSONL has been deleted but whose SQLite row remains
are orphans; `claude --resume <id>` errors on these. They can be detected by
cross-referencing `id` against `payload.id` values found in
`~/.codex/sessions/**/*.jsonl` first lines.

#### Secondary Schema: `history.jsonl`

One JSON object per line:

```json
{ "session_id": "uuid", "ts": 1234567890.0, "text": "display string" }
```

`ts` is a float in Unix **seconds**. Grouped by `session_id`, sorted
newest-first, used as richer summaries when present.

#### Legacy Fallback: `sessions/**/*.jsonl`

Files are named `rollout-*.jsonl`. First line schema:

```json
{
  "type": "session_meta",
  "payload": {
    "id": "uuid",
    "cwd": "/abs/path",
    "timestamp": "2026-04-29T00:00:00Z",
    "git": { "branch": "main" }
  }
}
```

`timestamp` is an ISO 8601 string.

---

### Cursor Agent

**Version investigated:** Unknown
**Investigation method:** agf source review (v0.11.1)
**Investigation date:** 2026-04-14

#### Storage Locations

| Location | Format | Role |
|----------|--------|------|
| `~/.cursor/projects/<encoded_path>/agent-transcripts/<session_id>.txt` | Plain text | Session transcript |
| `~/.cursor/chats/<workspace_hash>/<session_id>/store.db` | SQLite | Session metadata |

#### Path Encoding

Project directory names encode the absolute path by replacing all `/`
separators with `-` and dropping the leading slash
(e.g. `/Users/dsu/Desktop/foo` → `Users-dsu-Desktop-foo`).

Directories starting with `var-folders` are temporary and are skipped.

Decoding requires backtracking: split on `-`, greedily join segments
left-to-right checking `is_dir()` to resolve ambiguous `-` vs `/`.

#### Metadata Schema: `store.db`

Table `cursorDiskKV`, key `composerData`, value is a **hex-encoded** JSON string:

```json
{ "name": "session title", "createdAt": 1234567890000 }
```

| Field | Type | Notes |
|-------|------|-------|
| `name` | string | Session title |
| `createdAt` | integer | Unix **milliseconds** |

Timestamp fallback: if `store.db` is absent or `composerData` key is missing,
use the filesystem mtime of the `.txt` transcript file.

---

### Gemini

**Version investigated:** Unknown
**Investigation method:** agf source review (v0.11.1)
**Investigation date:** 2026-04-14

#### Storage Locations

| Location | Format | Role |
|----------|--------|------|
| `~/.gemini/projects.json` | JSON | Maps project paths to storage directory names |
| `~/.gemini/tmp/<dir_name>/chats/session-*.json` | JSON | Per-session chat files |

#### Path Map: `projects.json`

```json
{ "projects": { "/abs/project/path": "dir_name", ... } }
```

Maps `project_path → dir_name`. Legacy entries use `SHA256(project_path)` as
the key instead of the raw path; both forms may coexist. When the same
`sessionId` appears in both a hash dir and a named dir (migration artifact),
keep the entry with the latest `lastUpdated`.

#### Session File Schema

```json
{
  "sessionId": "uuid",
  "lastUpdated": "2026-01-01T00:00:00Z",
  "startTime": "2026-01-01T00:00:00Z",
  "messages": [
    { "type": "user", "content": "plain text or [{\"text\": \"...\"}]" }
  ]
}
```

| Field | Type | Notes |
|-------|------|-------|
| `sessionId` | string | Session UUID |
| `lastUpdated` | string | ISO 8601; preferred timestamp source |
| `startTime` | string | ISO 8601; fallback if `lastUpdated` absent |
| `messages` | array | `type` is `"user"` or `"assistant"`; `content` is a plain string or `[{text}]` array |

**Size note:** session files can reach 28 MB or larger. Read at most 64 KB;
if JSON parsing fails on the truncated slice, fall back to string-search
extraction for `sessionId`, `lastUpdated`/`startTime`, and first user message.

---

### Kiro (AWS)

**Version investigated:** Unknown
**Investigation method:** agf source review (v0.11.1)
**Investigation date:** 2026-04-14

#### Storage Locations

| Location | Format | Role |
|----------|--------|------|
| `~/Library/Application Support/kiro-cli/data.sqlite3` | SQLite | Primary (macOS path) |

#### Schema: `conversations_v2` Table

```sql
SELECT key, conversation_id, value, updated_at
FROM conversations_v2
ORDER BY updated_at DESC
```

| Column | Type | Notes |
|--------|------|-------|
| `key` | string | Absolute project directory path |
| `conversation_id` | string | Session UUID |
| `value` | string | JSON blob (see below) |
| `updated_at` | integer | Unix **milliseconds** |

`value` JSON structure:

```json
{
  "messages": [
    { "role": "user", "content": "string or [{text: ...}]" },
    { "role": "assistant", "content": "..." }
  ]
}
```

Session summary: first message where `role == "user"`, `content` field.

---

### Pi

**Version investigated:** Unknown
**Investigation method:** agf source review (v0.11.1)
**Investigation date:** 2026-04-14

#### Storage Locations

| Location | Format | Role |
|----------|--------|------|
| `~/.pi/agent/sessions/**/*.jsonl` | JSONL | Session files (recursive walk) |

#### Session File Schema

First line of each `.jsonl` file is the session header:

```json
{ "type": "session", "id": "uuid", "timestamp": "2026-01-01T00:00:00Z", "cwd": "/abs/path" }
```

| Field | Type | Notes |
|-------|------|-------|
| `type` | string | Always `"session"` for header lines |
| `id` | string | Session UUID |
| `timestamp` | string | ISO 8601; fallback to file mtime if absent or unparseable |
| `cwd` | string | Absolute working directory |

No title or summary field in the session header. `pi --resume` always resumes
the most recent session in a directory, so when multiple sessions share the
same `cwd`, only the one with the latest timestamp is relevant for resuming.

---

### Hermes Agent

**Version investigated:** Unknown
**Investigation method:** agf source review (v0.11.1)
**Investigation date:** 2026-05-12
**Note:** Added in agf v0.11.0; not present in the original 2026-04-14 research.

#### Storage Locations

| Location | Format | Role |
|----------|--------|------|
| `~/.hermes/state.db` | SQLite | Primary |

#### Schema: `sessions` + `messages` Tables

```sql
SELECT s.id, s.title, s.source, s.model, s.message_count,
       CAST(COALESCE(m.last_active, s.started_at) * 1000 AS INTEGER) AS ts_ms,
       GROUP_CONCAT(child.title, '|||'),
       -- first 4 user messages, chronological, '|||'-joined
       (SELECT GROUP_CONCAT(content, '|||') FROM (
           SELECT content FROM messages
           WHERE session_id = s.id AND role = 'user' AND content IS NOT NULL
           ORDER BY timestamp ASC LIMIT 4
       )) AS user_msgs
FROM sessions s
LEFT JOIN (SELECT session_id, MAX(timestamp) AS last_active FROM messages GROUP BY session_id) m
  ON m.session_id = s.id
LEFT JOIN sessions child ON child.parent_session_id = s.id
WHERE s.parent_session_id IS NULL
GROUP BY s.id
ORDER BY ts_ms DESC
```

| Column | Type | Notes |
|--------|------|-------|
| `id` | string | Session identifier; CLI/TUI sessions follow the format `YYYYMMDD_HHMMSS_<6hex>` |
| `title` | string \| null | Session title (may be null for short sessions) |
| `source` | string | Session source (e.g. `"cli"`) |
| `model` | string \| null | Model name (e.g. `"anthropic/claude-opus-4-6"`) |
| `message_count` | integer | Total message count |
| `started_at` | float | Session start time, Unix **seconds** |
| `parent_session_id` | string \| null | Non-null for delegation/compression child sessions |

Timestamps in `sessions.started_at` and `messages.timestamp` are Unix **seconds**
(float); multiply × 1000 for milliseconds.

**Session ID formats:**
- CLI/TUI sessions: `YYYYMMDD_HHMMSS_<6hex>` (e.g. `20260504_093414_1e69e0`)
- API integration sessions: `api-<16hex>`
- Dashboard sessions: `dashboard:<scope>:<name>`
- Named sessions: freeform string (e.g. `research`, `test`)

API/dashboard/named sessions receive messages from external integrations
(webhooks, scheduled jobs), so their `role='user'` rows are not the user's
own prompts.

**Project path:** Hermes is cwd-independent (runs from `~/.hermes` regardless
of invocation directory). `project_path` is empty; resume should not `cd`.
