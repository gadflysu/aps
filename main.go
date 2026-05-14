package main

import (
	"fmt"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/gadflysu/aps/cmd"
	"github.com/gadflysu/aps/dbg"
	"github.com/gadflysu/aps/display"
	"github.com/gadflysu/aps/launcher"
	"github.com/gadflysu/aps/picker"
	"github.com/gadflysu/aps/source"
)

func main() {
	cfg := cmd.Parse(os.Args[1:])

	if cfg.DebugLog != "" {
		if err := dbg.Open(cfg.DebugLog); err != nil {
			fmt.Fprintf(os.Stderr, "warning: cannot open debug log %q: %v\n", cfg.DebugLog, err)
		} else {
			defer dbg.Close()
		}
	}

	t0 := time.Now()
	sessions, err := loadSessions(cfg)
	dbg.Log("loadSessions: %v (%d sessions)", time.Since(t0), len(sessions))
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
	switch cfg.Color {
	case "always":
		os.Setenv("COLORTERM", "truecolor")
	case "never":
		os.Setenv("NO_COLOR", "1")
	// "auto": lipgloss detects TTY automatically; nothing to do
	}

	combined := cfg.Claude && cfg.Opencode
	termWidth := display.TermWidth(os.Stdout)
	w := display.ComputeListWidths(sessions, combined, termWidth)

	fmt.Println(display.Header(w))
	var prevDir string
	for _, s := range sessions {
		fmt.Println(display.FormatListRow(s, w, s.CWDDisplay == prevDir))
		prevDir = s.CWDDisplay
	}
}

func runInteractive(sessions []source.Session, cfg cmd.Config) {
	combined := cfg.Claude && cfg.Opencode

	cache := source.LoadPIDCache()

	// GC runs in background; we wait for it before exiting so cache is consistent.
	var wg sync.WaitGroup
	wg.Add(1)
	go cache.GC(&wg)

	session, err := picker.Run(sessions, combined, cache)
	wg.Wait() // block until GC finishes before returning
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
		NoLaunch:    cfg.NoLaunch,
		Verbose:     cfg.Verbose,
		DangerMode:  cfg.DangerMode,
		ClaudeCmd:   cfg.ClaudeCmd,
		OpencodeCmd: cfg.OpencodeCmd,
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
