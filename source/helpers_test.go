package source

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// --- Client.String ---

func TestClientString_Claude(t *testing.T) {
	got := ClientClaude.String()
	if got != "Claude Code" {
		t.Errorf("ClientClaude.String() = %q, want \"Claude Code\"", got)
	}
}

func TestClientString_Opencode(t *testing.T) {
	got := ClientOpencode.String()
	if got != "OpenCode" {
		t.Errorf("ClientOpencode.String() = %q, want \"OpenCode\"", got)
	}
}

func TestClientString_Unknown(t *testing.T) {
	got := Client(99).String()
	if got != "Unknown" {
		t.Errorf("Client(99).String() = %q, want \"Unknown\"", got)
	}
}

// --- fileExists (source/shared.go) ---

func TestSourceFileExists_Existing(t *testing.T) {
	dir := t.TempDir()
	if !fileExists(dir) {
		t.Errorf("fileExists(%q) should be true for existing dir", dir)
	}
}

func TestSourceFileExists_NonExisting(t *testing.T) {
	if fileExists("/nonexistent/path/xyz999") {
		t.Error("fileExists should be false for nonexistent path")
	}
}

// --- dirExists ---

func TestDirExists_Directory(t *testing.T) {
	dir := t.TempDir()
	if !dirExists(dir) {
		t.Errorf("dirExists(%q) should be true for real dir", dir)
	}
}

func TestDirExists_NonExisting(t *testing.T) {
	if dirExists("/nonexistent/dir/abc123") {
		t.Error("dirExists should be false for nonexistent path")
	}
}

func TestDirExists_File(t *testing.T) {
	dir := t.TempDir()
	f, err := os.CreateTemp(dir, "test*.txt")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	// A file path should return false from dirExists
	if dirExists(f.Name()) {
		t.Errorf("dirExists(%q) should be false for a regular file", f.Name())
	}
}

// --- parseTimestamp ---

func TestParseTimestamp_Invalid(t *testing.T) {
	v := sql.NullFloat64{Valid: false}
	got := parseTimestamp(v)
	if !got.IsZero() {
		t.Errorf("parseTimestamp(invalid) = %v, want zero time", got)
	}
}

func TestParseTimestamp_Seconds(t *testing.T) {
	// Unix timestamp in seconds: 1_700_000_000 (Nov 2023)
	sec := float64(1_700_000_000)
	v := sql.NullFloat64{Valid: true, Float64: sec}
	got := parseTimestamp(v)
	want := time.Unix(1_700_000_000, 0)
	if !got.Equal(want) {
		t.Errorf("parseTimestamp(seconds) = %v, want %v", got, want)
	}
}

func TestParseTimestamp_Milliseconds(t *testing.T) {
	// Unix timestamp in milliseconds: > 9_999_999_999
	ms := float64(1_700_000_000_000)
	v := sql.NullFloat64{Valid: true, Float64: ms}
	got := parseTimestamp(v)
	want := time.Unix(1_700_000_000, 0)
	if !got.Equal(want) {
		t.Errorf("parseTimestamp(ms) = %v, want %v", got, want)
	}
}

func TestParseTimestamp_Zero(t *testing.T) {
	v := sql.NullFloat64{Valid: true, Float64: 0}
	got := parseTimestamp(v)
	if !got.Equal(time.Unix(0, 0)) {
		t.Errorf("parseTimestamp(0) = %v, want epoch", got)
	}
}

// --- opencodeDataDir ---

func TestOpencodeDataDir_EnvOverride(t *testing.T) {
	t.Setenv("OPENCODE_DATA_DIR", "/custom/data/dir")
	got := opencodeDataDir()
	if got != "/custom/data/dir" {
		t.Errorf("opencodeDataDir env override = %q, want \"/custom/data/dir\"", got)
	}
}

func TestOpencodeDataDir_DefaultContainsOpencode(t *testing.T) {
	t.Setenv("OPENCODE_DATA_DIR", "")
	got := opencodeDataDir()
	if !strings.Contains(got, "opencode") {
		t.Errorf("opencodeDataDir default = %q, should contain \"opencode\"", got)
	}
}

// --- loadOpencodeJSON ---

func writeJSONSession(t *testing.T, sessionDir string, id, title, dir string, updatedMs float64) {
	t.Helper()
	js := map[string]any{
		"id": id, "title": title, "directory": dir,
		"time": map[string]any{"updated": updatedMs},
	}
	data, _ := json.Marshal(js)
	p := filepath.Join(sessionDir, "ses_"+id+".json")
	if err := os.WriteFile(p, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLoadOpencodeJSON_ReturnsSessions(t *testing.T) {
	base := t.TempDir()
	sessionDir := filepath.Join(base, "session", "global")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeJSONSession(t, sessionDir, "abc", "My Session", t.TempDir(), 1_700_000_000_000)

	sessions, err := loadOpencodeJSON(base, "", false)
	if err != nil {
		t.Fatalf("loadOpencodeJSON error: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].Title != "My Session" {
		t.Errorf("title = %q, want \"My Session\"", sessions[0].Title)
	}
}

func TestLoadOpencodeJSON_EmptyDir(t *testing.T) {
	base := t.TempDir()
	sessionDir := filepath.Join(base, "session", "global")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	sessions, err := loadOpencodeJSON(base, "", false)
	if err != nil {
		t.Fatalf("loadOpencodeJSON error: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(sessions))
	}
}

func TestLoadOpencodeJSON_InvalidJSONSkipped(t *testing.T) {
	base := t.TempDir()
	sessionDir := filepath.Join(base, "session", "global")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Write invalid JSON file
	if err := os.WriteFile(filepath.Join(sessionDir, "ses_bad.json"), []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Write valid session
	writeJSONSession(t, sessionDir, "good", "Good Session", t.TempDir(), 1_700_000_000_000)

	sessions, err := loadOpencodeJSON(base, "", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 1 {
		t.Errorf("expected 1 session (invalid skipped), got %d", len(sessions))
	}
}

// --- LoadClaude ---

func TestLoadClaude_NoProjectsDir(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // home with no .claude/projects
	sessions, err := LoadClaude("", false, false)
	if err != nil {
		t.Fatalf("LoadClaude unexpected error: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(sessions))
	}
}

func TestLoadClaude_WithSession(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	projectDir := filepath.Join(home, ".claude", "projects", "%2Ftmp%2Fmyproject")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	lines := []string{
		`{"type":"summary","cwd":"/tmp/myproject"}`,
		`{"type":"user","message":{"content":"hello"}}`,
	}
	jsonlPath := filepath.Join(projectDir, "abc123.jsonl")
	if err := os.WriteFile(jsonlPath, []byte(strings.Join(lines, "\n")), 0o600); err != nil {
		t.Fatal(err)
	}

	sessions, err := LoadClaude("", false, false)
	if err != nil {
		t.Fatalf("LoadClaude error: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].ID != "abc123" {
		t.Errorf("session ID = %q, want \"abc123\"", sessions[0].ID)
	}
}

func TestLoadOpencodeJSON_SortedByTimeDesc(t *testing.T) {
	base := t.TempDir()
	sessionDir := filepath.Join(base, "session", "global")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeJSONSession(t, sessionDir, "old", "Old", t.TempDir(), 1_600_000_000_000)
	writeJSONSession(t, sessionDir, "new", "New", t.TempDir(), 1_700_000_000_000)

	sessions, err := loadOpencodeJSON(base, "", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}
	if sessions[0].Title != "New" {
		t.Errorf("first session should be newest, got %q", sessions[0].Title)
	}
}
