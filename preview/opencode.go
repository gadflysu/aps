package preview

import (
	"database/sql"
	"fmt"
	"io"
	"math"
	"os"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// RenderOpencode writes a preview of an Opencode session to w.
func RenderOpencode(w io.Writer, sessionID, directory string) {
	dbPath := opencodeDBPath()

	if dbPath != "" {
		printOpencodeInfo(w, dbPath, sessionID, directory)
	}

	fmt.Fprintf(w, "%s\n\n", previewHeader.Render("━━━ DIRECTORY LIST ━━━"))
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

	fmt.Fprintf(w, "%s\n", previewHeader.Render("━━━━━━━━━━━━━━━ SESSION INFO ━━━━━━━━━━━━━━━"))
	fmt.Fprintf(w, "%s     %s\n", previewLabelTitle.Render("Title:"), title)
	fmt.Fprintf(w, "%s      %s\n", previewLabelTime.Render("Time:"), timeStr)
	fmt.Fprintf(w, "%s  %d\n", previewLabelMsg.Render("Messages:"), msgCount)
	fmt.Fprintf(w, "%s %s\n", previewLabelDir.Render("Directory:"), directory)
	fmt.Fprintf(w, "%s\n\n", previewHeader.Render("━━━━━━━━━━━━━━ DIRECTORY LIST ━━━━━━━━━━━━━━"))
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

// OpencodeInfo returns the session info fields from the Opencode SQLite DB
// as a styled string for the info viewport section.
// Returns empty string when the DB is absent or the session is not found.
func OpencodeInfo(sessionID, directory string) string {
	dbPath := opencodeDBPath()
	if dbPath == "" {
		return ""
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return ""
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
		return ""
	}

	timeStr := formatTimestamp(timeUpdated)

	var sb strings.Builder
	fmt.Fprintf(&sb, "%s     %s\n", previewLabelTitle.Render("Title:"), title)
	fmt.Fprintf(&sb, "%s      %s\n", previewLabelTime.Render("Time:"), timeStr)
	fmt.Fprintf(&sb, "%s  %d\n", previewLabelMsg.Render("Messages:"), msgCount)
	fmt.Fprintf(&sb, "%s %s\n", previewLabelDir.Render("Directory:"), directory)
	return sb.String()
}
