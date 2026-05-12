package source

import (
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/gadflysu/aps/dbg"
)

// DetectActive returns a set of session IDs that are currently active.
// A session is active when its CWD matches a running claude/opencode process CWD
// AND its last-activity timestamp is today (>= local midnight).
// Errors from ps/lsof are silently ignored — callers get an empty map on failure.
func DetectActive(sessions []Session) map[string]bool {
	result := make(map[string]bool)
	if len(sessions) == 0 {
		return result
	}

	processCWDs := collectProcessCWDs()
	todayMidnight := todayMidnight()

	dbg.Log("[DetectActive] process CWDs found: %d", len(processCWDs))
	for cwd := range processCWDs {
		dbg.Log("[DetectActive]   cwd: %s", cwd)
	}

	for _, s := range sessions {
		if !processCWDs[s.CWD] {
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
			dbg.Log("[DetectActive] active %s (claude, cwd=%s, mtime=%s)", s.ID, s.CWD, info.ModTime().Format("15:04:05"))
			result[s.ID] = true
		case ClientOpencode:
			if s.Time.Before(todayMidnight) {
				dbg.Log("[DetectActive] skip %s (time %s before midnight)", s.ID, s.Time.Format("15:04:05"))
				continue
			}
			dbg.Log("[DetectActive] active %s (opencode, cwd=%s)", s.ID, s.CWD)
			result[s.ID] = true
		}
	}
	return result
}

// collectProcessCWDs returns the set of working directories of all running
// claude and opencode processes. Errors are silently ignored.
func collectProcessCWDs() map[string]bool {
	cwds := make(map[string]bool)

	out, err := exec.Command("ps", "aux").Output()
	if err != nil {
		dbg.Log("[collectProcessCWDs] ps aux error: %v", err)
		return cwds
	}

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
		cwd := lsofCWD(pid)
		dbg.Log("[collectProcessCWDs] pid=%s cmd=%s cwd=%q", pid, base, cwd)
		if cwd != "" {
			cwds[cwd] = true
		}
	}
	return cwds
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
