package source

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gadflysu/aps/filter"
)

// LoadCodex returns all Codex CLI sessions, optionally filtered by path.
func LoadCodex(pathFilter string, strictMatch bool, verbose bool) ([]Session, error) {
	codexHome := CodexHomeDir()
	if codexHome == "" || !dirExists(codexHome) {
		return nil, nil
	}

	sqliteHome := resolveSQLiteHome(codexHome)
	dbPath := filepath.Join(sqliteHome, "state_5.sqlite")

	var dbSessions []Session
	var dbIDs map[string]bool

	if fileExists(dbPath) {
		sessions, ids, err := loadCodexSQL(dbPath, codexHome, pathFilter, strictMatch)
		if err == nil {
			dbSessions = sessions
			dbIDs = ids
		}
	}

	// Load rollout-only sessions (not in DB)
	rolloutSessions := loadCodexRollouts(codexHome, pathFilter, strictMatch, dbIDs)

	all := append(dbSessions, rolloutSessions...)
	sort.Slice(all, func(i, j int) bool {
		return all[i].Time.After(all[j].Time)
	})

	return all, nil
}

// CodexHomeDir returns the Codex home directory.
// Exported for use by picker package.
func CodexHomeDir() string {
	if v := os.Getenv("CODEX_HOME"); v != "" {
		return v
	}
	home, _ := os.UserHomeDir()
	if home == "" {
		return ""
	}
	return filepath.Join(home, ".codex")
}

func resolveSQLiteHome(codexHome string) string {
	// Try config.toml sqlite_home
	if configPath := filepath.Join(codexHome, "config.toml"); fileExists(configPath) {
		if v := parseConfigSQLiteHome(configPath); v != "" {
			if filepath.IsAbs(v) {
				return v
			}
			return filepath.Join(codexHome, v)
		}
	}

	// Try CODEX_SQLITE_HOME env
	if v := os.Getenv("CODEX_SQLITE_HOME"); v != "" {
		if filepath.IsAbs(v) {
			return v
		}
		// Relative to cwd (per Codex source)
		cwd, _ := os.Getwd()
		if cwd != "" {
			return filepath.Join(cwd, v)
		}
		return v
	}

	return codexHome
}

// parseConfigSQLiteHome extracts sqlite_home from a minimal TOML config.
func parseConfigSQLiteHome(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "#") || strings.HasPrefix(line, "[") {
			continue
		}
		if strings.HasPrefix(line, "sqlite_home") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				v := strings.TrimSpace(parts[1])
				v = strings.Trim(v, `"'`)
				if v != "" {
					return v
				}
			}
		}
	}
	return ""
}

