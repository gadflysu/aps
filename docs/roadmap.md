# Roadmap

## Near-term (polish)

- **Refactor shared path-filter helper** — eliminate the 6× duplication; makes future filter changes a single-site edit
- **Fix `|||` separator fragility** — replace with null-byte or line-per-message output in jq
- **Raise / make configurable the `LIMIT 50`** — add `-n N` flag or `APS_LIMIT` env var

## Medium-term (features)

- **Session caching** — write a `~/.cache/aps/sessions.json` with mtime-based invalidation; target <50ms cold start
- **Multi-select mode** — `fzf --multi` to open several sessions at once or batch-print their directories
- **Delete/rename keybind** — Ctrl+D to archive, Ctrl+R to rename (writes `custom-title` entry to JSONL)
- **`-v / --verbose` flag** — surface Python errors to stderr for easier debugging

## Long-term (architecture)

- **Extract Python to a standalone script** — `aps-data.py` called by the Bash wrapper; enables unit tests, IDE support, and type annotations
- **Add bats test suite** — cover path filter logic, title extraction, wcswidth accuracy
- **Parallel JSONL scanning** — `ThreadPoolExecutor` for large `~/.claude/projects/` trees
- **Plugin / data-source API** — allow third-party clients (e.g., Cursor, Windsurf) to register a data loader without modifying the core script
