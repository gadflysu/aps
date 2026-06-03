package cmd

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// --- expandShortFlags ---

func TestExpandShortFlags_NoChange(t *testing.T) {
	args := []string{"-n", "--verbose", "somepath"}
	got := expandShortFlags(args)
	if len(got) != len(args) {
		t.Fatalf("expandShortFlags: got %v, want %v", got, args)
	}
	for i, a := range args {
		if got[i] != a {
			t.Errorf("expandShortFlags[%d] = %q, want %q", i, got[i], a)
		}
	}
}

func TestExpandShortFlags_CombinedTwo(t *testing.T) {
	got := expandShortFlags([]string{"-nv"})
	want := []string{"-n", "-v"}
	if len(got) != len(want) {
		t.Fatalf("expandShortFlags(-nv) = %v, want %v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("expandShortFlags(-nv)[%d] = %q, want %q", i, got[i], w)
		}
	}
}

func TestExpandShortFlags_CombinedThree(t *testing.T) {
	got := expandShortFlags([]string{"-nla"})
	want := []string{"-n", "-l", "-a"}
	if len(got) != len(want) {
		t.Fatalf("expandShortFlags(-nla) = %v, want %v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("expandShortFlags(-nla)[%d] = %q, want %q", i, got[i], w)
		}
	}
}

func TestExpandShortFlags_LongFlagUnchanged(t *testing.T) {
	got := expandShortFlags([]string{"--no-launch"})
	if len(got) != 1 || got[0] != "--no-launch" {
		t.Errorf("expandShortFlags(--no-launch) = %v, want [--no-launch]", got)
	}
}

func TestExpandShortFlags_Mixed(t *testing.T) {
	got := expandShortFlags([]string{"-nv", "--all", "-l"})
	want := []string{"-n", "-v", "--all", "-l"}
	if len(got) != len(want) {
		t.Fatalf("expandShortFlags mixed = %v, want %v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("[%d] = %q, want %q", i, got[i], w)
		}
	}
}

// --- Parse ---

func TestParse_DefaultsToAllWhenNoClientFlag(t *testing.T) {
	cfg := Parse([]string{})
	if !cfg.Claude {
		t.Error("Parse with no client flags should default to Claude=true")
	}
	if !cfg.Opencode {
		t.Error("Parse with no client flags should default to Opencode=true")
	}
	if !cfg.All {
		t.Error("Parse with no client flags should default to All=true")
	}
}

func TestParse_ClaudeFlag(t *testing.T) {
	cfg := Parse([]string{"-c"})
	if !cfg.Claude {
		t.Error("-c should set Claude=true")
	}
	cfg2 := Parse([]string{"--claude"})
	if !cfg2.Claude {
		t.Error("--claude should set Claude=true")
	}
}

func TestParse_OpencodeFlag(t *testing.T) {
	cfg := Parse([]string{"-o"})
	if !cfg.Opencode {
		t.Error("-o should set Opencode=true")
	}
	if cfg.Claude {
		t.Error("-o alone should not set Claude=true (it was explicitly set)")
	}
}

func TestParse_AllFlag(t *testing.T) {
	cfg := Parse([]string{"-a"})
	if !cfg.All {
		t.Error("-a should set All=true")
	}
	if !cfg.Claude || !cfg.Opencode {
		t.Error("-a should set both Claude=true and Opencode=true")
	}
}

func TestParse_NoLaunchFlag(t *testing.T) {
	cfg := Parse([]string{"-n"})
	if !cfg.NoLaunch {
		t.Error("-n should set NoLaunch=true")
	}
}

func TestParse_ListFlag(t *testing.T) {
	cfg := Parse([]string{"-l"})
	if !cfg.ListOnly {
		t.Error("-l should set ListOnly=true")
	}
}

func TestParse_VerboseFlag(t *testing.T) {
	cfg := Parse([]string{"-v"})
	if !cfg.Verbose {
		t.Error("-v should set Verbose=true")
	}
}

func TestParse_DangerFlagsRemoved(t *testing.T) {
	for _, flag := range []string{"-d", "--danger"} {
		t.Run(flag, func(t *testing.T) {
			runParseExpectExitCode(t, []string{flag}, 2, "flag provided but not defined")
		})
	}
}

func TestParse_RecursiveFlag(t *testing.T) {
	cfg := Parse([]string{"-r"})
	if !cfg.Recursive {
		t.Error("-r should set Recursive=true")
	}
}

func TestParse_PathFilterPositionalArg(t *testing.T) {
	cfg := Parse([]string{"/some/path"})
	if cfg.PathFilter != "/some/path" {
		t.Errorf("PathFilter = %q, want \"/some/path\"", cfg.PathFilter)
	}
}

