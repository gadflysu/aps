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

func TestSessionLaunchDirPrefersLaunchDir(t *testing.T) {
	session := &source.Session{CWD: "/cwd", LaunchDir: "/launch"}
	if got := sessionLaunchDir(session); got != "/launch" {
		t.Errorf("sessionLaunchDir = %q, want /launch", got)
	}
}

func TestSessionLaunchDirFallsBackToCWD(t *testing.T) {
	session := &source.Session{CWD: "/cwd"}
	if got := sessionLaunchDir(session); got != "/cwd" {
		t.Errorf("sessionLaunchDir = %q, want /cwd", got)
	}
}

func TestMissingLaunchDirMessageMentionsExistingLastCWD(t *testing.T) {
	lastCWD := t.TempDir()
	session := &source.Session{
		CWD:       lastCWD,
		LaunchDir: "/missing/launch-dir",
	}
	msg := missingLaunchDirMessage(session, sessionLaunchDir(session))
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
