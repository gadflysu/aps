// Package cmd parses command-line flags and dispatches to the appropriate mode.
package cmd

import (
	"flag"
	"fmt"
	"os"
	"runtime/debug"
)

// Version is set at build time via -ldflags; falls back to a vcs-derived string.
var Version = devVersion()

func devVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "dev"
	}
	for _, s := range info.Settings {
		if s.Key == "vcs.revision" && len(s.Value) >= 7 {
			return "dev-" + s.Value[:7]
		}
	}
	return "dev"
}

// Config holds all parsed CLI state.
type Config struct {
	NoLaunch    bool
	Verbose     bool
	ListOnly    bool
	Claude      bool
	Opencode    bool
	Codex       bool
	All         bool
	Recursive   bool
	PathFilter  string
	From        string // date expression for lower bound (empty = unbounded)
	Until       string // date expression for upper bound (empty = unbounded)
	ClaudeCmd   string
	OpencodeCmd string
	CodexCmd    string
	Color       string // "auto" | "always" | "never"
	DebugLog    string // path to debug log file; empty = disabled
}

// MultiAgent returns true if 2 or more agents are active.
func (c Config) MultiAgent() bool {
	count := 0
	if c.Claude {
		count++
	}
	if c.Opencode {
		count++
	}
	if c.Codex {
		count++
	}
	return count > 1
}

func Parse(args []string) Config {
	fs := flag.NewFlagSet("aps", flag.ExitOnError)
	fs.Usage = usage

	var cfg Config
	var showHelp, showVersion bool

	fs.BoolVar(&cfg.NoLaunch, "n", false, "")
	fs.BoolVar(&showVersion, "V", false, "")
	fs.BoolVar(&showVersion, "version", false, "")
	fs.BoolVar(&cfg.NoLaunch, "no-launch", false, "")
	fs.BoolVar(&cfg.Verbose, "v", false, "")
	fs.BoolVar(&cfg.Verbose, "verbose", false, "")
	fs.BoolVar(&cfg.ListOnly, "l", false, "")
	fs.BoolVar(&cfg.ListOnly, "list", false, "")
	fs.BoolVar(&cfg.Claude, "c", false, "")
	fs.BoolVar(&cfg.Claude, "claude", false, "")
	fs.BoolVar(&cfg.Opencode, "o", false, "")
	fs.BoolVar(&cfg.Opencode, "opencode", false, "")
	fs.BoolVar(&cfg.Codex, "x", false, "")
	fs.BoolVar(&cfg.Codex, "codex", false, "")
	fs.BoolVar(&cfg.All, "a", false, "")
	fs.BoolVar(&cfg.All, "all", false, "")
	fs.BoolVar(&cfg.Recursive, "r", false, "")
	fs.BoolVar(&cfg.Recursive, "recursive", false, "")
	fs.BoolVar(&showHelp, "h", false, "")
	fs.BoolVar(&showHelp, "help", false, "")

	var rawCmd, rawClaudeCmd, rawOpencodeCmd, rawCodexCmd string
	fs.StringVar(&cfg.From, "from", "", "")
	fs.StringVar(&cfg.Until, "until", "", "")
	fs.StringVar(&rawClaudeCmd, "claude-cmd", "", "")
	fs.StringVar(&rawOpencodeCmd, "opencode-cmd", "", "")
	fs.StringVar(&rawCodexCmd, "codex-cmd", "", "")
	fs.StringVar(&rawCmd, "cmd", "", "")
	fs.StringVar(&cfg.Color, "color", "auto", "")
	fs.StringVar(&cfg.DebugLog, "debug-log", "", "")

	expanded := expandShortFlags(args)
	expanded = expandBareColor(expanded)
	_ = fs.Parse(expanded)

	if showVersion {
		fmt.Fprintf(os.Stdout, "aps %s\n", Version)
		os.Exit(0)
	}

	if showHelp {
		usage()
		os.Exit(0)
	}

	if !cfg.Claude && !cfg.Opencode && !cfg.Codex && !cfg.All {
		cfg.All = true
		cfg.Claude = true
		cfg.Opencode = true
		cfg.Codex = true
	}
	if cfg.All {
		cfg.Claude = true
		cfg.Opencode = true
		cfg.Codex = true
	}

	// conflict: --cmd with --claude-cmd, --opencode-cmd, or --codex-cmd
	if rawCmd != "" && rawClaudeCmd != "" {
		fmt.Fprintln(os.Stderr, "error: --cmd conflicts with --claude-cmd")
		os.Exit(1)
	}
	if rawCmd != "" && rawOpencodeCmd != "" {
		fmt.Fprintln(os.Stderr, "error: --cmd conflicts with --opencode-cmd")
		os.Exit(1)
	}
	if rawCmd != "" && rawCodexCmd != "" {
		fmt.Fprintln(os.Stderr, "error: --cmd conflicts with --codex-cmd")
		os.Exit(1)
	}
	// conflict: --cmd with multiple clients
	activeClients := 0
	if cfg.Claude {
		activeClients++
	}
	if cfg.Opencode {
		activeClients++
	}
	if cfg.Codex {
		activeClients++
	}
	if rawCmd != "" && activeClients > 1 {
		fmt.Fprintln(os.Stderr, "error: --cmd is ambiguous when multiple clients are selected; use --claude-cmd, --opencode-cmd, or --codex-cmd")
		os.Exit(1)
	}
	// resolve --cmd into the active client's field
	if rawCmd != "" {
		if cfg.Claude {
			cfg.ClaudeCmd = rawCmd
		} else if cfg.Opencode {
			cfg.OpencodeCmd = rawCmd
		} else {
			cfg.CodexCmd = rawCmd
		}
	}
	cfg.ClaudeCmd = firstNonEmpty(cfg.ClaudeCmd, rawClaudeCmd)
	cfg.OpencodeCmd = firstNonEmpty(cfg.OpencodeCmd, rawOpencodeCmd)
	cfg.CodexCmd = firstNonEmpty(cfg.CodexCmd, rawCodexCmd)

	if fs.NArg() > 0 {
		cfg.PathFilter = fs.Arg(0)
	}

	if cfg.PathFilter == "." {
		if cwd, err := os.Getwd(); err == nil {
			cfg.PathFilter = cwd
		}
	}

	return cfg
}

