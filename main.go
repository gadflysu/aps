package main

import (
	"fmt"
	"os"
	"sort"

	"local/aps/cmd"
	"local/aps/display"
	"local/aps/launcher"
	"local/aps/picker"
	"local/aps/source"
)

func main() {
	cfg := cmd.Parse(os.Args[1:])

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

	sort.Slice(all, func(i, j int) bool {
		return all[i].Time.After(all[j].Time)
	})

	return all, nil
}

func runList(sessions []source.Session, cfg cmd.Config) {
	combined := cfg.Claude && cfg.Opencode
	termWidth := display.TermWidth(os.Stdout)
	w := display.ComputeListWidths(sessions, combined, termWidth)

	fmt.Println(display.Header(w))
	for _, s := range sessions {
		fmt.Println(display.FormatListRow(s, w))
	}
}

func runInteractive(sessions []source.Session, cfg cmd.Config) {
	combined := cfg.Claude && cfg.Opencode

	session, err := picker.Run(sessions, combined)
	if err != nil {
		fmt.Fprintf(os.Stderr, "picker error: %v\n", err)
		os.Exit(1)
	}
	if session == nil {
		os.Exit(0) // user cancelled
	}

	if !dirExists(session.CWD) {
		fmt.Fprintf(os.Stderr, "Error: directory not found: %s\n", session.CWD)
		os.Exit(1)
	}

	launchOpts := launcher.Options{
		NoLaunch:   cfg.NoLaunch,
		Verbose:    cfg.Verbose,
		DangerMode: cfg.DangerMode,
	}

	switch session.Client {
	case source.ClientClaude:
		mustLaunch(launcher.Claude(session.ID, session.CWD, launchOpts))
	default:
		mustLaunch(launcher.Opencode(session.ID, session.CWD, launchOpts))
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
