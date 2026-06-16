# Issue #4 Performance Acceptance Summary

## 1. Verdict

**PASS**

All six acceptance gates passed. The merged startup optimizations (MetaCache, worker pool, source
concurrency, streaming picker) satisfy every predetermined threshold.

## 2. Compared Revisions

| Role    | Commit    | Description                                                       |
|---------|-----------|-------------------------------------------------------------------|
| Baseline | `e7dac9c` | Serial Claude JSONL loading, no MetaCache, no parallel sources    |
| Current  | `11ac847` | MetaCache, worker pool, source concurrency, streaming picker      |

Binary SHA-256 hashes (authoritative identity):

```
5e5384b7d88681462b14141940bb43b797de5906c203d27704704f7a4de6ef43  baseline-src/bin/aps
2fa595b6b67eb3944e9b02ce3e9e0f3784f0fc8afe2daf66723e9d27e5fcefd6  current-src/bin/aps
```

Both binaries report version string `aps dev-11ac847` because `go build` picks up VCS metadata from
the parent git repo. SHA-256 hashes are the authoritative binary identity.

## 3. Environment

| Field        | Value                                          |
|--------------|------------------------------------------------|
| Machine      | Apple M4, darwin/arm64                         |
| Go version   | go1.26.2 darwin/arm64                          |
| Benchmark    | UTC 2026-06-15T16:19:51Z                       |
| Blocking runner | hyperfine 1.20.0 (--warmup 3 --runs 20)     |
| Blocking samples | 20 per series                              |
| Interactive samples | 10 per series (PTY via `/usr/bin/expect`) |
| GOMAXPROCS   | arm64 (default, no override)                   |

## 4. Fixture

| Property           | Value                              |
|--------------------|------------------------------------|
| JSONL files        | 103                                |
| Total bytes        | 85,773,517 (~81.8 MiB)             |
| Records per file   | 1,000 user records                 |
| Baseline sessions  | 103 (confirmed via list mode)      |
| Current sessions   | 103 (confirmed via list mode)      |
| Location           | `.worktrees/perf-issue-4/perf-home/.claude/projects` |

## 5. Blocking Results

All times in milliseconds. n=20 per series.

| Series          | n  | Mean    | Stddev | Median  | p95     | Min     | Max     |
|-----------------|----|---------|--------|---------|---------|---------|---------|
| baseline-cold   | 20 | 342.155 | 13.395 | 337.324 | 357.665 | 332.979 | 390.300 |
| current-cold    | 20 | 259.118 |  4.813 | 257.088 | 268.807 | 254.949 | 273.100 |
| current-warm    | 20 |   9.987 |  0.607 |   9.913 |  10.913 |   9.311 |  11.491 |

Speedup summary:

| Comparison                          | Baseline median | Candidate median | Speedup | Reduction |
|-------------------------------------|-----------------|------------------|---------|-----------|
| Cold: current vs baseline           | 337.324 ms      | 257.088 ms       | 1.31x   | 23.79%    |
| Warm vs baseline cold               | 337.324 ms      |   9.913 ms       | 34.03x  | 97.06%    |

## 6. Interactive Results

Instrumented milestones extracted from debug logs. All times in milliseconds.

**Time-to-first-view** (from first debug checkpoint to `first View()` render):

| Series         | n  | Mean   | Stddev | Median  | p95    | Min    | Max    |
|----------------|----|--------|--------|---------|--------|--------|--------|
| baseline       | 10 | 27.518 |  1.623 |  27.026 | 31.567 | 26.178 | 31.567 |
| current-cold   | 10 |  0.399 |  0.288 |   0.318 |  0.807 |  0.115 |  0.807 |
| current-warm   | 10 |  0.936 |  1.097 |   0.182 |  2.695 |  0.117 |  2.695 |

Baseline time-to-first-view is measured from `loadSessions` (all sessions already loaded) to
`first View()`. Current time-to-first-view is measured from `interactiveLoad start` to
`first View()`.

**All-sessions-ready** (from first checkpoint to `interactiveLoad done: 103 sessions`):

| Series         | n  | Mean    | Stddev  | Median  | p95     | Min     | Max     |
|----------------|----|---------|---------|---------|---------|---------|---------|
| baseline       | 10 |  27.518 |   1.623 |  27.026 |  31.567 |  26.178 |  31.567 |
| current-cold   | 10 | 227.575 |  11.789 | 223.294 | 253.353 | 216.648 | 253.353 |
| current-warm   | 10 |   2.030 |   1.073 |   2.450 |   3.370 |   0.448 |   3.370 |

Note: For baseline, `all_sessions_ready` equals `time_to_first_view` because sessions are loaded
before the picker starts. For current-cold, the TUI renders in ~0.3 ms but the background loader
finishes at ~223 ms — this confirms the streaming picker design: show first, load in background.

## 7. Streaming Ordering

In all 10 current cold runs, `first View()` precedes `interactiveLoad done: 103 sessions`.

| Series       | first_view_before_done |
|--------------|------------------------|
| current-cold | 10/10 true             |
| current-warm | 10/10 true             |

## 8. Acceptance Gates