// loadCodexSQL loads sessions from the Codex SQLite state DB.
func loadCodexSQL(dbPath, codexHome, pathFilter string, strictMatch bool) ([]Session, map[string]bool, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, nil, err
	}
	defer db.Close()

	// Query active CLI sessions
	rows, err := db.Query(`
		SELECT id, rollout_path, cwd, title, preview, first_user_message,
		       COALESCE(updated_at_ms, updated_at * 1000) AS updated_ms,
		       COALESCE(created_at_ms, created_at * 1000) AS created_ms,
		       git_branch, source, model_provider, cli_version
		FROM threads
		WHERE archived = 0
		  AND source = 'cli'
		  AND cwd <> ''
		ORDER BY updated_ms DESC
	`)
	if err != nil {
		// Try fallback query without ms columns
		return loadCodexSQLFallback(db, codexHome, pathFilter, strictMatch)
	}
	defer rows.Close()

	home, _ := os.UserHomeDir()
	var sessions []Session
	ids := make(map[string]bool)

	for rows.Next() {
		var (
			id             string
			rolloutPath    string
			cwd            string
			title          sql.NullString
			preview        sql.NullString
			firstMsg       sql.NullString
			updatedMs      sql.NullInt64
			createdMs      sql.NullInt64
			gitBranch      sql.NullString
			source         string
			modelProvider  sql.NullString
			cliVersion     sql.NullString
		)
		if err := rows.Scan(&id, &rolloutPath, &cwd, &title, &preview, &firstMsg,
			&updatedMs, &createdMs, &gitBranch, &source, &modelProvider, &cliVersion); err != nil {
			continue
		}

		// Verify rollout path exists or find by ID
		if !verifyRolloutPath(rolloutPath, codexHome, id) {
			continue
		}

		if !filter.Matches(pathFilter, strictMatch, cwd) {
			continue
		}

		t := time.Time{}
		if updatedMs.Valid {
			t = time.UnixMilli(updatedMs.Int64)
		}

		sessionTitle := resolveTitle(title, preview, firstMsg, codexHome, id)
		cwdDisplay := abbreviateHome(cwd, home)

		// Count user messages from rollout file
		msgCount := 0
		actualRolloutPath := rolloutPath
		if !fileExists(actualRolloutPath) {
			actualRolloutPath = FindRolloutPath(codexHome, id)
		}
		if actualRolloutPath != "" {
			msgCount = countRolloutUserMessages(actualRolloutPath)
		}

		sessions = append(sessions, Session{
			Client:     ClientCodex,
			ID:         sanitize(id),
			Title:      sanitize(sessionTitle),
			CWD:        cwd,
			CWDDisplay: cwdDisplay,
			Time:       t,
			MsgCount:   msgCount,
		})
		ids[id] = true
	}

	return sessions, ids, rows.Err()
}

// loadCodexSQLFallback queries without ms columns for old DBs.
func loadCodexSQLFallback(db *sql.DB, codexHome, pathFilter string, strictMatch bool) ([]Session, map[string]bool, error) {
	rows, err := db.Query(`
		SELECT id, rollout_path, cwd, title, preview, first_user_message,
		       updated_at, created_at, git_branch, source, model_provider, cli_version
		FROM threads
		WHERE archived = 0
		  AND source = 'cli'
		  AND cwd <> ''
		ORDER BY updated_at DESC
	`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	home, _ := os.UserHomeDir()
	var sessions []Session
	ids := make(map[string]bool)

	for rows.Next() {
		var (
			id             string
			rolloutPath    string
			cwd            string
			title          sql.NullString
			preview        sql.NullString
			firstMsg       sql.NullString
			updatedAt      sql.NullInt64
			createdAt      sql.NullInt64
			gitBranch      sql.NullString
			source         string
			modelProvider  sql.NullString
			cliVersion     sql.NullString
		)
		if err := rows.Scan(&id, &rolloutPath, &cwd, &title, &preview, &firstMsg,
			&updatedAt, &createdAt, &gitBranch, &source, &modelProvider, &cliVersion); err != nil {
			continue
		}

		if !verifyRolloutPath(rolloutPath, codexHome, id) {
			continue
		}

		if !filter.Matches(pathFilter, strictMatch, cwd) {
			continue
		}

		t := time.Time{}
		if updatedAt.Valid {
			t = time.Unix(updatedAt.Int64, 0)
		}

		sessionTitle := resolveTitle(title, preview, firstMsg, codexHome, id)
		cwdDisplay := abbreviateHome(cwd, home)

		// Count user messages from rollout file
		msgCount := 0
		actualRolloutPath := rolloutPath
		if !fileExists(actualRolloutPath) {
			actualRolloutPath = FindRolloutPath(codexHome, id)
		}
		if actualRolloutPath != "" {
			msgCount = countRolloutUserMessages(actualRolloutPath)
		}

		sessions = append(sessions, Session{
			Client:     ClientCodex,
			ID:         sanitize(id),
			Title:      sanitize(sessionTitle),
			CWD:        cwd,
			CWDDisplay: cwdDisplay,
			Time:       t,
			MsgCount:   msgCount,
		})
		ids[id] = true
	}

	return sessions, ids, rows.Err()
}

// verifyRolloutPath checks if rollout_path exists or finds it by session ID.
func verifyRolloutPath(rolloutPath, codexHome, id string) bool {
	if rolloutPath != "" && fileExists(rolloutPath) {
		return true
	}
	// filepath.Glob doesn't support **; use FindRolloutPath instead.
	return FindRolloutPath(codexHome, id) != ""
}

// countRolloutUserMessages counts user messages in a rollout file.
func countRolloutUserMessages(rolloutPath string) int {
	if rolloutPath == "" {
		return 0
	}

	f, err := os.Open(rolloutPath)
	if err != nil {
		return 0
	}
	defer f.Close()

	count := 0
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, bufio.MaxScanTokenSize), 4*1024*1024)
	for scanner.Scan() {
		var event rolloutEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			continue
		}
		if event.Type == "event_msg" && event.Payload.Type == "user_message" && event.Payload.Message != "" {
			count++
		}
	}
	return count
}

