# aps — AI Pick Session

Interactive session picker for Opencode and Claude Code, with fzf-powered fuzzy search, preview, and path filtering.

## Usage

```bash
aps               # Interactive pick (Claude sessions, cwd filter)
aps -l .          # List mode, filter by current directory
aps -l scripts    # List mode, substring filter
aps -r -l foo     # Recursive: looser substring match
aps -c            # Claude Code only
aps -o            # Opencode only
aps -a            # All clients combined
aps -n            # No-launch: print target directory
aps -d            # Danger mode (--dangerously-skip-permissions)
```

## Data Sources

| Client | Storage | Format |
|--------|---------|--------|
| Opencode | `~/.local/share/opencode/opencode.db` | SQLite |
| Claude Code | `~/.claude/projects/*/*.jsonl` | JSONL |

## Architecture

```
PATH_FILTER + STRICT_MATCH
       │
       ▼
generate_*_list / generate_*_interactive_list
       │
  Python inline script
  ├── Query/Read  (SQLite JOIN or JSONL scan)
  ├── Filter      (exact → symlink → substring)
  ├── Transform   (title, time, wcswidth)
  └── Output      (TAB-delimited for fzf)
       │
       ▼
fzf (--delimiter='\t', --with-nth, --preview)
       │
       ▼
select_*_session → launch client or print dir
```

## Key Flags

| Variable | Default | Set by |
|----------|---------|--------|
| `STRICT_MATCH` | `true` | `-r` / `--recursive` sets to `false` |
| `LIST_ONLY` | `false` | `-l` / `--list` |
| `CLAUDE_MODE` | `false` | `-c` / `--claude` |
| `OPENCODE_MODE` | `false` | `-o` / `--opencode` |
| `ALL_MODE` | `false` | `-a` / `--all` |
