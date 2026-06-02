package source

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/gadflysu/aps/filter"
)

var titleSkipPrefixes = []string{
	"<local-command-caveat>",
	"<command-message>",
	"<command-name>",
	"<local-command-stdout>",
	"<bash-input>",
	"<bash-stdout>",
	"<task-notification>",
	"[Request interrupted",
	"[{'type': 'tool_result'",
}

// turnSkipPrefixes are content prefixes that should not count as user turns.
var turnSkipPrefixes = []string{
	"<local-command-caveat>",
	"<local-command-stdout>",
	"<local-command-stderr>",
	"<bash-input>",
	"<bash-stdout>",
	"<bash-stderr>",
	"<task-notification>",
	"<system-reminder>",
	"[Request interrupted",
	"[{'type': 'tool_result'",
}

// LoadClaude returns all Claude Code sessions, optionally filtered by path.
func LoadClaude(pathFilter string, strictMatch bool, verbose bool) ([]Session, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	baseDir := filepath.Join(home, ".claude", "projects")
	if _, err := os.Stat(baseDir); os.IsNotExist(err) {
		return nil, nil
	}

	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return nil, err
	}

	cache := LoadMetaCache()

	// Collect all (jsonlFile, projectDirName) pairs first.
	type fileEntry struct {
		jsonlFile string
		dirName   string // URL-encoded project dir name, used for fallback cwd decode
	}
	var allFiles []fileEntry
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		projectPath := filepath.Join(baseDir, entry.Name())
		jsonlFiles, err := filepath.Glob(filepath.Join(projectPath, "*.jsonl"))
		if err != nil || len(jsonlFiles) == 0 {
			continue
		}
		for _, f := range jsonlFiles {
			allFiles = append(allFiles, fileEntry{jsonlFile: f, dirName: entry.Name()})
		}
	}

	workers := max(1, runtime.NumCPU()/2)
	sem := make(chan struct{}, workers)
	var mu sync.Mutex
	var wg sync.WaitGroup
	var sessions []Session

	for _, fe := range allFiles {
		wg.Add(1)
		sem <- struct{}{}
		go func(fe fileEntry) {
			defer wg.Done()
			defer func() { <-sem }()
			s, ok := parseOne(fe.jsonlFile, fe.dirName, home, pathFilter, strictMatch, verbose, cache)
			if ok {
				mu.Lock()
				sessions = append(sessions, s)
				mu.Unlock()
			}
		}(fe)
	}
	wg.Wait()
	_ = cache.Save()

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].Time.After(sessions[j].Time)
	})

	return sessions, nil
}

// parseOne parses (or cache-hits) a single JSONL file into a Session.
// Returns (session, true) if the file produces a valid session that passes the path filter,
// (zero, false) otherwise.
func parseOne(jsonlFile, dirName, home, pathFilter string, strictMatch, verbose bool, cache *MetaCache) (Session, bool) {
	info, err := os.Stat(jsonlFile)
	if err != nil {
		return Session{}, false
	}
	mtime := info.ModTime()
	size := info.Size()
	sessionID := strings.TrimSuffix(filepath.Base(jsonlFile), ".jsonl")
	projectPath := filepath.Dir(jsonlFile)

	var title, cwd string
	var msgCount int

	if entry, hit := cache.Lookup(jsonlFile, mtime, size); hit {
		title = entry.Title
		cwd = entry.CWD
		msgCount = entry.MsgCount
	} else {
		title, cwd, msgCount = parseJSONL(jsonlFile, verbose)
		if cwd == "" {
			decoded, err := url.PathUnescape(dirName)
			if err != nil || !strings.HasPrefix(decoded, "/") {
				return Session{}, false
			}
			cwd = decoded
		}
		cache.Store(jsonlFile, MetaEntry{
			Mtime:    mtime,
			Size:     size,
			Title:    title,
			CWD:      cwd,
			MsgCount: msgCount,
		})
	}

	if cwd == "" {
		return Session{}, false
	}

	if !filter.Matches(pathFilter, strictMatch, cwd) {
		return Session{}, false
	}

	return Session{
		Client:      ClientClaude,
		ID:          sessionID,
		Title:       title,
		CWD:         cwd,
		CWDDisplay:  abbreviateHome(cwd, home),
		ProjectPath: projectPath,
		Time:        mtime,
		MsgCount:    msgCount,
		jsonlPath:   jsonlFile,
	}, true
}