func TestParse_CombinedFlags(t *testing.T) {
	cfg := Parse([]string{"-nv"})
	if !cfg.NoLaunch {
		t.Error("-nv should set NoLaunch=true")
	}
	if !cfg.Verbose {
		t.Error("-nv should set Verbose=true")
	}
}

func TestParse_ClaudeCmdFlag(t *testing.T) {
	cfg := Parse([]string{"--claude-cmd", "cc"})
	if cfg.ClaudeCmd != "cc" {
		t.Errorf("ClaudeCmd = %q, want \"cc\"", cfg.ClaudeCmd)
	}
}

func TestParse_OpencodeCmdFlag(t *testing.T) {
	cfg := Parse([]string{"-o", "--opencode-cmd", "npx opencode@1.0"})
	if cfg.OpencodeCmd != "npx opencode@1.0" {
		t.Errorf("OpencodeCmd = %q, want \"npx opencode@1.0\"", cfg.OpencodeCmd)
	}
}

func TestParse_CmdFlagDefaultAllIsAmbiguous(t *testing.T) {
	runParseExpectExit(t, []string{"--cmd", "cc"},
		"--cmd is ambiguous when multiple clients are selected")
}

func TestParse_CmdFlagSingleExplicitClaude(t *testing.T) {
	cfg := Parse([]string{"-c", "--cmd", "cc"})
	if cfg.ClaudeCmd != "cc" {
		t.Errorf("ClaudeCmd = %q, want \"cc\"", cfg.ClaudeCmd)
	}
}

// TestUsage_WritesToStderr runs usage() via subprocess and checks the output.
func TestUsage_WritesToStderr(t *testing.T) {
	if os.Getenv("TEST_USAGE_SUBPROCESS") == "1" {
		usage()
		os.Exit(0)
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestUsage_WritesToStderr")
	cmd.Env = append(os.Environ(), "TEST_USAGE_SUBPROCESS=1")
	out, err := cmd.CombinedOutput()
	var exitErr *exec.ExitError
	if err != nil && !errors.As(err, &exitErr) {
		t.Fatalf("subprocess error: %v", err)
	}
	if !strings.Contains(string(out), "Usage:") {
		t.Errorf("usage() output missing \"Usage:\": %q", string(out))
	}
	if strings.Contains(string(out), "danger") || strings.Contains(string(out), "dangerously-skip-permissions") {
		t.Errorf("usage() output should not mention removed danger mode: %q", string(out))
	}
}

func TestParse_CmdFlagSingleOpencode(t *testing.T) {
	cfg := Parse([]string{"-o", "--cmd", "npx opencode@1.0"})
	if cfg.OpencodeCmd != "npx opencode@1.0" {
		t.Errorf("OpencodeCmd = %q, want \"npx opencode@1.0\"", cfg.OpencodeCmd)
	}
}

// runParseExpectExit re-invokes the test binary via TestParseExitHelper,
// expects exit code 1 and the given stderr substring.
// Args are joined with \x01 (SOH) because null bytes cannot survive in
// environment variables on most Unix systems.
func runParseExpectExit(t *testing.T, args []string, wantStderr string) {
	t.Helper()
	runParseExpectExitCode(t, args, 1, wantStderr)
}

func runParseExpectExitCode(t *testing.T, args []string, wantCode int, wantStderr string) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=TestParseExitHelper")
	cmd.Env = append(os.Environ(),
		"TEST_PARSE_CRASH=1",
		"TEST_PARSE_ARGS="+strings.Join(args, "\x01"),
	)
	out, err := cmd.CombinedOutput()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *exec.ExitError, got %T: %v; output: %s", err, err, out)
	}
	if exitErr.ExitCode() != wantCode {
		t.Fatalf("expected exit code %d, got %d; output: %s", wantCode, exitErr.ExitCode(), out)
	}
	if !strings.Contains(string(out), wantStderr) {
		t.Errorf("stderr %q does not contain %q", string(out), wantStderr)
	}
}

// TestParseExitHelper is the subprocess entry-point (never called directly by name).
func TestParseExitHelper(t *testing.T) {
	if os.Getenv("TEST_PARSE_CRASH") != "1" {
		return
	}
	raw := os.Getenv("TEST_PARSE_ARGS")
	if raw == "" {
		return
	}
	Parse(strings.Split(raw, "\x01"))
}

// --- shell-init ---

func TestShellInit_Supported(t *testing.T) {
	for _, shell := range []string{"zsh", "bash"} {
		t.Run(shell, func(t *testing.T) {
			out, err := ShellInitOutput(shell)
			if err != nil {
				t.Fatalf("ShellInitOutput(%s): %v", shell, err)
			}
			for _, want := range []string{"aps()", "command aps", "--no-launch"} {
				if !strings.Contains(out, want) {
					t.Errorf("%s output missing %q: %s", shell, want, out)
				}
			}
		})
	}
}

