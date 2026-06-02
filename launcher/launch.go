// Package launcher replaces the current process with the selected Claude or Opencode session via syscall.Exec.
package launcher

import (
	"fmt"
	"os"
	"os/exec"
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
		return syscall.Exec(shell, argv, os.Environ())
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