// ReloadSession re-parses a single JSONL file and returns an updated Session.
// The caller is responsible for providing the correct projectPath (parent dir of jsonlFile).
func ReloadSession(jsonlFile string, verbose bool) (Session, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Session{}, err
	}

	info, err := os.Stat(jsonlFile)
	if err != nil {
		return Session{}, err
	}

	projectPath := filepath.Dir(jsonlFile)
	sessionID := strings.TrimSuffix(filepath.Base(jsonlFile), ".jsonl")
	title, cwd, msgCount := parseJSONL(jsonlFile, verbose)

	if cwd == "" {
		dirName := filepath.Base(projectPath)
		decoded, err := url.PathUnescape(dirName)
		if err != nil || !strings.HasPrefix(decoded, "/") {
			return Session{}, fmt.Errorf("cannot determine cwd for %s", jsonlFile)
		}
		cwd = decoded
	}

	return Session{
		Client:      ClientClaude,
		ID:          sessionID,
		Title:       title,
		CWD:         cwd,
		CWDDisplay:  abbreviateHome(cwd, home),
		ProjectPath: projectPath,
		Time:        info.ModTime(),
		MsgCount:    msgCount,
		jsonlPath:   jsonlFile,
	}, nil
}

// parseJSONL extracts title, cwd, and message count from a JSONL session file.
func parseJSONL(path string, verbose bool) (title, cwd string, msgCount int) {
	f, err := os.Open(path)
	if err != nil {
		return "Untitled", "", 0
	}
	defer f.Close()

	var (
		lastAgentName     string
		lastCustomTitle   string
		lastAiTitle       string
		lastSummary       string
		lastPrompt        string
		firstUserMsgTitle string
	)

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1 MB line buffer
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var rec map[string]json.RawMessage
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			continue
		}

		// Extract cwd from first record that has it
		if cwd == "" {
			if raw, ok := rec["cwd"]; ok {
				var s string
				if json.Unmarshal(raw, &s) == nil {
					cwd = s
				}
			}
		}

		// Extract type
		var recType string
		if raw, ok := rec["type"]; ok {
			json.Unmarshal(raw, &recType)
		}

		switch recType {
		case "agent-name":
			var an string
			if raw, ok := rec["agentName"]; ok {
				if json.Unmarshal(raw, &an) == nil && an != "" {
					lastAgentName = applyTitleRules(an)
				}
			}

		case "custom-title":
			// Always update — last custom-title wins
			var ct string
			if raw, ok := rec["customTitle"]; ok {
				if json.Unmarshal(raw, &ct) == nil && ct != "" {
					lastCustomTitle = applyTitleRules(ct)
				}
			}

		case "ai-title":
			// AI-generated title — lower priority than custom-title
			var at string
			if raw, ok := rec["aiTitle"]; ok {
				if json.Unmarshal(raw, &at) == nil && at != "" {
					lastAiTitle = applyTitleRules(at)
				}
			}

		case "summary":
			var s string
			if raw, ok := rec["summary"]; ok {
				if json.Unmarshal(raw, &s) == nil && s != "" {
					lastSummary = applyTitleRules(s)
				}
			}

		case "last-prompt":
			var lp string
			if raw, ok := rec["lastPrompt"]; ok {
				if json.Unmarshal(raw, &lp) == nil && lp != "" {
					lastPrompt = applyTitleRules(lp)
				}
			}

		case "user":
			result := ClaudeUserTurnText(rec)
			if result.Countable {
				msgCount++
			}
			if firstUserMsgTitle == "" && result.Text != "" {
				firstUserMsgTitle = result.Text
			}
		}
	}

	// Priority: agent-name > custom-title > ai-title > summary > last-prompt > first user text > Untitled
	if lastAgentName != "" {
		return lastAgentName, cwd, msgCount
	}
	if lastCustomTitle != "" {
		return lastCustomTitle, cwd, msgCount
	}
	if lastAiTitle != "" {
		return lastAiTitle, cwd, msgCount
	}
	if lastSummary != "" {
		return lastSummary, cwd, msgCount
	}
	if lastPrompt != "" {
		return lastPrompt, cwd, msgCount
	}
	if firstUserMsgTitle != "" {
		return firstUserMsgTitle, cwd, msgCount
	}
	return "Untitled", cwd, msgCount
}

