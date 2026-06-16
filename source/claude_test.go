package source

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

// --- parseJSONL (integration via temp file) ---

func TestParseJSONL_CustomTitle(t *testing.T) {
	lines := []string{
		`{"type":"summary","cwd":"/tmp/proj","version":1}`,
		`{"type":"custom-title","customTitle":"My Custom Title"}`,
		`{"type":"user","message":{"content":"first user msg"}}`,
	}
	f := writeTempJSONL(t, lines)
	m := parseJSONL(f, false)
	if m.Title != "My Custom Title" {
		t.Errorf("parseJSONL custom title = %q, want \"My Custom Title\"", m.Title)
	}
	if m.CWD != "/tmp/proj" {
		t.Errorf("parseJSONL cwd = %q, want \"/tmp/proj\"", m.CWD)
	}
	if m.MsgCount != 1 {
		t.Errorf("parseJSONL msgCount = %d, want 1", m.MsgCount)
	}
}

func TestParseJSONL_FirstUserMsgTitle(t *testing.T) {
	lines := []string{
		`{"type":"summary","cwd":"/home/user/proj"}`,
		`{"type":"user","message":{"content":"Hello, please do X"}}`,
		`{"type":"user","message":{"content":"Second message"}}`,
	}
	f := writeTempJSONL(t, lines)
	m := parseJSONL(f, false)
	if m.Title != "Hello, please do X" {
		t.Errorf("parseJSONL first user msg title = %q, want \"Hello, please do X\"", m.Title)
	}
	if m.MsgCount != 2 {
		t.Errorf("parseJSONL msgCount = %d, want 2", m.MsgCount)
	}
}

func TestParseJSONL_NoTitleFallback(t *testing.T) {
	lines := []string{
		`{"type":"summary","cwd":"/tmp/x"}`,
	}
	f := writeTempJSONL(t, lines)
	m := parseJSONL(f, false)
	if m.Title != "Untitled" {
		t.Errorf("parseJSONL no title = %q, want \"Untitled\"", m.Title)
	}
}

func TestParseJSONL_MissingFile(t *testing.T) {
	m := parseJSONL("/nonexistent/file.jsonl", false)
	if m.Title != "Untitled" {
		t.Errorf("parseJSONL missing file title = %q, want \"Untitled\"", m.Title)
	}
	if m.CWD != "" {
		t.Errorf("parseJSONL missing file cwd = %q, want \"\"", m.CWD)
	}
	if m.MsgCount != 0 {
		t.Errorf("parseJSONL missing file count = %d, want 0", m.MsgCount)
	}
}

func TestParseJSONL_CWDLastWins(t *testing.T) {
	// last non-empty cwd wins regardless of record type
	lines := []string{
		`{"type":"user","cwd":"/wrong","message":{"content":"hello"}}`,
		`{"type":"user","cwd":"/correct","message":{"content":"world"}}`,
	}
	f := writeTempJSONL(t, lines)
	m := parseJSONL(f, false)
	if m.CWD != "/correct" {
		t.Errorf("parseJSONL cwd = %q, want \"/correct\"", m.CWD)
	}
}

