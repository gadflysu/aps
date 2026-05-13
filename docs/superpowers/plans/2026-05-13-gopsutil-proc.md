# gopsutil proc collection Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace `ps aux` + `lsof` external commands in `CollectProcs` and `procStillRunning` with `gopsutil/v4`, reducing single-call cost from ~0.8s to ~0.05s, then tighten the poll interval from 10s to 3s.

**Architecture:** `LStart` in `ProcInfo` currently stores an opaque string from `ps -o lstart=`. With gopsutil, we use `process.CreateTime()` (Unix ms int64) as the process-identity timestamp instead — it is stable, cross-platform, and needs no string parsing. `ProcInfo.LStart` becomes the decimal string representation of that int64 so the rest of the codebase (cache keys, comparisons) is unchanged. The existing `PIDCache` file format (`pid|lstart|sessionID`) is compatible because lstart is already opaque.

**Tech Stack:** `github.com/shirou/gopsutil/v4` (process, process.Process)

---

### Task 1: Add gopsutil dependency

**Files:**
- Modify: `go.mod`, `go.sum`

- [ ] **Step 1: Add gopsutil**

```bash
go get github.com/shirou/gopsutil/v4@latest
```

- [ ] **Step 2: Verify it resolves**

```bash
go build ./...
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "build: add gopsutil/v4 dependency"
```

---

### Task 2: Replace `CollectProcs` with gopsutil

**Files:**
- Modify: `source/pidcache.go`

The new implementation:
1. Calls `process.Processes()` to get all procs (pure Go, no fork).
2. Filters by `p.Name()` == `"claude"` or `"opencode"`.
3. Gets CWD via `p.Cwd()` — returns `""` on permission error, skip those.
4. Gets create time via `p.CreateTime()` (Unix ms int64) and formats as decimal string for `LStart`.

- [ ] **Step 1: Write a smoke test that calls CollectProcs and checks the result shape**

Add to `source/pidcache_test.go`:

```go
func TestCollectProcs_ReturnsValidShape(t *testing.T) {
    procs := CollectProcs()
    // May be empty if no claude/opencode running — that's fine.
    for _, p := range procs {
        if p.PID == "" {
            t.Error("ProcInfo.PID must not be empty")
        }
        if p.LStart == "" {
            t.Error("ProcInfo.LStart must not be empty")
        }
        if p.CWD == "" {
            t.Error("ProcInfo.CWD must not be empty")
        }
    }
}
```

- [ ] **Step 2: Run test to verify it passes with current implementation**

```bash
go test ./source/... -run TestCollectProcs_ReturnsValidShape -v
```

Expected: PASS (establishes baseline contract).

- [ ] **Step 3: Replace CollectProcs in `source/pidcache.go`**

Remove the old `CollectProcs` function (lines ~139–204) and the `lsofCWD` function in `source/active.go` (lines ~114–134). Replace with:

In `source/pidcache.go`, add import `"fmt"` and `"github.com/shirou/gopsutil/v4/process"`, then:

```go
// CollectProcs returns running claude/opencode processes with pid, lstart, and CWD.
// Uses gopsutil for pure-Go process inspection (no ps/lsof subprocess overhead).
func CollectProcs() []ProcInfo {
    all, err := process.Processes()
    if err != nil {
        dbg.Log("[CollectProcs] process.Processes error: %v", err)
        return nil
    }
    var procs []ProcInfo
    for _, p := range all {
        name, err := p.Name()
        if err != nil {
            continue
        }
        if name != "claude" && name != "opencode" {
            continue
        }
        cwd, err := p.Cwd()
        if err != nil || cwd == "" {
            continue
        }
        ct, err := p.CreateTime()
        if err != nil {
            continue
        }
        procs = append(procs, ProcInfo{
            PID:    fmt.Sprintf("%d", p.Pid),
            LStart: fmt.Sprintf("%d", ct),
            CWD:    cwd,
        })
    }
    return procs
}
```

Remove the `"os/exec"` import from `source/pidcache.go` if it is no longer used after removing the old `CollectProcs`. Also remove the `strings` import if unused.

In `source/active.go`, delete the entire `lsofCWD` function (it is no longer called).

- [ ] **Step 4: Build to check for compile errors**

```bash
go build ./...
```

Fix any unused import errors that appear.

- [ ] **Step 5: Run smoke test**

```bash
go test ./source/... -run TestCollectProcs_ReturnsValidShape -v
```

Expected: PASS.

- [ ] **Step 6: Run full test suite**

```bash
go test ./...
```

Expected: all pass.

- [ ] **Step 7: Commit**

```bash
git add source/pidcache.go source/active.go source/pidcache_test.go
git commit -m "fix(source): replace ps+lsof with gopsutil in CollectProcs"
```

---

### Task 3: Replace `procStillRunning` with gopsutil

