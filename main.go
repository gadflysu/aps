package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gadflysu/aps/cmd"
	"github.com/gadflysu/aps/dbg"
	"github.com/gadflysu/aps/display"
	"github.com/gadflysu/aps/filter"
	"github.com/gadflysu/aps/launcher"
	"github.com/gadflysu/aps/picker"
	"github.com/gadflysu/aps/source"
)

func main() {
	// Handle shell-init before normal flag parsing.
	if len(os.Args) >= 2 && os.Args[1] == "shell-init" {
		shell := ""
		if len(os.Args) >= 3 {
			shell = os.Args[2]
		}
		out, err := cmd.ShellInitOutput(shell)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Print(out)
		return
	}

	cfg := cmd.Parse(os.Args[1:])

	if cfg.DebugLog != "" {
		if err := dbg.Open(cfg.DebugLog); err != nil {
			fmt.Fprintf(os.Stderr, "warning: cannot open debug log %q: %v\n", cfg.DebugLog, err)
		} else {
			defer dbg.Close()
		}
	}

	from, until, err := parseDateBounds(cfg.From, cfg.Until)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	t0 := time.Now()
	sessions, statusMsg, err := loadSessions(cfg, from, until)
	dbg.Log("loadSessions: %v (%d sessions)", time.Since(t0), len(sessions))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading sessions: %v\n", err)
		os.Exit(1)
	}
	if len(sessions) == 0 && cfg.ListOnly {
		fmt.Fprintln(os.Stderr, "No sessions found.")
		os.Exit(0)
	}

	if cfg.ListOnly {
		runList(sessions, cfg)
		return
	}

	if len(sessions) == 0 {
		msg := statusMsg
		if msg == "" {
			msg = "No sessions found."
		}
		fmt.Fprintln(os.Stderr, msg)
		os.Exit(0)
	}
	runInteractive(sessions, cfg, statusMsg)
}

func loadSessions(cfg cmd.Config, from, until *time.Time) ([]source.Session, string, error) {
	strictMatch := !cfg.Recursive
	var (
		claudeSessions   []source.Session
		opencodeSessions []source.Session
		codexSessions    []source.Session
		claudeErr        error
		opencodeErr      error
		codexErr         error
		wg               sync.WaitGroup
	)

	if cfg.Claude {
		wg.Add(1)
		go func() {
			defer wg.Done()
			claudeSessions, claudeErr = source.LoadClaude(cfg.PathFilter, strictMatch, cfg.Verbose)
		}()
	}
	if cfg.Opencode {
		wg.Add(1)
		go func() {
			defer wg.Done()
			opencodeSessions, opencodeErr = source.LoadOpencode(cfg.PathFilter, strictMatch, cfg.Verbose)
		}()
	}
	if cfg.Codex {
		wg.Add(1)
		go func() {
			defer wg.Done()
			codexSessions, codexErr = source.LoadCodex(cfg.PathFilter, strictMatch, cfg.Verbose)
		}()
	}
	wg.Wait()

	if claudeErr != nil && cfg.Verbose {
		fmt.Fprintf(os.Stderr, "claude: %v\n", claudeErr)
	}
	if opencodeErr != nil && cfg.Verbose {
		fmt.Fprintf(os.Stderr, "opencode: %v\n", opencodeErr)
	}
	if codexErr != nil && cfg.Verbose {
		fmt.Fprintf(os.Stderr, "codex: %v\n", codexErr)
	}

	// Build a concise non-fatal error summary for the status bar.
	var failed []string
	if claudeErr != nil {
		failed = append(failed, "Claude")
	}
	if opencodeErr != nil {
		failed = append(failed, "Opencode")
	}
	if codexErr != nil {
		failed = append(failed, "Codex")
	}
	var statusMsg string
	if len(failed) > 0 {
		statusMsg = fmt.Sprintf("%s load failed", joinNames(failed))
		if len(failed) < cfg.SourceCount() {
			statusMsg += "; showing other sessions"
		}
	}

	all := append(claudeSessions, opencodeSessions...)
	all = append(all, codexSessions...)
	sort.Slice(all, func(i, j int) bool {
		return all[i].Time.After(all[j].Time)
	})

	if from != nil || until != nil {
		all = filterByDate(all, from, until)
	}

	return all, statusMsg, nil
}

// joinNames joins ["A", "B", "C"] into "A, B and C".
func joinNames(names []string) string {
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0]
	case 2:
		return names[0] + " and " + names[1]
	default:
		return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
	}
}

// filterByDate returns sessions whose Time falls within [from, until].
func filterByDate(sessions []source.Session, from, until *time.Time) []source.Session {
	out := sessions[:0]
	for _, s := range sessions {
		if filter.DateInRange(s.Time, from, until) {
			out = append(out, s)
		}
	}
	return out
}

// parseDateBounds parses --from and --until date expressions.
// Returns nil pointers for unbounded sides.
func parseDateBounds(fromStr, untilStr string) (*time.Time, *time.Time, error) {
	var from, until *time.Time
	if fromStr != "" {
		t, err := filter.ParseDateExpr(fromStr)
		if err != nil {
			return nil, nil, fmt.Errorf("--from: %w", err)
		}
		from = &t
	}
	if untilStr != "" {
		t, err := filter.ParseDateExpr(untilStr)
		if err != nil {
			return nil, nil, fmt.Errorf("--until: %w", err)
		}
		// For date-only expressions (midnight), extend to end of day
		// so "--until 2026-06-01" includes all of June 1st.
		if t.Hour() == 0 && t.Minute() == 0 && t.Second() == 0 {
			t = t.Add(24*time.Hour - time.Second)
		}
		until = &t
	}
	return from, until, nil
}

func runList(sessions []source.Session, cfg cmd.Config) {
	switch cfg.Color {
	case "always":
		os.Setenv("COLORTERM", "truecolor")
	case "never":
		os.Setenv("NO_COLOR", "1")
	// "auto": lipgloss detects TTY automatically; nothing to do
	}

	combined := cfg.MultiAgent()
	termWidth := display.TermWidth(os.Stdout)
	w := display.ComputeListWidths(sessions, combined, termWidth)

	fmt.Println(display.Header(w))
	var prevDir string
	for _, s := range sessions {
		fmt.Println(display.FormatListRow(s, w, s.CWDDisplay == prevDir))
		prevDir = s.CWDDisplay
	}
}

func runInteractive(sessions []source.Session, cfg cmd.Config, statusText string) {
	combined := cfg.MultiAgent()

	cache := source.LoadPIDCache()

	// GC runs in background; we wait for it before exiting so cache is consistent.
	var wg sync.WaitGroup
	wg.Add(1)
	go cache.GC(&wg)

	session, err := picker.Run(sessions, combined, cache, statusText, false)
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
		ClaudeCmd:   cfg.ClaudeCmd,
		OpencodeCmd: cfg.OpencodeCmd,
		CodexCmd:    cfg.CodexCmd,
	}

	switch session.Client {
	case source.ClientClaude:
		mustLaunch(launcher.Claude(session.ID, session.CWD, launchOpts))
	case source.ClientCodex:
		mustLaunch(launcher.Codex(session.ID, session.CWD, launchOpts))
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
