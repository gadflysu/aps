package main

import (
	"strings"
	"testing"

	"github.com/gadflysu/aps/source"
)

func TestJoinNames_Empty(t *testing.T) {
	if got := joinNames(nil); got != "" {
		t.Errorf("joinNames(nil) = %q, want \"\"", got)
	}
}

func TestJoinNames_One(t *testing.T) {
	if got := joinNames([]string{"Claude"}); got != "Claude" {
		t.Errorf("joinNames([Claude]) = %q, want \"Claude\"", got)
	}
}

func TestJoinNames_Two(t *testing.T) {
	if got := joinNames([]string{"Claude", "Opencode"}); got != "Claude and Opencode" {
		t.Errorf("joinNames = %q, want \"Claude and Opencode\"", got)
	}
}

func TestJoinNames_Three(t *testing.T) {
	if got := joinNames([]string{"Claude", "Opencode", "Codex"}); got != "Claude, Opencode and Codex" {
		t.Errorf("joinNames = %q, want \"Claude, Opencode and Codex\"", got)
	}
}

func TestSessionLaunchCWDPrefersLaunchCWD(t *testing.T) {
	session := &source.Session{CWD: "/cwd", LaunchCWD: "/launch"}
	if got := sessionLaunchCWD(session); got != "/launch" {
		t.Errorf("sessionLaunchCWD = %q, want /launch", got)
	}
}

func TestSessionLaunchCWDFallsBackToCWD(t *testing.T) {
	session := &source.Session{CWD: "/cwd"}
	if got := sessionLaunchCWD(session); got != "/cwd" {
		t.Errorf("sessionLaunchCWD = %q, want /cwd", got)
	}
}

func TestMissingLaunchCWDMessageMentionsExistingLastCWD(t *testing.T) {
	lastCWD := t.TempDir()
	session := &source.Session{
		CWD:       lastCWD,
		LaunchCWD: "/missing/launch-dir",
	}
	msg := missingLaunchCWDMessage(session, sessionLaunchCWD(session))
	if !strings.Contains(msg, "launch directory not found: /missing/launch-dir") {
		t.Errorf("missing message should name launch dir, got %q", msg)
	}
	if !strings.Contains(msg, "Last session directory exists") {
		t.Errorf("missing message should mention existing last cwd, got %q", msg)
	}
	if !strings.Contains(msg, lastCWD) {
		t.Errorf("missing message should include last cwd %q, got %q", lastCWD, msg)
	}
}
