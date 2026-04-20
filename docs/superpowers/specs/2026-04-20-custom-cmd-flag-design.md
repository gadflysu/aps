# Custom Command Flag Design

Date: 2026-04-20

## Summary

Allow users to override the binary/command used to launch Claude Code or Opencode sessions.
Supports shell aliases, functions, and versioned commands like `npx claude@2.1`.

## New Flags

| Flag | Description |
|------|-------------|
| `--claude-cmd <str>` | Override the command used to launch Claude Code |
| `--opencode-cmd <str>` | Override the command used to launch Opencode |
| `--cmd <str>` | Shorthand: applies to the single selected client; error if both or neither client is active |

### Validation Rules

- `--claude-cmd` and `--opencode-cmd` are always syntactically valid; they are silently ignored if the corresponding client is not selected.
- `--cmd` works when exactly one client is active, including the default Claude fallback. It errors only when both clients are active (via `-a` or `-c -o`): `"--cmd is ambiguous when multiple clients are selected; use --claude-cmd or --opencode-cmd"`.
- `--cmd` and `--claude-cmd` (or `--opencode-cmd`) cannot be used together; exit with an error: `"--cmd conflicts with --claude-cmd / --opencode-cmd"`.

## Data Model

`cmd.Config` gains three new fields:

```go
ClaudeCmd   string // raw string from --claude-cmd or --cmd (when Claude is the single client)
OpencodeCmd string // raw string from --opencode-cmd or --cmd (when Opencode is the single client)
```

`--cmd` resolution happens inside `cmd.Parse` after client flags are resolved:
- if exactly one client is active: copy the value into the appropriate field
- otherwise: print error and exit

`launcher.Options` gains two new fields:

```go
ClaudeCmd   string // empty = use default "claude" binary via LookPath
OpencodeCmd string // empty = use default "opencode" binary via LookPath
```

## Launch Behavior

### No custom cmd (existing path, unchanged)

```
LookPath("claude") → syscall.Exec(claudePath, ["claude", "--resume", id], env)
```

### With custom cmd

The custom cmd string is passed verbatim to an interactive shell. Shell-specific argv:

- Claude: `[$SHELL, "-i", "-c", "exec <custom-cmd> --resume <sessionID>"]`
- Opencode: `[$SHELL, "-i", "-c", "exec <custom-cmd> -s <sessionID>"]`

Using `exec` inside the shell string ensures the shell process is replaced by the target command (no lingering shell wrapper).

Shell is resolved via `os.Getenv("SHELL")`, falling back to `/bin/sh`.

The `-i` flag loads the user's interactive init files (`.bashrc`, `.zshrc`), enabling aliases and shell functions. **Note:** heavy init scripts (e.g. `nvm`, `pyenv`) or interactive prompts in those files may cause a noticeable delay or hang on launch.

DangerMode with custom cmd (Claude only): append `--dangerously-skip-permissions` before `--resume` in the shell command string.

### `-nv` verbose output

When `--no-launch --verbose` is set:

- Default: `cd "/path" && claude "--resume" "abc123"`
- Custom cmd: `cd "/path" && cc --resume abc123` (raw user string, no quoting)

## Files to Change

| File | Change |
|------|--------|
| `cmd/root.go` | Add `ClaudeCmd`, `OpencodeCmd` to `Config`; parse `--claude-cmd`, `--opencode-cmd`, `--cmd`; validate conflicts |
| `cmd/root.go` | Update `usage()` to document new flags |
| `launcher/launch.go` | Add `ClaudeCmd`, `OpencodeCmd` to `Options`; branch on non-empty custom cmd |
| `main.go` | Pass new fields from `cfg` to `launchOpts` |
| `cmd/root_test.go` | Tests for flag parsing and conflict validation |
| `launcher/launch_test.go` | Tests for custom cmd branch |

## Error Messages

```
error: --cmd is ambiguous when multiple clients are selected; use --claude-cmd or --opencode-cmd
error: --cmd conflicts with --claude-cmd
error: --cmd conflicts with --opencode-cmd
```
