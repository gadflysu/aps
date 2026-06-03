# Plan: Issue #40 — Clarify No-Launch Output Flags

## Goal

Separate no-launch behavior from output-format selection. `--verbose` should not decide whether no-launch prints a directory or a launch command.

## Current State

- `-n` / `--no-launch` opens picker selection, then prints the selected session directory.
- `-n -v` / `--no-launch --verbose` opens picker selection, then prints a full launch command.
- `-v` also controls verbose diagnostics in loading paths.
- README and help advertise `aps -nv` as "No-launch verbose: print full launch command".

## Problem

`-n -v` overloads `--verbose` with an output-format role. This is unclear for users and awkward for shell integration, where a machine-readable launch command should have an explicit flag.

The old `-nv` shorthand does not need to remain compatible.

## Proposed CLI

- `--print-dir`: open picker, then print the selected session directory without launching.
- `--print-launch-command`: open picker, then print the selected session launch command without launching.
- `-n` / `--no-launch`: either remain as an alias for `--print-dir` or be removed only if the user approves a breaking cleanup.
- `-v` / `--verbose`: diagnostics only; do not use it to select output format.

Recommended first implementation: keep `-n` as an alias for `--print-dir` to preserve the useful historical workflow, but remove `-n -v` as a documented or required command-printing interface. If `-n -v` remains parseable temporarily, it should not be used by new code or shell integration.

## Relationship To #3

#3's shell-init work needs a machine-readable launch-command output path. Earlier #3 drafts relied on `-n -v`, but this issue should replace that dependency with `--print-launch-command`.

When implementing #40, update `docs/agent/plan-issue-3-job-control.md` or the #3 branch so shell-init calls:

```sh
command aps --print-launch-command ...
```

and never depends on:

```sh
command aps --no-launch --verbose ...
```

This issue may be implemented before #3, or #3 may implement the flag as part of its work, but both plans must agree on the same command-output interface.

## Behavior

- All print modes still select a session interactively; never select `sessions[0]` automatically.
- `--print-dir` writes only the selected directory to stdout.
- `--print-launch-command` writes only the selected launch command to stdout.
- Cancellation writes nothing to stdout and exits successfully.
- Errors write to stderr.
- List mode remains separate and unchanged.

## Files To Change

| File | Change |
|------|--------|
| `cmd/root.go` | Add `PrintDir` / `PrintLaunchCommand` config, parse explicit flags, update conflicts and usage |
| `cmd/root_test.go` | Cover new flags, `-v` no longer changing no-launch output type, and conflict behavior |
| `main.go` | Route explicit print modes through picker selection and selected-session output |
| `launcher/launch.go` | Replace `NoLaunch+Verbose` output branching with explicit output-mode handling |
| `launcher/launch_test.go` | Cover directory output, launch-command output, and custom command rendering |
| `README.md` | Replace `aps -nv` example with `--print-launch-command`; document `--print-dir` |
| `docs/agent/plan-issue-3-job-control.md` | Replace any shell-init dependency on `-n -v` with `--print-launch-command` |

## TDD Tests

Write or update tests before implementation:

| Test | What it proves |
|------|----------------|
| `TestParsePrintDir` | `--print-dir` selects directory-output mode |
| `TestParsePrintLaunchCommand` | `--print-launch-command` selects command-output mode |
| `TestVerboseDoesNotSelectOutputFormat` | `-v` does not change print output type by itself |
| `TestPrintModesConflict` | `--print-dir` and `--print-launch-command` cannot both be set |
| `TestPrintLaunchCommand_CustomCommand` | custom command renders full selected-session launch command |
| `TestPrintModesStillUsePicker` | print modes use selected session, not the newest session |

## Acceptance Criteria

- `go test ./...` passes.
- `go vet ./...` passes.
- `go build .` passes, then `go install .` is run immediately.
- `aps --print-dir` opens picker and prints only the selected directory.
- `aps --print-launch-command` opens picker and prints only the selected launch command.
- `aps -v` is diagnostics-only and does not choose a no-launch output format.
- README/help no longer promote `aps -nv`.

## Non-Goals

- Do not change list mode output.
- Do not implement shell-init in this issue.
- Do not bypass picker selection in any print mode.
- Do not add a machine-readable protocol beyond printing the selected launch command.
