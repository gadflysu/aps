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
		lastCustomTitle   string
		lastAiTitle       string
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

		case "user":
			msgCount++
			if firstUserMsgTitle == "" {
				// Try message.content
				if raw, ok := rec["message"]; ok {
					var msg map[string]json.RawMessage
					if json.Unmarshal(raw, &msg) == nil {
						if contentRaw, ok := msg["content"]; ok {
							t := extractTextFromContent(contentRaw)
							if t != "" {
								firstUserMsgTitle = t
							}
						}
					}
				}
			}
		}
	}

	if lastCustomTitle != "" {
		return lastCustomTitle, cwd, msgCount
	}
	if lastAiTitle != "" {
		return lastAiTitle, cwd, msgCount
	}
	if firstUserMsgTitle != "" {
		return firstUserMsgTitle, cwd, msgCount
	}
	return "Untitled", cwd, msgCount
}

// extractTextFromContent extracts the first meaningful line from a content value
// (string or []object with type=text).
func extractTextFromContent(raw json.RawMessage) string {
	// Try string
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return applyTitleRules(s)
	}

	// Try array
	var items []map[string]json.RawMessage
	if json.Unmarshal(raw, &items) == nil {
		for _, item := range items {
			var t string
			if typeRaw, ok := item["type"]; ok {
				json.Unmarshal(typeRaw, &t)
			}
			if t != "text" {
				continue
			}
			var text string
			if textRaw, ok := item["text"]; ok {
				if json.Unmarshal(textRaw, &text) == nil && text != "" {
					return applyTitleRules(strings.TrimSpace(text))
				}
			}
		}
	}
	return ""
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

