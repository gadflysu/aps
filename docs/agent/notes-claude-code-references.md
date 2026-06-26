# Claude Code References

Source: `~/workspace/cc/extracted/src/` (decompiled from CC 2.1.88), plus local
`~/.claude` schema scan from Claude Code 2.1.117 on 2026-06-01.

## Spinner

### Animation timing

| Path | Value |
|------|-------|
| Animation clock | 50ms tick (20fps) |
| Normal spinner frame advance | every 120ms (`Math.floor(time / 120)`) |
| Reduced-motion cycle | 2000ms total (1s visible, 1s dim) |

### Characters

Normal spinner (`getDefaultCharacters()` palindrome):
```
· ✢ ✳ ✶ ✻ ✽  →  ✽ ✻ ✶ ✳ ✢ ·
```
Full `SPINNER_FRAMES = [...DEFAULT_CHARACTERS, ...[...DEFAULT_CHARACTERS].reverse()]`

Reduced-motion glyph: `●` (slow flash, not a sequence)

### Color

Default `messageColor`: `'claude'` → `rgb(215,119,87)` (Claude orange)

Theme key → color mapping (from `src/utils/theme.ts`):
- `claude`: `rgb(215,119,87)` — Claude orange (normal spinner)
- `claudeShimmer`: `rgb(245,149,117)` — lighter orange (shimmer effect)
- `claudeBlue_FOR_SYSTEM_SPINNER`: `rgb(87,105,247)` — medium blue (system spinner)
- `error`: `rgb(171,43,63)` — red (stalled spinner interpolates toward this)

ANSI 16-color approximation used in aps: `"9"` (bright red, renders as orange in most terminals)

### Spinner modes (`SpinnerMode`)

| Mode | Trigger | Mode glyph (beside spinner) | Visual behavior |
|------|---------|----------------------------|-----------------|
| `responding` | Streaming response text | `↓` (dimmed) | Normal spinner + glimmer sweep (200ms/cycle) |
| `tool-use` | Tool call executing | `↓` (dimmed) | Normal spinner + flash effect (sin wave 1s period) |
| `tool-input` | Tool input being written | `↓` (dimmed) | Same as tool-use |
| `thinking` | Extended thinking active | `↓` (dimmed) | Normal spinner + thinking shimmer text |
| `requesting` | Sending request to API | `↑` (dimmed) | Fast glimmer sweep (50ms/cycle, left→right) |

Initial state on mount: `'responding'`

Stall detection: if response length stops growing for ~3s, `stalledIntensity` ramps 0→1, interpolating spinner color from `claude` orange toward `error` red.

### Source files

- `src/components/Spinner.tsx` — top-level component, sets `messageColor = 'claude'`
- `src/components/Spinner/SpinnerGlyph.tsx` — renders the glyph, handles stalled interpolation
- `src/components/Spinner/SpinnerAnimationRow.tsx` — owns the 50ms animation clock, computes `frame`
- `src/components/Spinner/utils.ts` — `getDefaultCharacters()`, color interpolation helpers

## JSONL record types

Each session `.jsonl` file is one typed record per line. Types discovered across sampled sessions:

### Conversation core

| type | description | key fields |
|------|-------------|------------|
| `user` | User input **or tool result**. ~71% are `tool_result` blocks, not real user input. | `message.content` (string or content-block array), `userType`, `entrypoint`, `promptId`, `uuid`/`parentUuid`, `isMeta`, `toolUseResult`, `sourceToolAssistantUUID` |
| `assistant` | Model reply — thinking, text, and/or tool_use blocks. | `message.content[]` (type: `thinking`/`text`/`tool_use`), `message.model`, `message.usage` |
| `system` | System-level messages with `subtype` discriminator. | `subtype`, `content`, `level`, `isMeta` |

`user` content variants:
- `plain_text` — actual user input (string content, no `<command-` prefix)
- `blocks=(tool_result,)` — tool execution results fed back into the conversation (71% of all `user` records)
- `blocks=(text,)` — user input as content-block array
- `starts_with=<command-message>` / `<command-name>` — slash command invocation (e.g. `/init`); `<command-args>` carries arguments
- `starts_with=<local-command-caveat>` / `<local-command-stdout>` / `<local-command-stderr>` — local command context and output
- `starts_with=<bash-input>` / `<bash-stdout>` / `<bash-stderr>` — shell command and output
- `starts_with=<task-notification>` / `<system-reminder>` — system-generated notifications
- `[Request interrupted...]` — user-cancelled tool use (string or inside array text block)

`user` record flags:
- `isMeta: true` — hidden prompt (e.g. system-injected context), not visible to user
- `toolUseResult` present — row carries tool result metadata
- `sourceToolAssistantUUID` present — row linked to assistant tool use

