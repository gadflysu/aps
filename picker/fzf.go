package picker

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// Capabilities holds detected fzf feature flags.
type Capabilities struct {
	BorderLeft bool // fzf >= 0.27.0
	FocusEvent bool // fzf >= 0.31.0
}

// DetectCapabilities runs `fzf --version` and parses the result.
func DetectCapabilities() Capabilities {
	out, err := exec.Command("fzf", "--version").Output()
	if err != nil {
		return Capabilities{}
	}
	fields := strings.Fields(string(out))
	if len(fields) == 0 {
		return Capabilities{}
	}
	major, minor := parseVersion(fields[0])
	return Capabilities{
		BorderLeft: major > 0 || (major == 0 && minor >= 27),
		FocusEvent: major > 0 || (major == 0 && minor >= 31),
	}
}

func parseVersion(v string) (major, minor int) {
	parts := strings.Split(v, ".")
	if len(parts) >= 2 {
		major, _ = strconv.Atoi(parts[0])
		minor, _ = strconv.Atoi(parts[1])
	}
	return
}

// Result holds the parsed output of a single fzf selection.
type Result struct {
	Fields []string // TAB-split fields
}

// Config holds fzf invocation parameters.
type Config struct {
	Header       string
	PreviewCmd   string // shell command string for --preview
	WithNth      int    // which field to display (1-based)
	CWDField     int    // TAB field index (1-based) that contains the cwd, used for focus label
	Caps         Capabilities
	PreviewWidth string // e.g. "40%"
}

// Run pipes lines into fzf and returns the selected line, or "" if cancelled.
func Run(lines []string, cfg Config) (string, error) {
	previewWidth := cfg.PreviewWidth
	if previewWidth == "" {
		previewWidth = "40%"
	}
	previewWindow := fmt.Sprintf("right:%s:wrap", previewWidth)
	if cfg.Caps.BorderLeft {
		previewWindow += ":border-left"
	}

	args := []string{
		"--ansi",
		"--reverse",
		"--delimiter=\t",
		fmt.Sprintf("--with-nth=%d", cfg.WithNth),
		fmt.Sprintf("--header=%s", cfg.Header),
		"--height=90%",
		"--border",
		fmt.Sprintf("--preview=%s", cfg.PreviewCmd),
		fmt.Sprintf("--preview-window=%s", previewWindow),
	}
	if cfg.Caps.FocusEvent && cfg.CWDField > 0 {
		// Show cwd in preview label on focus; field index is mode-dependent
		bind := fmt.Sprintf("focus:transform-preview-label:echo \" [ {%d} ] \"", cfg.CWDField)
		args = append(args, "--bind", bind)
	}

	cmd := exec.Command("fzf", args...)
	cmd.Stdin = strings.NewReader(strings.Join(lines, "\n"))

	out, err := cmd.Output()
	if err != nil {
		// Exit code 130 = user cancelled with Ctrl-C / Escape
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 130 {
			return "", nil
		}
		return "", err
	}
	return strings.TrimRight(string(out), "\n"), nil
}

// Parse splits a fzf output line into its TAB-delimited fields.
func Parse(line string) []string {
	return strings.Split(line, "\t")
}
