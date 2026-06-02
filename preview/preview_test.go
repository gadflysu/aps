package preview

import (
	"bytes"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stripANSI removes ANSI escape sequences so plain text can be asserted.
func stripANSI(s string) string {
	var out strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '\033' && i+1 < len(s) && s[i+1] == '[' {
			for i < len(s) && s[i] != 'm' {
				i++
			}
			i++ // skip 'm'
		} else {
			out.WriteByte(s[i])
			i++
		}
	}
	return out.String()
}

// writeJSONL creates a minimal JSONL session file for testing.
func writeJSONL(t *testing.T, dir, sessionID, userMsg string) {
	t.Helper()
	line := `{"type":"user","message":{"content":"` + userMsg + `"}}` + "\n"
	p := filepath.Join(dir, sessionID+".jsonl")
	if err := os.WriteFile(p, []byte(line), 0600); err != nil {
		t.Fatal(err)
	}
}

// --- RenderClaude ---

func TestRenderClaude_SectionHeaders(t *testing.T) {
	dir := t.TempDir()
	writeJSONL(t, dir, "ses1", "hello world")

	var buf bytes.Buffer
	RenderClaude(&buf, "ses1", dir, "/work/dir")
	plain := stripANSI(buf.String())

	for _, want := range []string{"SESSION INFO", "DIRECTORY LIST"} {
		if !strings.Contains(plain, want) {
			t.Errorf("output missing section header %q\noutput:\n%s", want, plain)
		}
	}
}

func TestRenderClaude_FieldLabels(t *testing.T) {
	dir := t.TempDir()
	writeJSONL(t, dir, "ses2", "test message")

	var buf bytes.Buffer
	RenderClaude(&buf, "ses2", dir, "/some/path")
	plain := stripANSI(buf.String())

	for _, want := range []string{"Title:", "Time:", "Messages:", "Directory:"} {
		if !strings.Contains(plain, want) {
			t.Errorf("output missing field label %q\noutput:\n%s", want, plain)
		}
	}
}

func TestRenderClaude_WorkingDirInOutput(t *testing.T) {
	dir := t.TempDir()
	writeJSONL(t, dir, "ses3", "test")

	var buf bytes.Buffer
	RenderClaude(&buf, "ses3", dir, "/expected/workdir")
	plain := stripANSI(buf.String())

	if !strings.Contains(plain, "/expected/workdir") {
		t.Errorf("output missing working directory\noutput:\n%s", plain)
	}
}

func TestRenderClaude_RecentMessagesSection(t *testing.T) {
	dir := t.TempDir()
	writeJSONL(t, dir, "ses4", "recent message content")

	var buf bytes.Buffer
	RenderClaude(&buf, "ses4", dir, "/tmp")
	plain := stripANSI(buf.String())

	if !strings.Contains(plain, "RECENT MESSAGES") {
		t.Errorf("output missing RECENT MESSAGES section\noutput:\n%s", plain)
	}
	if !strings.Contains(plain, "recent message content") {
		t.Errorf("output missing message text\noutput:\n%s", plain)
	}
}

func TestRenderClaude_MissingJSONL_NoSessionInfo(t *testing.T) {
	// When JSONL file doesn't exist, SESSION INFO block still renders (with "Untitled").
	dir := t.TempDir()

	var buf bytes.Buffer
	RenderClaude(&buf, "nonexistent", dir, "/tmp")
	plain := stripANSI(buf.String())

	if !strings.Contains(plain, "SESSION INFO") {
		t.Errorf("output missing SESSION INFO even for missing JSONL\noutput:\n%s", plain)
	}
	if !strings.Contains(plain, "Untitled") {
		t.Errorf("output missing Untitled fallback title\noutput:\n%s", plain)
	}
}

// --- RenderOpencode ---

func TestRenderOpencode_NoDB_WritesDirectoryListHeader(t *testing.T) {
	// With no opencode DB, should still write the DIRECTORY LIST header.
	t.Setenv("OPENCODE_DATA_DIR", t.TempDir()) // empty dir — no opencode.db

	var buf bytes.Buffer
	RenderOpencode(&buf, "any-id", t.TempDir())
	plain := stripANSI(buf.String())

	if !strings.Contains(plain, "DIRECTORY LIST") {
		t.Errorf("expected DIRECTORY LIST header even without DB\noutput:\n%s", plain)
	}
}

