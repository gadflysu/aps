## Goal

Implement an opt-in `aps shell-init` integration for users who need shell alias/function custom commands, and detect the known Ctrl-Z failure in the fallback subprocess path so aps can report an actionable error.

`shell-init` is the real recovery path because it evaluates the launch command in the parent shell. The subprocess detection path is an interim mitigation for users who have not installed the shell integration.

## Current Status

- Issue: #3, open.
- Current `master` still launches custom commands through `$SHELL -i -c "<customCmd> ..."` via `syscall.Exec`.
- All self-contained subprocess approaches are exhausted (see Failed / Deprecated Tries). No code change has been merged to `master`.
- Branch `fix/3-job-control` contains partial implementation commits: Ctrl-Z fallback diagnostics, an initial `aps shell-init` wrapper, picker `/dev/tty` routing when stdout is reserved, and follow-up planning. It is not ready to merge.
- Treat the current branch as an implementation draft. Before continuing, reconcile this plan with the actual code and remove or revise any behavior that conflicts with the plan, especially any skip-picker shortcut introduced for shell-init compatibility.

## Confirmed Facts

- Plain `claude`, `opencode`, and `codex` paths are unaffected because `aps` directly replaces itself with the target binary through `syscall.Exec`.
- The bug is limited to custom command paths that invoke an intermediate shell with `-i -c`.
- `zsh -i -c "cmd"` exits with status `146` (`128 + SIGTSTP`) when Ctrl-Z stops its foreground job, so `fg` cannot recover the launched session.
- Shell aliases/functions live in the invoking shell process; a subprocess cannot inherit the parent shell's alias table.
- In zsh job output, `+` marks the current job and `-` marks the previous job. A line printed immediately after Ctrl-Z and a later `jobs` line with the same job number refer to the same stopped job, not two jobs.
- `suspended (tty output)` is a `SIGTTOU` stop reason: a process group that is not the foreground owner of the controlling terminal attempted terminal output or terminal-parameter control.
- Command substitution alone is not a proven cause of `suspended (tty output)`. A minimal zsh test showed a command-substitution helper can run in the foreground process group (`pgrp == tpgid`). Attribute a tty-output stop only after inspecting `jobs -l` and `ps`.

## Failed / Deprecated Tries

- Do not revisit self-contained subprocess fixes: `exec.Command` keeps `aps` alive but the intermediate shell still exits 146; `exec` prefix loses aliases/functions; `trap '' TSTP` is rejected by interactive zsh; non-interactive rc sourcing skips common alias/function setup.
- Do not skip picker selection in command-print modes. The draft shortcut that selected `sessions[0]` is deprecated because users must still choose the target session interactively.
- Do not explain `suspended (tty output)` by command substitution alone. Treat `$()` as insufficient evidence unless `jobs -l` and `ps` show the stopped process group was not the terminal foreground owner.

## Open Direction

No viable approach remains that works within the subprocess model. The fundamental constraint is that aliases live in the parent shell's memory and are inaccessible to any child process.

Accepted direction:
- Implement `aps shell-init` as an explicit opt-in parent-shell wrapper. It must print shell code only; aps must not edit rc files.
- Keep subprocess execution as fallback for users who do not install shell integration, but detect exit 146 and recommend `aps shell-init`.
- Document that external wrapper scripts remain the simplest self-contained alternative when users do not want shell integration.

Future CLI cleanup is tracked separately: shell-init should eventually use an explicit launch-command print mode instead of overloading `-n -v`. Do not block this plan update on that future work.

## Shell Init Integration

Add `aps shell-init <shell>` to print shell-specific code that users can install with:

```sh
eval "$(aps shell-init zsh)"
```

The generated shell wrapper should:

- call the real binary with `command aps` to avoid recursive wrapper calls;
- use `command aps --print-launch-command ...` to produce the final `cd ... && <customCmd> ...` launch command;
- evaluate that launch command in the current parent shell so aliases/functions resolve from the parent shell's live state;
- only use this path when a custom command is present; direct agent binary launches can still call `command aps` normally;
- preserve user arguments and quoting as strictly as shell code permits;
- avoid modifying any rc file automatically.

This mode should make Ctrl-Z / `fg` behave like a normal shell-launched job because the agent process becomes a child job of the user's current shell, not a grandchild behind `$SHELL -i -c`.

