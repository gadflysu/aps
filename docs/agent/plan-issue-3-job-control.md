## Goal

Fix Ctrl-Z / `fg` recovery when launching Claude or Opencode through `--claude-cmd`, `--opencode-cmd`, or resolved `--cmd`, without regressing normal direct `syscall.Exec` launches.

## Current Status

- Issue: #3, open.
- Current `master` still launches custom commands through `$SHELL -i -c "<customCmd> ..."` via `syscall.Exec`.
- All in-process approaches exhausted (see Rejected Approaches). No code change has been merged.
- Old branch `fix/3-job-control` is stale; superseded by `fix/3-job-control-v2` which contains only this research plan.

## Confirmed Facts

- Plain `claude` and `opencode` paths are unaffected because `aps` directly replaces itself with the target binary through `syscall.Exec`.
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
- Redefine `--claude-cmd` contract: accept only external binaries/scripts, not aliases. Document that users should create a wrapper script instead of relying on alias expansion.
- Investigate whether aps can resolve the alias by reading the parent shell's config files (fragile, shell-specific).
- Investigate terminal ioctl approaches (e.g. `TIOCSTI`) to inject keystrokes into the parent shell, though this is platform-specific and potentially blocked by security policies.

## Research Done

1. Reproduced the bug: `zsh -i -c "sleep 60"` → Ctrl-Z → exit 146, `fg` cannot recover.
2. Tested `exec.Command` replacement: aps stays alive but intermediate shell still exits 146.
3. Tested `exec` prefix: alias `ccaws` not resolved (`command not found`).
4. Tested `trap '' TSTP`: interactive zsh rejects it.
5. Tested `source ~/.zshrc` without `-i`: rc guard `[[ $- == *i* ]]` skips alias definitions.
6. Confirmed alias is not expanded in argument position (even unquoted): `aps ccaws` passes literal `ccaws` to binary.
7. Proposed `aps shell-init` + eval: works technically but rejected because it requires modifying user's rc file.

## Non-Goals

- Do not change plain `claude` or `opencode` direct-launch behavior.
- Do not claim alias/function support is fixed until an interactive shell reproduction confirms Ctrl-Z and `fg` behavior.
