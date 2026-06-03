package main

import "testing"

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
