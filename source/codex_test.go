package source

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadCodex_MissingHome(t *testing.T) {
	// When codex_home doesn't exist, return zero sessions without error.
	dir := t.TempDir()
	t.Setenv("CODEX_HOME", filepath.Join(dir, "nonexistent"))

	sessions, err := LoadCodex("", false, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("expected 0 sessions, got %d", len(sessions))
	}
}

func TestLoadCodex_SQLiteActiveCLI(t *testing.T) {
	// Active CLI row loads with title, cwd, timestamp, branch.
	dir := t.TempDir()
	t.Setenv("CODEX_HOME", dir)

	// Create rollout file first
	rolloutDir := filepath.Join(dir, "sessions", "2026", "06", "02")
	if err := os.MkdirAll(rolloutDir, 0o755); err != nil {
		t.Fatal(err)
	}
	rolloutPath := filepath.Join(rolloutDir, "rollout-2026-06-02T00-00-00-test.jsonl")
	if err := os.WriteFile(rolloutPath, []byte(`{"timestamp":"2026-06-02T00:00:00.000Z","type":"session_meta","payload":{"id":"test-id","timestamp":"2026-06-02T00:00:00Z","cwd":"/test/dir","originator":"codex_cli_rs","cli_version":"0.136.0","source":"cli","model_provider":"openai"}}
`), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create SQLite DB
	dbPath := filepath.Join(dir, "state_5.sqlite")
	db := createTestDB(t, dbPath)
	defer db.Close()

	_, err := db.Exec(`
		INSERT INTO threads (id, rollout_path, cwd, title, preview, source, model_provider, cli_version, archived, updated_at_ms, created_at_ms, git_branch)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "test-id", rolloutPath, "/test/dir", "Test Title", "Test preview", "cli", "openai", "0.136.0", 0, 1717286400000, 1717286400000, "main")
	if err != nil {
		t.Fatal(err)
	}

	sessions, err := LoadCodex("", false, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}

	s := sessions[0]
	if s.ID != "test-id" {
		t.Errorf("ID = %q, want %q", s.ID, "test-id")
	}
	if s.Title != "Test Title" {
		t.Errorf("Title = %q, want %q", s.Title, "Test Title")
	}
	if s.CWD != "/test/dir" {
		t.Errorf("CWD = %q, want %q", s.CWD, "/test/dir")
	}
	if s.Client != ClientCodex {
		t.Errorf("Client = %v, want %v", s.Client, ClientCodex)
	}
	if s.Time.IsZero() {
		t.Error("Time should not be zero")
	}
}

func TestLoadCodex_ExcludeArchived(t *testing.T) {
	// Archived rows are excluded.
	dir := t.TempDir()
	t.Setenv("CODEX_HOME", dir)

	dbPath := filepath.Join(dir, "state_5.sqlite")
	db := createTestDB(t, dbPath)
	defer db.Close()

	_, err := db.Exec(`
		INSERT INTO threads (id, rollout_path, cwd, title, source, archived, updated_at_ms, created_at_ms)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, "test-id", "/nonexistent/path.jsonl", "/test/dir", "Archived", "cli", 1, 1717286400000, 1717286400000)
	if err != nil {
		t.Fatal(err)
	}

	sessions, err := LoadCodex("", false, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("expected 0 sessions, got %d", len(sessions))
	}
}

func TestLoadCodex_ExcludeNonCLI(t *testing.T) {
	// Non-CLI rows are excluded.
	dir := t.TempDir()
	t.Setenv("CODEX_HOME", dir)

	rolloutDir := filepath.Join(dir, "sessions", "2026", "06", "02")
	if err := os.MkdirAll(rolloutDir, 0o755); err != nil {
		t.Fatal(err)
	}
	rolloutPath := filepath.Join(rolloutDir, "rollout-2026-06-02T00-00-00-test.jsonl")
	if err := os.WriteFile(rolloutPath, []byte(`{"timestamp":"2026-06-02T00:00:00.000Z","type":"session_meta","payload":{"id":"test-id","timestamp":"2026-06-02T00:00:00Z","cwd":"/test/dir","originator":"codex_cli_rs","source":"vscode"}}
`), 0o644); err != nil {
		t.Fatal(err)
	}

	dbPath := filepath.Join(dir, "state_5.sqlite")
	db := createTestDB(t, dbPath)
	defer db.Close()

	_, err := db.Exec(`
		INSERT INTO threads (id, rollout_path, cwd, title, source, archived, updated_at_ms, created_at_ms)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, "test-id", rolloutPath, "/test/dir", "VSCode", "vscode", 0, 1717286400000, 1717286400000)
	if err != nil {
		t.Fatal(err)
	}

	sessions, err := LoadCodex("", false, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("expected 0 sessions, got %d", len(sessions))
	}
}

func TestLoadCodex_StaleDBRow(t *testing.T) {
	// DB row with missing rollout path is skipped unless filename fallback finds it.
	dir := t.TempDir()
	t.Setenv("CODEX_HOME", dir)

	dbPath := filepath.Join(dir, "state_5.sqlite")
	db := createTestDB(t, dbPath)
	defer db.Close()

	_, err := db.Exec(`
		INSERT INTO threads (id, rollout_path, cwd, title, source, archived, updated_at_ms, created_at_ms)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, "stale-id", "/nonexistent/path.jsonl", "/test/dir", "Stale", "cli", 0, 1717286400000, 1717286400000)
	if err != nil {
		t.Fatal(err)
	}

	sessions, err := LoadCodex("", false, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("expected 0 sessions (stale row), got %d", len(sessions))
	}
}

func TestLoadCodex_RolloutOnly(t *testing.T) {
	// Rollout-only session loads when DB is absent.
	dir := t.TempDir()
	t.Setenv("CODEX_HOME", dir)

	rolloutDir := filepath.Join(dir, "sessions", "2026", "06", "02")
	if err := os.MkdirAll(rolloutDir, 0o755); err != nil {
		t.Fatal(err)
	}
	rolloutPath := filepath.Join(rolloutDir, "rollout-2026-06-02T00-00-00-test.jsonl")
	if err := os.WriteFile(rolloutPath, []byte(`{"timestamp":"2026-06-02T00:00:00.000Z","type":"session_meta","payload":{"id":"rollout-id","timestamp":"2026-06-02T00:00:00Z","cwd":"/project","originator":"codex_cli_rs","source":"cli","cli_version":"0.136.0"}}
{"timestamp":"2026-06-02T00:00:01.000Z","type":"event_msg","payload":{"type":"user_message","message":"Hello"}}
`), 0o644); err != nil {
		t.Fatal(err)
	}

	sessions, err := LoadCodex("", false, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}

	s := sessions[0]
	if s.ID != "rollout-id" {
		t.Errorf("ID = %q, want %q", s.ID, "rollout-id")
	}
	if s.CWD != "/project" {
		t.Errorf("CWD = %q, want %q", s.CWD, "/project")
	}
	if s.MsgCount != 1 {
		t.Errorf("MsgCount = %d, want 1", s.MsgCount)
	}
}

func TestLoadCodex_TimestampPreferMs(t *testing.T) {
	// updated_at_ms is preferred over updated_at.
	dir := t.TempDir()
	t.Setenv("CODEX_HOME", dir)

	rolloutDir := filepath.Join(dir, "sessions", "2026", "06", "02")
	if err := os.MkdirAll(rolloutDir, 0o755); err != nil {
		t.Fatal(err)
	}
	rolloutPath := filepath.Join(rolloutDir, "rollout-2026-06-02T00-00-00-test.jsonl")
	if err := os.WriteFile(rolloutPath, []byte(`{"timestamp":"2026-06-02T00:00:00.000Z","type":"session_meta","payload":{"id":"test-id","timestamp":"2026-06-02T00:00:00Z","cwd":"/test","originator":"codex_cli_rs","source":"cli"}}
`), 0o644); err != nil {
		t.Fatal(err)
	}

	dbPath := filepath.Join(dir, "state_5.sqlite")
	db := createTestDB(t, dbPath)
	defer db.Close()

	// Insert with both seconds and milliseconds; ms should win
	_, err := db.Exec(`
		INSERT INTO threads (id, rollout_path, cwd, title, source, archived, updated_at, created_at, updated_at_ms, created_at_ms)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "test-id", rolloutPath, "/test", "Title", "cli", 0, 1717286400, 1717286400, 1717286400123, 1717286400456)
	if err != nil {
		t.Fatal(err)
	}

	sessions, err := LoadCodex("", false, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}

	expected := time.Unix(1717286400, 123000000) // 1717286400123 ms
	if !sessions[0].Time.Equal(expected) {
		t.Errorf("Time = %v, want %v", sessions[0].Time, expected)
	}
}

func TestLoadCodex_SessionIndexTitleFallback(t *testing.T) {
	// session_index.jsonl can provide title fallback when DB title/preview are empty.
	dir := t.TempDir()
	t.Setenv("CODEX_HOME", dir)

	rolloutDir := filepath.Join(dir, "sessions", "2026", "06", "02")
	if err := os.MkdirAll(rolloutDir, 0o755); err != nil {
		t.Fatal(err)
	}
	rolloutPath := filepath.Join(rolloutDir, "rollout-2026-06-02T00-00-00-test.jsonl")
	if err := os.WriteFile(rolloutPath, []byte(`{"timestamp":"2026-06-02T00:00:00.000Z","type":"session_meta","payload":{"id":"test-id","timestamp":"2026-06-02T00:00:00Z","cwd":"/test","originator":"codex_cli_rs","source":"cli"}}
`), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create session_index.jsonl with thread name
	indexPath := filepath.Join(dir, "session_index.jsonl")
	if err := os.WriteFile(indexPath, []byte(`{"id":"test-id","thread_name":"My Thread","updated_at":"2026-06-02T00:00:00Z"}
`), 0o644); err != nil {
		t.Fatal(err)
	}

	dbPath := filepath.Join(dir, "state_5.sqlite")
	db := createTestDB(t, dbPath)
	defer db.Close()

	_, err := db.Exec(`
		INSERT INTO threads (id, rollout_path, cwd, title, preview, source, archived, updated_at_ms, created_at_ms)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "test-id", rolloutPath, "/test", "", "", "cli", 0, 1717286400000, 1717286400000)
	if err != nil {
		t.Fatal(err)
	}

	sessions, err := LoadCodex("", false, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}

	if sessions[0].Title != "My Thread" {
		t.Errorf("Title = %q, want %q", sessions[0].Title, "My Thread")
	}
}

func TestLoadCodex_TurnCount(t *testing.T) {
	// Turn count counts only rollout user messages.
	dir := t.TempDir()
	t.Setenv("CODEX_HOME", dir)

	rolloutDir := filepath.Join(dir, "sessions", "2026", "06", "02")
	if err := os.MkdirAll(rolloutDir, 0o755); err != nil {
		t.Fatal(err)
	}
	rolloutPath := filepath.Join(rolloutDir, "rollout-2026-06-02T00-00-00-test.jsonl")
	if err := os.WriteFile(rolloutPath, []byte(`{"timestamp":"2026-06-02T00:00:00.000Z","type":"session_meta","payload":{"id":"test-id","timestamp":"2026-06-02T00:00:00Z","cwd":"/test","originator":"codex_cli_rs","source":"cli"}}
{"timestamp":"2026-06-02T00:00:01.000Z","type":"event_msg","payload":{"type":"user_message","message":"Hello"}}
{"timestamp":"2026-06-02T00:00:02.000Z","type":"response_item","payload":{"type":"message"}}
{"timestamp":"2026-06-02T00:00:03.000Z","type":"event_msg","payload":{"type":"user_message","message":"World"}}
{"timestamp":"2026-06-02T00:00:04.000Z","type":"turn_context","payload":{}}
`), 0o644); err != nil {
		t.Fatal(err)
	}

	sessions, err := LoadCodex("", false, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}

	if sessions[0].MsgCount != 2 {
		t.Errorf("MsgCount = %d, want 2", sessions[0].MsgCount)
	}
}

func TestLoadCodex_PathFilter(t *testing.T) {
	// Path filter uses existing matching behavior.
	dir := t.TempDir()
	t.Setenv("CODEX_HOME", dir)

	rolloutDir := filepath.Join(dir, "sessions", "2026", "06", "02")
	if err := os.MkdirAll(rolloutDir, 0o755); err != nil {
		t.Fatal(err)
	}
	rolloutPath := filepath.Join(rolloutDir, "rollout-2026-06-02T00-00-00-test.jsonl")
	if err := os.WriteFile(rolloutPath, []byte(`{"timestamp":"2026-06-02T00:00:00.000Z","type":"session_meta","payload":{"id":"test-id","timestamp":"2026-06-02T00:00:00Z","cwd":"/project/a","originator":"codex_cli_rs","source":"cli"}}
`), 0o644); err != nil {
		t.Fatal(err)
	}

	sessions, err := LoadCodex("/project/a", false, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}

	sessions, err = LoadCodex("/project/b", false, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("expected 0 sessions, got %d", len(sessions))
	}
}

// createTestDB creates a minimal SQLite DB with the threads table for testing.
func createTestDB(t *testing.T, dbPath string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}

	// Create threads table with all columns we might use
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS threads (
			id TEXT PRIMARY KEY,
			rollout_path TEXT,
			cwd TEXT,
			title TEXT,
			preview TEXT,
			first_user_message TEXT,
			source TEXT,
			model_provider TEXT,
			cli_version TEXT,
			archived INTEGER DEFAULT 0,
			updated_at INTEGER,
			created_at INTEGER,
			updated_at_ms INTEGER,
			created_at_ms INTEGER,
			git_branch TEXT,
			tokens_used INTEGER,
			has_user_event INTEGER,
			approval_mode TEXT,
			sandbox_policy TEXT,
			git_sha TEXT,
			git_origin_url TEXT
		)
	`)
	if err != nil {
		t.Fatal(err)
	}

	return db
}

func TestFindRolloutPath_NestedDirectories(t *testing.T) {
	// FindRolloutPath should find files in nested YYYY/MM/DD directories.
	dir := t.TempDir()

	// Create nested directory structure
	rolloutDir := filepath.Join(dir, "sessions", "2026", "06", "02")
	if err := os.MkdirAll(rolloutDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create rollout file with session ID in filename
	sessionID := "test-session-123"
	rolloutPath := filepath.Join(rolloutDir, "rollout-2026-06-02T00-00-00-"+sessionID+".jsonl")
	if err := os.WriteFile(rolloutPath, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}

	// Test FindRolloutPath
	result := FindRolloutPath(dir, sessionID)
	if result != rolloutPath {
		t.Errorf("FindRolloutPath() = %q, want %q", result, rolloutPath)
	}

	// Test with non-existent session
	result = FindRolloutPath(dir, "nonexistent-session")
	if result != "" {
		t.Errorf("FindRolloutPath() = %q, want empty string", result)
	}
}

func TestFindRolloutPath_MultipleMatches(t *testing.T) {
	// FindRolloutPath should return the first match found.
	dir := t.TempDir()

	// Create two rollout files with the same session ID in different directories
	sessionID := "test-session-456"

	dir1 := filepath.Join(dir, "sessions", "2026", "06", "01")
	if err := os.MkdirAll(dir1, 0o755); err != nil {
		t.Fatal(err)
	}
	path1 := filepath.Join(dir1, "rollout-2026-06-01T00-00-00-"+sessionID+".jsonl")
	if err := os.WriteFile(path1, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}

	dir2 := filepath.Join(dir, "sessions", "2026", "06", "02")
	if err := os.MkdirAll(dir2, 0o755); err != nil {
		t.Fatal(err)
	}
	path2 := filepath.Join(dir2, "rollout-2026-06-02T00-00-00-"+sessionID+".jsonl")
	if err := os.WriteFile(path2, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}

	// Should return one of them (implementation dependent)
	result := FindRolloutPath(dir, sessionID)
	if result == "" {
		t.Error("FindRolloutPath() returned empty string, expected a path")
	}
	if result != path1 && result != path2 {
		t.Errorf("FindRolloutPath() = %q, want %q or %q", result, path1, path2)
	}
}

func TestFindRolloutPath_SubstringNoMatch(t *testing.T) {
	// FindRolloutPath must not match "abc" against a file containing "abc123".
	dir := t.TempDir()

	rolloutDir := filepath.Join(dir, "sessions", "2026", "06", "02")
	if err := os.MkdirAll(rolloutDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// File has ID "abc123"; searching for "abc" must NOT match.
	path := filepath.Join(rolloutDir, "rollout-2026-06-02T00-00-00-abc123.jsonl")
	if err := os.WriteFile(path, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}

	result := FindRolloutPath(dir, "abc")
	if result != "" {
		t.Errorf("FindRolloutPath(%q) = %q, want empty (substring false positive)", "abc", result)
	}

	// Exact match should work.
	result = FindRolloutPath(dir, "abc123")
	if result != path {
		t.Errorf("FindRolloutPath(%q) = %q, want %q", "abc123", result, path)
	}
}

func TestRolloutFileMatchesID(t *testing.T) {
	tests := []struct {
		name string
		id   string
		want bool
	}{
		{"rollout-2026-06-02T00-00-00-test.jsonl", "test", true},
		{"rollout-2026-06-02T00-00-00-test-session-123.jsonl", "test-session-123", true},
		{"rollout-2026-06-02T00-00-00-abc123.jsonl", "abc", false},       // substring must not match
		{"rollout-2026-06-02T00-00-00-abc123.jsonl", "abc123", true},     // exact suffix match
		{"rollout-2026-06-02T00-00-00-abc.jsonl", "abc", true},           // short ID
		{"not-a-rollout.jsonl", "test", false},                            // no matching prefix
		{"rollout-2026-06-02T00-00-00-test.txt", "test", false},          // wrong extension
	}
	for _, tt := range tests {
		got := rolloutFileMatchesID(tt.name, tt.id)
		if got != tt.want {
			t.Errorf("rolloutFileMatchesID(%q, %q) = %v, want %v", tt.name, tt.id, got, tt.want)
		}
	}
}