// --- listDir ---

func TestListDir_NonExistentDir_WritesErrorMessage(t *testing.T) {
	var buf bytes.Buffer
	listDir(&buf, "/this/path/does/not/exist/ever")
	plain := stripANSI(buf.String())

	if !strings.Contains(plain, "directory not found") {
		t.Errorf("expected 'directory not found' message\noutput:\n%s", plain)
	}
}

// --- Section render functions ---

func TestClaudeInfo_ContainsAllFields(t *testing.T) {
	dir := t.TempDir()
	writeJSONL(t, dir, "s1", "hello")

	plain := stripANSI(ClaudeInfo("s1", dir, "/work/path"))

	for _, want := range []string{"Title:", "Time:", "Turns:", "Directory:", "/work/path"} {
		if !strings.Contains(plain, want) {
			t.Errorf("ClaudeInfo missing %q\noutput:\n%s", want, plain)
		}
	}
}

func TestClaudeMsgs_ReturnsMessages(t *testing.T) {
	dir := t.TempDir()
	writeJSONL(t, dir, "s2", "recent message text")

	plain := stripANSI(ClaudeMsgs("s2", dir))

	if !strings.Contains(plain, "recent message text") {
		t.Errorf("ClaudeMsgs missing message text\noutput:\n%s", plain)
	}
}

func TestClaudeMsgs_EmptyWhenNoJSONL(t *testing.T) {
	result := ClaudeMsgs("nonexistent", t.TempDir())
	if result != "" {
		t.Errorf("ClaudeMsgs expected empty string for missing JSONL, got %q", result)
	}
}

func TestOpencodeInfo_EmptyWhenNoDB(t *testing.T) {
	t.Setenv("OPENCODE_DATA_DIR", t.TempDir())
	result := OpencodeInfo("any-id", "/some/dir")
	if result != "" {
		t.Errorf("OpencodeInfo expected empty string when no DB, got %q", result)
	}
}

func TestDirListing_ExistingDir_ReturnsContent(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "testfile.txt"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	result := DirListing(dir)
	if result == "" {
		t.Error("DirListing returned empty for existing directory")
	}
}

func TestDirListing_NonExistentDir_ReturnsErrorMessage(t *testing.T) {
	plain := stripANSI(DirListing("/no/such/path/ever"))
	if !strings.Contains(plain, "directory not found") {
		t.Errorf("DirListing missing error message\noutput:\n%s", plain)
	}
}

// --- formatTimestamp ---

func TestFormatTimestamp_Invalid(t *testing.T) {
	got := formatTimestamp(sql.NullFloat64{Valid: false})
	if got != "Unknown" {
		t.Errorf("formatTimestamp invalid = %q, want \"Unknown\"", got)
	}
}

func TestFormatTimestamp_Seconds(t *testing.T) {
	got := formatTimestamp(sql.NullFloat64{Valid: true, Float64: 1_700_000_000})
	if got == "Unknown" || got == "" {
		t.Errorf("formatTimestamp seconds returned %q, want a date string", got)
	}
}

func TestFormatTimestamp_Milliseconds(t *testing.T) {
	// ms value > 9_999_999_999 should be divided by 1000
	got := formatTimestamp(sql.NullFloat64{Valid: true, Float64: 1_700_000_000_000})
	want := formatTimestamp(sql.NullFloat64{Valid: true, Float64: 1_700_000_000})
	if got != want {
		t.Errorf("formatTimestamp ms = %q, want %q", got, want)
	}
}

// --- opencodeDBPath ---

func TestOpencodeDBPath_EnvSetNoDB(t *testing.T) {
	t.Setenv("OPENCODE_DATA_DIR", t.TempDir()) // dir exists but no opencode.db
	got := opencodeDBPath()
	if got != "" {
		t.Errorf("opencodeDBPath with no db = %q, want \"\"", got)
	}
}

func TestOpencodeDBPath_EnvSetWithDB(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("OPENCODE_DATA_DIR", dir)
	dbFile := filepath.Join(dir, "opencode.db")
	if err := os.WriteFile(dbFile, []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}
	got := opencodeDBPath()
	if got != dbFile {
		t.Errorf("opencodeDBPath with db = %q, want %q", got, dbFile)
	}
}

