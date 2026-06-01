package source

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

// --- IsRealUserMsg ---

func TestIsRealUserMsg_StringContent(t *testing.T) {
	rec := map[string]json.RawMessage{
		"message": json.RawMessage(`{"content":"hello"}`),
	}
	if !IsRealUserMsg(rec) {
		t.Error("string content should be real user message")
	}
}

func TestIsRealUserMsg_ArrayContent(t *testing.T) {
	rec := map[string]json.RawMessage{
		"message": json.RawMessage(`{"content":[{"type":"tool_result"}]}`),
	}
	if IsRealUserMsg(rec) {
		t.Error("array content (tool result) should not be real user message")
	}
}

func TestIsRealUserMsg_NoMessageKey(t *testing.T) {
	rec := map[string]json.RawMessage{
		"type": json.RawMessage(`"user"`),
	}
	if IsRealUserMsg(rec) {
		t.Error("missing message key should return false")
	}
}

func TestIsRealUserMsg_InvalidMessage(t *testing.T) {
	rec := map[string]json.RawMessage{
		"message": json.RawMessage(`"not a map"`),
	}
	if IsRealUserMsg(rec) {
		t.Error("non-object message should return false")
	}
}

func TestIsRealUserMsg_NoContentKey(t *testing.T) {
	rec := map[string]json.RawMessage{
		"message": json.RawMessage(`{"role":"user"}`),
	}
	if IsRealUserMsg(rec) {
		t.Error("missing content key should return false")
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

func TestParseJSONL_ToolResultNotCounted(t *testing.T) {
	lines := []string{
		`{"type":"summary","cwd":"/tmp"}`,
		`{"type":"user","message":{"content":"real user message"}}`,
		`{"type":"user","message":{"content":[{"type":"tool_result","content":[{"type":"text","text":"result"}]}]}}`,
		`{"type":"user","message":{"content":"another real message"}}`,
	}
	f := writeTempJSONL(t, lines)
	_, _, count := parseJSONL(f, false)
	if count != 2 {
		t.Errorf("parseJSONL msgCount = %d, want 2 (tool result must not be counted)", count)
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

func TestParseJSONL_AiTitle(t *testing.T) {
	lines := []string{
		`{"type":"ai-title","aiTitle":"AI Generated Title"}`,
		`{"type":"user","message":{"content":"first user msg"}}`,
	}
	f := writeTempJSONL(t, lines)
	title, _, _ := parseJSONL(f, false)
	if title != "AI Generated Title" {
		t.Errorf("parseJSONL ai-title = %q, want \"AI Generated Title\"", title)
	}
}

func TestParseJSONL_AiTitle_LosesToCustomTitle(t *testing.T) {
	lines := []string{
		`{"type":"ai-title","aiTitle":"AI Title"}`,
		`{"type":"custom-title","customTitle":"User Title"}`,
	}
	f := writeTempJSONL(t, lines)
	title, _, _ := parseJSONL(f, false)
	if title != "User Title" {
		t.Errorf("parseJSONL custom-title should beat ai-title = %q, want \"User Title\"", title)
	}
}

func TestParseJSONL_LastAiTitleWins(t *testing.T) {
	lines := []string{
		`{"type":"ai-title","aiTitle":"First AI Title"}`,
		`{"type":"ai-title","aiTitle":"Second AI Title"}`,
	}
	f := writeTempJSONL(t, lines)
	title, _, _ := parseJSONL(f, false)
	if title != "Second AI Title" {
		t.Errorf("parseJSONL last ai-title = %q, want \"Second AI Title\"", title)
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

// --- ReloadSession ---

func TestReloadSession_UpdatesTitleAndCount(t *testing.T) {
	dir := t.TempDir()
	// Arrange: a project dir containing a JSONL file, using /tmp as cwd so no decoding needed.
	projectDir := filepath.Join(dir, "-tmp-proj")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	jsonlPath := filepath.Join(projectDir, "abc123.jsonl")

	initial := []string{
		`{"type":"summary","cwd":"/tmp/proj"}`,
		`{"type":"user","message":{"content":"Hello"}}`,
	}
	if err := os.WriteFile(jsonlPath, []byte(strings.Join(initial, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}

	s1, err := ReloadSession(jsonlPath, false)
	if err != nil {
		t.Fatalf("ReloadSession initial: %v", err)
	}
	if s1.MsgCount != 1 {
		t.Errorf("initial MsgCount = %d, want 1", s1.MsgCount)
	}
	if s1.Title != "Hello" {
		t.Errorf("initial Title = %q, want \"Hello\"", s1.Title)
	}

	// Act: append more content.
	extra := []string{
		`{"type":"user","message":{"content":"Second"}}`,
		`{"type":"custom-title","customTitle":"New Title"}`,
	}
	f, _ := os.OpenFile(jsonlPath, os.O_APPEND|os.O_WRONLY, 0o644)
	f.WriteString("\n" + strings.Join(extra, "\n"))
	f.Close()

	s2, err := ReloadSession(jsonlPath, false)
	if err != nil {
		t.Fatalf("ReloadSession updated: %v", err)
	}
	if s2.MsgCount != 2 {
		t.Errorf("updated MsgCount = %d, want 2", s2.MsgCount)
	}
	if s2.Title != "New Title" {
		t.Errorf("updated Title = %q, want \"New Title\"", s2.Title)
	}
	if !s2.Time.After(s1.Time) && s2.Time != s1.Time {
		// mtime should be >= s1.Time (OS may have 1s resolution)
	}
	if s2.ID != "abc123" {
		t.Errorf("ID = %q, want \"abc123\"", s2.ID)
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

// makeClaudeProjectsDir creates a minimal ~/.claude/projects layout under home
// and returns (home, projectDir, jsonlPath).
func makeClaudeProjectsDir(t *testing.T, lines []string) (home, projectDir, jsonlPath string) {
	t.Helper()
	home = t.TempDir()
	projectDir = filepath.Join(home, ".claude", "projects", "%2Ftmp%2Ftest")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	jsonlPath = filepath.Join(projectDir, "sess1.jsonl")
	if err := os.WriteFile(jsonlPath, []byte(strings.Join(lines, "\n")), 0o600); err != nil {
		t.Fatal(err)
	}
	return home, projectDir, jsonlPath
}

// --- TestLoadClaude_Concurrent ---

func TestLoadClaude_Concurrent(t *testing.T) {
	lines := []string{
		`{"type":"summary","cwd":"/tmp/test"}`,
		`{"type":"user","message":{"content":"concurrent test"}}`,
	}
	home, _, _ := makeClaudeProjectsDir(t, lines)
	t.Setenv("HOME", home)

	// Run LoadClaude twice: result must be identical (order-independent by ID).
	got1, err := LoadClaude("", false, false)
	if err != nil {
		t.Fatalf("LoadClaude first run: %v", err)
	}
	got2, err := LoadClaude("", false, false)
	if err != nil {
		t.Fatalf("LoadClaude second run: %v", err)
	}
	if len(got1) != len(got2) {
		t.Fatalf("result lengths differ: %d vs %d", len(got1), len(got2))
	}
	ids1 := make(map[string]bool, len(got1))
	for _, s := range got1 {
		ids1[s.ID] = true
	}
	for _, s := range got2 {
		if !ids1[s.ID] {
			t.Errorf("session %q in second run not found in first run", s.ID)
		}
	}
}

// --- TestLoadClaude_CacheHit ---

func TestLoadClaude_CacheHit(t *testing.T) {
	lines := []string{
		`{"type":"summary","cwd":"/tmp/test"}`,
		`{"type":"user","message":{"content":"cache hit test"}}`,
	}
	home, _, jsonlPath := makeClaudeProjectsDir(t, lines)
	t.Setenv("HOME", home)

	// Pre-populate cache with known values.
	cacheDir := filepath.Join(home, ".cache", "aps")
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		t.Fatal(err)
	}
	cachePath := filepath.Join(cacheDir, "session-meta.gob")
	cache := newMetaCacheWithPath(cachePath)

	info, err := os.Stat(jsonlPath)
	if err != nil {
		t.Fatal(err)
	}
	cache.Store(jsonlPath, MetaEntry{
		Mtime:    info.ModTime(),
		Size:     info.Size(),
		Title:    "Cached Title",
		CWD:      "/tmp/test",
		MsgCount: 5,
	})
	if err := cache.Save(); err != nil {
		t.Fatal(err)
	}

	sessions, err := LoadClaude("", false, false)
	if err != nil {
		t.Fatalf("LoadClaude: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	// Cache hit: title and msgCount should come from cache, not the file.
	if sessions[0].Title != "Cached Title" {
		t.Errorf("title = %q, want \"Cached Title\" (from cache)", sessions[0].Title)
	}
	if sessions[0].MsgCount != 5 {
		t.Errorf("MsgCount = %d, want 5 (from cache)", sessions[0].MsgCount)
	}
}

// --- TestLoadClaude_CacheMiss ---

func TestLoadClaude_CacheMiss(t *testing.T) {
	lines := []string{
		`{"type":"summary","cwd":"/tmp/test"}`,
		`{"type":"user","message":{"content":"cache miss test"}}`,
	}
	home, _, jsonlPath := makeClaudeProjectsDir(t, lines)
	t.Setenv("HOME", home)

	// Pre-populate cache with stale mtime so it won't match.
	cacheDir := filepath.Join(home, ".cache", "aps")
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		t.Fatal(err)
	}
	cachePath := filepath.Join(cacheDir, "session-meta.gob")
	cache := newMetaCacheWithPath(cachePath)

	info, err := os.Stat(jsonlPath)
	if err != nil {
		t.Fatal(err)
	}
	staleEntry := MetaEntry{
		Mtime:    info.ModTime().Add(-time.Second), // stale mtime → miss
		Size:     info.Size(),
		Title:    "Stale Title",
		CWD:      "/tmp/test",
		MsgCount: 99,
	}
	cache.Store(jsonlPath, staleEntry)
	if err := cache.Save(); err != nil {
		t.Fatal(err)
	}

	sessions, err := LoadClaude("", false, false)
	if err != nil {
		t.Fatalf("LoadClaude: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	// Cache miss: title and msgCount should come from actual file parse.
	if sessions[0].Title == "Stale Title" {
		t.Errorf("title should not be stale cached title, got %q", sessions[0].Title)
	}
	if sessions[0].MsgCount == 99 {
		t.Errorf("MsgCount should not be stale cached value 99, got %d", sessions[0].MsgCount)
	}
	// After LoadClaude, cache must have been updated with fresh entry.
	freshCache := newMetaCacheWithPath(cachePath)
	freshInfo, _ := os.Stat(jsonlPath)
	entry, ok := freshCache.Lookup(jsonlPath, freshInfo.ModTime(), freshInfo.Size())
	if !ok {
		t.Error("cache was not updated after cache miss")
	}
	if entry.Title == "Stale Title" {
		t.Errorf("cache entry title after miss = %q, should be updated from file", entry.Title)
	}
}

// BenchmarkLoadClaude exercises LoadClaude against a temp directory with 20 JSONL files.
// Run with: go test -bench=BenchmarkLoadClaude -benchtime=5s ./source/
func BenchmarkLoadClaude(b *testing.B) {
	home := b.TempDir()
	b.Setenv("HOME", home)

	// Create 20 project dirs each with 1 JSONL file.
	for i := range 20 {
		projectDir := filepath.Join(home, ".claude", "projects", fmt.Sprintf("%%2Ftmp%%2Fproj%d", i))
		if err := os.MkdirAll(projectDir, 0o755); err != nil {
			b.Fatal(err)
		}
		lines := []string{
			fmt.Sprintf(`{"type":"summary","cwd":"/tmp/proj%d"}`, i),
			`{"type":"user","message":{"content":"benchmark session"}}`,
		}
		p := filepath.Join(projectDir, "sessbench.jsonl")
		if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")), 0o600); err != nil {
			b.Fatal(err)
		}
	}

	b.ResetTimer()
	for range b.N {
		sessions, err := LoadClaude("", false, false)
		if err != nil {
			b.Fatal(err)
		}
		if len(sessions) != 20 {
			b.Fatalf("expected 20 sessions, got %d", len(sessions))
		}
	}
}
