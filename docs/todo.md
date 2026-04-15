# TODO

## Pending (Go rewrite)

- [ ] Add README badges after publishing to GitHub: Go Report Card, Go version, Latest release, pkg.go.dev
- [ ] Add `.github/ISSUE_TEMPLATE/` bug report template

## Bugs / Correctness

- [ ] `IFS='|||'` split in preview is fragile — any `|||` inside a message body breaks parsing; use `\x00` or per-line output instead
- [ ] Recent messages preview: `jq .[-10:] | reverse` shows newest-first but the `|||` join loses newlines inside messages — multi-line messages show collapsed
- [ ] `LIMIT 50` on Opencode SQLite query is hardcoded; sessions beyond 50 are invisible even with no path filter

## Refactor

- [ ] Extract duplicated path-filter logic into a shared Python function — currently copy-pasted across all 6 `generate_*` functions
- [ ] Extract duplicated JSONL title/cwd extraction into a shared helper — `extract_title_from_jsonl` and `extract_cwd_from_jsonl` are defined twice

## Performance

- [ ] Cache session list (file-based, TTL ~30s) to avoid re-scanning JSONL on every keystroke in fzf reload
- [ ] Parallelize JSONL scanning across project directories (`concurrent.futures.ThreadPoolExecutor`)

## UX

- [ ] Add `-v / --verbose` flag: print Python stderr to a temp log for debugging path filter misses
- [ ] Add keybind in fzf to delete/archive a Claude session (Ctrl+D)
- [ ] Truncated titles lose context — consider showing `…` suffix when truncated

## Testing

- [ ] Add `bats` test suite covering: path filter exact match, symlink match, substring match (strict vs recursive), non-existent path fallback
