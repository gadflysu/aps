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
	PreviewMode string // internal: "--_preview-claude" or "--_preview-opencode"
	PreviewArgs []string
}

func Parse(args []string) Config {
	// Handle internal preview subcommands before flag parsing.
	if len(args) >= 1 {
		switch args[0] {
		case "--_preview-claude":
			return Config{PreviewMode: "claude", PreviewArgs: args[1:]}
		case "--_preview-opencode":
			return Config{PreviewMode: "opencode", PreviewArgs: args[1:]}
		case "--_preview-all":
			// args: <source> <id> <project_path> <cwd>
			return Config{PreviewMode: "all", PreviewArgs: args[1:]}
		}
	}

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

	// Support combined short flags like -nv, -la, -lo
	expanded := expandShortFlags(args)
	_ = fs.Parse(expanded)

	if showHelp {
		usage()
		os.Exit(0)
	}

	// Default: Claude if no client specified
	if !cfg.Claude && !cfg.Opencode && !cfg.All {
		cfg.Claude = true
	}
	if cfg.All {
		cfg.Claude = true
		cfg.Opencode = true
	}

	// First positional arg is PATH_FILTER
	if fs.NArg() > 0 {
		cfg.PathFilter = fs.Arg(0)
	}

	// Normalize '.' to cwd
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
			// Combined short flags
			for _, c := range a[1:] {
				out = append(out, "-"+string(c))
			}
		} else {
			out = append(out, a)
		}
	}
	return out
}

func usage() {
	fmt.Fprintf(os.Stderr, `Usage: aps [OPTIONS] [PATH_FILTER]

Interactive session picker for Claude Code and Opencode.

Options:
  -n, --no-launch    Print target directory instead of launching client
  -v, --verbose      With -n: print full launch command
  -l, --list         Non-interactive table output and exit
  -c, --claude       Include Claude Code sessions (default if no client flag)
  -o, --opencode     Include Opencode sessions
  -a, --all          Include both clients
  -d, --danger       Claude: launch with --dangerously-skip-permissions
  -r, --recursive    Looser path filter (substring match)
  -h, --help         Show this help

Arguments:
  PATH_FILTER        Filter sessions by directory path. Use '.' for cwd.

Examples:
  aps               Interactive pick (Claude sessions, cwd filter default)
  aps -l .          List mode, current directory
  aps -l scripts    List mode, substring filter
  aps -r -l foo     Recursive substring match
  aps -c            Claude Code only
  aps -o            Opencode only
  aps -a            Both clients combined
  aps -n            No-launch: print target directory
  aps -nv           No-launch verbose: print full command
  aps -d            Danger mode (--dangerously-skip-permissions)
`)
}