// FindRolloutPath finds the rollout file path for a session ID.
// Exported for use by preview package.
func FindRolloutPath(codexHome, id string) string {
	if codexHome == "" {
		return ""
	}
	sessionsDir := filepath.Join(codexHome, "sessions")
	if !dirExists(sessionsDir) {
		return ""
	}

	var result string
	filepath.Walk(sessionsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".jsonl") {
			return nil
		}
		if rolloutFileMatchesID(info.Name(), id) {
			result = path
			return filepath.SkipAll
		}
		return nil
	})
	return result
}

// rolloutFileMatchesID checks if a rollout filename contains the session ID
// as a complete segment (after the last timestamp dash, before .jsonl).
func rolloutFileMatchesID(name, id string) bool {
	if !strings.HasSuffix(name, ".jsonl") {
		return false
	}
	base := strings.TrimSuffix(name, ".jsonl")
	// Rollout filenames follow: rollout-<timestamp>-<id>
	// The ID starts after the last dash that precedes it.
	// Find the ID as a suffix segment: base must end with "-<id>" or equal id.
	if base == id {
		return true
	}
	suffix := "-" + id
	return strings.HasSuffix(base, suffix)
}

// resolveTitle determines the session title from available sources.
// Priority matches Codex CLI: session_index.jsonl thread_name wins.
func resolveTitle(title, preview, firstMsg sql.NullString, codexHome, id string) string {
	// Priority 1: session_index.jsonl (matches Codex CLI behavior)
	if name := LookupSessionIndex(codexHome, id); name != "" {
		return name
	}
	// Priority 2: SQLite title
	if title.Valid && title.String != "" {
		return title.String
	}
	// Priority 3: SQLite preview
	if preview.Valid && preview.String != "" {
		return preview.String
	}
	// Priority 4: SQLite first_user_message
	if firstMsg.Valid && firstMsg.String != "" {
		return firstMsg.String
	}
	return "Untitled"
}

// sessionIndexCache caches parsed session_index.jsonl per codexHome.
var (
	sessionIndexCacheMu sync.Mutex
	sessionIndexCache   = make(map[string]map[string]string)
)

// LookupSessionIndex returns the thread name for a session ID from session_index.jsonl.
// Exported for use by preview package. Results are cached per codexHome.
func LookupSessionIndex(codexHome, id string) string {
	if codexHome == "" {
		return ""
	}
	index := loadSessionIndex(codexHome)
	return index[id]
}

