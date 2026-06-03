package launcher

import (
	"errors"
	"os/exec"
	"strings"
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

func TestRunCustomCmd_ExitZero(t *testing.T) {
	err := runCustomCmd([]string{"/bin/sh", "-c", "exit 0"})
	if err != nil {
		t.Errorf("runCustomCmd exit 0: got %v, want nil", err)
	}
}

func TestRunCustomCmd_NonZeroExit(t *testing.T) {
	err := runCustomCmd([]string{"/bin/sh", "-c", "exit 1"})
	if err == nil {
		t.Fatal("runCustomCmd exit 1: got nil, want error")
	}
	var ee *exec.ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("runCustomCmd exit 1: err is %T (%v), want *exec.ExitError", err, err)
	}
	if ee.ExitCode() != 1 {
		t.Errorf("exit code = %d, want 1", ee.ExitCode())
	}
}

func TestRunCustomCmd_Exit146_Diagnostic(t *testing.T) {
	// 146 = 128 + SIGTSTP (18 on macOS/Linux)
	err := runCustomCmd([]string{"/bin/sh", "-c", "exit 146"})
	if err == nil {
		t.Fatal("runCustomCmd exit 146: got nil, want error")
	}
	if !isCtrlZExit(err) {
		t.Errorf("isCtrlZExit(%v) = false, want true", err)
	}
	diag := ctrlZDiagnostic("zsh")
	if diag == "" {
		t.Fatal("ctrlZDiagnostic returned empty string")
	}
	if !strings.Contains(diag, "shell-init zsh") {
		t.Errorf("diagnostic does not recommend shell-init zsh: %s", diag)
	}
	if !strings.Contains(diag, ".zshrc") {
		t.Errorf("diagnostic does not mention .zshrc: %s", diag)
	}
}

func TestCtrlZDiagnostic_Bash(t *testing.T) {
	diag := ctrlZDiagnostic("bash")
	if !strings.Contains(diag, "shell-init bash") {
		t.Errorf("diagnostic does not recommend shell-init bash: %s", diag)
	}
	if !strings.Contains(diag, ".bashrc") {
		t.Errorf("diagnostic does not mention .bashrc: %s", diag)
	}
}

func TestCtrlZDiagnostic_ShFallback(t *testing.T) {
	diag := ctrlZDiagnostic("/bin/sh")
	if strings.Contains(diag, "shell-init zsh") {
		t.Errorf("diagnostic should not assume zsh for sh, got: %s", diag)
	}
	if !strings.Contains(diag, "external wrapper") {
		t.Errorf("diagnostic should suggest wrapper script for sh, got: %s", diag)
	}
}

func TestRunCustomCmd_MissingBinary(t *testing.T) {
	err := runCustomCmd([]string{"/nonexistent/binary/zzz"})
	if err == nil {
		t.Fatal("runCustomCmd missing binary: got nil, want error")
	}
}

func TestIsCtrlZExit_Non146(t *testing.T) {
	err := runCustomCmd([]string{"/bin/sh", "-c", "exit 1"})
	if isCtrlZExit(err) {
		t.Error("isCtrlZExit(exit 1) = true, want false")
	}
}