Shell-init launch-command capture requires strict UI/data channel separation:

- picker input/output must use `/dev/tty`;
- stdout must contain only the final launch command for the selected session;
- stderr may contain errors after Bubble Tea exits alt screen;
- cancel must produce no stdout command and must not run `eval`.

Do not change existing `aps -n -v ...` semantics by skipping the picker and selecting `sessions[0]`. Users currently rely on `-n -v` opening the picker, then printing the launch command for the selected session. Shell-init must preserve that behavior through `/dev/tty` routing instead of bypassing selection.

Keep human-facing no-launch behavior separate from the machine-readable shell protocol:

- `-n`: picker selection, then print the selected session directory.
- `-n -v`: picker selection, then print the selected session launch command for human inspection/debugging.
- `--print-launch-command`: picker selection, then print only the selected session launch command as a machine-readable stdout protocol for shell-init.

Follow the common shell-integration pattern used by tools such as Starship, direnv, Atuin, mise, and fzf: generate shell-specific init code and show shell-specific rc commands.

Supported shells for the first implementation:

- zsh: `aps shell-init zsh`; install in `~/.zshrc`
- bash: `aps shell-init bash`; install in `~/.bashrc`

If the user runs `aps shell-init` without an explicit shell, infer from `$SHELL` only when it clearly ends in `zsh` or `bash`; otherwise print a concise error asking for `aps shell-init zsh` or `aps shell-init bash`.

## Fallback Mitigation: Detect And Report

Replace the custom-command `syscall.Exec(shell, argv, env)` fallback path with a child process only for custom commands. Keep direct binary launches as `syscall.Exec`.

Expected fallback behavior:

- Run `$SHELL -i -c "<customCmd> ..."` as a child process with stdin/stdout/stderr attached to the current terminal.
- Wait for the child shell to exit.
- If it exits with status `146` (`128 + SIGTSTP`), print a clear error explaining that Ctrl-Z stopped the foreground job but the intermediate shell exited, so `fg` cannot recover it.
- Suggest enabling the shell integration with `eval "$(aps shell-init)"`, or using an external wrapper script instead of a shell alias/function for `--*-cmd`.
- Propagate other exit statuses normally so command chaining semantics remain predictable.

This fallback does not solve alias/function inheritance or recover the stopped job. Its purpose is to make the failure visible and steer users toward `shell-init` or wrapper scripts.

## User-Facing Copy

Use direct wording that separates the installed shell integration from the fallback path.

Help / README wording:

- `shell-init SHELL`: `Print shell integration for alias/function custom commands`
- `--print-launch-command`: `Print the selected session launch command without launching`
- `--claude-cmd STR`: `Override command used to launch Claude Code`
- `--opencode-cmd STR`: `Override command used to launch Opencode`
- `--codex-cmd STR`: `Override command used to launch Codex`
- `--cmd STR`: `Override command for the single active agent`
- Example without shell-init: `aps -c --claude-cmd ./ccaws-wrapper`
- Example with shell-init: `eval "$(aps shell-init zsh)"`, then `aps -c --cmd ccaws`

Do not say that `--*-cmd` supports aliases/functions unconditionally. Say: alias/function custom commands require shell integration.

Suggested fallback Ctrl-Z diagnostic:

```text
aps: custom command shell stopped and exited; the launched job cannot be recovered with fg.
aps: enable for this shell: eval "$(aps shell-init zsh)"
aps: add permanently: echo 'eval "$(aps shell-init zsh)"' >> ~/.zshrc
aps: or use an external wrapper script for --*-cmd instead of a shell alias/function.
```

When `$SHELL` clearly indicates bash, use bash-specific commands instead:

```text
aps: enable for this shell: eval "$(aps shell-init bash)"
aps: add permanently: echo 'eval "$(aps shell-init bash)"' >> ~/.bashrc
```

Suggested shell-init install note:

```text
# zsh: run in the current shell
eval "$(aps shell-init zsh)"

# zsh: add permanently
echo 'eval "$(aps shell-init zsh)"' >> ~/.zshrc

# bash: run in the current shell
eval "$(aps shell-init bash)"

# bash: add permanently
echo 'eval "$(aps shell-init bash)"' >> ~/.bashrc
```

## Files To Change