// expandBareColor rewrites a bare "--color" (no value) to "--color=always"
// so that flag.StringVar can parse it without consuming the next argument.
func expandBareColor(args []string) []string {
	out := make([]string, 0, len(args))
	for _, a := range args {
		if a == "--color" || a == "-color" {
			out = append(out, "--color=always")
		} else {
			out = append(out, a)
		}
	}
	return out
}

// expandShortFlags splits combined short flags like -nv into -n -v.
func expandShortFlags(args []string) []string {
	var out []string
	for _, a := range args {
		if len(a) > 2 && a[0] == '-' && a[1] != '-' {
			for _, c := range a[1:] {
				out = append(out, "-"+string(c))
			}
		} else {
			out = append(out, a)
		}
	}
	return out
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func usage() {
	fmt.Fprintf(os.Stderr, `Usage: aps [OPTIONS] [PATH_FILTER]

Interactive session picker for Claude Code, Opencode, and Codex.

Options:
  -n, --no-launch       Print target directory instead of launching the agent
  -v, --verbose         With -n: print full launch command
  -l, --list            Non-interactive table output and exit
  -c, --claude          Include Claude Code sessions
  -o, --opencode        Include Opencode sessions
  -x, --codex           Include Codex sessions
  -a, --all             Include all agents (default if no agent flag)
  -r, --recursive       Looser path filter (substring match)
      --from DATE       Include sessions from DATE onward (inclusive)
      --until DATE      Include sessions up to DATE (inclusive)
      --claude-cmd STR  Override command used to launch Claude Code
      --opencode-cmd STR  Override command used to launch Opencode
      --codex-cmd STR   Override command used to launch Codex
      --cmd STR         Override command for the single active agent
      --color MODE      Color output: auto (default), always, never
      --debug-log FILE  Append debug log to FILE (active detection, cache ops)
  -V, --version         Print version and exit
  -h, --help            Show this help

Date formats: YYYY-MM-DD, YYYY-MM-DD HH:MM, today, yesterday, N days/weeks/months ago

Arguments:
  PATH_FILTER           Filter sessions by directory path. Use '.' for cwd.

Examples:
  aps                         Interactive pick (all agents, cwd filter default)
  aps -l .                    List mode, current directory
  aps --from today -l         List today's sessions
  aps --from "3 days ago"     Pick from recent sessions only
  aps --from 2026-06-01 --until 2026-06-30 -l   Sessions in June
  aps -c --claude-cmd "npx claude@2.1"   Use specific Claude version
  aps -c --cmd cc             Use 'cc' alias (single agent active)
  aps -o --cmd "npx opencode@1.0"  Use specific Opencode version
  aps -x --codex-cmd "codex-cli"  Use specific Codex version
`)
}
