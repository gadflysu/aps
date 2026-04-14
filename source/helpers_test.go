package source

import (
	"database/sql"
	"os"
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
