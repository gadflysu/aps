# Changelog

All notable changes to this project will be documented in this file.

## [v0.2.0] — 2026-04-15

Complete rewrite in Go. Replaces the bash+fzf+Python implementation.

### Added
- Pure Go TUI picker using bubbletea with fuzzy search (sahilm/fuzzy)
- Three-pane preview: SESSION INFO, RECENT MESSAGES, DIRECTORY
- `Space` toggles preview; `Tab` cycles panes; `j`/`k` scrolls focused pane
- Selected row highlighted with reverse video
- Fuzzy filter matches title, directory, session ID, and time
- List mode (`-l`) with lipgloss-formatted table output and adaptive column widths
- CJK-safe truncation (`display.TruncateWidth`) — works around lipgloss `Width+MaxWidth` boundary bug
- Combined mode (`-a`) shows both Claude Code and Opencode sessions with SRC column
- `-nv` verbose no-launch: prints full launch command

### Fixed
- CJK title overflow in TUI list and list mode
- Opencode session ID wrapping instead of truncating in TUI list

### Changed
- Module path: `local/aps` → `github.com/gadflysu/aps`
- `syscall.Exec` replaces subprocess launch (process replacement, not child)

## [v0.1.0-bash] — 2026-04-14

Initial release. bash + Python + fzf implementation.

### Features
- Interactive fzf picker for Claude Code and Opencode sessions
- Path filtering (exact → symlink → substring)
- Session preview via fzf `--preview`
- `-l` list mode, `-c`/`-o`/`-a` client selection, `-n` no-launch, `-d` danger mode
