# Plan: No-Header List Mode Output

## Goal

Add a `--no-header` option for list mode so script users can consume session rows without the table
header.

## Problem

`aps -l` always prints a table header. That is useful for human-readable output, but inconvenient in
pipelines:

- `aps -l --no-header | wc -l` should count sessions, not sessions plus one header.
- `aps -l --no-header | grep ...` should not match `TIME`, `TITLE`, `ID`, `TURNS`, `SRC`, or
  `DIRECTORY` header text.

## Target Behavior

| Command | Header |
|---------|--------|
| `aps -l` | printed |
| `aps --list` | printed |
| `aps -l --no-header` | omitted |
| `aps --no-header` without list mode | accepted but has no output effect unless list mode is used |

`--no-header` should not change column widths, row formatting, color behavior, session loading, or
interactive mode.

## Scope

### Modify

| File | Change |
|------|--------|
| `cmd/root.go` | Add `NoHeader` config field, parse `--no-header`, and document it in help output. |
| `main.go` | Skip `display.Header(w)` in `runList()` when `cfg.NoHeader` is true. |
| `cmd` tests | Add or update flag parsing tests for `--no-header`. |
| `main` or display-facing tests | Cover list output with and without the header. |

### Do Not Change

- Do not remove the header by default.
- Do not add a short flag unless the user explicitly requests one.
- Do not alter `display.Header()` formatting.
- Do not change interactive picker output.

## TDD Plan

Write failing tests first:

```bash
go test ./cmd/... -run 'Test.*NoHeader'
go test . -run 'TestRunList.*Header'
```

Target cases:

- `cmd.Parse([]string{"-l", "--no-header"})` sets `ListOnly` and `NoHeader`.
- `cmd.Parse([]string{"-l"})` leaves `NoHeader` false.
- `runList()` prints the header by default.
- `runList()` omits the header when `NoHeader` is true and still prints all session rows.

If direct `runList()` testing is awkward because it writes to `os.Stdout`, either add a small
testable helper such as `formatListOutput(sessions, cfg, width)` or use temporary stdout capture in
focused tests. Prefer the smaller change that matches existing patterns.

## Implementation Plan

1. Add `NoHeader bool` to `cmd.Config`.
2. Register `--no-header` in `cmd.Parse()`.
3. Add help text near `-l, --list`.
4. In `runList()`, print `display.Header(w)` only when `!cfg.NoHeader`.
5. Add focused tests.

## Verification

Run:

```bash
go test ./cmd/...
go test .
go test ./...
go build .
go install .
```

Manual smoke:

```bash
aps -l --color never | head -1
aps -l --no-header --color never | head -1
aps -l --no-header --color never | wc -l
```

Verify the first command starts with the header and the second starts with the first session row.

## Risks

- Existing scripts may rely on the default header; keep default behavior unchanged.
- If `--no-header` is used without `-l`, accepting it silently is less surprising than adding a new
  conflict, because the option is simply irrelevant outside list mode.
