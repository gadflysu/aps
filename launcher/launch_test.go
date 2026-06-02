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

func TestVerboseCmd(t *testing.T) {
	tests := []struct {
		customCmd string
		dir       string
		flag      string
		sessionID string
		want      string
	}{
		{"cc", "/my/dir", "--resume", "abc123", `cd "/my/dir" && cc --resume abc123`},
		{"mycode", "/my/dir", "-s", "sess-1", `cd "/my/dir" && mycode -s sess-1`},
		{"codex-cli", "/my/dir", "resume", "sess-2", `cd "/my/dir" && codex-cli resume sess-2`},
	}

	for _, tt := range tests {
		got := verboseCmd(tt.customCmd, tt.dir, tt.flag, tt.sessionID)
		if got != tt.want {
			t.Errorf("verboseCmd(%q, %q, %q, %q) = %q, want %q",
				tt.customCmd, tt.dir, tt.flag, tt.sessionID, got, tt.want)
		}
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

