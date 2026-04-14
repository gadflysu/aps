# Go Rewrite — Complete Specification

This document is the single source of truth for the Go rewrite of `aps`.
Reference the original `ai-pick-session.sh` only when this doc is ambiguous.

---

## 1. CLI Interface

Binary name: `aps`

### Flags

| Short | Long | Type | Default | Description |
|-------|------|------|---------|-------------|
| `-n` | `--no-launch` | bool | false | Print target directory instead of launching client |
| `-v` | `--verbose` | bool | false | With `-n`: print full launch command instead of just dir |
| `-l` | `--list` | bool | false | Non-interactive list mode; print table and exit |
| `-c` | `--claude` | bool | false | Include Claude Code sessions |
| `-o` | `--opencode` | bool | false | Include Opencode sessions |
| `-a` | `--all` | bool | false | Include both clients (equivalent to `-c -o`) |
| `-d` | `--danger` | bool | false | Claude only: launch with `--dangerously-skip-permissions` |
| `-r` | `--recursive` | bool | false | Sets `STRICT_MATCH=false` for looser path filtering |
| `-h` | `--help` | bool | false | Print help and exit |

Combined short flags MUST be supported: `-nv`, `-la`, `-lo`, `-lc`.

### Positional argument

`PATH_FILTER` — optional first positional argument.
If value is `.`, replace with `os.Getwd()`.

### Default mode

If none of `-c`, `-o`, `-a` is set → default to Claude Code only (`-c` implicit).

---

## 2. Data Sources

### 2a. Claude Code (JSONL)

**Base dir:** `~/.claude/projects/`

**Layout:**
```
~/.claude/projects/
  <url-encoded-project-path>/       ← directory name is URL-encoded abs path
    <session-uuid>.jsonl            ← one file per session
```

**Session ID:** filename without `.jsonl` extension (UUID format).

**Project path decoding:** `url.PathUnescape(dirName)` — directory names use `%2F` etc.

**Working directory (cwd):** read from the first JSONL line that has a `"cwd"` field.
Fallback: URL-decode the project directory name (use only if result starts with `/`).

**Modification time:** `os.Stat(jsonlFile).ModTime()` — used for sorting (newest first).

**JSONL record types relevant to us:**

| `type` field | What to extract |
|---|---|
| `"custom-title"` | `.customTitle` — string; LAST occurrence wins (user may rename) |
| `"user"` | First occurrence → candidate for title fallback; also count for msg count |
| any | First occurrence with `.cwd` field → working directory |

**Title extraction algorithm** (priority order):
1. LAST `custom-title` record's `.customTitle` (trim whitespace)
2. First `user` record's message text (first line only, max 50 chars)
3. `"Untitled"` fallback

For step 2, message content extraction:
- `.message.content` may be `string` or `[]object`
- If array: find first item with `"type":"text"`, use `.text`
- Skip candidates that start with any of:
  - `<local-command-caveat>`
  - `<command-message>`
  - `<command-name>`
  - `<local-command-stdout>`
  - `<bash-input>`
  - `<bash-stdout>`
  - `<task-notification>`
  - `[Request interrupted`
  - `[{'type': 'tool_result'`
- Special case: if first line is exactly `"Implement the following plan:"` and there are more lines, use `"Plan: " + firstNonEmptyLine`
- Take only `.Split("\n")[0]` (first line), cap at 50 chars

**Message count:** count all records where `"type" == "user"`.

---

### 2b. Opencode (SQLite)

**DB path:** `~/.local/share/opencode/opencode.db`
Overridable via env var `OPENCODE_DATA_DIR`.

**Schema used:**
```sql
SELECT s.id, s.title, s.directory, s.time_updated, COUNT(m.id) as message_count
FROM session s
LEFT JOIN message m ON s.id = m.session_id
GROUP BY s.id, s.title, s.directory, s.time_updated
ORDER BY s.time_updated DESC
```

Note: `time_updated` is a numeric timestamp. May be seconds (10 digits) or
milliseconds (13 digits). Detection: `if ts > 9_999_999_999 { ts /= 1000 }`.

**No hardcoded LIMIT in the rewrite** (bug fix vs original LIMIT 50).

---

### 2c. Opencode (JSON storage — legacy fallback)

Only used if SQLite DB does not exist.

**Base dir:** `~/.local/share/opencode/storage/session/global/`

**Files:** `ses_*.json`, each containing a single session object.

**Fields:**
```json
{
  "id": "...",
  "title": "...",
  "directory": "...",
  "time": { "updated": <unix_seconds> }
}
```

**Message count (JSON mode):** count files in `~/.local/share/opencode/storage/message/<session_id>/msg_*.json`.