func TestShellInit_Unsupported(t *testing.T) {
	_, err := ShellInitOutput("fish")
	if err == nil {
		t.Error("ShellInitOutput(fish) should return error")
	}
}

func TestShellInit_InferFromSHELL_Zsh(t *testing.T) {
	t.Setenv("SHELL", "/bin/zsh")
	out, err := ShellInitOutput("")
	if err != nil {
		t.Fatalf("ShellInitOutput('') with SHELL=/bin/zsh: %v", err)
	}
	if !strings.Contains(out, "aps()") {
		t.Errorf("inferred zsh output missing function definition")
	}
}

func TestShellInit_InferFromSHELL_Bash(t *testing.T) {
	t.Setenv("SHELL", "/usr/local/bin/bash")
	out, err := ShellInitOutput("")
	if err != nil {
		t.Fatalf("ShellInitOutput('') with SHELL=bash: %v", err)
	}
	if !strings.Contains(out, "aps()") {
		t.Errorf("inferred bash output missing function definition")
	}
}

func TestShellInit_InferFromSHELL_Unknown(t *testing.T) {
	t.Setenv("SHELL", "/bin/fish")
	_, err := ShellInitOutput("")
	if err == nil {
		t.Error("ShellInitOutput('') with SHELL=fish should return error")
	}
}

func TestShellInit_DoesNotModifyRcFiles(t *testing.T) {
	for _, shell := range []string{"zsh", "bash"} {
		out, err := ShellInitOutput(shell)
		if err != nil {
			t.Fatalf("ShellInitOutput(%s): %v", shell, err)
		}
		if strings.Contains(out, ">>") || strings.Contains(out, ".zshrc") || strings.Contains(out, ".bashrc") {
			t.Errorf("%s output should not reference rc files: %s", shell, out)
		}
	}
}

func TestParse_CmdConflictsWithClaudeCmd(t *testing.T) {
	runParseExpectExit(t, []string{"--cmd", "cc", "--claude-cmd", "cc"},
		"--cmd conflicts with --claude-cmd")
}

func TestParse_CmdConflictsWithOpencodeCmd(t *testing.T) {
	runParseExpectExit(t, []string{"-o", "--cmd", "oc", "--opencode-cmd", "oc"},
		"--cmd conflicts with --opencode-cmd")
}

func TestParse_CmdAmbiguousMultipleClients(t *testing.T) {
	runParseExpectExit(t, []string{"-a", "--cmd", "cc"},
		"--cmd is ambiguous when multiple clients are selected")
}

func TestParse_ColorDefault(t *testing.T) {
	cfg := Parse([]string{"-l"})
	if cfg.Color != "auto" {
		t.Errorf("Color default = %q, want \"auto\"", cfg.Color)
	}
}

func TestParse_ColorAlways(t *testing.T) {
	cfg := Parse([]string{"--color=always"})
	if cfg.Color != "always" {
		t.Errorf("Color = %q, want \"always\"", cfg.Color)
	}
}

func TestParse_ColorBare(t *testing.T) {
	// bare --color with no value should default to "always"
	cfg := Parse([]string{"-l", "--color"})
	if cfg.Color != "always" {
		t.Errorf("bare --color: Color = %q, want \"always\"", cfg.Color)
	}
	if !cfg.ListOnly {
		t.Error("bare --color must not consume -l as its value")
	}
}

func TestParse_ColorNever(t *testing.T) {
	cfg := Parse([]string{"--color=never"})
	if cfg.Color != "never" {
		t.Errorf("Color = %q, want \"never\"", cfg.Color)
	}
}

func TestParse_VersionFlag(t *testing.T) {
	for _, flag := range []string{"--version", "-V"} {
		t.Run(flag, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=TestParseVersionHelper")
			cmd.Env = append(os.Environ(),
				"TEST_PARSE_VERSION=1",
				"TEST_PARSE_VERSION_FLAG="+flag,
			)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("%s: expected exit 0, got %v; output: %s", flag, err, out)
			}
			if !strings.Contains(string(out), "aps") {
				t.Errorf("%s: output %q does not contain \"aps\"", flag, string(out))
			}
		})
	}
}

// TestParseVersionHelper is the subprocess entry-point for version flag tests.
func TestParseVersionHelper(t *testing.T) {
	if os.Getenv("TEST_PARSE_VERSION") != "1" {
		return
	}
	flag := os.Getenv("TEST_PARSE_VERSION_FLAG")
	if flag == "" {
		return
	}
	Parse([]string{flag})
}
