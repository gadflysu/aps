## Goal

Detect the known Ctrl-Z failure when launching through `--claude-cmd`, `--opencode-cmd`, `--codex-cmd`, or resolved `--cmd`, then report an actionable error instead of silently orphaning an unrecoverable stopped job.

This is an interim mitigation, not a full job-control fix.

## Current Status

- Issue: #3, open.
- Current `master` still launches custom commands through `$SHELL -i -c "<customCmd> ..."` via `syscall.Exec`.
- All in-process approaches exhausted (see Rejected Approaches). No code change has been merged.
- Branch `fix/3-job-control` currently contains this research/mitigation plan only.

## Confirmed Facts

- Plain `claude`, `opencode`, and `codex` paths are unaffected because `aps` directly replaces itself with the target binary through `syscall.Exec`.
- The bug is limited to custom command paths that invoke an intermediate shell with `-i -c`.
- `zsh -i -c "cmd"` exits with status `146` (`128 + SIGTSTP`) when Ctrl-Z stops its foreground job, so `fg` cannot recover the launched session.
- Replacing `syscall.Exec` with `exec.Command` alone is insufficient: `aps` stays alive, but the intermediate `zsh -i -c` process still exits.
- Prefixing the script with `exec` is insufficient because shell aliases and functions are not resolved as external binaries.
- Ignoring or trapping `SIGTSTP` inside an interactive zsh shell is not viable for this problem.
- Dropping `-i` and sourcing rc files is unreliable because common rc files guard on interactive mode and skip alias/function definitions in non-interactive shells.
- Shell aliases/functions live in the invoking shell process; a subprocess cannot inherit the parent shell's alias table.

## Rejected Approaches

| Approach | Failure reason |
|---|---|
| `exec.Command` replacing `syscall.Exec` | Necessary but not sufficient; intermediate shell still exits 146 |
| `exec` prefix in script | `exec` resolves external binaries only, not shell aliases/functions |
| `trap '' TSTP` | `zsh: can't trap SIGTSTP in interactive shells` |
| Drop `-i`, `source ~/.zshrc` explicitly | `.zshrc` guards on `[[ $- == *i* ]]`; aliases never load in non-interactive shells |
| `aps shell-init` + eval | Rejected: requires modifying user's rc file |

## Open Direction

No viable approach remains that works within the subprocess model. The fundamental constraint is that aliases live in the parent shell's memory and are inaccessible to any child process.

Possible avenues:
- Redefine custom command contract: accept only external binaries/scripts, not aliases. Document that users should create a wrapper script instead of relying on alias expansion.
- Investigate whether aps can resolve the alias by reading the parent shell's config files (fragile, shell-specific).
- Investigate terminal ioctl approaches (e.g. `TIOCSTI`) to inject keystrokes into the parent shell, though this is platform-specific and potentially blocked by security policies.

## Interim Mitigation: Detect And Report

Implement a narrow mitigation that replaces the custom-command `syscall.Exec(shell, argv, env)` path with a child process only for custom commands. Keep direct binary launches as `syscall.Exec`.

Expected behavior:

- Run `$SHELL -i -c "<customCmd> ..."` as a child process with stdin/stdout/stderr attached to the current terminal.
- Wait for the child shell to exit.
- If it exits with status `146` (`128 + SIGTSTP`), print a clear error explaining that Ctrl-Z stopped the foreground job but the intermediate shell exited, so `fg` cannot recover it.
- Suggest using an external wrapper script instead of a shell alias/function for `--*-cmd`.
- Propagate other exit statuses normally so command chaining semantics remain predictable.

This does not solve alias/function inheritance or recover the stopped job. Its purpose is to make the failure visible and steer users toward wrapper scripts.

## Files To Change

| File | Change |
|------|--------|
| `launcher/launch.go` | Add custom-command child runner, detect exit status 146, preserve direct `syscall.Exec` for plain agent binaries |
| `launcher/launch_test.go` | Cover custom runner exit-code propagation and Ctrl-Z diagnostic formatting |
| `README.md` | Document that `--*-cmd` should point to external binaries/scripts, not shell aliases/functions |
| `cmd/root.go` or usage text | Adjust custom command help if needed to avoid implying alias support |

## TDD Tests

- Add a launcher test that simulates a child exiting `146` and verifies the diagnostic text.
- Add tests that non-zero non-146 exits still propagate as ordinary child errors.
- Add tests that `NoLaunch` verbose output is unchanged.
- Keep direct binary launch tests or behavior unchanged.

## Acceptance Criteria

- `go test ./...` passes.
- `go vet ./...` passes.
- `go build .` passes, then `go install .` is run immediately.
- Manual zsh smoke test with a harmless custom command confirms Ctrl-Z produces the new diagnostic instead of silent failure.
- README/help no longer promise shell alias/function support for `--*-cmd`.

## Research Done

1. Reproduced the bug: `zsh -i -c "sleep 60"` → Ctrl-Z → exit 146, `fg` cannot recover.
2. Tested `exec.Command` replacement: aps stays alive but intermediate shell still exits 146.
3. Tested `exec` prefix: alias `ccaws` not resolved (`command not found`).
4. Tested `trap '' TSTP`: interactive zsh rejects it.
5. Tested `source ~/.zshrc` without `-i`: rc guard `[[ $- == *i* ]]` skips alias definitions.
6. Confirmed alias is not expanded in argument position (even unquoted): `aps ccaws` passes literal `ccaws` to binary.
7. Proposed `aps shell-init` + eval: works technically but rejected because it requires modifying user's rc file.

## Non-Goals

- Do not change plain `claude`, `opencode`, or `codex` direct-launch behavior.
- Do not claim Ctrl-Z / `fg` recovery is fixed; this task only detects and reports the failure.
- Do not claim alias/function support is fixed until an interactive shell reproduction confirms Ctrl-Z and `fg` behavior.
