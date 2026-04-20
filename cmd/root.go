package cmd

import (
	"flag"
	"fmt"
	"os"
)

// Config holds all parsed CLI state.
type Config struct {
	NoLaunch    bool
	Verbose     bool
	ListOnly    bool
	Claude      bool
	Opencode    bool
	All         bool
	DangerMode  bool
	Recursive   bool
	PathFilter  string
	ClaudeCmd   string
	OpencodeCmd string
}

func Parse(args []string) Config {
	fs := flag.NewFlagSet("aps", flag.ExitOnError)
	fs.Usage = usage

	var cfg Config
	var showHelp bool

	fs.BoolVar(&cfg.NoLaunch, "n", false, "")
	fs.BoolVar(&cfg.NoLaunch, "no-launch", false, "")
	fs.BoolVar(&cfg.Verbose, "v", false, "")
	fs.BoolVar(&cfg.Verbose, "verbose", false, "")
	fs.BoolVar(&cfg.ListOnly, "l", false, "")
	fs.BoolVar(&cfg.ListOnly, "list", false, "")
	fs.BoolVar(&cfg.Claude, "c", false, "")
	fs.BoolVar(&cfg.Claude, "claude", false, "")
	fs.BoolVar(&cfg.Opencode, "o", false, "")
	fs.BoolVar(&cfg.Opencode, "opencode", false, "")
	fs.BoolVar(&cfg.All, "a", false, "")
	fs.BoolVar(&cfg.All, "all", false, "")
	fs.BoolVar(&cfg.DangerMode, "d", false, "")
	fs.BoolVar(&cfg.DangerMode, "danger", false, "")
	fs.BoolVar(&cfg.Recursive, "r", false, "")
	fs.BoolVar(&cfg.Recursive, "recursive", false, "")
	fs.BoolVar(&showHelp, "h", false, "")
	fs.BoolVar(&showHelp, "help", false, "")

	var rawCmd, rawClaudeCmd, rawOpencodeCmd string
	fs.StringVar(&rawClaudeCmd, "claude-cmd", "", "")
	fs.StringVar(&rawOpencodeCmd, "opencode-cmd", "", "")
	fs.StringVar(&rawCmd, "cmd", "", "")

	expanded := expandShortFlags(args)
	_ = fs.Parse(expanded)

	if showHelp {
		usage()
		os.Exit(0)
	}

	if !cfg.Claude && !cfg.Opencode && !cfg.All {
		cfg.Claude = true
	}
	if cfg.All {
		cfg.Claude = true
		cfg.Opencode = true
	}

	// conflict: --cmd with --claude-cmd or --opencode-cmd
	if rawCmd != "" && rawClaudeCmd != "" {
		fmt.Fprintln(os.Stderr, "error: --cmd conflicts with --claude-cmd")
		os.Exit(1)
	}
	if rawCmd != "" && rawOpencodeCmd != "" {
		fmt.Fprintln(os.Stderr, "error: --cmd conflicts with --opencode-cmd")
		os.Exit(1)
	}
	// conflict: --cmd with multiple clients
	if rawCmd != "" && cfg.Claude && cfg.Opencode {
		fmt.Fprintln(os.Stderr, "error: --cmd is ambiguous when multiple clients are selected; use --claude-cmd or --opencode-cmd")
		os.Exit(1)
	}
	// resolve --cmd into the active client's field
	if rawCmd != "" {
		if cfg.Claude {
			cfg.ClaudeCmd = rawCmd
		} else {
			cfg.OpencodeCmd = rawCmd
		}
	}
	cfg.ClaudeCmd = firstNonEmpty(cfg.ClaudeCmd, rawClaudeCmd)
	cfg.OpencodeCmd = firstNonEmpty(cfg.OpencodeCmd, rawOpencodeCmd)

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

Interactive session picker for Claude Code and Opencode.

Options:
  -n, --no-launch       Print target directory instead of launching client
  -v, --verbose         With -n: print full launch command
  -l, --list            Non-interactive table output and exit
  -c, --claude          Include Claude Code sessions (default if no client flag)
  -o, --opencode        Include Opencode sessions
  -a, --all             Include both clients
  -d, --danger          Claude: launch with --dangerously-skip-permissions
  -r, --recursive       Looser path filter (substring match)
      --claude-cmd STR  Override command used to launch Claude Code
      --opencode-cmd STR Override command used to launch Opencode
      --cmd STR         Override command for the single active client
  -h, --help            Show this help

Arguments:
  PATH_FILTER           Filter sessions by directory path. Use '.' for cwd.

Examples:
  aps                         Interactive pick (Claude sessions, cwd filter default)
  aps -l .                    List mode, current directory
  aps -d                      Danger mode (--dangerously-skip-permissions)
  aps --claude-cmd "npx claude@2.1"   Use specific Claude version
  aps --cmd cc                Use 'cc' alias (single client active)
  aps -o --cmd "npx opencode@1.0"  Use specific Opencode version
`)
}
