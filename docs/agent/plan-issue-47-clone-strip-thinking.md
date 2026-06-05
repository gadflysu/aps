---
name: plan-issue-47-clone-strip-thinking
description: Implementation plan for aps clone --strip-thinking subcommand and picker keybinding
metadata:
  type: project
---

# Plan: Clone session with thinking blocks stripped (#47)

## Goal

`aps clone <session-id> [--strip-thinking]` produces a new Claude session JSONL with a
fresh UUID, rebuilding the `parentUuid` chain and optionally removing all `thinking`
content blocks from assistant messages. Interactive picker adds a `T` keybinding that
clones the selected session with `--strip-thinking` and shows the result in the status bar.

## Non-goals

- In-place modification of the original session
- Auto-launching into the cloned session
- Support for Opencode / Codex sessions (Claude JSONL only)

## Clone logic (mirrors CC `/branch`)

Source: `~/workspace/cc/extracted/src/commands/branch/branch.ts`

1. Read source JSONL; keep only entries where `isTranscriptMessage(entry) && !entry.isSidechain`
2. Generate new UUID as `newSessionID`
3. For each kept entry, emit a new record:
   - `sessionId` → `newSessionID`
   - `parentUuid` → UUID of the previous emitted entry (null for first)
   - `isSidechain: false`
   - `forkedFrom: {sessionId: srcID, messageUuid: entry.uuid}`
   - all other fields preserved verbatim
   - if `--strip-thinking`: remove any `content` array element whose `"type"` is `"thinking"` from assistant messages
4. Copy `content-replacement` entries (rewrite their `sessionId` to `newSessionID`)
5. Write to `<projectDir>/<newSessionID>.jsonl` (mode 0600)
6. Print new session ID and resume command:
   ```
   <newSessionID>
   cd "<cwd>" && claude --resume <newSessionID>
   ```
   Format matches `launcher.verboseCmd` exactly.

## Session ID resolution (prefix matching)

`aps clone <prefix>` must resolve the prefix to a full UUID before cloning.

- Load all Claude sessions via `source.LoadClaude("", false, false)`
- Filter sessions whose `ID` has the given string as a prefix (case-insensitive)
- Exact match takes priority; if multiple prefix matches → error with list of candidates
- Full UUID always matches exactly

Needs: `source.ResolveClaudeSession(prefix string) (*Session, error)`
Also needs: `Session.JSONLPath() string` accessor (exposes the unexported `jsonlPath` field)

## Target files

| File | Change |
|------|--------|
| `source/session.go` | Add `JSONLPath() string` accessor |
| `source/clone.go` | New: `ResolveClaudeSession`, `CloneSession(src Session, stripThinking bool) (newID, cwd string, err error)` |
| `source/clone_test.go` | New: unit tests for clone logic and prefix resolution |
| `main.go` | Add `clone` subcommand dispatch (pre-`cmd.Parse`, same pattern as `shell-init`) |
| `cmd/root.go` | No change needed (clone parsed independently) |

## CLI interface

```
aps clone <session-id-or-prefix> [--strip-thinking]
```

- `session-id-or-prefix`: required positional arg; full UUID or unambiguous prefix
- `--strip-thinking`: remove `thinking` blocks from assistant content arrays
- No other flags needed; always targets Claude sessions only

Exit codes: 0 on success, 1 on error (ambiguous prefix, session not found, I/O error).

## Interactive picker

File: `picker/model.go` (or wherever key handling lives)

- Key `T` (shift+t) on a selected session:
  1. Only active for Claude sessions (skip / show error for others)
  2. Show confirm prompt in status bar: `Clone + strip thinking? [y/N]`
  3. On `y`: call `source.CloneSession(session, true)`; show `Cloned: <newID>` in status bar
  4. On `n` / any other key: cancel, restore normal status bar
  5. Stay in picker — do not launch

## Tests

- `source/clone_test.go`:
  - Clone preserves all non-thinking fields verbatim
  - Clone rebuilds `parentUuid` chain correctly
  - `--strip-thinking` removes all thinking blocks; other content blocks survive
  - `--strip-thinking` on a session with no thinking blocks is a no-op
  - Prefix resolution: exact match, prefix match, ambiguous prefix (error), not found (error)
  - Source JSONL is never modified

## Verification

```bash
# Build + install
go build . && go install .

# CLI: clone a session (use a real session UUID prefix)
aps clone <prefix> --strip-thinking
# → prints new UUID and cd command

# Confirm source is unchanged
diff <(cat ~/.claude/projects/.../<srcID>.jsonl) <original>

# Confirm new file has no thinking blocks
grep '"type":"thinking"' ~/.claude/projects/.../<newID>.jsonl  # must be empty

# Interactive: run aps, press T on a Claude session, confirm with y
# → status bar shows new session ID; new .jsonl appears in project dir

go test ./source/... -run TestClone
```