`system` subtypes observed locally: `turn_duration` · `stop_hook_summary` · `local_command` ·
`away_summary` · `compact_boundary` · `api_error` · `scheduled_task_fire` · `informational`

`assistant` block types: `thinking` · `text` · `tool_use` (tool names: `Bash`, `Read`, `Edit`, `Write`, etc.)

### Titles

`/resume` uses a two-path loading model:

| Path | Source function | JSONL access | Purpose |
|------|-----------------|--------------|---------|
| Fast/progressive | `getSessionFilesLite()` → `enrichLogs()` → `readLiteMetadata()` | `stat` all files, then read head+tail 64KB for visible batches | Show the first page quickly and progressively enrich more sessions |
| Full | `loadAllLogsFromSessionFile()` → `loadTranscriptFile()` | Parse the transcript, with large-file pre-compact skipping | Build full message chains and complete metadata maps |

User experience: `/resume` first sorts first-level session JSONL files by mtime, displays the first
enriched batch quickly, then continues enriching additional sessions as the user waits, scrolls, or
searches. The fast path avoids parsing every large transcript before the picker appears.

Fast-path metadata extraction (`readLiteMetadata` in `sessionStorage.ts:4771`) reads:

| Field | JSONL region | Extraction |
|-------|--------------|------------|
| `isSidechain` | head | string search for `"isSidechain": true` |
| `projectPath` | head | first `cwd` string |
| `teamName` | head | first `teamName` string |
| `agentSetting` | head | first `agentSetting` string |
| `firstPrompt` | tail then head | last `lastPrompt`, else first prompt from head, else head `content`/`text` prefix |
| `customTitle` | tail/head | last `customTitle`, else last `aiTitle` |
| `summary` | tail | last `summary` |
| `tag` | tail | last `tag` |
| `gitBranch` | tail then head | last `gitBranch`, else first head `gitBranch` |
| `pr-link` fields | tail | `prUrl`, `prRepository`, `prNumber` |

Fast-path title field priority inside `readLiteMetadata`:

```typescript
const customTitle =
  extractLastJsonStringField(tail, 'customTitle') ??  // 1. user rename (tail)
  extractLastJsonStringField(head, 'customTitle') ??  // 2. user rename (head)
  extractLastJsonStringField(tail, 'aiTitle') ??      // 3. AI title (tail)
  extractLastJsonStringField(head, 'aiTitle')          // 4. AI title (head)
```

Display priority in `getLogDisplayTitle` (`log.ts:30`) after metadata is attached to a `LogOption`:

| priority | source | field |
|----------|--------|-------|
| 1 | Agent name (`/rename`) | `agentName` |
| 2 | User-set title | `customTitle` (from `custom-title` entries) |
| 3 | AI-generated title | `customTitle` (from `ai-title` entries, if no user title) |
| 4 | Session summary | `summary` |
| 5 | First user message | `firstPrompt` after stripping display tags; skipped for autonomous `<tick>` prompts |
| 6 | Caller fallback | `defaultTitle` |
| 7 | Autonomous fallback | `"Autonomous session"` |
| 8 | Truncated session ID | `sessionId.slice(0, 8)` |

Important distinction: the fast/progressive path in Claude Code 2.1.88 does not extract
`agentName` in `readLiteMetadata`, so first-page `/resume` titles usually begin at
`customTitle`/`aiTitle`. The full path parses `agent-name` entries into the `agentNames` map, so
full logs can display `/rename` titles at priority 1.

Key design decisions:
- `readLiteMetadata` reads only first+last 64KB per JSONL (fast scan, no full parse)
- `reAppendSessionMetadata` re-writes only `custom-title` at file tail on exit — keeps user titles in the 64KB window; `ai-title` is NOT re-appended (ephemeral by design)
- `loadTranscriptFile` (full resume) populates session metadata maps including `customTitles`,
  `agentNames`, `agentSettings`, `modes`, `worktreeStates`, and `pr-link` fields. The
  `customTitles` map is populated from `custom-title` entries only, not `ai-title`, so AI titles
  are not cached and re-appended as user titles.
- Source: `src/commands/resume/resume.tsx`, `src/utils/sessionStorage.ts`, `src/utils/log.ts`

### Session state

| type | description |
|------|-------------|
| `mode` | Interaction mode (`normal`, etc.) |
| `permission-mode` | Permission level (`bypassPermissions`, etc.) |
| `last-prompt` | Marks the leaf UUID of the most recent prompt; used for session resume |
| `file-history-snapshot` | File backup snapshot for undo; `trackedFileBackups` + `timestamp` |
| `agent-name` | Agent session name |
| `queue-operation` | Message queue events (`enqueue`); `content` holds the queued text |
| `attachment` | System/tool-injected context (e.g. `mcp_instructions_delta` with `addedNames`/`addedBlocks`) |
| `agent-setting` | Agent-specific setting |
| `worktree-state` | Worktree metadata |
| `pr-link` | Pull request metadata |