// --- parseJSONLPreview with unified predicate ---

func TestParseJSONLPreview_ArrayContent(t *testing.T) {
	// array-style content with text blocks counts as a real user turn (issue #28)
	line := `{"type":"user","message":{"content":[{"type":"text","text":"array content"}]}}` + "\n"
	p := filepath.Join(t.TempDir(), "s.jsonl")
	if err := os.WriteFile(p, []byte(line), 0600); err != nil {
		t.Fatal(err)
	}
	title, count, msgs := parseJSONLPreview(p)
	if title != "array content" {
		t.Errorf("title = %q, want \"array content\"", title)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1 (array with text block is a real turn)", count)
	}
	if len(msgs) == 0 || msgs[0] != "array content" {
		t.Errorf("msgs = %v, want [\"array content\"]", msgs)
	}
}

func TestParseJSONLPreview_ArrayContentSkipsNonText(t *testing.T) {
	line := `{"type":"user","message":{"content":[{"type":"tool_use","text":"ignored"},{"type":"text","text":"real"}]}}` + "\n"
	p := filepath.Join(t.TempDir(), "s.jsonl")
	if err := os.WriteFile(p, []byte(line), 0600); err != nil {
		t.Fatal(err)
	}
	title, _, _ := parseJSONLPreview(p)
	if title != "real" {
		t.Errorf("title = %q, want \"real\"", title)
	}
}

func TestParseJSONLPreview_IsMetaSkipped(t *testing.T) {
	// isMeta:true records (skill injections) must not appear in messages or count.
	lines := `{"type":"user","isMeta":true,"message":{"content":"Base directory for this skill: /some/path\n\n# Skill Content"}}` + "\n" +
		`{"type":"user","message":{"content":"real user message"}}` + "\n"
	p := filepath.Join(t.TempDir(), "s.jsonl")
	if err := os.WriteFile(p, []byte(lines), 0600); err != nil {
		t.Fatal(err)
	}
	title, count, msgs := parseJSONLPreview(p)
	if count != 1 {
		t.Errorf("count = %d, want 1 (isMeta record must not be counted)", count)
	}
	if title != "real user message" {
		t.Errorf("title = %q, want \"real user message\"", title)
	}
	for _, m := range msgs {
		if strings.Contains(m, "Base directory") {
			t.Errorf("isMeta message leaked into msgs: %q", m)
		}
	}
}

func TestParseJSONLPreview_ToolResultNotCounted(t *testing.T) {
	lines := `{"type":"user","message":{"content":"real message"}}` + "\n" +
		`{"type":"user","message":{"content":[{"type":"tool_result","content":[{"type":"text","text":"result"}]}]}}` + "\n" +
		`{"type":"user","message":{"content":"another real message"}}` + "\n"
	p := filepath.Join(t.TempDir(), "s.jsonl")
	if err := os.WriteFile(p, []byte(lines), 0600); err != nil {
		t.Fatal(err)
	}
	_, count, _ := parseJSONLPreview(p)
	if count != 2 {
		t.Errorf("count = %d, want 2 (tool result must not be counted)", count)
	}
}

func TestParseJSONLPreview_NoMessageField(t *testing.T) {
	// user record with no "message" key — not a real message, should not count
	line := `{"type":"user"}` + "\n"
	p := filepath.Join(t.TempDir(), "s.jsonl")
	if err := os.WriteFile(p, []byte(line), 0600); err != nil {
		t.Fatal(err)
	}
	title, count, msgs := parseJSONLPreview(p)
	if count != 0 {
		t.Errorf("count = %d, want 0", count)
	}
	if title != "Untitled" {
		t.Errorf("title = %q, want \"Untitled\"", title)
	}
	if len(msgs) != 0 {
		t.Errorf("msgs = %v, want empty", msgs)
	}
}

func TestParseJSONLPreview_LongMessageTruncated(t *testing.T) {
	longMsg := strings.Repeat("x", 100)
	line := `{"type":"user","message":{"content":"` + longMsg + `"}}` + "\n"
	p := filepath.Join(t.TempDir(), "s.jsonl")
	if err := os.WriteFile(p, []byte(line), 0600); err != nil {
		t.Fatal(err)
	}
	_, _, msgs := parseJSONLPreview(p)
	if len(msgs) == 0 {
		t.Fatal("expected at least one message")
	}
	if len([]rune(msgs[0])) > 80 {
		t.Errorf("message not truncated to 80 runes, got %d", len([]rune(msgs[0])))
	}
}

