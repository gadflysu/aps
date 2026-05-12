package source

import (
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/gadflysu/aps/dbg"
)

// DetectActive returns a set of session IDs that are currently active.
//
// For each running claude/opencode process:
//  1. If the cache has a pid+lstart → sessionID mapping, use it directly.
//  2. Otherwise fall back: session CWD must match process CWD AND
//     last-activity timestamp must be today (>= local midnight).
//
// Errors from ps/lsof are silently ignored — callers get an empty map on failure.
func DetectActive(sessions []Session, cache *PIDCache) map[string]bool {
	result := make(map[string]bool)
	if len(sessions) == 0 {
		return result
	}

	procs := CollectProcs()
	todayMidnight := todayMidnight()

	dbg.Log("[DetectActive] procs found: %d", len(procs))

	// Build cwd → []proc index for fallback path.
	cwdToProcs := make(map[string][]ProcInfo, len(procs))
	for _, p := range procs {
		cwdToProcs[p.CWD] = append(cwdToProcs[p.CWD], p)
	}

	// Build sessionID lookup map.
	byID := make(map[string]Session, len(sessions))
	for _, s := range sessions {
		byID[s.ID] = s
	}

	// Pass 1: cache-precise matches — any proc whose pid+lstart is known.
	if cache != nil {
		for _, p := range procs {
			sid := cache.Lookup(p)
			if sid == "" {
				continue
			}
			if _, ok := byID[sid]; ok {
				dbg.Log("[DetectActive] active %s via cache (pid=%s)", sid, p.PID)
				result[sid] = true
			}
		}
	}

	// Pass 2: fallback for procs with no cache entry.
	// Collect CWDs of unmapped procs.
	unmappedCWDs := make(map[string]bool)
	for _, p := range procs {
		if cache == nil || cache.Lookup(p) == "" {
			unmappedCWDs[p.CWD] = true
		}
	}

	for _, s := range sessions {
		if result[s.ID] {
			continue // already marked by cache
		}
		if !unmappedCWDs[s.CWD] {
			continue
		}
		switch s.Client {
		case ClientClaude:
			if s.jsonlPath == "" {
				dbg.Log("[DetectActive] skip %s (no jsonlPath)", s.ID)
				continue
			}
			info, err := os.Stat(s.jsonlPath)
			if err != nil {
				dbg.Log("[DetectActive] skip %s (stat error: %v)", s.ID, err)
				continue
			}
			if info.ModTime().Before(todayMidnight) {
				dbg.Log("[DetectActive] skip %s (mtime %s before midnight)", s.ID, info.ModTime().Format("15:04:05"))
				continue
			}
			dbg.Log("[DetectActive] active %s (claude fallback, cwd=%s, mtime=%s)", s.ID, s.CWD, info.ModTime().Format("15:04:05"))
			result[s.ID] = true
		case ClientOpencode:
			if s.Time.Before(todayMidnight) {
				dbg.Log("[DetectActive] skip %s (time %s before midnight)", s.ID, s.Time.Format("15:04:05"))
				continue
			}
			dbg.Log("[DetectActive] active %s (opencode fallback, cwd=%s)", s.ID, s.CWD)
			result[s.ID] = true
		}
	}
	return result
}

// lsofCWD returns the working directory of the given PID via lsof, or "".
func lsofCWD(pid string) string {
	cmd := exec.Command("lsof", "-p", pid, "-Fn")
	out, err := cmd.Output()
	// lsof exits 1 when it hits inaccessible files but still writes output;
	// only bail if there is no output at all.
	if err != nil && len(out) == 0 {
		return ""
	}
	// lsof -Fn output: lines starting with 'n' for name; cwd entry has type 'cwd'
	// With -Fn the format is: pPID\nfFD\nnNAME — we need the 'n' line after 'fcwd'
	lines := strings.Split(string(out), "\n")
	for i, l := range lines {
		if l == "fcwd" && i+1 < len(lines) {
			name := lines[i+1]
			if strings.HasPrefix(name, "n") {
				return name[1:]
			}
		}
	}
	return ""
}

// todayMidnight returns 00:00:00 of today in local time.
func todayMidnight() time.Time {
	now := time.Now()
	y, m, d := now.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, now.Location())
}
