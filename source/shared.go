package source

import (
	"bufio"
	"io"
	"os"
	"strings"
)

const jsonlScannerMaxToken = 4 * 1024 * 1024

// newJSONLScanner returns a bufio.Scanner with a 4 MiB token limit for JSONL files.
func newJSONLScanner(r io.Reader) *bufio.Scanner {
	s := bufio.NewScanner(r)
	s.Buffer(make([]byte, bufio.MaxScanTokenSize), jsonlScannerMaxToken)
	return s
}

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
