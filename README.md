# aps — AI Pick Session

[![Go Report Card](https://goreportcard.com/badge/github.com/gadflysu/aps)](https://goreportcard.com/report/github.com/gadflysu/aps)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

Interactive session picker for Claude Code and Opencode. Fuzzy search, three-pane preview, and path filtering — all in a pure Go TUI.

## Install

```bash
go install github.com/gadflysu/aps@latest
```

Or build from source:

```bash
git clone https://github.com/gadflysu/aps.git
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

## Dependencies

| Package | Purpose |
|---------|---------|
| [charmbracelet/bubbletea](https://github.com/charmbracelet/bubbletea) | TUI framework (Elm MVU event loop, alt-screen) |
| [charmbracelet/bubbles](https://github.com/charmbracelet/bubbles) | TUI components: text input (search bar), viewport (scrollable preview panes) |
| [charmbracelet/lipgloss](https://github.com/charmbracelet/lipgloss) | Terminal styling: colors, padding, bold/faint, horizontal joins |
| [charmbracelet/x/term](https://github.com/charmbracelet/x) | TTY detection and terminal width query (list mode) |
| [sahilm/fuzzy](https://github.com/sahilm/fuzzy) | Fuzzy matching for the interactive search filter |
| [modernc.org/sqlite](https://gitlab.com/cznic/sqlite) | Pure-Go SQLite driver (no cgo) — reads Opencode session database |

## Data Sources

| Client | Storage | Format |
|--------|---------|--------|
| Claude Code | `~/.claude/projects/*/*.jsonl` | JSONL |
| Opencode | `~/.local/share/opencode/opencode.db` | SQLite |

Default client is Claude Code. Use `-o` / `-a` to include Opencode.
