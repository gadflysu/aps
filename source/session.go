// Package source loads and normalises Claude and Opencode sessions; detects active sessions via PID/lsof.
package source

import "time"

type Client int

const (
	ClientClaude Client = iota
	ClientOpencode
	ClientCodex
)

func (c Client) String() string {
	switch c {
	case ClientClaude:
		return "Claude Code"
	case ClientOpencode:
		return "OpenCode"
	case ClientCodex:
		return "Codex"
	default:
		return "Unknown"
	}
}

type Session struct {
	Client      Client
	ID          string // UUID (Claude) or Opencode session ID
	Title       string
	CWD         string    // Latest working directory (display/filter)
	CWDDisplay  string    // ~ abbreviated
	LaunchDir   string    // Directory to cd into before resuming (first cwd seen; equals CWD for non-worktree sessions)
	ProjectPath string    // Claude only: full path to project dir
	Time        time.Time // Used for sorting (newest first)
	MsgCount    int
	jsonlPath   string // unexported: path to the .jsonl file, Claude only
}
