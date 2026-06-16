# Issue #4 Performance Acceptance Plan

## Goal

Recompute the pre-optimization baseline for issue #4 and decide whether the merged startup work is
accepted. This plan is an acceptance protocol, not a rigid script. Commands below are probes or
examples; if a probe contradicts the plan, stop, keep the evidence, and revise the plan before taking
more samples.

## Fixed Comparison

| Role | Commit | Meaning |
|---|---|---|
| Baseline | `e7dac9c` | Serial Claude JSONL loading with debug checkpoints, before MetaCache and parallel loading |
| Current | `11ac847` | MetaCache, worker pool, source concurrency, and streaming picker |

Use the same generated fixture for both binaries:

- workspace-only fake home: `.worktrees/perf-issue-4/perf-home`
- Claude data root: `.worktrees/perf-issue-4/perf-home/.claude/projects`
- 103 JSONL files
- 85,773,517 bytes total, approximately 81.8 MiB
- 1,000 valid user records per file

Cold means aps `MetaCache` is absent. Do not claim the macOS filesystem cache is cold.

## Execution Rules

- Stay inside `/Users/sd/projects/aps`.
- Do not use real `~/.claude` data.
- Do not modify production code while measuring.
- Preserve every raw sample. Do not delete individual outliers.
- Rerun only a complete invalid series, and document the superseded series.
- Prefer `hyperfine` for blocking measurements when already installed; do not install it during this task.
- Use `--color=never`, not `--color never`; the latter becomes `PATH_FILTER=never` in these commits.
- Treat interactive TUI automation as environment-sensitive. Prove the chosen PTY/input method with a
  one-run probe before collecting 10 samples.
- If a preflight fails twice for the same reason, write `docs/agent/status-issue-4-performance-acceptance.md`
  and stop.

## Artifact Layout

Keep temporary artifacts under:

```text
.worktrees/perf-issue-4/
├── baseline-src/
├── current-src/
├── perf-home/
├── raw/
└── derived/
```

Tracked deliverables:

- `docs/agent/summary-issue-4-performance-acceptance.md`
- `docs/agent/status-issue-4-performance-acceptance.md` only if blocked or inconclusive before summary

## Phase 1: Rehydrate And Build

1. Read this plan, `docs/agent/notes-performance.md`, and issue #4/#9 context if needed.
2. Record environment facts in `.worktrees/perf-issue-4/raw/environment.txt`:
   - UTC timestamp
   - `git rev-parse HEAD e7dac9c 11ac847`
   - `go version`
   - `go env GOOS GOARCH GOMAXPROCS`
   - CPU model
   - `hyperfine --version` or `hyperfine=unavailable`
3. Export commits with `git archive` into `baseline-src/` and `current-src/`.
4. Build each binary inside its exported tree. After each successful `go build`, run `go install` with
   workspace-local `GOBIN`, not the user's home:

```bash
GOBIN="$PWD/bin" go install .
```

5. Record SHA-256 hashes for both binaries.

Stop if either binary cannot build.

## Phase 2: Fixture Preflight

Generate the fake Claude fixture inside `.worktrees/perf-issue-4/perf-home`. The exact generator may be
shell, Go, Ruby, or another workspace-local method, but it must produce:

- 103 `*.jsonl` files below `.claude/projects`
- total byte count `85773517`
- at least one record with `cwd="/Users/sd/projects/aps"` in every file
- exactly 1,000 countable user records per file
- a stable title record per file

Before measuring, prove all assumptions with small probes:

```bash
fd -e jsonl . .worktrees/perf-issue-4/perf-home/.claude/projects | wc -l
fd -0 -e jsonl . .worktrees/perf-issue-4/perf-home/.claude/projects | xargs -0 stat -f '%z' | awk '{n+=$1} END {print n}'
HOME=/Users/sd/projects/aps/.worktrees/perf-issue-4/perf-home <baseline-bin> -c -l --color=never | awk 'NR > 1 {n++} END {print n+0}'
HOME=/Users/sd/projects/aps/.worktrees/perf-issue-4/perf-home <current-bin> -c -l --color=never | awk 'NR > 1 {n++} END {print n+0}'
```

Expected:

- file count: `103`
- bytes: `85773517`
- baseline sessions: `103`
- current sessions: `103`

If any value differs, stop. Do not benchmark non-equivalent data.

## Phase 3: Blocking Load Measurement

Measure terminating list-mode commands. This captures discovery, parse/cache behavior, sorting, and
list rendering. Use one runner for all blocking series.

Required series:

| Series | Cache State | Command Shape | Samples |
|---|---|---|---:|
| `baseline-cold` | no aps MetaCache | baseline `-c -l --color=never` | 20 |
| `current-cold` | remove MetaCache before each run | current `-c -l --color=never` | 20 |
| `current-warm` | prewarm MetaCache once, then keep it | current `-c -l --color=never` | 20 |

Recommended runner:

```bash
hyperfine --warmup 3 --runs 20 ...
```

Fallback runner:

```bash
/usr/bin/time -p env HOME=<fixture-home> <binary> -c -l --color=never >/dev/null
```

Preflight the exact command once per series before collecting 20 samples. Each preflight must:

- exit zero;
- discover 103 sessions, either via stdout count or debug log;
- write any expected debug checkpoint when `--debug-log` is used.

If `hyperfine` and fallback disagree structurally, pick one runner, rerun all blocking series with it,
and disclose the discarded attempt.

## Phase 4: Interactive Milestone Measurement

This phase measures instrumented TUI milestones, not total process startup time.

Required checkpoints:

