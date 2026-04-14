package source

import (
	"os"
	"strings"
)

// abbreviateHome replaces the home directory prefix with ~.
func abbreviateHome(path, home string) string {
	if strings.HasPrefix(path, home) {
		return "~" + path[len(home):]
	}
	return path
}

// sanitize strips tabs and newlines to prevent breaking TAB-delimited output.
func sanitize(s string) string {
	s = strings.ReplaceAll(s, "\t", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}

// fileExists reports whether a path exists.
func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
