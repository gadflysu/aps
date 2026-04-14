package source

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- abbreviateHome ---

func TestAbbreviateHome_WithPrefix(t *testing.T) {
	got := abbreviateHome("/home/user/projects/foo", "/home/user")
	want := "~/projects/foo"
	if got != want {
		t.Errorf("abbreviateHome = %q, want %q", got, want)
	}
}

func TestAbbreviateHome_ExactHome(t *testing.T) {
	got := abbreviateHome("/home/user", "/home/user")
	want := "~"
	if got != want {
		t.Errorf("abbreviateHome(home, home) = %q, want %q", got, want)
	}
}

func TestAbbreviateHome_NoPrefix(t *testing.T) {
	got := abbreviateHome("/other/path", "/home/user")
	if got != "/other/path" {
		t.Errorf("abbreviateHome without prefix = %q, want unchanged", got)
	}
}

// --- sanitize ---

func TestSanitize_TabsReplaced(t *testing.T) {
	got := sanitize("hello\tworld")
	if strings.Contains(got, "\t") {
		t.Errorf("sanitize should replace tabs, got %q", got)
	}
	if got != "hello world" {
		t.Errorf("sanitize(\"hello\\tworld\") = %q, want \"hello world\"", got)
	}
}

func TestSanitize_NewlinesReplaced(t *testing.T) {
	got := sanitize("line1\nline2")
	if strings.Contains(got, "\n") {
		t.Errorf("sanitize should replace newlines, got %q", got)
	}
	if got != "line1 line2" {
		t.Errorf("sanitize(\"line1\\nline2\") = %q, want \"line1 line2\"", got)
	}
}

func TestSanitize_Clean(t *testing.T) {
	got := sanitize("clean string")
	if got != "clean string" {
		t.Errorf("sanitize(clean) = %q, want unchanged", got)
	}
}

// --- truncateStr ---

func TestTruncateStr_WithinLimit(t *testing.T) {
	got := truncateStr("hello", 10)
	if got != "hello" {
		t.Errorf("truncateStr within limit = %q, want \"hello\"", got)
	}
}

func TestTruncateStr_ExactLimit(t *testing.T) {
	s := strings.Repeat("a", 50)
	got := truncateStr(s, 50)
	if got != s {
		t.Errorf("truncateStr at exact limit should return unchanged")
	}
}

func TestTruncateStr_Exceeds(t *testing.T) {
	s := strings.Repeat("a", 55)
	got := truncateStr(s, 50)
	if len([]rune(got)) != 50 {
		t.Errorf("truncateStr exceeds: got len %d, want 50", len([]rune(got)))
	}
}

func TestTruncateStr_CJK(t *testing.T) {
	// 60 CJK chars, truncate to 50
	s := strings.Repeat("字", 60)
	got := truncateStr(s, 50)
	if len([]rune(got)) != 50 {
		t.Errorf("truncateStr CJK: got %d runes, want 50", len([]rune(got)))
	}
}

// --- applyTitleRules ---

func TestApplyTitleRules_Empty(t *testing.T) {
	got := applyTitleRules("")
	if got != "" {
		t.Errorf("applyTitleRules(\"\") = %q, want \"\"", got)
	}
}

func TestApplyTitleRules_Normal(t *testing.T) {
	got := applyTitleRules("  hello world  ")
	if got != "hello world" {
		t.Errorf("applyTitleRules trim = %q, want \"hello world\"", got)
	}
}

func TestApplyTitleRules_SkippedPrefix(t *testing.T) {
	skipped := []string{
		"<local-command-caveat>some thing",
		"<command-message>blah",
		"<command-name>foo",
		"<bash-input>cmd",
		"[Request interrupted by something",
	}
	for _, s := range skipped {
		got := applyTitleRules(s)
		if got != "" {
			t.Errorf("applyTitleRules(%q) = %q, want \"\"", s, got)
		}
	}
}

func TestApplyTitleRules_MultilineUsesFirstLine(t *testing.T) {
	got := applyTitleRules("First line\nSecond line")
	if got != "First line" {
		t.Errorf("applyTitleRules multiline = %q, want \"First line\"", got)
	}
}

func TestApplyTitleRules_ImplementPlanSpecial(t *testing.T) {
	s := "Implement the following plan:\n- Step one\n- Step two"
	got := applyTitleRules(s)
	if !strings.HasPrefix(got, "Plan: ") {
		t.Errorf("applyTitleRules ImplementPlan = %q, want \"Plan: ...\"", got)
	}
	if !strings.Contains(got, "Step one") {
		t.Errorf("applyTitleRules ImplementPlan should include first step, got %q", got)
	}
}

func TestApplyTitleRules_TruncatesLong(t *testing.T) {
	long := strings.Repeat("x", 60)
	got := applyTitleRules(long)
	if len([]rune(got)) > 50 {
		t.Errorf("applyTitleRules should truncate to 50, got %d runes", len([]rune(got)))
	}
}

// --- extractTextFromContent ---