---

## 3. Path Filter Algorithm

Applies to both Claude and Opencode sessions, identically.

**Inputs:**
- `pathFilter string` — raw CLI argument (already `.`→`pwd` normalized)
- `strictMatch bool` — `true` by default; `false` when `-r` is set
- `cwd string` — session's working directory (absolute path)

**Algorithm (evaluate in order, stop at first match):**

```
resolved_filter = realpath(expanduser(pathFilter))
path_exists     = FileExists(resolved_filter)

if path_exists:
    if resolved_filter == cwd:
        → MATCH (exact)
    else:
        resolved_cwd = realpath(cwd)  // only if cwd exists on disk
        if resolved_filter == resolved_cwd:
            → MATCH (symlink-resolved)
        if !strictMatch && len(resolved_filter) > 10:
            if resolved_filter in cwd || resolved_filter in resolved_cwd:
                → MATCH (resolved substring, non-strict only)

// Fallback: string match on raw pathFilter
if len(pathFilter) > 2 && pathFilter in cwd:
    if !strictMatch || !path_exists:
        → MATCH (raw substring)

→ NO MATCH
```

Key rule: **substring match in strict mode is allowed only when the path does not exist on disk.**
This is the fix from commit 9391bcf (e.g., `aps -l scripts` when `./scripts` doesn't exist).

---

## 4. Session Data Model

```go
type Source int
const (
    SourceClaude  Source = iota
    SourceOpencode
)

type Session struct {
    Source      Source
    ID          string    // UUID (Claude) or Opencode session ID
    Title       string    // Extracted/stored title
    CWD         string    // Absolute working directory (for launch/filter)
    CWDDisplay  string    // ~ abbreviated display path
    ProjectPath string    // Claude only: ~/.claude/projects/<encoded>
    Time        time.Time // Sort key (newest first)
    MsgCount    int
}
```

---

## 5. Display & Column Layout

### Column order (all modes)

```
TIME  ｜  TITLE  ｜  ID  ｜  MSG  ｜  [SRC]  ｜  DIRECTORY
```

`SRC` column only appears in `--all` / combined mode.

### Column widths

| Column | Width |
|---|---|
| TIME | 19 (fixed: `2006-01-02 15:04:05`) |
| TITLE | adaptive: `min(40, maxActualWidth)` across all rows |
| ID (Claude interactive) | 12 (truncate UUID to first 12 chars for display) |
| ID (Claude list) | 36 (full UUID) |
| ID (Opencode) | 30 |
| MSG | 6 |
| SRC | 11 (`"Claude Code"` width) |
| DIRECTORY | remainder (no padding) |

### ANSI colors

| Column | Interactive mode | List mode |
|---|---|---|
| TIME | `\033[32m` Green | `\033[32m` Green |
| TITLE | `\033[33m` Yellow | `\033[33m` Yellow |
| ID | `\033[36m` Cyan | `\033[36m` Cyan |
| MSG | `\033[35m` Magenta | `\033[35m` Magenta |
| SRC | `\033[35m` Magenta | `\033[35m` Magenta |
| DIRECTORY | `\033[90m` Dark grey (interactive) | `\033[37m` White (list) |
| HEADER | `\033[4;37m` Underline white | (list mode only) |

Column separator: `｜` (U+FF5C FULLWIDTH VERTICAL LINE, not ASCII `|`).

### Unicode / CJK width

Use `github.com/mattn/go-runewidth` (`runewidth.StringWidth`).
All padding and truncation MUST use display width, not `len()` or `utf8.RuneCountInString()`.

Truncation suffix: `"..."` (3 ASCII chars = width 3).

---

## 6. fzf Tab-Separated Field Formats

All hidden fields use TAB (`\t`) as delimiter. The display string is always the LAST field.

### Claude interactive (`--claude`)

```
<session_id>\t<project_path>\t<cwd>\t<display_string>
```

fzf: `--with-nth=4`, preview uses `{1}`, `{2}`, `{3}`.

### Opencode interactive (`--opencode`)

```
<session_id>\t<cwd>\t<display_string>
```

fzf: `--with-nth=3`, preview uses `{1}`, `{2}`.

### All/combined interactive (`--all`)

```
<source>\t<session_id>\t<project_path>\t<cwd>\t<display_string>
```

Where `<source>` is `"Claude Code"` or `"OpenCode"`.
`<project_path>` is `~/.claude/projects/<encoded>` for Claude sessions; empty string for Opencode.
fzf: `--with-nth=5`, preview uses `{1}`, `{2}`, `{3}`, `{4}`.

### List mode

No TAB fields. Print header row, then one formatted display string per session.
Header uses `\033[4;37m` (underline white).

---

## 7. fzf Configuration

```
--ansi
--reverse
--delimiter=\t
--height=90%
--border
--header="Select Session (Enter to Launch)"   // or client-specific
--preview-window=right:40%:wrap[:border-left] // border-left if fzf >= 0.27.0
--bind focus:transform-preview-label:...       // only if fzf >= 0.31.0
```

**Version detection:** parse `fzf --version` output.
- `border-left`: requires fzf >= 0.27.0
- `focus` event: requires fzf >= 0.31.0

**Preview command:** in the Go version, the preview is a subprocess call:
`aps --_preview-claude <session_id> <project_path> <cwd>`
(internal hidden subcommand, not exposed to users).

---

## 8. Preview Functions

### Claude preview (`preview_claude_session`)

1. Stat the `.jsonl` file for mtime
2. Parse JSONL for: title (last custom-title), message count, last 10 user messages
3. Print header block:
   ```
   ━━━ SESSION INFO ━━━
   Title:     <title>
   Time:      <mtime>
   Messages:  <count>
   Directory: <cwd>
   ━━━ RECENT MESSAGES ━━━
   • <msg1[:80]>
   • <msg2[:80]>
   ...
   ━━━ DIRECTORY LIST ━━━
   ```
4. List directory: prefer `eza`, else `ls` (BSD/GNU detection via `runtime.GOOS`)

Recent messages: last 10 user messages, newest-first, text content only, max 80 chars each.
Skip system messages (same skip-list as title extraction).
**Do NOT use `|||` separator** (bug fix) — iterate line by line in Go.

### Opencode preview (`preview_opencode_session`)

1. Query SQLite for title, time_updated, message count WHERE id = session_id
2. Print same header block format
3. List directory same as above

---

## 9. Launch Commands

### Claude Code

```
cd <cwd> && claude --resume <session_id>
// with --danger:
cd <cwd> && claude --dangerously-skip-permissions --resume <session_id>
```

`exec.Command` with `cd` first, then `exec` syscall (replace process).
If `claude` not in PATH: fall back to `exec $SHELL`.

### Opencode

```
cd <cwd> && opencode -s <session_id>
```

If `opencode` not in PATH: fall back to `exec $SHELL`.

### `--no-launch` mode

- Without `--verbose`: print `<cwd>` only
- With `--verbose`: print `cd "<cwd>" && <full_command>`

---

## 10. Environment Variables

| Variable | Default | Purpose |
|---|---|---|
| `OPENCODE_DATA_DIR` | `~/.local/share/opencode` | Override Opencode data directory |

---

## 11. Error Handling

Original silently `pass`es all exceptions. The Go rewrite should:
- Skip malformed JSONL lines silently (not fatal)
- Skip unreadable `.jsonl` files silently (not fatal)
- Log errors to stderr when `-v` / `--verbose` is set
- Fatal-exit with message on: no sessions found, client binary not in PATH (after selection)

---

## 12. Known Bugs Fixed in Rewrite

| # | Bug | Fix |
|---|---|---|
| 1 | Preview uses `|||` as message separator — breaks on `|||` in body | Iterate slice directly in Go, no join/split |
| 2 | LIMIT 50 hardcoded in Opencode SQLite query | No LIMIT; fetch all |
| 3 | Path filter logic copy-pasted 6× | Single `matchesPathFilter(filter, strict, cwd string) bool` function |
| 4 | `extract_title_from_jsonl` defined twice | Single `extractTitle(path string) string` function |
| 5 | All Python exceptions silently `pass` | Log to stderr when verbose; skip only parse errors |

---

## 13. Package Layout (suggested)

```
aps/
├── main.go
├── cmd/
│   └── root.go          // cobra/flag CLI wiring
├── source/
│   ├── claude.go        // JSONL scanning, title/cwd extraction
│   └── opencode.go      // SQLite + JSON storage loading
├── filter/
│   └── path.go          // matchesPathFilter() — single implementation
├── display/
│   ├── columns.go       // column widths, padding, truncation (runewidth)
│   └── color.go         // ANSI constants
├── picker/
│   └── fzf.go           // fzf subprocess wiring, version detection
├── preview/
│   ├── claude.go        // preview_claude_session
│   └── opencode.go      // preview_opencode_session
└── launcher/
    └── launch.go        // exec client or print dir/command
```

---

## 14. Dependencies

| Package | Purpose |
|---|---|
| `modernc.org/sqlite` | Pure-Go SQLite, no CGO, cross-compile friendly |
| `github.com/mattn/go-runewidth` | CJK-aware display width |
| standard library only for everything else | no cobra needed; `flag` pkg is sufficient |
