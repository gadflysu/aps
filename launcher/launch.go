// Package launcher replaces the current process with the selected Claude or Opencode session via syscall.Exec.
package launcher

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

// Options controls launch behavior.
type Options struct {
	NoLaunch    bool
	Verbose     bool
	ClaudeCmd   string
	OpencodeCmd string
	CodexCmd    string
}

// Claude changes to dir and execs `claude --resume sessionID`.
func Claude(sessionID, dir string, opts Options) error {
	return launchAgent("claude", "--resume", opts.ClaudeCmd, sessionID, dir, opts)
}

// Opencode changes to dir and execs `opencode -s sessionID`.
func Opencode(sessionID, dir string, opts Options) error {
	return launchAgent("opencode", "-s", opts.OpencodeCmd, sessionID, dir, opts)
}

// fallbackShell execs the user's default shell when the agent binary is missing.
func fallbackShell() error {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/bash"
	}
	shellPath, err := exec.LookPath(shell)
	if err != nil {
		return fmt.Errorf("agent not found and shell %s not found: %w", shell, err)
	}
	fmt.Fprintf(os.Stderr, "Agent not found in PATH. Falling back to %s\n", shellPath)
	return syscall.Exec(shellPath, []string{shellPath}, os.Environ())
}

func joinArgs(args []string) string {
	result := ""
	for i, a := range args {
		if i > 0 {
			result += " "
		}
		result += fmt.Sprintf("%q", a)
	}
	return result
}

func resolveShell() string {
	if s := os.Getenv("SHELL"); s != "" {
		return s
	}
	return "/bin/sh"
}

// buildShellCmd returns argv for: $SHELL -i -c "<customCmd> <sessionFlag> <sessionID>"
func buildShellCmd(shell, customCmd, sessionFlag, sessionID string) []string {
	script := customCmd + " " + sessionFlag + " " + sessionID
	return []string{shell, "-i", "-c", script}
}

// ctrlZExitCode is the exit status when zsh -i -c is stopped by SIGTSTP (128 + 18).
const ctrlZExitCode = 146

// runCustomCmd runs argv as a child process with stdin/stdout/stderr attached.
// Returns the child's exit error so callers can inspect the exit code.
func runCustomCmd(argv []string) error {
	if len(argv) == 0 {
		return fmt.Errorf("runCustomCmd: empty argv")
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	return cmd.Run()
}

// isCtrlZExit reports whether err is an exit from a SIGTSTP-stopped intermediate shell.
func isCtrlZExit(err error) bool {
	var ee *exec.ExitError
	if !errors.As(err, &ee) {
		return false
	}
	return ee.ExitCode() == ctrlZExitCode
}

// ctrlZDiagnostic returns a user-facing explanation when the intermediate shell
// was killed by SIGTSTP and fg cannot recover the session.
func ctrlZDiagnostic(shell string) string {
	shellName := filepath.Base(shell)
	if shellName == "" || shellName == "." {
		shellName = "zsh"
	}
	return fmt.Sprintf(
		"aps: custom command shell stopped and exited; the launched job cannot be recovered with fg.\n"+
			"aps: enable shell integration: eval \"$(aps shell-init %s)\"\n"+
			"aps: add permanently: echo 'eval \"$(aps shell-init %s)\"' >> ~/.%src\n"+
			"aps: or use an external wrapper script for --*-cmd instead of a shell alias/function.\n"+
			"aps: note: using %s from $SHELL; if you are in a different shell, run: aps shell-init <your-shell>",
		shellName, shellName, shellName, shell)
}

// Codex changes to dir and execs `codex resume sessionID`.
func Codex(sessionID, dir string, opts Options) error {
	return launchAgent("codex", "resume", opts.CodexCmd, sessionID, dir, opts)
}

// launchAgent is the generic launcher implementation.
func launchAgent(binary, resumeFlag, customCmd, sessionID, dir string, opts Options) error {
	if opts.NoLaunch {
		if opts.Verbose {
			if customCmd != "" {
				fmt.Println(verboseCmd(customCmd, dir, resumeFlag, sessionID))
			} else {
				fmt.Printf("cd %q && %s %s %s\n", dir, binary, resumeFlag, sessionID)
			}
		} else {
			fmt.Println(dir)
		}
		return nil
	}

	fmt.Printf("Resuming %s session: %s\n", binary, sessionID)
	fmt.Printf("Directory: %s\n", dir)

	if err := os.Chdir(dir); err != nil {
		return fmt.Errorf("chdir %s: %w", dir, err)
	}

	if customCmd != "" {
		shell := resolveShell()
		argv := buildShellCmd(shell, customCmd, resumeFlag, sessionID)
		err := runCustomCmd(argv)
		if isCtrlZExit(err) {
			fmt.Fprintln(os.Stderr, ctrlZDiagnostic(shell))
			return err
		}
		return err
	}

	agentPath, err := exec.LookPath(binary)
	if err != nil {
		return fallbackShell()
	}
	return syscall.Exec(agentPath, []string{binary, resumeFlag, sessionID}, os.Environ())
}

func verboseCmd(customCmd, dir, sessionFlag, sessionID string) string {
	return fmt.Sprintf("cd %q && %s %s %s", dir, customCmd, sessionFlag, sessionID)
}
