package source

import "time"

type Client int

const (
	ClientClaude   Client = iota
	ClientOpencode
)

func (c Client) String() string {
	switch c {
	case ClientClaude:
		return "Claude Code"
	case ClientOpencode:
		return "OpenCode"
	default:
		return "Unknown"
	}
}

type Session struct {
	Client      Client
	ID          string    // UUID (Claude) or Opencode session ID
	Title       string
	CWD         string    // Absolute working directory
	CWDDisplay  string    // ~ abbreviated
	ProjectPath string    // Claude only: full path to project dir
	Time        time.Time // Used for sorting (newest first)
	MsgCount    int
}
