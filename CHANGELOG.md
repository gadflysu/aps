# Changelog

All notable changes to this project will be documented in this file.

## [v0.3.2] — 2026-05-13

### Docs
- Add package-level doc comments to all packages so pkg.go.dev renders documentation

## [v0.3.1] — 2026-05-13

### Added
- `PIDCache`: persists `pid|lstart→sessionID` to `~/.cache/aps/pid-session.txt` for stable active-session detection across restarts
- `--debug-log FILE` flag; all log calls are no-ops when disabled
- Dual-speed spinner: confirmed sessions at 120 ms/frame, guessed sessions at 600 ms/frame (dim)
- watcher: idle-triggered stat-only poll after 5 s of no fsnotify events

### Refactored
- `DetectActive` returns `ActiveResult{Confirmed, Guessed}` sets

### Fixed
- Guessed sibling evicted on proc confirmation
- Guessed session count capped to unmapped proc count per CWD
- Spinner frame rate corrected to 120 ms (matching Claude Code)

## [v0.3.0] — 2026-05-12

### Added
- Live refresh: session list updates in-place while Claude Code is running
- `watcher` package: FSNotify (root + project subdirs) with 1s rate-limit and 5s stat-only fallback poll
- `source.ReloadSession`: incremental single-file re-parse without full reload
- Cursor anchored by session ID across refreshes; preview mode buffers changes until pane is closed

## [v0.2.9] — 2026-05-07

### Added
- Table-based preview pane with automatic ├ junctions (lipgloss/v2 table)
- Per-section focus color in preview (active section highlighted)
- Column header row in interactive mode
- Adaptive ID/MSG column widths in interactive TUI
- `--color` flag for list mode (`auto`/`always`/`never`)
- `--version` / `-V` flag; dev builds include git hash
- `Messages` label renamed to `Turns` in SESSION INFO preview

### Fixed
- List rows no longer word-wrap in preview mode
- `Esc` in preview collapses pane instead of quitting
- Separator flushes correctly against `│` border
- Directory label color aligned with list mode cyan
- `--color` accepts bare flag without explicit value
- SRC column accounted for in preview-mode title width calculation

### Changed
- Migrated to `charm.land/lipgloss/v2`; replaced `SetColorProfile` with env-var detection
- Fullwidth separator replaced with per-field padding (cleaner alignment)

## [v0.2.5] — 2026-04-23

### Added
- CI: GitHub Actions workflow with `go vet` + test + Codecov coverage upload
- GoReleaser config and release workflow
- Homebrew cask and `--HEAD` install option
- `--version` / `-V` flag groundwork

### Changed
- README overhauled: new structure, screenshots section, contributing and license

## [v0.2.2] — 2026-04-20

### Added
- `--claude-cmd`, `--opencode-cmd`, `--cmd` flags to override the launched binary (supports shell aliases and functions)

### Fixed
- `buildShellCmd` no longer prepends `exec`, enabling shell aliases/functions to work correctly

### Changed
- Color palette centralized in `display/colors.go` (no behavior change)

## [v0.2.1] — 2026-04-15

### Added
- Adaptive column widths in list mode (`AdaptiveTitleWidth`)
- Bold basename in directory column; underline padding in header cells
- Dim repeated directory entries
- `MSG` column renamed to `TURNS`

### Fixed
- Title width bonus capped at natural max width
- CJK separator counted as 2 columns

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
