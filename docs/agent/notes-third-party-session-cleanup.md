---
name: notes-third-party-session-cleanup
description: Mechanism to detect and clean Claude session data left by CodexBar/ClaudeBar
metadata:
  type: project
---

Third-party macOS menu bar apps (CodexBar, ClaudeBar) periodically spawn `claude` CLI processes
to probe usage via `/usage` and `/status`. Each spawn creates a session transcript under
`~/.claude/projects/`, which is never cleaned up by the host app.

## Background

CodexBar (steipete/CodexBar) and ClaudeBar (tddworks/ClaudeBar) are macOS menu bar apps
that monitor AI coding assistant usage quotas. Both periodically spawn `claude` CLI processes
to scrape `/usage` output. Each spawn creates an orphaned session transcript in
`~/.claude/projects/` that the host app never cleans up, polluting `aps` listings.

## Problem

### CodexBar (steipete/CodexBar)

- Runs `claude --allowed-tools ""` inside a PTY every N minutes (configurable: 1/2/5/15/30 min)
- Sends `/usage` and `/status` commands to parse the TUI output
- Probe working directory: `~/Library/Application Support/CodexBar/ClaudeProbe/`
- Session transcripts land in: `~/.claude/projects/-Users-dsu-Library-Application-Support-CodexBar-ClaudeProbe/*.jsonl`
- Watchdog wrapper: `CodexBarClaudeWatchdog` (optional, kills orphaned child processes)
- Cleanup: kills the process (`/exit` + SIGTERM + SIGKILL) but **never deletes transcript files**
- Old version used the name "ClaudeBar" → leftover dir: `~/.claude/projects/-Users-dsu-Library-Application-Support-ClaudeBar-Probe/`

### ClaudeBar (tddworks/ClaudeBar)

- Runs `claude /usage --allowed-tools ""` as a subprocess (uses SwiftTerm for TUI rendering)
- Probe working directory: likely similar `~/Library/Application Support/ClaudeBar/Probe/` or temp dir
- Session transcripts land in: `~/.claude/projects/-Users-dsu-Library-Application-Support-ClaudeBar-Probe/*.jsonl`
- Cleanup: same issue — process killed, transcripts persist

### Impact

- Each probe creates a new `.jsonl` transcript file (15KB–500KB each)
- At 5-minute intervals over days, this accumulates to hundreds of orphaned files
- Observed: 68 files / 3.3MB in the CodexBar probe directory alone
- These pollute `aps` session listings (they appear as valid sessions)

## Detection Strategy

### Identify third-party probe sessions by path pattern

Third-party probe directories encode the app's working directory path into the `~/.claude/projects/` key:

```
~/.claude/projects/-Users-<user>-Library-Application-Support-<AppName>-<ProbeDir>/
```

Known patterns:

| Pattern | Source |
|---------|--------|
| `*-CodexBar-ClaudeProbe*` | CodexBar (current) |
| `*-ClaudeBar-Probe*` | CodexBar (old name) or ClaudeBar |
| `*-ClaudeBar-*Probe*` | ClaudeBar variants |

### Identify by transcript content

Probe transcripts contain characteristic markers:
- Very short (single /usage + /status exchange)
- No real user messages — only tool results or system prompts
- `session_name` in metadata matches probe directory name
- `--allowed-tools ""` in the launch arguments

### Identify by running process

```bash
ps aux | grep -E 'claude.*--allowed-tools.*""'
```

This catches active probe processes from both apps.

## Cleanup Proposal

### Option A: Directory-based cleanup (simple, recommended)

Delete entire project directories matching known third-party probe patterns:

```bash
# Known probe directory patterns
PATTERNS=(
  "*-CodexBar-ClaudeProbe*"
  "*-ClaudeBar-Probe*"
  "*-ClaudeBar-*Probe*"
  "*-CodexBar-*Probe*"
)

for pattern in "${PATTERNS[@]}"; do
  find ~/.claude/projects -maxdepth 1 -type d -name "$pattern" -exec rm -rf {} +
done
```

### Option B: Content-based cleanup (thorough)

For each `~/.claude/projects/*/` directory, check if it's a probe session:
1. Path matches known probe pattern → delete
2. All `.jsonl` files are < 100KB and contain no `role: user` string content → probe session → delete

### Option C: aps integration

Add a `aps --clean` subcommand that:
1. Scans `~/.claude/projects/` for third-party probe directories
2. Lists candidates with file counts and sizes
3. Asks for confirmation (or `--force` flag)
4. Removes them

This keeps the cleanup discoverable from within the tool that's affected by the pollution.

## Recommended Implementation

1. Add a `source.IsThirdPartyProbe(path string) bool` function in `aps/source/`
2. Match directory name against known patterns (glob or regex)
3. In list mode, optionally filter out probe sessions (flag: `--hide-probes`)
4. Add `aps --clean-probes` to delete them with confirmation

## Related

- CodexBar repo: `github.com/steipete/CodexBar`
- ClaudeBar repo: `github.com/tddworks/ClaudeBar`
- SweetCookieKit (shared dependency): `github.com/steipete/SweetCookieKit`
