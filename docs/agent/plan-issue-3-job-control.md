## Goal

Implement an opt-in `aps shell-init` integration for users who need shell alias/function custom commands, and detect the known Ctrl-Z failure in the fallback subprocess path so aps can report an actionable error.

`shell-init` is the real recovery path because it evaluates the launch command in the parent shell. The subprocess detection path is an interim mitigation for users who have not installed the shell integration.

## Current Status

- Issue: #3, open.
- Current `master` still launches custom commands through `$SHELL -i -c "<customCmd> ..."` via `syscall.Exec`.
- All self-contained subprocess approaches are exhausted (see Rejected Approaches). No code change has been merged.
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

## Open Direction

No viable approach remains that works within the subprocess model. The fundamental constraint is that aliases live in the parent shell's memory and are inaccessible to any child process.

Possible avenues:
- Implement `aps shell-init` as an explicit opt-in parent-shell wrapper. It must print shell code only; aps must not edit rc files.
- Keep subprocess execution as fallback for users who do not install shell integration, but detect exit 146 and recommend `aps shell-init`.
- Document that external wrapper scripts remain the simplest self-contained alternative when users do not want shell integration.

## Shell Init Integration

Add `aps shell-init <shell>` to print shell-specific code that users can install with:

```sh
eval "$(aps shell-init zsh)"
```

The generated shell wrapper should:

- call the real binary with `command aps` to avoid recursive wrapper calls;
- use `command aps --no-launch --verbose ...` to produce the final `cd ... && <customCmd> ...` launch command;
- evaluate that launch command in the current parent shell so aliases/functions resolve from the parent shell's live state;
- only use this path when a custom command is present; direct agent binary launches can still call `command aps` normally;
- preserve user arguments and quoting as strictly as shell code permits;
- avoid modifying any rc file automatically.

This mode should make Ctrl-Z / `fg` behave like a normal shell-launched job because the agent process becomes a child job of the user's current shell, not a grandchild behind `$SHELL -i -c`.

Shell-init command substitution requires strict UI/data channel separation:

- picker input/output must use `/dev/tty`;
- stdout must contain only the final launch command for the selected session;
- stderr may contain errors after Bubble Tea exits alt screen;
- cancel must produce no stdout command and must not run `eval`.

Do not change existing `aps -n -v ...` semantics by skipping the picker and selecting `sessions[0]`. Users currently rely on `-n -v` opening the picker, then printing the launch command for the selected session. Shell-init must preserve that behavior through `/dev/tty` routing instead of bypassing selection.

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
| `cmd/root_test.go` | Cover explicit shell parsing, `$SHELL` inference, unsupported shell errors, and normal flag parsing |
| `main.go` | Dispatch shell-init output; preserve existing `-n -v` picker selection behavior; route picker UI to `/dev/tty` when stdout is captured for shell command output |
| `picker/model.go` | Support Bubble Tea program input/output on `/dev/tty` for shell-command/no-launch command-substitution mode |
| `launcher/launch.go` | Add custom-command child runner, detect exit status 146, preserve direct `syscall.Exec` for plain agent binaries |
| `launcher/launch_test.go` | Cover custom runner exit-code propagation and Ctrl-Z diagnostic formatting |
| `README.md` | Document `aps shell-init`, its opt-in nature, and wrapper-script fallback |
| `cmd/root.go` usage text | Mention `shell-init` and avoid implying alias support works without it |

## TDD Tests

- Add parser/dispatch tests for `aps shell-init`.
- Add snapshot-style tests for generated zsh and bash shell code.
- Add tests that shell-init output uses `command aps` and does not edit rc files.
- Add tests that `-n -v` still opens picker selection instead of selecting `sessions[0]`.
- Add tests or integration smoke coverage that command-substitution mode writes only the final launch command to stdout.
- Add a launcher test that simulates a child exiting `146` and verifies the diagnostic recommends `aps shell-init`.
- Add tests or golden checks for help/README wording so alias/function support is not advertised outside shell-init.
- Add tests that non-zero non-146 exits still propagate as ordinary child errors.
- Add tests that `NoLaunch` verbose output is unchanged.
- Keep direct binary launch tests or behavior unchanged.

## Acceptance Criteria

- `go test ./...` passes.
- `go vet ./...` passes.
- `go build .` passes, then `go install .` is run immediately.
- `aps shell-init zsh` and `aps shell-init bash` print shell-specific code and do not read, write, or modify rc files.
- `aps shell-init` infers zsh/bash from `$SHELL` only when unambiguous; otherwise it asks for an explicit shell.
- Existing `aps -n -v ...` behavior is preserved: it still opens picker selection and prints the selected session's launch command.
- In shell-init command-substitution flow, picker UI renders through `/dev/tty` and stdout contains only the selected launch command.
- Manual zsh smoke test with `eval "$(aps shell-init zsh)"` confirms an alias-backed custom command can Ctrl-Z and `fg` normally.
- Manual zsh smoke test without shell-init confirms Ctrl-Z produces the new diagnostic instead of silent failure.
- README/help clearly state alias/function support requires shell-init; otherwise use external binaries/scripts.

## Research Done

1. Reproduced the bug: `zsh -i -c "sleep 60"` → Ctrl-Z → exit 146, `fg` cannot recover.
2. Tested `exec.Command` replacement: aps stays alive but intermediate shell still exits 146.
3. Tested `exec` prefix: alias `ccaws` not resolved (`command not found`).
4. Tested `trap '' TSTP`: interactive zsh rejects it.
5. Tested `source ~/.zshrc` without `-i`: rc guard `[[ $- == *i* ]]` skips alias definitions.
6. Confirmed alias is not expanded in argument position (even unquoted): `aps ccaws` passes literal `ccaws` to binary.
7. Proposed `aps shell-init` + eval: technically viable when explicitly installed by the user; aps must not modify rc files automatically.

## Non-Goals

- Do not change plain `claude`, `opencode`, or `codex` direct-launch behavior.
- Do not bypass picker selection in `--no-launch --verbose` by selecting the first or newest session automatically.
- Do not automatically edit shell rc files.
- Do not claim alias/function support is fixed until `aps shell-init` passes an interactive zsh reproduction confirming Ctrl-Z and `fg` behavior.
