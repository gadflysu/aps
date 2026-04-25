package launcher

import (
	"testing"
)

func TestBuildShellCmd_Claude(t *testing.T) {
	shell := "/bin/zsh"
	got := buildShellCmd(shell, "cc", "--resume", "abc123")
	if len(got) != 4 {
		t.Fatalf("buildShellCmd: len=%d, want 4; got %v", len(got), got)
	}
	if got[0] != shell {
		t.Errorf("argv[0] = %q, want %q", got[0], shell)
	}
	if got[1] != "-i" {
		t.Errorf("argv[1] = %q, want \"-i\"", got[1])
	}
	if got[2] != "-c" {
		t.Errorf("argv[2] = %q, want \"-c\"", got[2])
	}
	wantScript := "cc --resume abc123"
	if got[3] != wantScript {
		t.Errorf("argv[3] = %q, want %q", got[3], wantScript)
	}
}

func TestBuildShellCmd_Opencode(t *testing.T) {
	shell := "/bin/bash"
	got := buildShellCmd(shell, "npx opencode@1.0", "-s", "sess-xyz")
	if got[3] != "npx opencode@1.0 -s sess-xyz" {
		t.Errorf("argv[3] = %q", got[3])
	}
}

func TestBuildShellCmd_DangerMode(t *testing.T) {
	shell := "/bin/zsh"
	got := buildShellCmd(shell, "cc", "--dangerously-skip-permissions --resume", "abc123")
	want := "cc --dangerously-skip-permissions --resume abc123"
	if got[3] != want {
		t.Errorf("argv[3] = %q, want %q", got[3], want)
	}
}

func TestResolveShell_EnvSet(t *testing.T) {
	t.Setenv("SHELL", "/usr/local/bin/zsh")
	if got := resolveShell(); got != "/usr/local/bin/zsh" {
		t.Errorf("resolveShell = %q, want /usr/local/bin/zsh", got)
	}
}

func TestResolveShell_Fallback(t *testing.T) {
	t.Setenv("SHELL", "")
	if got := resolveShell(); got != "/bin/sh" {
		t.Errorf("resolveShell fallback = %q, want /bin/sh", got)
	}
}

func TestVerboseOutput_CustomCmd(t *testing.T) {
	got := verboseClaudeCmd("cc", "/my/dir", "abc123", false)
	want := `cd "/my/dir" && cc --resume abc123`
	if got != want {
		t.Errorf("verboseClaudeCmd = %q, want %q", got, want)
	}
}

func TestVerboseOutput_CustomCmdDanger(t *testing.T) {
	got := verboseClaudeCmd("cc", "/my/dir", "abc123", true)
	want := `cd "/my/dir" && cc --dangerously-skip-permissions --resume abc123`
	if got != want {
		t.Errorf("verboseClaudeCmd danger = %q, want %q", got, want)
	}
}

func TestVerboseOutput_OpencodeCustomCmd(t *testing.T) {
	got := verboseOpencodeCmd("mycode", "/my/dir", "sess-1")
	want := `cd "/my/dir" && mycode -s sess-1`
	if got != want {
		t.Errorf("verboseOpencodeCmd = %q, want %q", got, want)
	}
}

func TestJoinArgs_Empty(t *testing.T) {
	got := joinArgs(nil)
	if got != "" {
		t.Errorf("joinArgs(nil) = %q, want \"\"", got)
	}
}

func TestJoinArgs_Single(t *testing.T) {
	got := joinArgs([]string{"hello"})
	if got != `"hello"` {
		t.Errorf("joinArgs single = %q, want %q", got, `"hello"`)
	}
}

func TestJoinArgs_Multiple(t *testing.T) {
	got := joinArgs([]string{"--resume", "abc 123"})
	want := `"--resume" "abc 123"`
	if got != want {
		t.Errorf("joinArgs multiple = %q, want %q", got, want)
	}
}
