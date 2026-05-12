# Similar Projects

Projects with overlapping goals to aps: session listing and resuming for AI
coding agents.

---

## agf (subinium/agf)

**Repository:** https://github.com/subinium/agf
**Language:** Rust
**Version researched:** v0.11.1
**Research date:** 2026-04-14 (source review); updated 2026-05-12

### Session Data Model

```
session_id    String    — unique identifier
project_path  String    — absolute path to working directory
project_name  String    — basename of project_path (empty for Hermes)
summaries     Vec<String> — display strings (title / messages), newest-first
timestamp     i64       — Unix milliseconds
git_branch    Option<String>
worktree      Option<String> — set when session ran inside a git worktree
recap         Option<String> — latest away_summary, optionally prefixed with aiTitle (Claude only)
```

All scanners normalize `timestamp` to Unix milliseconds. Clients that store
seconds (Codex `updated_at`, Hermes `started_at`) are multiplied by 1000 at
read time.

### Architecture Comparison with aps

| Aspect | agf (v0.11.1) | aps (current) |
|--------|---------------|---------------|
| Language | Rust | Go |
| Clients supported | 8 (Claude, Opencode, Codex, Cursor, Gemini, Kiro, Pi, Hermes) | 2 (Claude, Opencode) |
| Parallelism | `thread::spawn` per client (8 threads) + `rayon::par_iter` within Claude scanner | Sequential |
| Claude primary source | `history.jsonl` (fast index) + per-session JSONL for worktree/recap | `projects/*/*.jsonl` (full parse, richer title logic) |
| Claude title | `display` field from history (whitespace-collapsed) | `custom-title` → first user message (skip-prefix filtered) |
| Opencode | SQLite + `time_archived` filter + subagent title JOIN | SQLite, no archived filter, no subagent JOIN |
| CJK alignment | Not handled | `TruncateWidth` (correct) |
| Shell integration | TUI only | `-l` list, `-n` print-dir, `-nv` full cmd |
| Recap / ai-title | Extracted from per-session JSONL | Not extracted |
| Git branch | Read from `.git/HEAD` | Not extracted |
| Worktree | Detected, shown in TUI | Not extracted |
