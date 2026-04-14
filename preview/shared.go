package preview

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
)

func listDir(dir string) {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		fmt.Printf("(directory not found: %s)\n", dir)
		return
	}

	if path, err := exec.LookPath("eza"); err == nil {
		cmd := exec.Command(path,
			"-lF", "--time-style=+%Y-%m-%d %H:%M:%S",
			"--group-directories-first", "--binary",
			"--color=always", "--no-permissions", "--no-user", "-M", dir)
		if out, err := cmd.Output(); err == nil {
			fmt.Print(string(out))
			return
		}
	}

	lsPath, err := exec.LookPath("ls")
	if err != nil {
		return
	}
	if runtime.GOOS == "darwin" {
		cmd := exec.Command(lsPath, "-lF", "-D", "%Y-%m-%d %H:%M:%S",
			"-h", "--color=always", "-o", "-g", dir)
		if out, err := cmd.Output(); err == nil {
			fmt.Print(string(out))
		}
	} else {
		cmd := exec.Command(lsPath, "-lF",
			"--time-style=+%Y-%m-%d %H:%M:%S",
			"--group-directories-first", "-h",
			"--color=always", "-o", "-g", dir)
		if out, err := cmd.Output(); err == nil {
			fmt.Print(string(out))
		}
	}
}
