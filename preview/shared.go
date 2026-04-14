package preview

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

func listDir(w io.Writer, dir string) {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		fmt.Fprintf(w, "%s\n", previewMissing.Render(fmt.Sprintf("(directory not found: %s)", dir)))
		return
	}

	if path, err := exec.LookPath("eza"); err == nil {
		cmd := exec.Command(path,
			"-lF", "--time-style=+%Y-%m-%d %H:%M:%S",
			"--group-directories-first", "--binary",
			"--color=always", "--no-permissions", "--no-user", "-M", dir)
		if out, err := cmd.Output(); err == nil {
			fmt.Fprint(w, string(out))
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
			fmt.Fprint(w, string(out))
		}
	} else {
		cmd := exec.Command(lsPath, "-lF",
			"--time-style=+%Y-%m-%d %H:%M:%S",
			"--group-directories-first", "-h",
			"--color=always", "-o", "-g", dir)
		if out, err := cmd.Output(); err == nil {
			fmt.Fprint(w, string(out))
		}
	}
}

// DirListing returns the directory listing as a string.
// Delegates to listDir; the caller is responsible for providing the section header.
func DirListing(dir string) string {
	var sb strings.Builder
	listDir(&sb, dir)
	return sb.String()
}
