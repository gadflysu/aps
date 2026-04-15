# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Test

```bash
go build .          # compile binary to ./aps
go install .        # install to ~/go/bin/aps (already in PATH)
go test ./...       # run all tests
go test ./picker/... -run TestVisibleRange  # run a single test
```

## Architecture

`aps` is an interactive session picker for Claude Code and Opencode. It replaces the original bash/fzf implementation with a pure Go binary.

**Data flow:**
1. `main.go` calls `source.LoadClaude` / `source.LoadOpencode` → `[]source.Session`
2. In list mode (`-l`): `display.FormatListRow` prints each session
3. In interactive mode: `picker.Run` starts a bubbletea TUI, returns the chosen `*source.Session`
4. `launcher.Claude` / `launcher.Opencode` `syscall.Exec`s into the client

**Package responsibilities:**

| Package | Responsibility |
|---------|---------------|
| `source` | Parse Claude JSONL and Opencode SQLite into `[]Session`; title extraction via `applyTitleRules` |
| `filter` | Three-tier path matching: exact → symlink → substring |
| `display` | List-mode table formatting with lipgloss; `AdaptiveTitleWidth` + CJK-safe `truncateWidth` |
| `picker` | bubbletea TUI: fuzzy filter, three-pane preview (SESSION INFO / RECENT MESSAGES / DIRECTORY), `j/k` scroll, `Tab` cycles panes, `Space` toggles preview |
| `preview` | Section render functions (`ClaudeInfo`, `ClaudeMsgs`, `OpencodeInfo`, `DirListing`) writing styled strings; `RenderClaude`/`RenderOpencode` for backwards-compatible single-writer API |
| `launcher` | `syscall.Exec` into `claude --resume` or `opencode -s`; falls back to shell if binary not found |
| `cmd` | Flag parsing; combined short flags (`-nv` → `-n -v`) |

**Key design constraints:**
- `picker/styles.go` and `preview/styles.go` both use ANSI 16-color palette (`lipgloss.Color("N")`) — do not introduce hex/RGB colors
- `preview.listDir()` calls `eza`/`ls --color=always` and forwards raw output; do not pass it through lipgloss
- `launcher` uses `syscall.Exec` (replaces the process), not `exec.Command` (subprocess)
- Title extraction: `applyTitleRules` strips skip-prefixes, takes the first line, handles the `"Implement the following plan:"` special case; `customTitle` records must also pass through `applyTitleRules` (not just `TrimSpace`) to strip embedded newlines

**Preview pane height allocation** (`picker/model.go`):
- SESSION INFO: fixed `infoContentLines` (4) rows, `sectionHeaderLines` (2) overhead = 6 total
- RECENT MESSAGES: `(available / 3)` rows when `hasMsgs=true`, else height=0
- DIRECTORY: remaining rows

## Git Commits

Format: `<type>(<scope>): <short imperative phrase>` — no trailing period, details in body if needed.

| Type | Use for |
|------|---------|
| `feat` | new feature code |
| `fix` | bug fix |
| `refactor` | code change that neither fixes a bug nor adds a feature |
| `test` | adding or modifying tests |
| `docs` | documentation only (plans, specs, CLAUDE.md) |
| `build` | build system files (go.mod, go.sum, Makefile) |
| `chore` | housekeeping files that don't affect build or code (.gitignore) |

Rules:
- One logical change per commit; never bundle different types
- Stage files by name explicitly — never `git add -A` or `git add .`
- Title must match the actual diff — check `git show --stat` before wording
- `build` ≠ `chore`: Makefile → `build`, .gitignore → `chore`
