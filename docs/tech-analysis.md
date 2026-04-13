# aps Technical Analysis

## Core Stack

### 1. Bash + Python Hybrid

**Rationale:** Bash owns process orchestration and tool integration (fzf, argument parsing, client launch); Python handles the parts Bash does poorly — SQLite queries, JSON parsing, Unicode width calculation, and formatted output.

**Trade-offs:**
- Pro: each language does what it is good at
- Con: Python is embedded in heredocs, losing IDE support and making debugging harder
- Con: two mental models to maintain

**Alternatives considered:**
- Pure Python: cleaner, but loses tight Bash ecosystem integration
- Pure Bash + jq + sqlite3: feasible but CJK column alignment becomes a nightmare

### 2. Data Sources: SQLite vs JSONL

| Client | Format | Why |
|--------|--------|-----|
| Opencode | SQLite (`opencode.db`) | Relational JOIN for message counts; structured queries |
| Claude Code | JSONL (`~/.claude/projects/*/*.jsonl`) | Append-only log; one line per event |

**Known issues:**
- JSONL requires full file scan on every invocation (no index)
- No caching — cold start re-reads all files every time

### 3. fzf UI

**Key design choices:**
- TAB-delimited output: `ID\tDIR\tDISPLAY_STRING` — separates hidden data from visible columns
- `--with-nth` hides internal fields so fzf only searches visible text
- Preview via `source`-ing the script itself — reuses Bash functions without a separate helper binary

### 4. Unicode / CJK Column Width

```python
def wcswidth(s):
    for c in s:
        ea = unicodedata.east_asian_width(c)
        width += 2 if ea in ('F', 'W') else 1
```

`len()` counts code points, not terminal columns. CJK characters occupy 2 columns each. Without `wcswidth`, all alignment breaks when titles contain Chinese/Japanese/Korean text.

### 5. Path Filter Logic

Three-tier match (evaluated in order):

1. **Exact** — `resolved_filter == cwd`
2. **Symlink-resolved** — `realpath(resolved_filter) == realpath(cwd)`
3. **Substring** — `path_filter in cwd`
   - In strict mode (`STRICT_MATCH=true`, default): only allowed when `path_filter` does not resolve to an existing path
   - In recursive mode (`-r`): always allowed

`STRICT_MATCH=true` is the default; `-r / --recursive` sets it to `false`.

## Architecture Assessment

### What works well
- Clean function boundaries: `generate_*`, `select_*`, `preview_*`
- fzf version detection (`fzf_supports_border_left`) for backward compatibility
- Guard against recursive `source`: `[[ "${BASH_SOURCE[0]}" != "${0}" ]]`
- Centralized ANSI color constants

### Known weaknesses
- Path filter logic copy-pasted across 6 `generate_*` functions — a single-site fix would require 6 edits
- `extract_title_from_jsonl` and `extract_cwd_from_jsonl` defined twice (interactive vs list path)
- All Python exceptions silently `pass` — mismatches and parse errors are invisible
- No caching; performance degrades linearly with number of JSONL files
- No tests

## Scores

| Dimension | Score | Notes |
|-----------|-------|-------|
| Feature completeness | 5/5 | Both clients, path filter, preview, list mode |
| Maintainability | 3/5 | Bash+Python mix; heavy duplication |
| Performance | 3/5 | Fast start-up; no cache; slow at scale |
| UX | 5/5 | fzf, CJK-safe alignment, color output |
| Extensibility | 4/5 | Clear structure; adding a new client is straightforward |

---

## Fix Log

### 2026-03-20 — path filter substring match in strict mode (commit 9391bcf)

**Symptom:** `aps -l scripts` returned nothing while `aps -l .` worked correctly.

**Root cause:** `os.path.realpath("scripts")` → `/path/to/scripts/scripts` (does not exist). Because `STRICT_MATCH=true`, the `not strict_match` guard on the substring branch was `False`, so the branch was skipped entirely.

**Fix:** Introduced `path_exists` flag. Substring match is now allowed even in strict mode when the resolved path does not exist:

```python
if len(path_filter) > 2 and path_filter in cwd and (not strict_match or not path_exists):
    match_found = True
```

**Scope:** All `generate_*` functions (Claude / Opencode / All × interactive / list).
