package preview

import (
	"bytes"
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

	for _, want := range []string{"Title:", "Time:", "Messages:", "Directory:", "/work/path"} {
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
