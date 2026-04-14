package preview

import (
	"database/sql"
	"fmt"
	"io"
	"math"
	"os"
	"time"

	_ "modernc.org/sqlite"
)

// RenderOpencode writes a preview of an Opencode session to w.
func RenderOpencode(w io.Writer, sessionID, directory string) {
	dbPath := opencodeDBPath()

	if dbPath != "" {
		printOpencodeInfo(w, dbPath, sessionID, directory)
	}

	fmt.Fprintf(w, "\033[1;36m━━━ DIRECTORY LIST ━━━\033[0m\n\n")
	listDir(w, directory)
}

func printOpencodeInfo(w io.Writer, dbPath, sessionID, directory string) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return
	}
	defer db.Close()

	var (
		title       string
		timeUpdated sql.NullFloat64
		msgCount    int
	)
	err = db.QueryRow(`
		SELECT s.title, s.time_updated, COUNT(m.id)
		FROM session s
		LEFT JOIN message m ON s.id = m.session_id
		WHERE s.id = ?
		GROUP BY s.id
	`, sessionID).Scan(&title, &timeUpdated, &msgCount)
	if err != nil {
		return
	}

	timeStr := formatTimestamp(timeUpdated)

	fmt.Fprintf(w, "\033[1;36m━━━━━━━━━━━━━━━ SESSION INFO ━━━━━━━━━━━━━━━\033[0m\n")
	fmt.Fprintf(w, "\033[1;33mTitle:\033[0m     %s\n", title)
	fmt.Fprintf(w, "\033[1;32mTime:\033[0m      %s\n", timeStr)
	fmt.Fprintf(w, "\033[1;35mMessages:\033[0m  %d\n", msgCount)
	fmt.Fprintf(w, "\033[1;90mDirectory:\033[0m %s\n", directory)
	fmt.Fprintf(w, "\033[1;36m━━━━━━━━━━━━━━ DIRECTORY LIST ━━━━━━━━━━━━━━\033[0m\n\n")
}

func formatTimestamp(v sql.NullFloat64) string {
	if !v.Valid {
		return "Unknown"
	}
	ts := v.Float64
	if ts > 9_999_999_999 {
		ts /= 1000.0
	}
	sec := int64(math.Floor(ts))
	nsec := int64((ts - float64(sec)) * 1e9)
	return time.Unix(sec, nsec).Format("2006-01-02 15:04:05")
}

func opencodeDBPath() string {
	dataDir := os.Getenv("OPENCODE_DATA_DIR")
	if dataDir == "" {
		home, _ := os.UserHomeDir()
		dataDir = home + "/.local/share/opencode"
	}
	p := dataDir + "/opencode.db"
	if _, err := os.Stat(p); err == nil {
		return p
	}
	return ""
}
