package source

import (
	"database/sql"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"sort"
	"time"

	"local/aps/filter"
	_ "modernc.org/sqlite"
)

// LoadOpencode returns all Opencode sessions, optionally filtered by path.
// It auto-detects storage format: SQLite takes precedence over JSON.
func LoadOpencode(pathFilter string, strictMatch bool, verbose bool) ([]Session, error) {
	dataDir := opencodeDataDir()
	dbPath := filepath.Join(dataDir, "opencode.db")
	jsonStoragePath := filepath.Join(dataDir, "storage")

	if fileExists(dbPath) {
		return loadOpencodeSQL(dbPath, pathFilter, strictMatch)
	}
	if dirExists(filepath.Join(jsonStoragePath, "session", "global")) {
		return loadOpencodeJSON(jsonStoragePath, pathFilter, strictMatch)
	}
	return nil, nil
}

func opencodeDataDir() string {
	if v := os.Getenv("OPENCODE_DATA_DIR"); v != "" {
		return v
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "opencode")
}

// --- SQLite ---

func loadOpencodeSQL(dbPath, pathFilter string, strictMatch bool) ([]Session, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.Query(`
		SELECT s.id, s.title, s.directory, s.time_updated, COUNT(m.id) as message_count
		FROM session s
		LEFT JOIN message m ON s.id = m.session_id
		GROUP BY s.id, s.title, s.directory, s.time_updated
		ORDER BY s.time_updated DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	home, _ := os.UserHomeDir()
	var sessions []Session

	for rows.Next() {
		var (
			id          string
			title       string
			directory   string
			timeUpdated sql.NullFloat64
			msgCount    int
		)
		if err := rows.Scan(&id, &title, &directory, &timeUpdated, &msgCount); err != nil {
			continue
		}

		if !filter.Matches(pathFilter, strictMatch, directory) {
			continue
		}

		t := parseTimestamp(timeUpdated)
		cwdDisplay := abbreviateHome(directory, home)

		sessions = append(sessions, Session{
			Client:     ClientOpencode,
			ID:         sanitize(id),
			Title:      sanitize(title),
			CWD:        directory,
			CWDDisplay: cwdDisplay,
			Time:       t,
			MsgCount:   msgCount,
		})
	}

	return sessions, rows.Err()
}

// parseTimestamp converts a nullable float64 timestamp (seconds or ms) to time.Time.
func parseTimestamp(v sql.NullFloat64) time.Time {
	if !v.Valid {
		return time.Time{}
	}
	ts := v.Float64
	if ts > 9_999_999_999 {
		ts /= 1000.0
	}
	sec := int64(math.Floor(ts))
	nsec := int64((ts - float64(sec)) * 1e9)
	return time.Unix(sec, nsec)
}

// --- JSON storage (legacy) ---

type jsonSession struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Directory string `json:"directory"`
	Time      struct {
		Updated float64 `json:"updated"`
	} `json:"time"`
}

func loadOpencodeJSON(storagePath, pathFilter string, strictMatch bool) ([]Session, error) {
	sessionDir := filepath.Join(storagePath, "session", "global")
	entries, err := filepath.Glob(filepath.Join(sessionDir, "ses_*.json"))
	if err != nil {
		return nil, err
	}

	home, _ := os.UserHomeDir()
	var sessions []Session

	for _, f := range entries {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var js jsonSession
		if err := json.Unmarshal(data, &js); err != nil {
			continue
		}

		if !filter.Matches(pathFilter, strictMatch, js.Directory) {
			continue
		}

		// Count message files
		msgDir := filepath.Join(storagePath, "message", js.ID)
		msgFiles, _ := filepath.Glob(filepath.Join(msgDir, "msg_*.json"))
		msgCount := len(msgFiles)

		ts := js.Time.Updated
		if ts > 9_999_999_999 {
			ts /= 1000.0
		}
		t := time.Unix(int64(ts), 0)

		cwdDisplay := abbreviateHome(js.Directory, home)
		sessions = append(sessions, Session{
			Client:     ClientOpencode,
			ID:         sanitize(js.ID),
			Title:      sanitize(js.Title),
			CWD:        js.Directory,
			CWDDisplay: cwdDisplay,
			Time:       t,
			MsgCount:   msgCount,
		})
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].Time.After(sessions[j].Time)
	})

	return sessions, nil
}

func dirExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}
