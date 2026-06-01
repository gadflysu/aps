# Claude Code References

Source: `~/workspace/cc/extracted/src/` (decompiled from CC 2.1.88)

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

Theme key → hex mapping (from `src/utils/theme.js`):
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
| `user` | User input **or tool result**. ~71% are `tool_result` blocks, not real user input. | `message.content` (string or content-block array), `userType`, `entrypoint`, `promptId`, `uuid`/`parentUuid` |
| `assistant` | Model reply — thinking, text, and/or tool_use blocks. | `message.content[]` (type: `thinking`/`text`/`tool_use`), `message.model`, `message.usage` |
| `system` | System-level messages with `subtype` discriminator. | `subtype`, `content`, `level`, `isMeta` |

`user` content variants:
- `plain_text` — actual user input (string content, no `<command-` prefix)
- `blocks=(tool_result,)` — tool execution results fed back into the conversation (71% of all `user` records)
- `blocks=(text,)` — user input as content-block array
- `starts_with=<command->` — slash command invocation (e.g. `/init`)
- `starts_with=<local-command>` — local command stdout (e.g. `<local-command-stdout>...`)

`system` subtypes: `turn_duration` · `stop_hook_summary` · `local_command` · `away_summary`

`assistant` block types: `thinking` · `text` · `tool_use` (tool names: `Bash`, `Read`, `Edit`, `Write`, etc.)

### Titles

Title resolution in `/resume` picker (`readLiteMetadata` in `sessionStorage.ts:4771`):

```typescript
const customTitle =
  extractLastJsonStringField(tail, 'customTitle') ??  // 1. user rename (tail)
  extractLastJsonStringField(head, 'customTitle') ??  // 2. user rename (head)
  extractLastJsonStringField(tail, 'aiTitle') ??      // 3. AI title (tail)
  extractLastJsonStringField(head, 'aiTitle')          // 4. AI title (head)
```

Display priority in `getLogDisplayTitle` (`log.ts:30`):

| priority | source | field |
|----------|--------|-------|
| 1 | Agent name (`/rename`) | `agentName` |
| 2 | User-set title | `customTitle` (from `custom-title` entries) |
| 3 | AI-generated title | `customTitle` (from `ai-title` entries, if no user title) |
| 4 | Session summary | `summary` |
| 5 | First user message | `firstPrompt` (stripped, 200 chars) |
| 6 | Truncated session ID | `sessionId.slice(0, 8)` |

Key design decisions:
- `readLiteMetadata` reads only first+last 64KB per JSONL (fast scan, no full parse)
- `reAppendSessionMetadata` re-writes only `custom-title` at file tail on exit — keeps user titles in the 64KB window; `ai-title` is NOT re-appended (ephemeral by design)
- `loadTranscriptFile` (full resume) only populates from `custom-title`, not `ai-title`
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

## Local data location

Claude Code stores session data under `~/.claude/projects/<project>/` where `<project>` uses
dash-based path encoding (e.g. `-Users-sd-projects-dotfiles`).

Files: `<session-uuid>.jsonl` and `<session-uuid>.todos`.

The `source` package discovers sessions via glob on `~/.claude/projects/*/*.jsonl`. When the JSONL
lacks a `cwd` field, the encoded directory name is decoded with `url.PathUnescape` as fallback.