| Binary | Required Debug Lines |
|---|---|
| Baseline | `loadSessions: ... (103 sessions)`, `picker.Run start`, `first View()` |
| Current | `interactiveLoad start`, `first View()`, `interactiveLoad done: 103 sessions` |

Do not assume stdin controls Bubble Tea. In current code, non-TTY stdout may cause the picker to open
`/dev/tty`; a pipe like `(sleep 3; printf q) | ...` is not a valid plan until proven.

Acceptable PTY/input methods include:

- `expect`, if it can spawn the binary, wait for first render, send `q`, and exit cleanly;
- a small workspace-local harness that allocates a PTY and sends `q`;
- manual terminal sampling, if automated PTY control remains unreliable and the report clearly marks
  this limitation.

Interactive preflight is mandatory:

1. Run one baseline sample and one current sample with the chosen PTY/input method.
2. Confirm the process exits without external `kill`.
3. Confirm every required checkpoint appears in the debug log.
4. Confirm the current log has `first View()` before `interactiveLoad done`.

Only after preflight passes, collect:

- 10 baseline samples
- 10 current cold samples, removing MetaCache before each run
- 10 current warm samples, after one prewarm run

If no PTY/input method can produce complete logs after two attempts, mark interactive evidence
`INCONCLUSIVE`, retain the logs, and do not invent a replacement metric from list mode.

## Phase 5: Statistics

Keep raw sample files in `.worktrees/perf-issue-4/raw/`. Write normalized data under
`.worktrees/perf-issue-4/derived/`:

- `blocking.tsv`: `series`, `run`, `seconds`
- `interactive.csv`: `series`, `run`, `sessions`, `time_to_first_view_ms`,
  `all_sessions_ready_ms`, `first_view_before_done`
- `statistics.tsv`: `series`, `unit`, `n`, `mean`, `stddev`, `median`, `p95`, `min`, `max`
- `comparisons.tsv`: `comparison`, `baseline_median_ms`, `candidate_median_ms`, `speedup`,
  `reduction_percent`

Use these definitions:

- mean: arithmetic mean
- stddev: sample standard deviation, denominator `n - 1`
- median: average of two middle values for even `n`
- p95: nearest-rank percentile, `ceil(0.95 * n)`
- speedup: `baseline_median / candidate_median`
- reduction: `(1 - candidate_median / baseline_median) * 100`

Do not remove outliers. If a value looks suspicious, keep it and explain it.

## Phase 6: Acceptance Gates

| Gate | Requirement |
|---|---|
| G1 | Both binaries discover exactly 103 sessions in every measured run |
| G2 | Current cold blocking median is at least 20% lower than baseline cold median |
| G3 | Current warm blocking median is at least 5.00x faster than baseline cold median |
| G4 | Current cold instrumented time-to-first-view median is at least 50% lower than baseline |
| G5 | `first View()` precedes `interactiveLoad done` in all 10 current cold runs |
| G6 | Required verification commands pass: `go vet ./...`, `go test -coverprofile=coverage.txt ./...`, `go build .`, workspace-local `go install .` |

Verdict rules:

- `PASS`: G1 through G6 all pass.
- `FAIL`: evidence is valid and at least one of G2 through G5 fails.
- `INCONCLUSIVE`: preflight fails, fixture/toolchain/commit comparison is invalid, required logs are
  missing, or G1/G6 cannot be established.

Do not recommend closing #4 unless the verdict is `PASS`.

## Required Summary Report

Write `docs/agent/summary-issue-4-performance-acceptance.md` with these sections:

1. `Verdict`: exactly `PASS`, `FAIL`, or `INCONCLUSIVE`, plus the decisive reason.
2. `Compared Revisions`: commits and binary hashes.
3. `Environment`: hardware, OS/arch, Go version, runner, sample counts.
4. `Fixture`: files, bytes, records per file, baseline/current discovered sessions.
5. `Blocking Results`: table in milliseconds with n, mean, stddev, median, p95, min, max, speedup, reduction.
6. `Interactive Results`: instrumented time-to-first-view and all-sessions-ready tables.
7. `Streaming Ordering`: current cold/warm counts for first-view-before-done.
8. `Acceptance Gates`: G1-G6, evidence, result.
9. `Verification`: command results and elapsed time.
10. `Historical Context`: original 1.05s Intel i7 result is context only, not a speedup denominator.
11. `Anomalies And Limitations`: failed probes, superseded series, runner fallback, scheduler noise.
12. `Reproduction`: artifact paths and commands or command families used.
13. `Recommendation`: exactly one of:
    - `Close #4: all acceptance gates passed.`
    - `Keep #4 open: valid measurements failed gates ...`
    - `Keep #4 open: evidence is inconclusive because ...`

Before finalizing, hash retained evidence:

```bash
fd -0 -t f . .worktrees/perf-issue-4/raw .worktrees/perf-issue-4/derived \
  | sort -z \
  | xargs -0 shasum -a 256 \
  > .worktrees/perf-issue-4/raw/artifacts.sha256
```

Reference `artifacts.sha256` in the summary. Do not clean `.worktrees/perf-issue-4/` until the user
reviews the report.

## Stop Conditions

Stop and write `docs/agent/status-issue-4-performance-acceptance.md` when:

- a fixed commit no longer exists;
- either binary cannot build;
- fixture equivalence fails;
- the chosen blocking runner cannot produce valid samples;
- interactive PTY preflight fails twice;
- verification fails after measurement.

The status file must include the failed probe, exact command, observed output, retained artifact path,
and recommended next plan change.

## Engineering Philosophy

Benchmark plans should freeze goals, inputs, evidence, and decision rules. They should not freeze
unproven shell mechanics. Probe the mechanics first, then scale only the commands that survived.
