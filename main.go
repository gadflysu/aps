package main

import (
	"fmt"
	"os"
	"sort"

	"local/aps/cmd"
	"local/aps/display"
	"local/aps/launcher"
	"local/aps/picker"
	"local/aps/preview"
	"local/aps/source"
)

func main() {
	cfg := cmd.Parse(os.Args[1:])

	// Internal preview subcommands (invoked by fzf --preview)
	if cfg.PreviewMode != "" {
		runPreview(cfg)
		return
	}

	// Load sessions
	sessions, err := loadSessions(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading sessions: %v\n", err)
		os.Exit(1)
	}
	if len(sessions) == 0 {
		fmt.Fprintln(os.Stderr, "No sessions found.")
		os.Exit(0)
	}

	if cfg.ListOnly {
		runList(sessions, cfg)
		return
	}

	runInteractive(sessions, cfg)
}

// runPreview handles internal preview subcommands invoked by fzf --preview.
//   --_preview-claude  <id> <project_path> <cwd>
//   --_preview-opencode <id> <directory>
//   --_preview-all     <source> <id> <project_path> <cwd>
func runPreview(cfg cmd.Config) {
	switch cfg.PreviewMode {
	case "claude":
		if len(cfg.PreviewArgs) < 3 {
			return
		}
		preview.Claude(cfg.PreviewArgs[0], cfg.PreviewArgs[1], cfg.PreviewArgs[2])
	case "opencode":
		if len(cfg.PreviewArgs) < 2 {
			return
		}
		preview.Opencode(cfg.PreviewArgs[0], cfg.PreviewArgs[1])
	case "all":
		if len(cfg.PreviewArgs) < 4 {
			return
		}
		src, id, projectPath, cwd := cfg.PreviewArgs[0], cfg.PreviewArgs[1], cfg.PreviewArgs[2], cfg.PreviewArgs[3]
		if src == "Claude Code" {
			preview.Claude(id, projectPath, cwd)
		} else {
			preview.Opencode(id, cwd)
		}
	}
}

func loadSessions(cfg cmd.Config) ([]source.Session, error) {
	strictMatch := !cfg.Recursive
	var all []source.Session

	if cfg.Claude {
		sessions, err := source.LoadClaude(cfg.PathFilter, strictMatch, cfg.Verbose)
		if err != nil && cfg.Verbose {
			fmt.Fprintf(os.Stderr, "claude: %v\n", err)
		}
		all = append(all, sessions...)
	}

	if cfg.Opencode {
		sessions, err := source.LoadOpencode(cfg.PathFilter, strictMatch, cfg.Verbose)
		if err != nil && cfg.Verbose {
			fmt.Fprintf(os.Stderr, "opencode: %v\n", err)
		}
		all = append(all, sessions...)
	}

	// Merge sort by time descending
	sort.Slice(all, func(i, j int) bool {
		return all[i].Time.After(all[j].Time)
	})

	return all, nil
}

func runList(sessions []source.Session, cfg cmd.Config) {
	combined := cfg.Claude && cfg.Opencode

	titles := make([]string, len(sessions))
	for i, s := range sessions {
		titles[i] = s.Title
	}
	titleWidth := display.AdaptiveTitleWidth(titles)

	fmt.Println(display.Header(titleWidth, combined))
	for _, s := range sessions {
		fmt.Println(display.FormatListRow(s, titleWidth, combined))
	}
}

func runInteractive(sessions []source.Session, cfg cmd.Config) {
	combined := cfg.Claude && cfg.Opencode
	caps := picker.DetectCapabilities()

	titles := make([]string, len(sessions))
	for i, s := range sessions {
		titles[i] = s.Title
	}
	titleWidth := display.AdaptiveTitleWidth(titles)

	// Build fzf input lines and picker config
	var lines []string
	var fzfCfg picker.Config

	exe, _ := os.Executable()

	switch {
	case combined:
		for _, s := range sessions {
			lines = append(lines, display.FormatInteractiveAll(s, titleWidth))
		}
		// TAB fields: source\tid\tproject_path\tcwd\tdisplay → with-nth=5, cwd={4}
		fzfCfg = picker.Config{
			Header:     "Select Session (Enter to Launch)",
			PreviewCmd: exe + " --_preview-all {1} {2} {3} {4}",
			WithNth:    5,
			CWDField:   4,
			Caps:       caps,
		}

	case cfg.Claude:
		for _, s := range sessions {
			lines = append(lines, display.FormatInteractiveClaude(s, titleWidth))
		}
		// TAB fields: id\tproject_path\tcwd\tdisplay → with-nth=4, cwd={3}
		fzfCfg = picker.Config{
			Header:     "Select Claude Code Session (Enter to Launch)",
			PreviewCmd: exe + " --_preview-claude {1} {2} {3}",
			WithNth:    4,
			CWDField:   3,
			Caps:       caps,
		}

	default: // Opencode only
		for _, s := range sessions {
			lines = append(lines, display.FormatInteractiveOpencode(s, titleWidth))
		}
		// TAB fields: id\tcwd\tdisplay → with-nth=3, cwd={2}
		fzfCfg = picker.Config{
			Header:     "Select Opencode Session (Enter to Launch)",
			PreviewCmd: exe + " --_preview-opencode {1} {2}",
			WithNth:    3,
			CWDField:   2,
			Caps:       caps,
		}
	}

	selected, err := picker.Run(lines, fzfCfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fzf error: %v\n", err)
		os.Exit(1)
	}
	if selected == "" {
		os.Exit(0) // user cancelled
	}

	fields := picker.Parse(selected)
	launchOpts := launcher.Options{
		NoLaunch:   cfg.NoLaunch,
		Verbose:    cfg.Verbose,
		DangerMode: cfg.DangerMode,
	}

	switch {
	case combined:
		// Fields: source\tid\tproject_path\tcwd\tdisplay
		if len(fields) < 4 {
			os.Exit(1)
		}
		src, id, dir := fields[0], fields[1], fields[3]
		if !dirExists(dir) {
			fmt.Fprintf(os.Stderr, "Error: directory not found: %s\n", dir)
			os.Exit(1)
		}
		if src == "Claude Code" {
			mustLaunch(launcher.Claude(id, dir, launchOpts))
		} else {
			mustLaunch(launcher.Opencode(id, dir, launchOpts))
		}

	case cfg.Claude:
		if len(fields) < 3 {
			os.Exit(1)
		}
		id, _, dir := fields[0], fields[1], fields[2]
		if !dirExists(dir) {
			fmt.Fprintf(os.Stderr, "Error: directory not found: %s\n", dir)
			os.Exit(1)
		}
		mustLaunch(launcher.Claude(id, dir, launchOpts))

	default: // Opencode
		if len(fields) < 2 {
			os.Exit(1)
		}
		id, dir := fields[0], fields[1]
		if !dirExists(dir) {
			fmt.Fprintf(os.Stderr, "Error: directory not found: %s\n", dir)
			os.Exit(1)
		}
		mustLaunch(launcher.Opencode(id, dir, launchOpts))
	}
}

func mustLaunch(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "launch error: %v\n", err)
		os.Exit(1)
	}
}

func dirExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}