// IsRealUserMsg returns true for user records that count as a user turn.
// Use ClaudeUserTurnText for new code to also get display text.
func IsRealUserMsg(rec map[string]json.RawMessage) bool {
	return ClaudeUserTurnText(rec).Countable
}

// ClaudeUserTurnResult holds the parsed result of a Claude user turn.
type ClaudeUserTurnResult struct {
	Countable bool   // whether this record counts as a user turn
	Text      string // display text for preview (empty if not countable)
}

// ClaudeUserTurnText is the unified predicate for Claude user-turn classification.
// It returns (text, true) for records that should count as a user turn and appear in preview.
//
// A record is countable when all are true:
//   - type == "user"
//   - isMeta != true
//   - no toolUseResult
//   - no sourceToolAssistantUUID
//   - content is displayable user-submitted input
//
// Displayable input: plain string, <command-message>/<command-name>,
// array with visible text/image/document blocks (not tool_result-only).
func ClaudeUserTurnText(rec map[string]json.RawMessage) ClaudeUserTurnResult {
	// Check isMeta
	var isMeta bool
	if raw, ok := rec["isMeta"]; ok {
		json.Unmarshal(raw, &isMeta)
	}
	if isMeta {
		return ClaudeUserTurnResult{}
	}

	// Check toolUseResult
	if _, ok := rec["toolUseResult"]; ok {
		return ClaudeUserTurnResult{}
	}

	// Check sourceToolAssistantUUID
	if _, ok := rec["sourceToolAssistantUUID"]; ok {
		return ClaudeUserTurnResult{}
	}

	// Extract message.content
	msgRaw, ok := rec["message"]
	if !ok {
		return ClaudeUserTurnResult{}
	}
	var msg map[string]json.RawMessage
	if json.Unmarshal(msgRaw, &msg) != nil {
		return ClaudeUserTurnResult{}
	}
	contentRaw, ok := msg["content"]
	if !ok {
		return ClaudeUserTurnResult{}
	}

	// String content
	var s string
	if json.Unmarshal(contentRaw, &s) == nil {
		return classifyStringContent(s)
	}

	// Array content
	var items []map[string]json.RawMessage
	if json.Unmarshal(contentRaw, &items) != nil {
		return ClaudeUserTurnResult{}
	}
	return classifyArrayContent(items)
}

// classifyStringContent checks a string content value against skip prefixes
// and formats command/bash inputs for display.
func classifyStringContent(s string) ClaudeUserTurnResult {
	s = strings.TrimSpace(s)
	if s == "" {
		return ClaudeUserTurnResult{}
	}
	// Check skip prefixes
	for _, prefix := range turnSkipPrefixes {
		if strings.HasPrefix(s, prefix) {
			return ClaudeUserTurnResult{}
		}
	}
	// Command XML → format as /command args
	if strings.HasPrefix(s, "<command-message>") || strings.HasPrefix(s, "<command-name>") {
		name := extractCommandName(s)
		if name != "" {
			return ClaudeUserTurnResult{Countable: true, Text: name}
		}
		return ClaudeUserTurnResult{}
	}
	// Regular text → first line
	if idx := strings.Index(s, "\n"); idx >= 0 {
		s = s[:idx]
	}
	return ClaudeUserTurnResult{Countable: true, Text: strings.TrimSpace(s)}
}

