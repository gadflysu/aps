# Agent Instructions

This file provides guidance to coding agents working with code in this repository.

## Meta Rules

- After every task, propose a minimal update to this file to prevent recurring mistakes: add missing rules, improve partial coverage, or do nothing if fully covered.
- Keep this file under 120 lines. Merge redundancies and cut filler to stay within the limit before adding new rules.

## Communication

- Respond in Chinese. Never use Korean or Japanese — always substitute Chinese.

## Build & Test

```bash
go build .                                      # compile binary to ./aps
go install .                                    # install to ~/go/bin/aps (already in PATH)
go test ./...                                   # run all tests
go test ./picker/... -run TestVisibleRange      # run a single test
```

After every successful `go build .`, immediately run `go install .`.

CI runs `go vet ./...` then `go test -coverprofile=coverage.txt ./...` on every push/PR to `master`, and uploads coverage to Codecov.

## Architecture

`aps` is an interactive session picker for Claude Code and Opencode. It replaces the original bash/fzf implementation with a pure Go binary.

**Data flow:**
1. `main.go` calls `source.LoadClaude` / `source.LoadOpencode` → `[]source.Session`
2. In list mode (`-l`): `display.FormatListRow` prints each session
3. In interactive mode: `picker.Run` starts a bubbletea TUI, returns the chosen `*source.Session`
4. `launcher.Claude` / `launcher.Opencode` `syscall.Exec`s into the client

**Architectural boundaries:**

| Package | Responsibility |
|---------|---------------|
| `source` | Own session discovery, metadata extraction, active-session detection, and source-specific persistence details |
| `filter` | Three-tier path matching: exact → symlink → substring |
| `display` | Own list-mode width calculation, table formatting, color handling, and terminal-width adaptation |
| `picker` | Own interactive state, input handling, filtering, preview orchestration, and active-session refresh |
| `preview` | Own preview section rendering only; keep data loading and TUI state outside this package |
| `launcher` | Own final process replacement into the selected agent client; do not launch agents from UI or source packages |
| `watcher` | Own filesystem refresh signals; keep rate limiting and fallback polling local to this package |
| `dbg` | Own optional diagnostic logging; all callers must remain safe when logging is disabled |
| `cmd` | Own CLI parsing, defaults, conflicts, and help/version output; keep execution behavior outside this package |

**Key design constraints:**
- `picker/styles.go` and `preview/styles.go` both use ANSI 16-color palette (`lipgloss.Color("N")`) — do not introduce hex/RGB colors
- `preview.listDir()` calls `eza`/`ls --color=always` and forwards raw output; do not pass it through lipgloss
- `launcher` uses `syscall.Exec` (replaces the process), not `exec.Command` (subprocess)
- Title extraction: `applyTitleRules` strips skip-prefixes, takes the first line, handles the `"Implement the following plan:"` special case; `customTitle` records must also pass through `applyTitleRules` to strip embedded newlines
- CJK truncation: always use `display.TruncateWidth(s, maxCols, tail)` before passing to lipgloss — `Width(N)+MaxWidth(N)` has a known upstream bug where CJK characters at the truncation boundary produce N−1 columns

## Versioning & Releases

Version scheme: `vMAJOR.MINOR.PATCH`.

- **MAJOR / MINOR** bumps are decided by the user — never propose them
- **PATCH** bumps (`v0.2.0 → v0.2.1`) are your responsibility to propose: after merging a feature branch or a meaningful fix, evaluate whether the result warrants a new patch tag and actively suggest it to the user

A patch bump is warranted when: a user-visible feature ships, a significant bug is fixed, or the binary behaves noticeably better. Docs, tests, and refactors alone do not warrant a tag.

Before tagging, always update `CHANGELOG.md` with a new section for the version, then commit it, then tag. The CHANGELOG entry must cover **all** user-visible changes since the previous tag — not just the most recent task. Review `git log vPREV..HEAD` to find every `feat`, `fix`, and other user-facing commit.

```bash
# 1. Add CHANGELOG entry and commit
git add CHANGELOG.md && git commit -m "docs(CHANGELOG): add vX.Y.Z entry"
# 2. Tag on the resulting HEAD
git tag -a vX.Y.Z -m "vX.Y.Z"
git push origin vX.Y.Z
```

Tag on `master` HEAD only, never mid-branch.

## Git Commits

Format: `<type>(<scope>): <short imperative phrase>` — no trailing period, details in body if needed.

| Type | Use for |
|------|---------|
| `feat` | new feature code |
| `fix` | bug fix |
| `refactor` | code change that neither fixes a bug nor adds a feature |
| `test` | adding or modifying tests |
| `docs` | documentation only (plans, specs, AGENTS.md, CLAUDE.md) |
| `build` | build system files (go.mod, go.sum, Makefile) |
| `chore` | housekeeping files that don't affect build or code (.gitignore) |

Rules:
- One logical change per commit; never bundle different types
- Stage files by name explicitly — never `git add -A` or `git add .`
- Title must match the actual diff — check `git show --stat` before wording
- `build` ≠ `chore`: Makefile → `build`, .gitignore → `chore`

## GitHub Workflow

- Issue titles use descriptive noun phrases, not commit-style `type(scope): ...` format
- Every issue must have a label (`--label`); use `enhancement` for features, `bug` for defects, `documentation` for docs-only work
- When revising an issue after review, edit the issue body (`gh issue edit N --body "..."`) — never append a comment.

**Plan:**
- Read related `docs/agent/plan-*.md` and `docs/agent/notes-*.md` before creating or resuming an issue; update existing context instead of duplicating scope
- Clarify the problem or requirement, then create the issue and note `#N`
- If the solution is clear, create `docs/agent/plan-issue-N-*.md` with goal, target files, non-goals, tests, and verification; edit the issue body to link it, then commit the plan with `docs` type before closing the planning step
- If the solution is unclear or complex, leave the issue body focused on requirements, constraints, and research questions; let the executor create and commit the plan after investigation

**Execute:**
- Read the issue plus related plan/notes; create branch `<type>/N-short-desc` from master, using `feat` for features and `fix` for bugs
- Before implementation, verify the linked plan exists and matches the intended scope; if missing or stale, create/update it, link it from the issue body, and commit it with `docs` type
- Implement with TDD; each implementation commit body includes `Closes #N`

**Review and merge:** Open a PR titled without issue numbers and put `Closes #N` in the body; wait for checks to pass; before squash merge, rewrite the squash title to commit format without appended issue/PR numbers; `cd` to the main worktree, remove the issue worktree, run `gh pr merge N --squash -d`, pull master, confirm local/remote issue branches are gone, then evaluate CHANGELOG + patch tag if user-visible code shipped.
