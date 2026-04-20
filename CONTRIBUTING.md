# Contributing to aps

Bug reports and pull requests are welcome.

## Before You Start

Open an issue to discuss any significant change before writing code. This avoids wasted effort if the direction doesn't fit the project.

## Development Setup

```bash
git clone https://github.com/gadflysu/aps.git
cd aps
go build .
go test ./...
```

## Workflow

1. Fork the repo and create a feature branch from `master`.
2. Write a failing test for your change first (TDD).
3. Implement the fix or feature.
4. Confirm `go test ./...` and `go vet ./...` both pass.
5. Commit with the format `<type>(<scope>): <short imperative phrase>` (see commit type table in README).
6. Open a pull request against `master`.

## Commit Types

| Type | Use for |
|------|---------|
| `feat` | new feature |
| `fix` | bug fix |
| `refactor` | code restructuring |
| `test` | test-only changes |
| `docs` | documentation only |
| `build` | build system (go.mod, Makefile) |
| `chore` | housekeeping (.gitignore, etc.) |

## Code Style

- Run `go fmt ./...` before committing.
- No emojis in code, identifiers, or comments.
- Keep changes minimal and scoped — prefer editing existing files over creating new ones.
- ANSI 16-color palette only (no hex/RGB) in lipgloss styling.

## License

By contributing you agree that your contributions will be licensed under the MIT License.
