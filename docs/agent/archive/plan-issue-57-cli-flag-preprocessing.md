# Plan: Issue #57 — CLI Flag Preprocessing

Issue: #57

## Goal

Make aps CLI preprocessing preserve normal Go `flag` value syntax while retaining supported boolean
short-flag clusters.

## Problem

`cmd.Parse` preprocesses arguments before calling `flag.Parse`:

- `expandShortFlags` exists to support clusters such as `-nla`.
- `expandBareColor` makes bare `--color` / `-color` mean `always`.

The old short-cluster logic split every single-dash token longer than two characters, which breaks
standard Go value forms such as `-color=never` and `-from=2026-06-01`.

The bare-color rewrite also breaks `--color never`: `--color` becomes `--color=always`, leaving
`never` as the positional `PATH_FILTER`.

## Target Behavior

- Keep supported boolean clusters:
  - `-nv` -> `-n -v`
  - `-nla` -> `-n -l -a`
- Do not split non-cluster single-dash forms:
  - `-color`
  - `-color=never`
  - `-from=2026-06-01`
  - `-nz`
- Support explicit Go-style color values:
  - `--color=never`
  - `--color never`
  - `-color=never`
  - `-color never`
- Remove bare color support:
  - `--color` should error as a missing value.
  - `-color` should error as a missing value.
- Reject invalid color values with a clear parse error.

## Target Files

| File | Change |
|------|--------|
| `cmd/root.go` | Restrict `expandShortFlags`; remove `expandBareColor`; validate `cfg.Color`. |
| `cmd/root_test.go` | Add parser and preprocessing tests for standard value syntax, bare color errors, and known boolean clusters. |
| `README.md` | Update examples only if any example implies bare `--color` support. |
| `docs/agent/plan-issue-4-performance-acceptance.md` | Keep `--color=never` examples; no behavior dependency on bare color. |

## Non-Goals

- Do not add new flags.
- Do not switch from Go `flag` to another CLI framework.
- Do not remove `-nv` / `-nla` cluster support.
- Do not change list rendering or color output behavior beyond parsing.

## TDD Plan

1. Keep the already-added `TestExpandShortFlags_OnlyKnownBooleanClusters` coverage.
2. Add parser tests that fail before removing `expandBareColor`:
   - `Parse([]string{"--color", "never"})` sets `Color == "never"` and leaves `PathFilter == ""`.
   - `Parse([]string{"-color", "never"})` sets `Color == "never"` and leaves `PathFilter == ""`.
   - bare `--color` exits with missing value.
   - bare `-color` exits with missing value.
   - invalid `--color=bad` exits with an invalid color error.
3. Run the focused tests and watch the new parser tests fail.
4. Remove `expandBareColor` from the parse pipeline.
5. Add explicit color validation after `fs.Parse`.
6. Re-run focused tests and full verification.

## Implementation Steps

1. Update `expandShortFlags` so it only splits a token when:
   - it starts with one `-`,
   - it does not contain `=`,
   - it has more than one flag character,
   - every character after `-` is a known boolean short flag.
2. Remove the call to `expandBareColor`.
3. Delete `expandBareColor` if no tests need it.
4. Add color validation:

```go
switch cfg.Color {
case "auto", "always", "never":
default:
    fmt.Fprintf(os.Stderr, "error: invalid --color value %q; use auto, always, or never\n", cfg.Color)
    os.Exit(2)
}
```

5. Update tests that currently expect bare `--color` to mean `always`.
6. Check README/help examples for `--color` syntax and prefer `--color=never` in examples that must be
   unambiguous.

## Verification

Run:

```bash
go test ./cmd -run 'TestExpandShortFlags|TestParse_Color' -count=1
go test ./cmd -count=1
go test ./...
go vet ./...
go build .
GOBIN=/Users/sd/projects/aps/.worktrees/verify-bin go install .
```

Manual checks:

```bash
./aps -c -l --color=never
./aps -c -l --color never
./aps -c -l -color=never
./aps -c -l -color never
./aps -c -l --color
./aps -c -l --color=bad
```

Expected:

- The four explicit color-value forms parse as color values and do not create a `PATH_FILTER`.
- Bare `--color` fails with a missing value.
- Invalid color value fails with an invalid-value error.

## Git Plan

Use branch:

```bash
fix/57-cli-flag-preprocessing
```

Commit implementation with:

```text
fix(cmd): preserve standard color flag value parsing

Closes #57
```