// --- Unified predicate tests (issue #30) ---

func TestParseJSONLPreview_CountEqualsMessageCount(t *testing.T) {
	// Mixed JSONL: some countable, some not. Count must equal len(msgs).
	lines := []string{
		`{"type":"user","message":{"content":"plain prompt"}}`,
		`{"type":"user","isMeta":true,"message":{"content":"hidden"}}`,
		`{"type":"user","message":{"content":"<local-command-caveat>caveat</local-command-caveat>"}}`,
		`{"type":"user","message":{"content":"<local-command-stdout>output</local-command-stdout>"}}`,
		`{"type":"user","message":{"content":"<bash-stdout>output</bash-stdout>"}}`,
		`{"type":"user","message":{"content":"<task-notification>done</task-notification>"}}`,
		`{"type":"user","message":{"content":"<system-reminder>reminder</system-reminder>"}}`,
		`{"type":"user","message":{"content":"[Request interrupted by user]"}}`,
		`{"type":"user","message":{"content":[{"type":"tool_result","content":[]}]}}`,
		`{"type":"user","message":{"content":"second real message"}}`,
		`{"type":"user","message":{"content":[{"type":"text","text":"array text"}]}}`,
	}
	p := filepath.Join(t.TempDir(), "s.jsonl")
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	_, count, msgs := parseJSONLPreview(p)
	if count != len(msgs) {
		t.Errorf("count (%d) != len(msgs) (%d)", count, len(msgs))
	}
	if count != 3 {
		t.Errorf("count = %d, want 3 (plain, second, array)", count)
	}
}

func TestParseJSONLPreview_RequestInterruptArrayNotCounted(t *testing.T) {
	// Issue #30: [Request interrupted by user for tool use] in array must not count.
	lines := []string{
		`{"type":"user","message":{"content":"real message"}}`,
		`{"type":"user","message":{"content":[{"type":"text","text":"[Request interrupted by user for tool use]"}]}}`,
		`{"type":"user","message":{"content":"another real"}}`,
	}
	p := filepath.Join(t.TempDir(), "s.jsonl")
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	_, count, msgs := parseJSONLPreview(p)
	if count != 2 {
		t.Errorf("count = %d, want 2 (interrupt array must not count)", count)
	}
	if count != len(msgs) {
		t.Errorf("count (%d) != len(msgs) (%d)", count, len(msgs))
	}
}

func TestParseJSONLPreview_CommandMessageFormatsCorrectly(t *testing.T) {
	lines := []string{
		`{"type":"user","message":{"content":"<command-message>/review</command-message>\n<command-name>/review</command-name>\n<command-args>#29</command-args>"}}`,
		`{"type":"user","message":{"content":"plain text"}}`,
	}
	p := filepath.Join(t.TempDir(), "s.jsonl")
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	_, count, msgs := parseJSONLPreview(p)
	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}
	if len(msgs) != 2 {
		t.Fatalf("len(msgs) = %d, want 2", len(msgs))
	}
	// msgs are reversed (newest first)
	if msgs[0] != "plain text" {
		t.Errorf("msgs[0] = %q, want %q", msgs[0], "plain text")
	}
	if msgs[1] != "/review #29" {
		t.Errorf("msgs[1] = %q, want %q", msgs[1], "/review #29")
	}
}

// --- Session 33acf421 verification (issue #30) ---

func TestParseJSONLPreview_Session33acf421_CountMatchesMsgs(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot get home dir")
	}
	jsonlPath := filepath.Join(home, ".claude", "projects", "-Users-dsu-projects-local-aps", "33acf421-6fec-4ff6-a090-987c0cec924a.jsonl")
	if _, err := os.Stat(jsonlPath); os.IsNotExist(err) {
		t.Skip("session file not found")
	}
	_, count, msgs := parseJSONLPreview(jsonlPath)
	if count != 6 {
		t.Errorf("session 33acf421 count = %d, want 6", count)
	}
	if count != len(msgs) {
		t.Errorf("count (%d) != len(msgs) (%d)", count, len(msgs))
	}
}
