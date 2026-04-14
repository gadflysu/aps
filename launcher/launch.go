package launcher

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

// Options controls launch behavior.
type Options struct {
	NoLaunch   bool
	Verbose    bool
	DangerMode bool // Claude only
}

// Claude changes to dir and execs `claude --resume sessionID`.
func Claude(sessionID, dir string, opts Options) error {
	args := []string{"--resume", sessionID}
	if opts.DangerMode {
		args = []string{"--dangerously-skip-permissions", "--resume", sessionID}
	}

	if opts.NoLaunch {
		if opts.Verbose {
			fmt.Printf("cd %q && claude %s\n", dir, joinArgs(args))
		} else {
			fmt.Println(dir)
		}
		return nil
	}

	fmt.Printf("Resuming Claude Code session: %s\n", sessionID)
	fmt.Printf("Directory: %s\n", dir)
	if opts.DangerMode {
		fmt.Fprintf(os.Stderr, "\033[31mWARNING: DANGER MODE: Skipping all permissions checks\033[0m\n")
	}

	if err := os.Chdir(dir); err != nil {
		return fmt.Errorf("chdir %s: %w", dir, err)
	}

	claudePath, err := exec.LookPath("claude")
	if err != nil {
		return fallbackShell()
	}

	return syscall.Exec(claudePath, append([]string{"claude"}, args...), os.Environ())
}

// Opencode changes to dir and execs `opencode -s sessionID`.
func Opencode(sessionID, dir string, opts Options) error {
	if opts.NoLaunch {
		if opts.Verbose {
			fmt.Printf("cd %q && opencode -s %q\n", dir, sessionID)
		} else {
			fmt.Println(dir)
		}
		return nil
	}

	fmt.Printf("Resuming Opencode session: %s\n", sessionID)
	fmt.Printf("Directory: %s\n", dir)

	if err := os.Chdir(dir); err != nil {
		return fmt.Errorf("chdir %s: %w", dir, err)
	}

	opPath, err := exec.LookPath("opencode")
	if err != nil {
		return fallbackShell()
	}

	return syscall.Exec(opPath, []string{"opencode", "-s", sessionID}, os.Environ())
}

// fallbackShell execs the user's default shell when the client binary is missing.
func fallbackShell() error {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/bash"
	}
	shellPath, err := exec.LookPath(shell)
	if err != nil {
		return fmt.Errorf("client not found and shell %s not found: %w", shell, err)
	}
	fmt.Fprintf(os.Stderr, "Client not found in PATH. Falling back to %s\n", shellPath)
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