| Gate | Requirement                                              | Evidence                                      | Result |
|------|----------------------------------------------------------|-----------------------------------------------|--------|
| G1   | Both binaries discover exactly 103 sessions every run   | fixture.txt confirms 103/103; blocking logs 20×103; interactive logs 30×103 | **PASS** |
| G2   | Current cold blocking median ≥20% lower than baseline   | 337.3→257.1 ms = 23.79% reduction             | **PASS** |
| G3   | Current warm blocking median ≥5.00x faster than baseline cold | 337.3/9.9 = 34.03x                      | **PASS** |
| G4   | Current cold interactive ttfv median ≥50% lower than baseline | 27.0→0.318 ms = 98.83% reduction        | **PASS** |
| G5   | first View() precedes interactiveLoad done in all 10 current cold runs | 10/10 confirmed            | **PASS** |
| G6   | go vet, go test, go build, workspace-local go install pass | All pass, zero test failures              | **PASS** |

## 9. Verification

Commands run against master HEAD (`11ac847`) in `/Users/sd/projects/aps`:

```
go vet ./...                                                  exit 0
go build .                                                    exit 0
GOBIN=.worktrees/perf-issue-4/bin go install .               exit 0
go test -coverprofile=.worktrees/perf-issue-4/raw/coverage.txt ./...
  github.com/gadflysu/aps         ok  1.264s  coverage: 2.6%
  github.com/gadflysu/aps/cmd     ok  1.025s  coverage: 92.3%
  github.com/gadflysu/aps/display ok  2.379s  coverage: 82.6%
  github.com/gadflysu/aps/filter  ok  1.527s  coverage: 97.0%
  github.com/gadflysu/aps/launcher ok 2.127s  coverage: 34.5%
  github.com/gadflysu/aps/picker  ok  8.217s  coverage: 76.8%
  github.com/gadflysu/aps/preview ok  2.842s  coverage: 55.9%
  github.com/gadflysu/aps/source  ok  3.115s  coverage: 72.9%
  github.com/gadflysu/aps/watcher ok  9.618s  coverage: 81.4%
```

## 10. Historical Context

The original performance issue (#4) referenced a 1.05s startup time on an Intel i7 Mac. That
measurement used a different machine, older binary, and uncontrolled fixture. It is provided as
qualitative motivation only and is not used as a speedup denominator in any gate above.

## 11. Anomalies and Limitations

**warm-1 log pollution**: The prewarm list-mode run shared the same `--debug-log` path as
interactive-current-warm-1. The resulting file had 215 lines (107 prewarm + 108 interactive).
The first 107 lines (ending at `14:02`) were stripped; the retained 108 lines (starting at
`14:38`) contain the correct interactive session. This is documented and the cleanup is
reproducible.

**macOS filesystem cache**: The benchmark does not clear the macOS page cache between cold runs.
"Cold" means only that aps MetaCache (`~/.cache/aps/session-meta.gob`) is absent. Filesystem-cached
JSONL reads will be faster than a true cold OS state. The baseline binary has no MetaCache and
therefore always runs under filesystem-warm conditions.

**Baseline binary version string**: Both binaries show `aps dev-11ac847` in their version string
because `go build` picks up the parent repo's HEAD VCS metadata during `git archive` export.
This is cosmetic only; SHA-256 hashes confirm distinct binaries.

**Interactive sample size**: 10 samples per series. Scheduler jitter on Apple M4 is low but the
sample is small. The ttfv values (sub-millisecond for current) are dominated by clock resolution.

## 12. Reproduction

Artifact workspace: `.worktrees/perf-issue-4/`

```
.worktrees/perf-issue-4/
├── env.zsh                     # source this first
├── analyze.rb                  # statistics generator
├── baseline-src/bin/aps        # pinned e7dac9c binary
├── current-src/bin/aps         # pinned 11ac847 binary
├── perf-home/                  # fixture home (103 JSONL files, 85,773,517 bytes)
├── raw/
│   ├── environment.txt
│   ├── binaries.sha256
│   ├── fixture.txt
│   ├── blocking-cold.json      # hyperfine --runs 20, 2 commands
│   ├── blocking-warm.json      # hyperfine --runs 20, 1 command
│   ├── blocking-baseline.log   # 20 debug checkpoints
│   ├── blocking-current-cold.log
│   ├── blocking-current-warm.log
│   ├── interactive-baseline-{1..10}.log
│   ├── interactive-current-cold-{1..10}.log
│   ├── interactive-current-warm-{1..10}.log
│   ├── coverage.txt
│   └── artifacts.sha256        # SHA-256 of all raw + derived files
└── derived/
    ├── blocking.tsv
    ├── interactive.csv
    ├── statistics.tsv
    └── comparisons.tsv
```

Key command families used:

- **Build**: `git archive <commit> | tar -xf - -C <dir>; GOBIN=$PWD/bin go install .`
- **Blocking benchmark**: `hyperfine --warmup 3 --runs 20 --export-json ... --prepare 'rm -f ...' ...`
- **Interactive sampling**: `/usr/bin/expect` with `set stty_init "rows 40 cols 120"` and `COLUMNS=120 LINES=40 TERM=xterm-256color` env vars; `after 3000; send "q"`
- **Statistics**: `ruby .worktrees/perf-issue-4/analyze.rb`
- **Artifact hash**: `fd -0 -t f . raw derived | sort -z | xargs -0 shasum -a 256 > raw/artifacts.sha256`

Artifact integrity manifest: `.worktrees/perf-issue-4/raw/artifacts.sha256` (50 files).

## 13. Recommendation

**Close #4: all acceptance gates passed.**

Good luck, Master!
