## Goal

Fix Ctrl-Z / `fg` recovery when launching Claude or Opencode through `--claude-cmd`, `--opencode-cmd`, or resolved `--cmd`, without regressing normal direct `syscall.Exec` launches.

## Current Status

- Issue: #3, open.
- Current `master` still launches custom commands through `$SHELL -i -c "<customCmd> ..."` via `syscall.Exec`.
- Local branch `fix/3-job-control` contains experiments in `launcher/launch.go` and `launcher/launch_test.go`, but it is stale relative to `master` and should not be merged directly.
- The branch tried `exec.Command` plus non-interactive rc sourcing. Treat that as an experiment, not the accepted solution.

## Confirmed Facts

- Plain `claude` and `opencode` paths are unaffected because `aps` directly replaces itself with the target binary through `syscall.Exec`.
- The bug is limited to custom command paths that invoke an intermediate shell with `-i -c`.
- `zsh -i -c "cmd"` exits with status `146` (`128 + SIGTSTP`) when Ctrl-Z stops its foreground job, so `fg` cannot recover the launched session.
- Replacing `syscall.Exec` with `exec.Command` alone is insufficient: `aps` stays alive, but the intermediate `zsh -i -c` process still exits.
- Prefixing the script with `exec` is insufficient because shell aliases and functions are not resolved as external binaries.
- Ignoring or trapping `SIGTSTP` inside an interactive zsh shell is not viable for this problem.
- Dropping `-i` and sourcing rc files is unreliable because common rc files guard on interactive mode and skip alias/function definitions in non-interactive shells.
- Shell aliases/functions live in the invoking shell process; a subprocess cannot inherit the parent shell's alias table.

## Research Direction

Investigate a parent-shell evaluation flow such as `aps shell-init`, where the user's rc file installs a wrapper that evaluates a no-launch verbose command in the current shell. This direction may allow aliases/functions to resolve in the parent shell and make Ctrl-Z behave as a native child job.

Do not treat this as the final design until the TUI/stdout implications are proven. In particular, command substitution may require rendering the picker through `/dev/tty` while stdout carries only the launch command.

## Research Method

1. Reproduce the bug outside `aps` first. Use a harmless long-running command behind a zsh alias/function, launch it through `zsh -i -c`, press Ctrl-Z, and confirm whether the shell exits with `146` and whether `fg` can recover it.
2. Reproduce the current `aps` failure with a harmless custom command, not Claude or Opencode. Prefer a command that prints its PID, sleeps, and handles resume visibly, so job-control behavior is observable without agent side effects.
3. Build a minimal parent-shell wrapper prototype before editing production code. The wrapper should call a temporary no-launch command generator, `eval` the generated `cd ... && <customCmd> ...` in the current shell, then verify that aliases/functions resolve and Ctrl-Z / `fg` works.
4. Test stdout capture separately from job control. Run the picker or a reduced Bubble Tea program inside command substitution and verify whether UI rendering can move to `/dev/tty` while stdout contains only the generated launch command.
5. Define the smallest acceptable user contract for shell integration. Decide whether `aps shell-init` prints shell functions, whether it must support zsh only first, and what command users must add to rc files.
6. Convert the proven prototype into tests and implementation. Add parser/generator tests for deterministic output, then add the minimal runtime changes needed for `/dev/tty` rendering and parent-shell evaluation.
7. Perform the final proof in a real interactive zsh session with an alias-backed custom command before claiming the issue fixed.

## Target Files To Investigate

- `cmd/root.go`: parse any shell-init or no-launch integration changes.
- `launcher/launch.go`: preserve direct `syscall.Exec` behavior while changing custom command flow only if research proves it.
- `picker` / Bubble Tea startup path: verify whether UI output can be redirected to `/dev/tty` when stdout is captured.
- `launcher/launch_test.go` and `cmd/root_test.go`: cover any command-generation and parser behavior introduced by the researched fix.

## Non-Goals

- Do not change plain `claude` or `opencode` direct-launch behavior.
- Do not merge or cherry-pick `fix/3-job-control` blindly; re-evaluate each idea against current `master`.
- Do not claim alias/function support is fixed until an interactive shell reproduction confirms Ctrl-Z and `fg` behavior.

## Verification Plan

- Add focused tests before implementation for any new command-generation or parser behavior.
- Run `go test ./...`.
- Run `go build .`, then immediately run `go install .`.
- Manually verify in zsh with an alias-backed custom command:
  - launch through the custom command path,
  - press Ctrl-Z,
  - run `fg`,
  - confirm the agent session resumes correctly.
