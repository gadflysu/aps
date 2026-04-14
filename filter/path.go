package filter

import (
	"os"
	"path/filepath"
	"strings"
)

// Matches reports whether cwd should be included given pathFilter and strictMatch.
//
// Algorithm (evaluated in order, stop at first match):
//  1. Resolve pathFilter to absolute path.
//  2. If the resolved path exists:
//     a. Exact match: resolved == cwd
//     b. Symlink match: realpath(resolved) == realpath(cwd)
//     c. Resolved substring (non-strict only, min len 10): resolved in cwd or in realpath(cwd)
//  3. Raw substring fallback (min len 3): pathFilter in cwd
//     - Allowed in non-strict mode always.
//     - Allowed in strict mode only when resolved path does NOT exist on disk.
//
// If pathFilter is empty, all sessions match.
func Matches(pathFilter string, strictMatch bool, cwd string) bool {
	if pathFilter == "" {
		return true
	}

	resolved, err := filepath.EvalSymlinks(expandHome(pathFilter))
	if err != nil {
		// Resolution failed — fall through to raw substring
		resolved = ""
	}
	pathExists := resolved != "" && fileExists(resolved)

	if pathExists {
		// 1a. Exact
		if resolved == cwd {
			return true
		}

		// 1b. Symlink-resolved
		resolvedCWD, err := filepath.EvalSymlinks(cwd)
		if err != nil {
			resolvedCWD = cwd
		}
		if resolved == resolvedCWD {
			return true
		}

		// 1c. Resolved substring (non-strict only, meaningful length)
		if !strictMatch && len(resolved) > 10 {
			if strings.Contains(cwd, resolved) || strings.Contains(resolvedCWD, resolved) {
				return true
			}
		}
	}

	// 2. Raw substring fallback
	if len(pathFilter) > 2 && strings.Contains(cwd, pathFilter) {
		if !strictMatch || !pathExists {
			return true
		}
	}

	return false
}

func expandHome(p string) string {
	if strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return home + p[1:]
		}
	}
	if p == "~" {
		home, err := os.UserHomeDir()
		if err == nil {
			return home
		}
	}
	return p
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