// loadSessionIndex loads and caches session_index.jsonl as a map[id]thread_name.
func loadSessionIndex(codexHome string) map[string]string {
	sessionIndexCacheMu.Lock()
	defer sessionIndexCacheMu.Unlock()

	if cached, ok := sessionIndexCache[codexHome]; ok {
		return cached
	}

	indexPath := filepath.Join(codexHome, "session_index.jsonl")
	f, err := os.Open(indexPath)
	if err != nil {
		sessionIndexCache[codexHome] = nil
		return nil
	}
	defer f.Close()

	index := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var entry struct {
			ID         string `json:"id"`
			ThreadName string `json:"thread_name"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}
		if entry.ID != "" && entry.ThreadName != "" {
			index[entry.ID] = entry.ThreadName // later entries overwrite; latest wins
		}
	}
	sessionIndexCache[codexHome] = index
	return index
}

// rolloutMeta represents the first session_meta line in a rollout file.
type rolloutMeta struct {
	Timestamp string `json:"timestamp"`
	Type      string `json:"type"`
	Payload   struct {
		ID            string `json:"id"`
		Timestamp     string `json:"timestamp"`
		CWD           string `json:"cwd"`
		Originator    string `json:"originator"`
		CLIVersion    string `json:"cli_version"`
		Source        string `json:"source"`
		ModelProvider string `json:"model_provider"`
		GitBranch     string `json:"git_branch"`
	} `json:"payload"`
}

// rolloutEvent represents an event_msg line for turn counting.
type rolloutEvent struct {
	Type    string `json:"type"`
	Payload struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"payload"`
}

// loadCodexRollouts scans rollout files for sessions not in the DB.
func loadCodexRollouts(codexHome, pathFilter string, strictMatch bool, dbIDs map[string]bool) []Session {
	sessionsDir := filepath.Join(codexHome, "sessions")
	if !dirExists(sessionsDir) {
		return nil
	}

	home, _ := os.UserHomeDir()
	var sessions []Session

	err := filepath.Walk(sessionsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".jsonl") {
			return nil
		}

		session, ok := parseRolloutFile(path, codexHome, pathFilter, strictMatch, home, dbIDs)
		if ok {
			sessions = append(sessions, session)
		}
		return nil
	})
	if err != nil {
		return nil
	}

	return sessions
}

// parseRolloutFile parses a rollout JSONL file and returns a session if valid.
func parseRolloutFile(path, codexHome, pathFilter string, strictMatch bool, home string, dbIDs map[string]bool) (Session, bool) {
	f, err := os.Open(path)
	if err != nil {
		return Session{}, false
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, bufio.MaxScanTokenSize), 4*1024*1024)

	// Read first line for session_meta
	if !scanner.Scan() {
		return Session{}, false
	}
	var meta rolloutMeta
	if err := json.Unmarshal(scanner.Bytes(), &meta); err != nil {
		return Session{}, false
	}
	if meta.Type != "session_meta" || meta.Payload.Source != "cli" {
		return Session{}, false
	}
	if meta.Payload.ID == "" || meta.Payload.CWD == "" {
		return Session{}, false
	}

	// Skip if already in DB
	if dbIDs != nil && dbIDs[meta.Payload.ID] {
		return Session{}, false
	}

	if !filter.Matches(pathFilter, strictMatch, meta.Payload.CWD) {
		return Session{}, false
	}

	// Count user messages and find first user message for title fallback
	msgCount := 0
	firstUserMsg := ""
	for scanner.Scan() {
		var event rolloutEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			continue
		}
		if event.Type == "event_msg" && event.Payload.Type == "user_message" && event.Payload.Message != "" {
			msgCount++
			if firstUserMsg == "" {
				firstUserMsg = event.Payload.Message
			}
		}
	}

	// Parse timestamp
	t := parseCodexTimestamp(meta.Payload.Timestamp)

	// Try session_index for title
	title := LookupSessionIndex(codexHome, meta.Payload.ID)
	if title == "" && firstUserMsg != "" {
		title = firstUserMsg
	}
	if title == "" {
		title = "Untitled"
	}

	cwdDisplay := abbreviateHome(meta.Payload.CWD, home)

	return Session{
		Client:     ClientCodex,
		ID:         sanitize(meta.Payload.ID),
		Title:      sanitize(title),
		CWD:        meta.Payload.CWD,
		CWDDisplay: cwdDisplay,
		Time:       t,
		MsgCount:   msgCount,
	}, true
}

// parseCodexTimestamp parses a Codex timestamp string.
func parseCodexTimestamp(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	// Try RFC3339
	t, err := time.Parse(time.RFC3339, s)
	if err == nil {
		return t
	}
	// Try without timezone
	t, err = time.Parse("2006-01-02T15:04:05", s)
	if err == nil {
		return t
	}
	return time.Time{}
}
