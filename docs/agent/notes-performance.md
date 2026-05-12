# Performance Notes: bubbletea picker

## Baseline (fzf-based, v0.1.0-bash)

### Startup latency
- Session loading is serial: Claude JSONL scan → Opencode SQLite query
- UI appears only after all data is piped to fzf stdin
- No streaming: fzf receives all lines in one shot before rendering

### Preview
- Every focus change forks a subprocess (`aps --_preview-*`)
- Fork + exec overhead: ~5–20 ms per focus event
- No caching: subprocess has no memory between focus changes
- JSONL re-read on every preview render

---

## Current (bubbletea, v0.2.0)

### Startup latency
- Unchanged from fzf baseline: still serial load, UI appears after all data loaded
- No regression introduced

### Fuzzy filtering (per keypress)
- sahilm/fuzzy on 1 000 sessions: < 1 ms
- Acceptable up to ~5 000 sessions without debounce
- At > 10 000 sessions: add 50 ms debounce

### Per-frame rendering
- lipgloss renders ~30 visible rows per frame: microsecond-level
- bubbletea only re-renders on state changes, never on a fixed tick

### Preview
- Trigger: Space keypress (manual), not every focus change
- Execution: direct Go function call, no fork
- File I/O cost unchanged, fork overhead eliminated
- Cache possible (see P2 below)

---

## Future Optimizations

### P1 — Parallel loading + streaming UI  *(highest impact)*

Load Claude and Opencode concurrently via goroutines; deliver partial results
to bubbletea via `tea.Cmd`. UI appears with first batch; remaining sessions
stream in as they load. fzf cannot do this (stdin pipe is linear).

Expected impact: UI appears ~2× faster when both sources are enabled.

### P2 — Preview content cache

```go
previewCache map[string]string  // session ID → rendered preview text
```

Preview is computed once on first Space press; subsequent toggles for the same
session are instant.

### P3 — Fuzzy match character highlighting

sahilm/fuzzy returns `MatchedIndexes []int`. Use these to apply per-character
lipgloss bold/color on matched runes in the title column.
Deferred: increases rendering complexity, does not affect correctness.

### P4 — Async preview loading

Move preview JSONL/SQLite read into a `tea.Cmd` goroutine. Show a spinner while
loading; update content via `tea.Msg` when ready. Unnecessary for typical
session sizes; useful if JSONL files grow very large.

---

## agf Concurrency Model (Reference)

Source: https://github.com/subinium/agf v0.11.1, `src/scanner/mod.rs`

agf achieves fast startup via two-level parallelism:

1. **Per-client threads** — `scan_all` spawns one `thread::spawn` per client
   (8 total in v0.11.1). All clients are scanned simultaneously regardless of
   which are installed. Results are collected and sorted by timestamp.

2. **Intra-client rayon** (Claude scanner only) — after `list_session_files`
   builds the full list of per-session JSONL paths, `scan_session_metadata`
   uses `rayon::into_par_iter()` to read worktree/recap metadata from all
   files concurrently across CPU cores.

The equivalent Go approach would be `sync.WaitGroup` + goroutines for the
per-client level, and a worker pool (e.g. `golang.org/x/sync/errgroup`) for
per-file parallelism inside the Claude scanner. See P1 above for the planned
aps implementation.