| File | Change |
|------|--------|
| `cmd/root.go` | Add `shell-init [zsh|bash]` command handling before normal picker/list execution; keep regular flag parsing intact |
| `cmd/root_test.go` | Cover explicit shell parsing, `$SHELL` inference, unsupported shell errors, `--print-launch-command`, and normal flag parsing |
| `main.go` | Dispatch shell-init output; add `--print-launch-command`; preserve existing `-n -v` picker selection behavior; route picker UI to `/dev/tty` when stdout is reserved for command output |
| `picker/model.go` | Support Bubble Tea program input/output on `/dev/tty` for `--print-launch-command` command-substitution mode |
| `launcher/launch.go` | Add custom-command child runner, detect exit status 146, preserve direct `syscall.Exec` for plain agent binaries |
| `launcher/launch_test.go` | Cover custom runner exit-code propagation and Ctrl-Z diagnostic formatting |
| `README.md` | Document `aps shell-init`, its opt-in nature, and wrapper-script fallback |
| `cmd/root.go` usage text | Mention `shell-init` and avoid implying alias support works without it |

## TDD Tests

- Add parser/dispatch tests for `aps shell-init`.
- Add parser/dispatch tests for `--print-launch-command`.
- Add snapshot-style tests for generated zsh and bash shell code.
- Add tests that shell-init output uses `command aps` and does not edit rc files.
- Add tests that `-n -v` still opens picker selection instead of selecting `sessions[0]`.
- Add tests or integration smoke coverage that `--print-launch-command` writes only the final launch command to stdout.
- Add a launcher test that simulates a child exiting `146` and verifies the diagnostic recommends `aps shell-init`.
- Add tests or golden checks for help/README wording so alias/function support is not advertised outside shell-init.
- Add tests that non-zero non-146 exits still propagate as ordinary child errors.
- Add tests that `NoLaunch` verbose output is unchanged.
- Keep direct binary launch tests or behavior unchanged.
- Add a manual zsh RCA checklist for Ctrl-Z cases: capture `jobs -l` plus `ps -o pid,ppid,pgid,tpgid,stat,command` for all stopped job PIDs before assigning root cause.

## Acceptance Criteria

- `go test ./...` passes.
- `go vet ./...` passes.
- `go build .` passes, then `go install .` is run immediately.
- `aps shell-init zsh` and `aps shell-init bash` print shell-specific code and do not read, write, or modify rc files.
- `aps shell-init` infers zsh/bash from `$SHELL` only when unambiguous; otherwise it asks for an explicit shell.
- Existing `aps -n -v ...` behavior is preserved: it still opens picker selection and prints the selected session's launch command.
- `--print-launch-command` opens picker selection, renders picker UI through `/dev/tty` when needed, and writes only the selected launch command to stdout.
- In shell-init launch-command capture flow, the wrapper calls `command aps --print-launch-command ...`.
- Manual zsh smoke test with `eval "$(aps shell-init zsh)"` confirms an alias-backed custom command can Ctrl-Z and `fg` normally.
- Manual zsh smoke test records `jobs -l` and `ps -o pid,ppid,pgid,tpgid,stat,command` if more than one stopped job appears, and the recorded process groups explain each job.
- Manual zsh smoke test without shell-init confirms Ctrl-Z produces the new diagnostic instead of silent failure.
- README/help clearly state alias/function support requires shell-init; otherwise use external binaries/scripts.

## Evidence To Preserve

- `zsh -i -c "sleep 60"` followed by Ctrl-Z exits 146 and cannot be recovered with `fg`.
- `aps ccaws` passes literal `ccaws`; aliases are not expanded in argument position.
- `aps shell-init` remains viable only as explicit parent-shell integration; it must not edit rc files automatically.
- A Ctrl-Z notification plus a later `jobs` entry with the same job number is one stopped job. Any extra `suspended (tty output)` entry is a separate stopped process group that needs `jobs -l` and `ps` attribution.

## Non-Goals

- Do not change plain `claude`, `opencode`, or `codex` direct-launch behavior.
- Do not bypass picker selection in `--no-launch --verbose` by selecting the first or newest session automatically.
- Do not automatically edit shell rc files.
- Do not claim alias/function support is fixed until `aps shell-init` passes an interactive zsh reproduction confirming Ctrl-Z and `fg` behavior.
- Do not claim `$()` caused a `suspended (tty output)` job unless `jobs -l` and `ps` prove the stopped process group was not the terminal foreground owner.
