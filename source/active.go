package source

import (
	"os"
	"os/exec"
	"strings"
	"time"
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

	for _, s := range sessions {
		if !processCWDs[s.CWD] {
			continue
		}
		switch s.Client {
		case ClientClaude:
			if s.jsonlPath == "" {
				continue
			}
			info, err := os.Stat(s.jsonlPath)
			if err != nil {
				continue
			}
			if info.ModTime().Before(todayMidnight) {
				continue
			}
			result[s.ID] = true
		case ClientOpencode:
			if s.Time.Before(todayMidnight) {
				continue
			}
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
		return cwds
	}

	for _, line := range strings.Split(string(out), "\n") {
		if !strings.Contains(line, "claude") && !strings.Contains(line, "opencode") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		pid := fields[1]
		cwd := lsofCWD(pid)
		if cwd != "" {
			cwds[cwd] = true
		}
	}
	return cwds
}

// lsofCWD returns the working directory of the given PID via lsof, or "".
func lsofCWD(pid string) string {
	out, err := exec.Command("lsof", "-p", pid, "-Fn").Output()
	if err != nil {
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