func TestExtractTextFromContent_String(t *testing.T) {
	raw, _ := json.Marshal("hello world")
	got := extractTextFromContent(raw)
	if got != "hello world" {
		t.Errorf("extractTextFromContent(string) = %q, want \"hello world\"", got)
	}
}

func TestExtractTextFromContent_StringWithSkipPrefix(t *testing.T) {
	raw, _ := json.Marshal("<command-message>something")
	got := extractTextFromContent(raw)
	if got != "" {
		t.Errorf("extractTextFromContent with skip prefix = %q, want \"\"", got)
	}
}

func TestExtractTextFromContent_ArrayTextItem(t *testing.T) {
	items := []map[string]any{
		{"type": "text", "text": "  hello from array  "},
	}
	raw, _ := json.Marshal(items)
	got := extractTextFromContent(raw)
	if got != "hello from array" {
		t.Errorf("extractTextFromContent(array text) = %q, want \"hello from array\"", got)
	}
}

func TestExtractTextFromContent_ArraySkipsNonText(t *testing.T) {
	items := []map[string]any{
		{"type": "tool_use", "text": "should be ignored"},
		{"type": "text", "text": "actual content"},
	}
	raw, _ := json.Marshal(items)
	got := extractTextFromContent(raw)
	if got != "actual content" {
		t.Errorf("extractTextFromContent(array skip non-text) = %q, want \"actual content\"", got)
	}
}

func TestExtractTextFromContent_ArrayEmpty(t *testing.T) {
	items := []map[string]any{}
	raw, _ := json.Marshal(items)
	got := extractTextFromContent(raw)
	if got != "" {
		t.Errorf("extractTextFromContent(empty array) = %q, want \"\"", got)
	}
}

// --- parseJSONL (integration via temp file) ---

func TestParseJSONL_CustomTitle(t *testing.T) {
	lines := []string{
		`{"type":"summary","cwd":"/tmp/proj","version":1}`,
		`{"type":"custom-title","customTitle":"My Custom Title"}`,
		`{"type":"user","message":{"content":"first user msg"}}`,
	}
	f := writeTempJSONL(t, lines)
	title, cwd, count := parseJSONL(f, false)
	if title != "My Custom Title" {
		t.Errorf("parseJSONL custom title = %q, want \"My Custom Title\"", title)
	}
	if cwd != "/tmp/proj" {
		t.Errorf("parseJSONL cwd = %q, want \"/tmp/proj\"", cwd)
	}
	if count != 1 {
		t.Errorf("parseJSONL msgCount = %d, want 1", count)
	}
}

func TestParseJSONL_FirstUserMsgTitle(t *testing.T) {
	lines := []string{
		`{"type":"summary","cwd":"/home/user/proj"}`,
		`{"type":"user","message":{"content":"Hello, please do X"}}`,
		`{"type":"user","message":{"content":"Second message"}}`,
	}
	f := writeTempJSONL(t, lines)
	title, _, count := parseJSONL(f, false)
	if title != "Hello, please do X" {
		t.Errorf("parseJSONL first user msg title = %q, want \"Hello, please do X\"", title)
	}
	if count != 2 {
		t.Errorf("parseJSONL msgCount = %d, want 2", count)
	}
}

func TestParseJSONL_NoTitleFallback(t *testing.T) {
	lines := []string{
		`{"type":"summary","cwd":"/tmp/x"}`,
	}
	f := writeTempJSONL(t, lines)
	title, _, _ := parseJSONL(f, false)
	if title != "Untitled" {
		t.Errorf("parseJSONL no title = %q, want \"Untitled\"", title)
	}
}

func TestParseJSONL_MissingFile(t *testing.T) {
	title, cwd, count := parseJSONL("/nonexistent/file.jsonl", false)
	if title != "Untitled" {
		t.Errorf("parseJSONL missing file title = %q, want \"Untitled\"", title)
	}
	if cwd != "" {
		t.Errorf("parseJSONL missing file cwd = %q, want \"\"", cwd)
	}
	if count != 0 {
		t.Errorf("parseJSONL missing file count = %d, want 0", count)
	}
}

func TestParseJSONL_LastCustomTitleWins(t *testing.T) {
	lines := []string{
		`{"type":"custom-title","customTitle":"First Title"}`,
		`{"type":"custom-title","customTitle":"Second Title"}`,
	}
	f := writeTempJSONL(t, lines)
	title, _, _ := parseJSONL(f, false)
	if title != "Second Title" {
		t.Errorf("parseJSONL last custom title = %q, want \"Second Title\"", title)
	}
}

func TestParseJSONL_InvalidLinesSkipped(t *testing.T) {
	lines := []string{
		`not valid json`,
		`{"type":"custom-title","customTitle":"Valid Title"}`,
	}
	f := writeTempJSONL(t, lines)
	title, _, _ := parseJSONL(f, false)
	if title != "Valid Title" {
		t.Errorf("parseJSONL invalid lines skipped = %q, want \"Valid Title\"", title)
	}
}

// writeTempJSONL creates a temp file with the given lines (one per line) and returns its path.
func writeTempJSONL(t *testing.T, lines []string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	content := strings.Join(lines, "\n")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writeTempJSONL: %v", err)
	}
	return path
}