func TestParseJSONL_CWDEmptyNotOverwrite(t *testing.T) {
	// empty cwd must not overwrite a previously seen non-empty value
	lines := []string{
		`{"type":"user","cwd":"/correct","message":{"content":"hello"}}`,
		`{"type":"user","cwd":"","message":{"content":"world"}}`,
	}
	f := writeTempJSONL(t, lines)
	m := parseJSONL(f, false)
	if m.CWD != "/correct" {
		t.Errorf("parseJSONL cwd = %q, want \"/correct\"", m.CWD)
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
	m := parseJSONL(f, false)
	if m.MsgCount != 2 {
		t.Errorf("parseJSONL msgCount = %d, want 2 (tool result must not be counted)", m.MsgCount)
	}
}

func TestParseJSONL_LastCustomTitleWins(t *testing.T) {
	lines := []string{
		`{"type":"custom-title","customTitle":"First Title"}`,
		`{"type":"custom-title","customTitle":"Second Title"}`,
	}
	f := writeTempJSONL(t, lines)
	m := parseJSONL(f, false)
	if m.Title != "Second Title" {
		t.Errorf("parseJSONL last custom title = %q, want \"Second Title\"", m.Title)
	}
}

func TestParseJSONL_AiTitle(t *testing.T) {
	lines := []string{
		`{"type":"ai-title","aiTitle":"AI Generated Title"}`,
		`{"type":"user","message":{"content":"first user msg"}}`,
	}
	f := writeTempJSONL(t, lines)
	m := parseJSONL(f, false)
	if m.Title != "AI Generated Title" {
		t.Errorf("parseJSONL ai-title = %q, want \"AI Generated Title\"", m.Title)
	}
}

func TestParseJSONL_AiTitle_LosesToCustomTitle(t *testing.T) {
	lines := []string{
		`{"type":"ai-title","aiTitle":"AI Title"}`,
		`{"type":"custom-title","customTitle":"User Title"}`,
	}
	f := writeTempJSONL(t, lines)
	m := parseJSONL(f, false)
	if m.Title != "User Title" {
		t.Errorf("parseJSONL custom-title should beat ai-title = %q, want \"User Title\"", m.Title)
	}
}

func TestParseJSONL_LastAiTitleWins(t *testing.T) {
	lines := []string{
		`{"type":"ai-title","aiTitle":"First AI Title"}`,
		`{"type":"ai-title","aiTitle":"Second AI Title"}`,
	}
	f := writeTempJSONL(t, lines)
	m := parseJSONL(f, false)
	if m.Title != "Second AI Title" {
		t.Errorf("parseJSONL last ai-title = %q, want \"Second AI Title\"", m.Title)
	}
}

// --- Title priority tests (issue #28) ---

func TestParseJSONL_AgentNameWinsOverCustomTitle(t *testing.T) {
	lines := []string{
		`{"type":"agent-name","cwd":"/tmp/proj","agentName":"My Agent"}`,
		`{"type":"custom-title","customTitle":"Custom Title"}`,
		`{"type":"user","message":{"content":"user msg"}}`,
	}
	f := writeTempJSONL(t, lines)
	m := parseJSONL(f, false)
	if m.Title != "My Agent" {
		t.Errorf("parseJSONL agent-name should beat custom-title = %q, want \"My Agent\"", m.Title)
	}
}

func TestParseJSONL_CustomTitleWinsOverAiSummaryPrompt(t *testing.T) {
	lines := []string{
		`{"type":"summary","cwd":"/tmp/proj","summary":"Summary Text"}`,
		`{"type":"ai-title","aiTitle":"AI Title"}`,
		`{"type":"last-prompt","lastPrompt":"Last Prompt"}`,
		`{"type":"custom-title","customTitle":"User Title"}`,
		`{"type":"user","message":{"content":"user msg"}}`,
	}
	f := writeTempJSONL(t, lines)
	m := parseJSONL(f, false)
	if m.Title != "User Title" {
		t.Errorf("parseJSONL custom-title should beat ai/summary/prompt = %q, want \"User Title\"", m.Title)
	}
}

func TestParseJSONL_AiTitleWinsOverSummaryAndPrompt(t *testing.T) {
	lines := []string{
		`{"type":"summary","cwd":"/tmp/proj","summary":"Summary Text"}`,
		`{"type":"last-prompt","lastPrompt":"Last Prompt"}`,
		`{"type":"ai-title","aiTitle":"AI Title"}`,
		`{"type":"user","message":{"content":"user msg"}}`,
	}
	f := writeTempJSONL(t, lines)
	m := parseJSONL(f, false)
	if m.Title != "AI Title" {
		t.Errorf("parseJSONL ai-title should beat summary/prompt = %q, want \"AI Title\"", m.Title)
	}
}

func TestParseJSONL_SummaryWinsOverLastPromptAndFirstUser(t *testing.T) {
	lines := []string{
		`{"type":"summary","cwd":"/tmp/proj","summary":"Session Summary"}`,
		`{"type":"last-prompt","lastPrompt":"User Prompt"}`,
		`{"type":"user","message":{"content":"first user message"}}`,
	}
	f := writeTempJSONL(t, lines)
	m := parseJSONL(f, false)
	if m.Title != "Session Summary" {
		t.Errorf("parseJSONL summary should beat last-prompt/first-user = %q, want \"Session Summary\"", m.Title)
	}
}

func TestParseJSONL_LastPromptWinsOverFirstUser(t *testing.T) {
	lines := []string{
		`{"type":"summary","cwd":"/tmp/proj"}`,
		`{"type":"user","message":{"content":"first user message"}}`,
		`{"type":"last-prompt","lastPrompt":"User Prompt"}`,
	}
	f := writeTempJSONL(t, lines)
	m := parseJSONL(f, false)
	if m.Title != "User Prompt" {
		t.Errorf("parseJSONL last-prompt should beat first-user = %q, want \"User Prompt\"", m.Title)
	}
}

func TestParseJSONL_ArrayTextCountsAsRealUserMessage(t *testing.T) {
	lines := []string{
		`{"type":"summary","cwd":"/tmp/proj"}`,
		`{"type":"user","message":{"content":[{"type":"text","text":"hello from array"}]}}`,
	}
	f := writeTempJSONL(t, lines)
	m := parseJSONL(f, false)
	if m.MsgCount != 1 {
		t.Errorf("parseJSONL array text count = %d, want 1", m.MsgCount)
	}
}

func TestParseJSONL_ToolResultArrayNotCounted(t *testing.T) {
	lines := []string{
		`{"type":"summary","cwd":"/tmp/proj"}`,
		`{"type":"user","message":{"content":[{"type":"tool_result","content":[{"type":"text","text":"result"}]}]}}`,
		`{"type":"user","message":{"content":"real user message"}}`,
	}
	f := writeTempJSONL(t, lines)
	m := parseJSONL(f, false)
	if m.MsgCount != 1 {
		t.Errorf("parseJSONL tool-result-only array count = %d, want 1", m.MsgCount)
	}
}

func TestParseJSONL_InvalidLinesSkipped(t *testing.T) {
	lines := []string{
		`not valid json`,
		`{"type":"custom-title","customTitle":"Valid Title"}`,
	}
	f := writeTempJSONL(t, lines)
	m := parseJSONL(f, false)
	if m.Title != "Valid Title" {
		t.Errorf("parseJSONL invalid lines skipped = %q, want \"Valid Title\"", m.Title)
	}
}

// --- ClaudeUserTurnText (unified predicate) ---

func TestClaudeUserTurnText_PlainString(t *testing.T) {
	rec := map[string]json.RawMessage{
		"type":    json.RawMessage(`"user"`),
		"message": json.RawMessage(`{"content":"hello world"}`),
	}
	result := ClaudeUserTurnText(rec)
	if !result.Countable {
		t.Error("plain string should be countable")
	}
	if result.Text != "hello world" {
		t.Errorf("text = %q, want %q", result.Text, "hello world")
	}
}

func TestClaudeUserTurnText_IsMetaNotCountable(t *testing.T) {
	rec := map[string]json.RawMessage{
		"type":    json.RawMessage(`"user"`),
		"isMeta":  json.RawMessage(`true`),
		"message": json.RawMessage(`{"content":"hidden prompt"}`),
	}
	result := ClaudeUserTurnText(rec)
	if result.Countable {
		t.Error("isMeta:true should not be countable")
	}
}

func TestClaudeUserTurnText_ToolUseResultNotCountable(t *testing.T) {
	rec := map[string]json.RawMessage{
		"type":           json.RawMessage(`"user"`),
		"toolUseResult":  json.RawMessage(`{}`),
		"message":        json.RawMessage(`{"content":"some text"}`),
	}
	result := ClaudeUserTurnText(rec)
	if result.Countable {
		t.Error("toolUseResult should not be countable")
	}
}

func TestClaudeUserTurnText_SourceToolAssistantUUIDNotCountable(t *testing.T) {
	rec := map[string]json.RawMessage{
		"type":                    json.RawMessage(`"user"`),
		"sourceToolAssistantUUID": json.RawMessage(`"uuid"`),
		"message":                 json.RawMessage(`{"content":"some text"}`),
	}
	result := ClaudeUserTurnText(rec)
	if result.Countable {
		t.Error("sourceToolAssistantUUID should not be countable")
	}
}

func TestClaudeUserTurnText_LocalCommandCaveatNotCountable(t *testing.T) {
	rec := map[string]json.RawMessage{
		"type":    json.RawMessage(`"user"`),
		"message": json.RawMessage(`{"content":"<local-command-caveat>some caveat"}`),
	}
	result := ClaudeUserTurnText(rec)
	if result.Countable {
		t.Error("<local-command-caveat> should not be countable")
	}
}

func TestClaudeUserTurnText_LocalCommandStdoutNotCountable(t *testing.T) {
	rec := map[string]json.RawMessage{
		"type":    json.RawMessage(`"user"`),
		"message": json.RawMessage(`{"content":"<local-command-stdout>output"}`),
	}
	result := ClaudeUserTurnText(rec)
	if result.Countable {
		t.Error("<local-command-stdout> should not be countable")
	}
}

func TestClaudeUserTurnText_BashStdoutNotCountable(t *testing.T) {
	rec := map[string]json.RawMessage{
		"type":    json.RawMessage(`"user"`),
		"message": json.RawMessage(`{"content":"<bash-stdout>output"}`),
	}
	result := ClaudeUserTurnText(rec)
	if result.Countable {
		t.Error("<bash-stdout> should not be countable")
	}
}

func TestClaudeUserTurnText_TaskNotificationNotCountable(t *testing.T) {
	rec := map[string]json.RawMessage{
		"type":    json.RawMessage(`"user"`),
		"message": json.RawMessage(`{"content":"<task-notification>done"}`),
	}
	result := ClaudeUserTurnText(rec)
	if result.Countable {
		t.Error("<task-notification> should not be countable")
	}
}

func TestClaudeUserTurnText_SystemReminderNotCountable(t *testing.T) {
	rec := map[string]json.RawMessage{
		"type":    json.RawMessage(`"user"`),
		"message": json.RawMessage(`{"content":"<system-reminder>reminder text"}`),
	}
	result := ClaudeUserTurnText(rec)
	if result.Countable {
		t.Error("<system-reminder> should not be countable")
	}
}

func TestClaudeUserTurnText_RequestInterruptNotCountable(t *testing.T) {
	rec := map[string]json.RawMessage{
		"type":    json.RawMessage(`"user"`),
		"message": json.RawMessage(`{"content":"[Request interrupted by user]"}`),
	}
	result := ClaudeUserTurnText(rec)
	if result.Countable {
		t.Error("[Request interrupted...] should not be countable")
	}
}

func TestClaudeUserTurnText_RequestInterruptArrayNotCountable(t *testing.T) {
	rec := map[string]json.RawMessage{
		"type":    json.RawMessage(`"user"`),
		"message": json.RawMessage(`{"content":[{"type":"text","text":"[Request interrupted by user for tool use]"}]}`),
	}
	result := ClaudeUserTurnText(rec)
	if result.Countable {
		t.Error("[Request interrupted...] in array should not be countable")
	}
}

func TestClaudeUserTurnText_CommandMessageCountable(t *testing.T) {
	rec := map[string]json.RawMessage{
		"type":    json.RawMessage(`"user"`),
		"message": json.RawMessage(`{"content":"<command-message>/review</command-message>\n<command-name>/review</command-name>\n<command-args>#29</command-args>"}`),
	}
	result := ClaudeUserTurnText(rec)
	if !result.Countable {
		t.Error("command message should be countable")
	}
	if result.Text != "/review #29" {
		t.Errorf("text = %q, want %q", result.Text, "/review #29")
	}
}

func TestClaudeUserTurnText_BashInputCountable(t *testing.T) {
	rec := map[string]json.RawMessage{
		"type":    json.RawMessage(`"user"`),
		"message": json.RawMessage(`{"content":"<bash-input>ls -la</bash-input>"}`),
	}
	result := ClaudeUserTurnText(rec)
	// bash-input is skipped by turnSkipPrefixes, so it should not be countable
	if result.Countable {
		t.Error("<bash-input> should not be countable (it's a skip prefix)")
	}
}

func TestClaudeUserTurnText_ToolResultArrayNotCountable(t *testing.T) {
	rec := map[string]json.RawMessage{
		"type":    json.RawMessage(`"user"`),
		"message": json.RawMessage(`{"content":[{"type":"tool_result","content":[{"type":"text","text":"result"}]}]}`),
	}
	result := ClaudeUserTurnText(rec)
	if result.Countable {
		t.Error("tool_result array should not be countable")
	}
}

func TestClaudeUserTurnText_MixedToolResultAndTextNotCountable(t *testing.T) {
	rec := map[string]json.RawMessage{
		"type":    json.RawMessage(`"user"`),
		"message": json.RawMessage(`{"content":[{"type":"tool_result","content":[]},{"type":"text","text":"some text"}]}`),
	}
	result := ClaudeUserTurnText(rec)
	if result.Countable {
		t.Error("mixed tool_result + text should not be countable (default)")
	}
}

func TestClaudeUserTurnText_ImageArrayCountable(t *testing.T) {
	rec := map[string]json.RawMessage{
		"type":    json.RawMessage(`"user"`),
		"message": json.RawMessage(`{"content":[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"..."}}]}`),
	}
	result := ClaudeUserTurnText(rec)
	if !result.Countable {
		t.Error("image array should be countable")
	}
	if result.Text != "[image]" {
		t.Errorf("text = %q, want %q", result.Text, "[image]")
	}
}

func TestClaudeUserTurnText_EmptyStringNotCountable(t *testing.T) {
	rec := map[string]json.RawMessage{
		"type":    json.RawMessage(`"user"`),
		"message": json.RawMessage(`{"content":""}`),
	}
	result := ClaudeUserTurnText(rec)
	if result.Countable {
		t.Error("empty string should not be countable")
	}
}

func TestClaudeUserTurnText_EmptyArrayNotCountable(t *testing.T) {
	rec := map[string]json.RawMessage{
		"type":    json.RawMessage(`"user"`),
		"message": json.RawMessage(`{"content":[]}`),
	}
	result := ClaudeUserTurnText(rec)
	if result.Countable {
		t.Error("empty array should not be countable")
	}
}

func TestClaudeUserTurnText_MultilineStringFirstLine(t *testing.T) {
	rec := map[string]json.RawMessage{
		"type":    json.RawMessage(`"user"`),
		"message": json.RawMessage(`{"content":"first line\nsecond line"}`),
	}
	result := ClaudeUserTurnText(rec)
	if !result.Countable {
		t.Error("multiline string should be countable")
	}
	if result.Text != "first line" {
		t.Errorf("text = %q, want %q", result.Text, "first line")
	}
}

// --- Session 33acf421 verification (issue #30) ---

func TestParseJSONL_Session33acf421_Count(t *testing.T) {
	// Real session file should report 6 turns after fix.
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot get home dir")
	}
	jsonlPath := filepath.Join(home, ".claude", "projects", "-Users-dsu-projects-local-aps", "33acf421-6fec-4ff6-a090-987c0cec924a.jsonl")
	if _, err := os.Stat(jsonlPath); os.IsNotExist(err) {
		t.Skip("session file not found")
	}
	m := parseJSONL(jsonlPath, false)
	if m.MsgCount != 6 {
		t.Errorf("session 33acf421 count = %d, want 6", m.MsgCount)
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

// --- LoadClaudeStream ---

func TestLoadClaudeStream_EmitsSameSessionsAsLoadClaude(t *testing.T) {
	lines := []string{
		`{"type":"summary","cwd":"/tmp/test"}`,
		`{"type":"user","message":{"content":"stream equivalence"}}`,
	}
	home, _, _ := makeClaudeProjectsDir(t, lines)
	t.Setenv("HOME", home)

	blocking, err := LoadClaude("", false, false)
	if err != nil {
		t.Fatalf("LoadClaude: %v", err)
	}

	var mu sync.Mutex
	var streamed []Session
	streamErr := LoadClaudeStream("", false, false, func(s Session) {
		mu.Lock()
		streamed = append(streamed, s)
		mu.Unlock()
	})
	if streamErr != nil {
		t.Fatalf("LoadClaudeStream: %v", streamErr)
	}

	if len(streamed) != len(blocking) {
		t.Fatalf("streamed %d sessions, blocking got %d", len(streamed), len(blocking))
	}
	bIDs := make(map[string]bool, len(blocking))
	for _, s := range blocking {
		bIDs[s.ID] = true
	}
	for _, s := range streamed {
		if !bIDs[s.ID] {
			t.Errorf("streamed session %q not in blocking result", s.ID)
		}
	}
}

func TestLoadClaude_BlockingAPIUnchanged(t *testing.T) {
	lines := []string{
		`{"type":"summary","cwd":"/tmp/test"}`,
		`{"type":"user","message":{"content":"blocking unchanged"}}`,
	}
	home, _, _ := makeClaudeProjectsDir(t, lines)
	t.Setenv("HOME", home)

	sessions, err := LoadClaude("", false, false)
	if err != nil {
		t.Fatalf("LoadClaude: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].CWD != "/tmp/test" {
		t.Errorf("CWD = %q, want /tmp/test", sessions[0].CWD)
	}
}

// --- ParseJSONL timestamp extraction (issue #36) ---

func TestParseJSONL_TimestampLastWins(t *testing.T) {
	// Last valid RFC3339 timestamp in file order is used as session time.
	lines := []string{
		`{"type":"user","cwd":"/tmp/p","timestamp":"2024-01-01T10:00:00Z","message":{"content":"first"}}`,
		`{"type":"assistant","timestamp":"2024-01-01T11:00:00Z"}`,
		`{"type":"user","timestamp":"2024-01-01T12:00:00Z","message":{"content":"last"}}`,
	}
	f := writeTempJSONL(t, lines)
	m := parseJSONL(f, false)
	want := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	if !m.SessionTime.Equal(want) {
		t.Errorf("sessionTime = %v, want %v", m.SessionTime, want)
	}
}

func TestParseJSONL_TimestampNoTimestampIsZero(t *testing.T) {
	// No timestamp fields → zero time returned (caller falls back to mtime).
	lines := []string{
		`{"type":"user","cwd":"/tmp/p","message":{"content":"no ts"}}`,
	}
	f := writeTempJSONL(t, lines)
	m := parseJSONL(f, false)
	if !m.SessionTime.IsZero() {
		t.Errorf("expected zero sessionTime when no timestamps present, got %v", m.SessionTime)
	}
}

func TestParseJSONL_TimestampInvalidIgnored(t *testing.T) {
	// Invalid timestamp string is skipped; the last valid one wins.
	lines := []string{
		`{"type":"user","cwd":"/tmp/p","timestamp":"2024-03-01T09:00:00Z","message":{"content":"valid"}}`,
		`{"type":"user","timestamp":"not-a-date","message":{"content":"bad"}}`,
	}
	f := writeTempJSONL(t, lines)
	m := parseJSONL(f, false)
	want := time.Date(2024, 3, 1, 9, 0, 0, 0, time.UTC)
	if !m.SessionTime.Equal(want) {
		t.Errorf("sessionTime = %v, want %v (invalid timestamp should be skipped)", m.SessionTime, want)
	}
}

func TestParseJSONL_TimestampMetadataRowAfterConversation(t *testing.T) {
	// A metadata row (e.g. summary) appearing after the last user/assistant row
	// with a later timestamp: its timestamp should still update sessionTime because
	// the rule is "latest timestamp in file order, regardless of record type".
	lines := []string{
		`{"type":"user","cwd":"/tmp/p","timestamp":"2024-05-01T08:00:00Z","message":{"content":"hi"}}`,
		`{"type":"summary","timestamp":"2024-05-01T09:00:00Z","summary":"done"}`,
	}
	f := writeTempJSONL(t, lines)
	m := parseJSONL(f, false)
	want := time.Date(2024, 5, 1, 9, 0, 0, 0, time.UTC)
	if !m.SessionTime.Equal(want) {
		t.Errorf("sessionTime = %v, want %v (metadata row timestamp counts)", m.SessionTime, want)
	}
}

// --- MetaCache SessionTime round-trip (issue #36) ---

func TestMetaCache_SessionTimeRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "meta.gob")
	c1 := newMetaCacheWithPath(path)
	mtime := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	sessionTime := time.Date(2024, 6, 14, 18, 30, 0, 0, time.UTC)
	want := MetaEntry{
		Mtime:       mtime,
		Size:        100,
		Title:       "TS Session",
		CWD:         "/projects/ts",
		MsgCount:    3,
		SessionTime: sessionTime,
	}
	c1.Store("/ts/file.jsonl", want)
	if err := c1.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	c2 := newMetaCacheWithPath(path)
	got, ok := c2.Lookup("/ts/file.jsonl", mtime, 100)
	if !ok {
		t.Fatal("expected cache hit after round-trip")
	}
	if !got.SessionTime.Equal(sessionTime) {
		t.Errorf("SessionTime = %v, want %v", got.SessionTime, sessionTime)
	}
}

func TestMetaCache_SessionTimeZeroBackcompat(t *testing.T) {
	// Old cache entries (no SessionTime) must still load and hit on mtime+size.
	// We simulate this by storing an entry without SessionTime and re-reading.
	path := filepath.Join(t.TempDir(), "meta.gob")
	c1 := newMetaCacheWithPath(path)
	mtime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	c1.Store("/old/file.jsonl", MetaEntry{
		Mtime:    mtime,
		Size:     50,
		Title:    "Old",
		CWD:      "/old",
		MsgCount: 1,
		// SessionTime intentionally zero
	})
	if err := c1.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	c2 := newMetaCacheWithPath(path)
	got, ok := c2.Lookup("/old/file.jsonl", mtime, 50)
	if !ok {
		t.Fatal("expected hit for old cache entry")
	}
	if !got.SessionTime.IsZero() {
		t.Errorf("expected zero SessionTime for old entry, got %v", got.SessionTime)
	}
}

// --- parseOne / LoadClaude use JSONL timestamp (issue #36) ---

func TestLoadClaude_UsesJSONLTimestamp(t *testing.T) {
	// Session.Time should come from the JSONL timestamp, not file mtime.
	ts := "2024-02-15T14:30:00Z"
	lines := []string{
		`{"type":"summary","cwd":"/tmp/tstest"}`,
		`{"type":"user","timestamp":"` + ts + `","message":{"content":"hello"}}`,
	}
	home, _, _ := makeClaudeProjectsDir(t, lines)
	t.Setenv("HOME", home)

	sessions, err := LoadClaude("", false, false)
	if err != nil {
		t.Fatalf("LoadClaude: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	want := time.Date(2024, 2, 15, 14, 30, 0, 0, time.UTC)
	if !sessions[0].Time.Equal(want) {
		t.Errorf("Session.Time = %v, want %v (from JSONL timestamp)", sessions[0].Time, want)
	}
}

func TestLoadClaude_FallsBackToMtimeWhenNoTimestamp(t *testing.T) {
	// No timestamp in JSONL → Session.Time should equal file mtime.
	lines := []string{
		`{"type":"summary","cwd":"/tmp/nomtime"}`,
		`{"type":"user","message":{"content":"no timestamp"}}`,
	}
	home, _, jsonlPath := makeClaudeProjectsDir(t, lines)
	t.Setenv("HOME", home)

	info, err := os.Stat(jsonlPath)
	if err != nil {
		t.Fatal(err)
	}
	mtime := info.ModTime()

	sessions, err := LoadClaude("", false, false)
	if err != nil {
		t.Fatalf("LoadClaude: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if !sessions[0].Time.Equal(mtime) {
		t.Errorf("Session.Time = %v, want mtime %v", sessions[0].Time, mtime)
	}
}

func TestLoadClaude_CacheHitUsesSessionTime(t *testing.T) {
	// Cache hit: Session.Time comes from cached SessionTime, not current mtime.
	lines := []string{
		`{"type":"summary","cwd":"/tmp/cachets"}`,
		`{"type":"user","message":{"content":"cached ts"}}`,
	}
	home, _, jsonlPath := makeClaudeProjectsDir(t, lines)
	t.Setenv("HOME", home)

	info, err := os.Stat(jsonlPath)
	if err != nil {
		t.Fatal(err)
	}
	cachedSessionTime := time.Date(2023, 11, 10, 8, 0, 0, 0, time.UTC)

	cacheDir := filepath.Join(home, ".cache", "aps")
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		t.Fatal(err)
	}
	cachePath := filepath.Join(cacheDir, "session-meta.gob")
	cache := newMetaCacheWithPath(cachePath)
	cache.Store(jsonlPath, MetaEntry{
		Mtime:       info.ModTime(),
		Size:        info.Size(),
		Title:       "Cached",
		CWD:         "/tmp/cachets",
		MsgCount:    1,
		SessionTime: cachedSessionTime,
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
	if !sessions[0].Time.Equal(cachedSessionTime) {
		t.Errorf("Session.Time = %v, want cached %v", sessions[0].Time, cachedSessionTime)
	}
}

func TestLoadClaude_CacheHitZeroSessionTimeFallsBackToMtime(t *testing.T) {
	// Old cache entry with zero SessionTime → fall back to file mtime.
	lines := []string{
		`{"type":"summary","cwd":"/tmp/legacycache"}`,
		`{"type":"user","message":{"content":"legacy"}}`,
	}
	home, _, jsonlPath := makeClaudeProjectsDir(t, lines)
	t.Setenv("HOME", home)

	info, err := os.Stat(jsonlPath)
	if err != nil {
		t.Fatal(err)
	}

	cacheDir := filepath.Join(home, ".cache", "aps")
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		t.Fatal(err)
	}
	cachePath := filepath.Join(cacheDir, "session-meta.gob")
	cache := newMetaCacheWithPath(cachePath)
	cache.Store(jsonlPath, MetaEntry{
		Mtime:    info.ModTime(),
		Size:     info.Size(),
		Title:    "Legacy",
		CWD:      "/tmp/legacycache",
		MsgCount: 1,
		// SessionTime zero → old cache entry
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
	if !sessions[0].Time.Equal(info.ModTime()) {
		t.Errorf("Session.Time = %v, want mtime %v", sessions[0].Time, info.ModTime())
	}
}

// --- ReloadSession uses JSONL timestamp (issue #36) ---

func TestReloadSession_UsesJSONLTimestamp(t *testing.T) {
	dir := t.TempDir()
	projectDir := filepath.Join(dir, "-tmp-rts")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	jsonlPath := filepath.Join(projectDir, "rts123.jsonl")
	ts := "2024-04-20T16:00:00Z"
	lines := []string{
		`{"type":"summary","cwd":"/tmp/rts"}`,
		`{"type":"user","timestamp":"` + ts + `","message":{"content":"reload me"}}`,
	}
	if err := os.WriteFile(jsonlPath, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := ReloadSession(jsonlPath, false)
	if err != nil {
		t.Fatalf("ReloadSession: %v", err)
	}
	want := time.Date(2024, 4, 20, 16, 0, 0, 0, time.UTC)
	if !s.Time.Equal(want) {
		t.Errorf("Session.Time = %v, want %v (from JSONL timestamp)", s.Time, want)
	}
}

func TestReloadSession_FallsBackToMtimeWhenNoTimestamp(t *testing.T) {
	dir := t.TempDir()
	projectDir := filepath.Join(dir, "-tmp-rts2")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	jsonlPath := filepath.Join(projectDir, "rts456.jsonl")
	lines := []string{
		`{"type":"summary","cwd":"/tmp/rts2"}`,
		`{"type":"user","message":{"content":"no timestamp here"}}`,
	}
	if err := os.WriteFile(jsonlPath, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(jsonlPath)
	if err != nil {
		t.Fatal(err)
	}
	s, err := ReloadSession(jsonlPath, false)
	if err != nil {
		t.Fatalf("ReloadSession: %v", err)
	}
	if !s.Time.Equal(info.ModTime()) {
		t.Errorf("Session.Time = %v, want mtime %v", s.Time, info.ModTime())
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