// classifyArrayContent checks an array content value.
// Returns countable if it has visible text/image/document blocks (not tool_result-only).
func classifyArrayContent(items []map[string]json.RawMessage) ClaudeUserTurnResult {
	if len(items) == 0 {
		return ClaudeUserTurnResult{}
	}

	var hasToolResult bool
	var lastText string
	var hasImageOrDoc bool

	for _, item := range items {
		var t string
		if typeRaw, ok := item["type"]; ok {
			json.Unmarshal(typeRaw, &t)
		}
		switch t {
		case "tool_result":
			hasToolResult = true
		case "text":
			if textRaw, ok := item["text"]; ok {
				var text string
				if json.Unmarshal(textRaw, &text) == nil {
					text = strings.TrimSpace(text)
					if text != "" {
						// Check if this text is a skip prefix
						skip := false
						for _, prefix := range turnSkipPrefixes {
							if strings.HasPrefix(text, prefix) {
								skip = true
								break
							}
						}
						if !skip {
							lastText = text
						}
					}
				}
			}
		case "image", "document":
			hasImageOrDoc = true
		}
	}

	// Mixed rows with tool_result: default to not counting
	if hasToolResult {
		return ClaudeUserTurnResult{}
	}

	if lastText != "" {
		// Extract first line for display
		if idx := strings.Index(lastText, "\n"); idx >= 0 {
			lastText = lastText[:idx]
		}
		return ClaudeUserTurnResult{Countable: true, Text: strings.TrimSpace(lastText)}
	}
	if hasImageOrDoc {
		return ClaudeUserTurnResult{Countable: true, Text: "[image]"}
	}
	return ClaudeUserTurnResult{}
}

// extractCommandName extracts /command args from command XML tags.
func extractCommandName(s string) string {
	var name string

	// prefer <command-name>/foo</command-name>
	if start := strings.Index(s, "<command-name>"); start >= 0 {
		start += len("<command-name>")
		if end := strings.Index(s[start:], "</command-name>"); end >= 0 {
			name = strings.TrimSpace(s[start : start+end])
		}
	}
	// fallback: synthesise from <command-message>foo</command-message>
	if name == "" {
		if start := strings.Index(s, "<command-message>"); start >= 0 {
			start += len("<command-message>")
			if end := strings.Index(s[start:], "</command-message>"); end >= 0 {
				name = strings.TrimSpace(s[start : start+end])
				if name != "" && !strings.HasPrefix(name, "/") {
					name = "/" + name
				}
			}
		}
	}
	if name == "" {
		return ""
	}

	// append <command-args> if non-empty
	if start := strings.Index(s, "<command-args>"); start >= 0 {
		start += len("<command-args>")
		if end := strings.Index(s[start:], "</command-args>"); end >= 0 {
			if args := strings.TrimSpace(s[start : start+end]); args != "" {
				name = name + " " + args
			}
		}
	}
	return name
}

// applyTitleRules filters, cleans, and truncates a candidate title string.
func applyTitleRules(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	for _, prefix := range titleSkipPrefixes {
		if strings.HasPrefix(s, prefix) {
			return ""
		}
	}

	lines := strings.Split(s, "\n")
	firstLine := strings.TrimSpace(lines[0])

	if firstLine == "Implement the following plan:" && len(lines) > 1 {
		for _, l := range lines[1:] {
			l = strings.TrimSpace(l)
			if l != "" {
				title := "Plan: " + l
				return truncateStr(title, 50)
			}
		}
	}

	return truncateStr(firstLine, 50)
}

func truncateStr(s string, max int) string {
	if len([]rune(s)) <= max {
		return s
	}
	runes := []rune(s)
	return string(runes[:max])
}

