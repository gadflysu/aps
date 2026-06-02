# Notes: Codex Storage Format

## Scope

Issue: #34 `Codex agent CLI support`

This note records the current Codex local storage facts needed by aps. It is based on
official OpenAI Codex source review and local CLI help, not on reading local `~/.codex`
session contents.

## Investigation Metadata

| Field | Value |
|-------|-------|
| Investigation date | 2026-06-02 |
| Local CLI version | `codex-cli 0.136.0` |
| Source repo | `openai/codex` |
| Source ref checked | `main` at `67b805fc111706aca5b32d465c94d95659bab6aa` |

## Primary Sources

| Source | Evidence |
|--------|----------|
| [`codex-rs/state/src/lib.rs`](https://github.com/openai/codex/blob/main/codex-rs/state/src/lib.rs) | `STATE_DB_FILENAME = "state_5.sqlite"` and `CODEX_SQLITE_HOME` env name |
| [`codex-rs/state/migrations/*.sql`](https://github.com/openai/codex/tree/main/codex-rs/state/migrations) | `threads` table schema and later columns such as `first_user_message`, `created_at_ms`, `updated_at_ms`, and `preview` |
| [`codex-rs/rollout/src/lib.rs`](https://github.com/openai/codex/blob/main/codex-rs/rollout/src/lib.rs) | `sessions` and `archived_sessions` subdirectory names; interactive source list |
| [`codex-rs/rollout/src/recorder.rs`](https://github.com/openai/codex/blob/main/codex-rs/rollout/src/recorder.rs) | rollout creation path and DB-vs-filesystem listing fallback behavior |
| [`codex-rs/rollout/src/list.rs`](https://github.com/openai/codex/blob/main/codex-rs/rollout/src/list.rs) | file listing, first-line `session_meta`, preview extraction, and ID lookup fallback |
| [`codex-rs/rollout/src/session_index.rs`](https://github.com/openai/codex/blob/main/codex-rs/rollout/src/session_index.rs) | `session_index.jsonl` thread-name sidecar schema |
| [`codex-rs/thread-store/src/local/list_threads.rs`](https://github.com/openai/codex/blob/main/codex-rs/thread-store/src/local/list_threads.rs) | local thread-store list flow and title enrichment order |
| [`codex-rs/protocol/src/protocol.rs`](https://github.com/openai/codex/blob/main/codex-rs/protocol/src/protocol.rs) | `SessionMeta`, `SessionSource`, `RolloutLine`, and `EventMsg::UserMessage` structures |
| [`codex-rs/message-history/src/lib.rs`](https://github.com/openai/codex/blob/main/codex-rs/message-history/src/lib.rs) | `history.jsonl` message-history schema |
| `codex resume --help` | CLI resumes by UUID or session name with `codex resume [SESSION_ID] [PROMPT]` |

## Home Directories

Codex uses two related base paths:

| Name | Resolution | Purpose |
|------|------------|---------|
| `codex_home` | `$CODEX_HOME`, else `~/.codex` | Rollout JSONL, `session_index.jsonl`, `history.jsonl`, logs, config |
| `sqlite_home` | `config.toml` `sqlite_home`, else `$CODEX_SQLITE_HOME`, else `codex_home` | SQLite state DB location |

`CODEX_SQLITE_HOME` may be relative; Codex resolves a relative value against the resolved current
working directory. `config.toml` `sqlite_home` is an absolute-path config field in official source.

## Storage Locations

| Location | Format | Role |
|----------|--------|------|
| `<sqlite_home>/state_5.sqlite` | SQLite | Queryable thread metadata index |
| `<codex_home>/sessions/YYYY/MM/DD/rollout-YYYY-MM-DDThh-mm-ss-<uuid>.jsonl` | JSONL, optionally compressed as `.zst` | Canonical active thread transcript |
| `<codex_home>/archived_sessions/...` | JSONL, optionally compressed as `.zst` | Archived thread transcript storage |
| `<codex_home>/session_index.jsonl` | JSONL | Append-only thread-name sidecar; latest entry wins |
| `<codex_home>/history.jsonl` | JSONL | Global user message history, not a complete session listing |

Current official source hard-codes `state_5.sqlite`; do not infer "highest numeric
`state_N.sqlite`" as the current authoritative rule.

## SQLite `threads` Table

The initial migration creates:

| Column | Type | Notes |
|--------|------|-------|
| `id` | text | Thread/session UUID |
| `rollout_path` | text | Path to the active or archived rollout file |
| `created_at` | integer | Unix seconds in the initial schema |
| `updated_at` | integer | Unix seconds in the initial schema |
| `source` | text | Serialized `SessionSource`, e.g. `cli`, `vscode`, custom sources, internal/subagent values |
| `model_provider` | text | Provider id |
| `cwd` | text | Working directory |
| `title` | text | Thread title |
| `sandbox_policy` | text | Stored session policy |
| `approval_mode` | text | Stored approval mode |
| `tokens_used` | integer | Token usage counter |
| `has_user_event` | integer | Legacy boolean indicating user activity |
| `archived` | integer | `0` active, `1` archived |
| `archived_at` | integer/null | Archive timestamp |
| `git_sha` | text/null | Git commit |
| `git_branch` | text/null | Git branch |
| `git_origin_url` | text/null | Git origin URL |

Later migrations add fields aps should prefer when present:

| Column | Migration | Notes |
|--------|-----------|-------|
| `cli_version` | `0005` | Codex CLI version stored in metadata |
| `first_user_message` | `0007` | First user message fallback |
| `agent_nickname` / `agent_role` / `agent_path` | `0013` / `0022` | Agent-control spawned thread metadata |
| `created_at_ms` / `updated_at_ms` | `0025` | Millisecond timestamps; migration converts older second values |
| `preview` | `0032` | Preferred list/display preview, seeded from `first_user_message` or thread goals |
| `thread_source` | `0030` | Optional analytics source classification |

Recommended aps timestamp strategy:

1. Use `updated_at_ms` when the column exists and is non-null.
2. Fall back to `updated_at * 1000`.
3. When loading from rollout files only, use file mtime for updated time and filename/session-meta
   timestamp for created time if needed.

## Rollout JSONL

Rollout files are the durable replay format. File layout:

```text
<codex_home>/sessions/YYYY/MM/DD/rollout-YYYY-MM-DDThh-mm-ss-<uuid>.jsonl
```

Each line is a `RolloutLine`:

```json
{
  "timestamp": "2026-06-02T00:00:00.000Z",
  "type": "session_meta",
  "payload": {
    "id": "019ec254-...",
    "timestamp": "2026-06-02T00:00:00Z",
    "cwd": "/abs/project",
    "originator": "codex_cli_rs",
    "cli_version": "0.136.0",
    "source": "cli",
    "model_provider": "openai"
  }
}
```

Relevant `type` values:

| Type | Purpose |
|------|---------|
| `session_meta` | First metadata line; contains thread id, cwd, source, provider, CLI version, optional git info |
| `event_msg` | Legacy UI/event messages, including `user_message` events used for preview/turn count |
| `response_item` | Model response items |
| `turn_context` | Turn context; can contain later cwd for resume matching |
| `compacted` | Compaction marker/item |

Codex list logic reads a limited head section first. It requires a readable `session_meta` and a
preview to return a file-backed item. For user preview text, it uses `EventMsg::UserMessage.message`
after stripping the internal user-message prefix; image-only user events become `[Image]`.

## Thread Names

`<codex_home>/session_index.jsonl` is append-only:

```json
{ "id": "019ec254-...", "thread_name": "name", "updated_at": "2026-06-02T00:00:00Z" }
```

Codex scans it from the end; the most recent entry for a thread id wins. Local thread-store title
enrichment prefers SQLite thread title metadata when distinct, then falls back to this sidecar.

## Message History

`<codex_home>/history.jsonl` stores user message history entries:

```json
{ "session_id": "019ec254-...", "ts": 1770000000, "text": "message" }
```

`ts` is Unix seconds. This file is append-only message history and should not be treated as the
authoritative session list. It can help with fallback summaries only after the rollout/SQLite path is
implemented.

## Recommended aps Loader Model

Use a DB-plus-rollout model:

1. Resolve `codex_home` and `sqlite_home`.
2. Try `<sqlite_home>/state_5.sqlite`.
3. Query active CLI rows from `threads` when the DB exists.
4. Verify `rollout_path` exists or can be matched by scanning rollout filenames; skip stale DB-only
   rows that cannot be resumed.
5. Scan `<codex_home>/sessions/**/*.jsonl[.zst]` as fallback and to include rollout files missing
   from SQLite.
6. De-duplicate by session id; prefer SQLite title/preview/timestamps, but preserve the verified
   rollout path.
7. Filter to `source = "cli"` for the initial Codex CLI support. Do not include internal/subagent
   sources in the picker.
8. Count turns by scanning rollout `event_msg` user messages. Do not count `response_item`,
   `turn_context`, or tool/result events.

## Launcher

Local CLI help for `codex-cli 0.136.0`:

```text
codex resume [OPTIONS] [SESSION_ID] [PROMPT]
```

`SESSION_ID` may be a UUID or session name; UUIDs take precedence. aps should exec:

```text
codex resume <session-id>
```

Use the same launcher boundary as existing agents. Change directory to the stored `cwd` before exec
for consistency with Claude/Opencode and to keep project config resolution predictable.

## Open Questions For Implementation

- Whether aps should support compressed `.jsonl.zst` rollout files in the first Codex release. Codex
  source can read compressed rollouts; Go support may require a dependency, so first-cut support may
  skip compressed archived files unless active sessions are observed compressed.
- Whether custom `config.toml` `sqlite_home` should be parsed without adding a TOML dependency. A
  minimal top-level string parser may be enough for this field.
- Whether non-CLI interactive sources (`vscode`, `atlas`, `chatgpt`) should be exposed behind a
  future flag. They are intentionally out of scope for issue #34.
