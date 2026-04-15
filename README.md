# aps — AI Pick Session

Interactive session picker for Claude Code and Opencode. Fuzzy search, three-pane preview, and path filtering — all in a pure Go TUI.

## Install

```bash
go install local/aps@latest
```

Or build from source:

```bash
git clone ...
cd aps
go install .
```

## Usage

```bash
aps                   # Interactive picker (Claude sessions, cwd filter)
aps -l .              # List mode, filter by current directory
aps -l scripts        # List mode, substring filter
aps -r -l foo         # Recursive: looser substring match
aps -c                # Claude Code only
aps -o                # Opencode only
aps -a                # Both clients combined
aps -n                # No-launch: print target directory
aps -nv               # No-launch verbose: print full launch command
aps -d                # Danger mode (--dangerously-skip-permissions)
```

### Interactive mode keys

| Key | Action |
|-----|--------|
| Type | Fuzzy filter by title, directory, ID, or time |
| `↑` / `↓` or `k` / `j` | Move cursor |
| `Space` | Toggle three-pane preview |
| `Tab` | Cycle preview focus (RECENT MESSAGES ↔ DIRECTORY) |
| `j` / `k` | Scroll focused preview pane |
| `Enter` | Launch session |
| `Esc` / `q` / `Ctrl+C` | Quit |

## Data Sources

| Client | Storage | Format |
|--------|---------|--------|
| Claude Code | `~/.claude/projects/*/*.jsonl` | JSONL |
| Opencode | `~/.local/share/opencode/opencode.db` | SQLite |

Default client is Claude Code. Use `-o` / `-a` to include Opencode.
