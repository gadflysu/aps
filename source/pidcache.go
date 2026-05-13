package source

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/gadflysu/aps/dbg"
)

// ProcInfo identifies a specific process instance by pid and start time.
// Using both fields prevents pid-reuse collisions: the same pid with a different
// lstart is a different process.
type ProcInfo struct {
	PID    string // numeric string from ps
	LStart string // raw lstart string from ps (opaque, used only for equality)
	CWD    string // working directory from lsof
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

// procStillRunning returns true if pid exists and its lstart matches.
func procStillRunning(pid, wantLstart string) bool {
	out, err := exec.Command("ps", "-p", pid, "-o", "pid=,lstart=").Output()
	if err != nil {
		return false
	}
	line := strings.TrimSpace(string(out))
	// output: "  PID lstart..." — strip pid prefix
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return false
	}
	// lstart is everything after the pid field
	gotLstart := strings.Join(fields[1:], " ")
	return gotLstart == wantLstart
}

// CollectProcs returns running claude/opencode processes with pid, lstart, and CWD.
func CollectProcs() []ProcInfo {
	out, err := exec.Command("ps", "aux").Output()
	if err != nil {
		dbg.Log("[CollectProcs] ps aux error: %v", err)
		return nil
	}

	// Collect pids first.
	type pidCmd struct{ pid, base string }
	var candidates []pidCmd
	seen := make(map[string]bool)
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 11 {
			continue
		}
		cmd := fields[10]
		base := cmd
		if i := strings.LastIndex(cmd, "/"); i >= 0 {
			base = cmd[i+1:]
		}
		if base != "claude" && base != "opencode" {
			continue
		}
		pid := fields[1]
		if seen[pid] {
			continue
		}
		seen[pid] = true
		candidates = append(candidates, pidCmd{pid, base})
	}

	if len(candidates) == 0 {
		return nil
	}

	// Fetch lstart for all candidate pids in one ps call.
	pids := make([]string, len(candidates))
	for i, c := range candidates {
		pids[i] = c.pid
	}
	lstartOut, err := exec.Command("ps", append([]string{"-p", strings.Join(pids, ","), "-o", "pid=,lstart="}, []string{}...)...).Output()
	lstartMap := make(map[string]string) // pid → lstart
	if err == nil {
		for _, line := range strings.Split(string(lstartOut), "\n") {
			fields := strings.Fields(line)
			if len(fields) < 2 {
				continue
			}
			pid := fields[0]
			lstart := strings.Join(fields[1:], " ")
			lstartMap[pid] = lstart
		}
	}

	var procs []ProcInfo
	for _, c := range candidates {
		cwd := lsofCWD(c.pid)
		lstart := lstartMap[c.pid]
		if cwd != "" {
			procs = append(procs, ProcInfo{PID: c.pid, LStart: lstart, CWD: cwd})
		}
	}
	return procs
}
