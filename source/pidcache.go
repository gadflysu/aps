package source

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/gadflysu/aps/dbg"
	"github.com/shirou/gopsutil/v4/process"
)

// ProcInfo identifies a specific process instance by pid and start time.
// Using both fields prevents pid-reuse collisions: the same pid with a different
// lstart is a different process.
type ProcInfo struct {
	PID    string // numeric process ID
	LStart string // process create time as Unix ms string (opaque, used only for equality)
	CWD    string // working directory
}

// key returns the cache line key: "pid|lstart".
func (p ProcInfo) key() string { return p.PID + "|" + p.LStart }

// PIDCache persists pid→sessionID mappings across aps invocations.
// The backing file lives at ~/.cache/aps/pid-session.txt.
// Format: one entry per line: "pid|lstart|sessionID"
type PIDCache struct {
	mu      sync.Mutex
	path    string
	entries map[string]string // key() → sessionID
}

// LoadPIDCache reads the cache from disk. Missing file is not an error.
func LoadPIDCache() *PIDCache {
	home, _ := os.UserHomeDir()
	path := filepath.Join(home, ".cache", "aps", "pid-session.txt")
	c := &PIDCache{
		path:    path,
		entries: make(map[string]string),
	}
	f, err := os.Open(path)
	if err != nil {
		return c // file absent: start empty
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		parts := strings.SplitN(sc.Text(), "|", 3)
		if len(parts) != 3 {
			continue
		}
		key := parts[0] + "|" + parts[1]
		c.entries[key] = parts[2]
	}
	dbg.Log("[PIDCache] loaded %d entries from %s", len(c.entries), path)
	return c
}

// Lookup returns the sessionID for a proc, or "" if not found.
func (c *PIDCache) Lookup(p ProcInfo) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.entries[p.key()]
}

// Set records pid → sessionID and persists to disk.
func (c *PIDCache) Set(p ProcInfo, sessionID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries[p.key()] == sessionID {
		return
	}
	c.entries[p.key()] = sessionID
	dbg.Log("[PIDCache] set %s → %s", p.key(), sessionID)
	c.flush()
}

// GC removes entries whose process is no longer running, then persists.
// Intended to run in a goroutine; call wg.Done when finished.
func (c *PIDCache) GC(wg *sync.WaitGroup) {
	defer wg.Done()
	c.mu.Lock()
	defer c.mu.Unlock()

	before := len(c.entries)
	for key := range c.entries {
		pid := strings.SplitN(key, "|", 2)[0]
		lstart := strings.SplitN(key, "|", 2)[1]
		if !procStillRunning(pid, lstart) {
			dbg.Log("[PIDCache] GC remove stale pid=%s lstart=%s", pid, lstart)
			delete(c.entries, key)
		}
	}
	if len(c.entries) != before {
		c.flush()
	}
	dbg.Log("[PIDCache] GC done: %d removed, %d remaining", before-len(c.entries), len(c.entries))
}

// flush writes all entries to disk. Caller must hold c.mu.
func (c *PIDCache) flush() {
	if err := os.MkdirAll(filepath.Dir(c.path), 0o700); err != nil {
		return
	}
	f, err := os.CreateTemp(filepath.Dir(c.path), "pid-session-*.tmp")
	if err != nil {
		return
	}
	for key, sid := range c.entries {
		parts := strings.SplitN(key, "|", 2)
		if len(parts) != 2 {
			continue
		}
		_, _ = f.WriteString(parts[0] + "|" + parts[1] + "|" + sid + "\n")
	}
	_ = f.Close()
	_ = os.Rename(f.Name(), c.path)
}

// procStillRunning returns true if the process with the given pid still runs
// and its CreateTime (Unix ms as decimal string) matches wantLstart.
func procStillRunning(pid, wantLstart string) bool {
	pidInt, err := strconv.ParseInt(pid, 10, 32)
	if err != nil {
		return false
	}
	p, err := process.NewProcess(int32(pidInt))
	if err != nil {
		return false
	}
	ct, err := p.CreateTime()
	if err != nil {
		return false
	}
	return fmt.Sprintf("%d", ct) == wantLstart
}

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
		// gopsutil on macOS reports the npm shim name as "claude.exe" / "opencode.exe".
		if name != "claude" && name != "claude.exe" && name != "opencode" && name != "opencode.exe" {
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