**Files:**
- Modify: `source/pidcache.go`

`procStillRunning(pid, wantLstart string)` currently calls `ps -p <pid> -o lstart=`. With gopsutil, we check `process.NewProcess(int32(pid))` exists and its `CreateTime()` matches.

- [ ] **Step 1: Write failing test**

`procStillRunning` is package-private. Test it indirectly via `PIDCache.GC`: set up a cache entry for a definitely-dead PID, run GC, verify the entry is removed.

Add to `source/pidcache_test.go`:

```go
func TestPIDCache_GC_RemovesDeadPID(t *testing.T) {
    dir := t.TempDir()
    c := &PIDCache{
        path:    filepath.Join(dir, "pid-session.txt"),
        entries: map[string]string{
            // PID 1 always exists but its CreateTime won't be "999".
            // Use an impossible lstart so the entry is treated as stale.
            "99999999|999": "some-session-id",
        },
    }
    var wg sync.WaitGroup
    wg.Add(1)
    c.GC(&wg)
    wg.Wait()
    if _, ok := c.entries["99999999|999"]; ok {
        t.Error("GC should have removed the dead PID entry")
    }
}
```

- [ ] **Step 2: Run test to confirm it passes with current implementation**

```bash
go test ./source/... -run TestPIDCache_GC_RemovesDeadPID -v
```

Expected: PASS (establishes contract before the refactor).

- [ ] **Step 3: Replace `procStillRunning` in `source/pidcache.go`**

The `LStart` stored in the cache is now the decimal string of `CreateTime()` Unix ms. `procStillRunning` must parse it and compare:

```go
// procStillRunning returns true if the process with the given pid still runs
// and its CreateTime (Unix ms as decimal string) matches wantLstart.
func procStillRunning(pid, wantLstart string) bool {
    pidInt, err := strconv.ParseInt(pid, 10, 32)
    if err != nil {
        return false
    }
    p, err := process.NewProcess(int32(pidInt))
    if err != nil {
        return false // process not found
    }
    ct, err := p.CreateTime()
    if err != nil {
        return false
    }
    return fmt.Sprintf("%d", ct) == wantLstart
}
```

Add `"strconv"` and `"github.com/shirou/gopsutil/v4/process"` to imports in `source/pidcache.go`.

Remove `"os/exec"` if it was still present.

- [ ] **Step 4: Build**

```bash
go build ./...
```

- [ ] **Step 5: Run tests**

```bash
go test ./source/... -run TestPIDCache_GC_RemovesDeadPID -v
go test ./...
```

Expected: all pass.

- [ ] **Step 6: Commit**

```bash
git add source/pidcache.go source/pidcache_test.go
git commit -m "fix(source): replace ps -p with gopsutil in procStillRunning"
```

---

### Task 4: Tighten poll interval to 3s

**Files:**
- Modify: `picker/model.go`

- [ ] **Step 1: Write a test that captures the current interval**

Add to `picker/model_test.go`:

```go
func TestProcsPollInterval_Is3Seconds(t *testing.T) {
    if procsPollInterval != 3*time.Second {
        t.Errorf("procsPollInterval = %v, want 3s", procsPollInterval)
    }
}
```

- [ ] **Step 2: Run test to confirm it fails**

```bash
go test ./picker/... -run TestProcsPollInterval_Is3Seconds -v
```

Expected: FAIL (`got 10s, want 3s`).

- [ ] **Step 3: Change the constant**

In `picker/model.go`, line ~44:

```go
procsPollInterval = 3 * time.Second
```

- [ ] **Step 4: Run test to confirm it passes**

```bash
go test ./picker/... -run TestProcsPollInterval_Is3Seconds -v
```

Expected: PASS.

- [ ] **Step 5: Run full suite**

```bash
go test ./...
```

Expected: all pass.

- [ ] **Step 6: Commit**

```bash
git add picker/model.go picker/model_test.go
git commit -m "fix(picker): tighten poll interval from 10s to 3s"
```

---

### Task 5: Benchmark and verify

- [ ] **Step 1: Measure new CollectProcs cost**

```bash
go test ./source/... -bench=BenchmarkCollectProcs -benchtime=5s -v
```

If no benchmark exists, add to `source/pidcache_test.go`:

```go
func BenchmarkCollectProcs(b *testing.B) {
    for i := 0; i < b.N; i++ {
        CollectProcs()
    }
}
```

Expected: single call well under 100ms (vs previous ~800ms).

- [ ] **Step 2: Run all tests with race detector**

```bash
go test ./... -race
```

Expected: no races, all pass.

- [ ] **Step 3: Build and install**

```bash
go build . && go install .
```

- [ ] **Step 4: Commit benchmark**

```bash
git add source/pidcache_test.go
git commit -m "test(source): add BenchmarkCollectProcs"
```