## Local data location

Claude Code stores session data under `~/.claude/projects/<project>/` where `<project>` uses
`sanitizePath`: non-alphanumeric characters become `-`, with a hash suffix when the sanitized path
would exceed the filesystem limit (e.g. `-Users-sd-projects-dotfiles`).

`sanitizePath` is a lossy filename mapping, not URL encoding:

```text
/a/b   -> -a-b
/a/b-1 -> -a-b-1
```

Original hyphens are not escaped, so the directory name cannot be decoded back to a unique path.
Claude finds a transcript by applying the same mapping to the cwd used for resume.

Claude session paths have two separate cwd meanings:

- First non-empty transcript `cwd`: resume launch cwd / storage namespace key. Use this as
  `LaunchCWD` for `cd <LaunchCWD> && claude --resume <session-id>`.
- Last non-empty transcript `cwd`: latest/display cwd. Use this as `CWD` for display, filtering,
  and preview context.

Local scan on 2026-06-26 found 260 top-level JSONL files with a first `cwd`; all 260 satisfied
`sanitizePath(first cwd) == dirname(jsonl parent)`. One top-level JSONL had no `cwd`.

Main transcript files are `<session-uuid>.jsonl`. No `<session-uuid>.todos` files were observed in
the 2026-06-01 local scan; current task/todo state is stored in transcript tool calls and
`~/.claude/tasks/<session-id>/*.json`.

The `source` package discovers sessions via glob on `~/.claude/projects/*/*.jsonl`. When the JSONL
lacks a `cwd` field, the sanitized directory name is not reliably reversible; skip or handle the
session conservatively instead of treating the project directory name as URL-encoded cwd.

### Current sidecar formats

Local scan summary from Claude Code 2.1.117:

| Path | Format | Count | Purpose |
|------|--------|------:|---------|
| `~/.claude/projects/<project>/<session-id>.jsonl` | JSONL | 166 | Main resumable transcript |
| `~/.claude/projects/<project>/<session-id>/subagents/agent-*.jsonl` | JSONL | 107 | Subagent transcript |
| `~/.claude/projects/<project>/<session-id>/subagents/agent-*.meta.json` | JSON | 107 | Subagent metadata |
| `~/.claude/sessions/<pid>.json` | JSON | 1 | Live-process registry |
| `~/.claude/tasks/<session-id>/*.json` | JSON | 16 | Task state |
| `~/.claude/jobs/**` | JSON/JSONL | 3 | Background job state |
| `~/.claude/session-env/<session-id>/` | files | observed | Per-session environment scripts |
| `~/.claude/file-history/<session-id>/` | files | observed | File change backups |

`~/.claude/sessions/<pid>.json` is a registry record, not conversation data. Observed keys:
`pid`, `sessionId`, `cwd`, `startedAt`, `procStart`, `version`, `peerProtocol`, `kind`,
`entrypoint`, `name`, and `updatedAt`; `status` may appear in newer builds. Use this only as an
optional active-session signal and still verify the PID. Claude Code 2.1.88 source writes this
registry through `utils/concurrentSessions.ts`; it initially records `pid`, `sessionId`, `cwd`,
`startedAt`, `kind`, and `entrypoint`, then may update fields such as `name`, `status`, and
`updatedAt`.

Nested subagent JSONL files include `agentId` and attribution fields but no title metadata. They
should not be mixed into the main picker unless Claude Code exposes a direct resume path for them.
Claude Code's own `getSessionFilesWithMtime()` only accepts first-level UUID `*.jsonl` files in a
project directory; subagent transcripts are loaded separately through `getAgentTranscriptPath()` and
`loadAllSubagentTranscriptsFromDisk()`.

aps compatibility notes:
- Main discovery via `~/.claude/projects/*/*.jsonl` still matches top-level resumable transcripts
  and excludes nested subagent JSONL files, matching Claude Code's own main-session scan shape.
- Preserve the `CWD` vs `LaunchCWD` distinction: `CWD` follows the latest transcript cwd for
  display/filtering, while `LaunchCWD` follows the first cwd/storage namespace for resume.
- Treat `*.jsonl.wakatime` and `subagents/*.jsonl` as sidecars for the main picker.
- Prefer `agent-name` over `custom-title` when matching Claude Code display priority.
- Count `user.message.content` arrays containing `{"type":"text"}` as real user turns; arrays
  containing only `tool_result` remain tool feedback.
