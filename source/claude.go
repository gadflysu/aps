package source

import (
	"bufio"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"local/aps/filter"
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

	var sessions []Session

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		projectPath := filepath.Join(baseDir, entry.Name())

		jsonlFiles, err := filepath.Glob(filepath.Join(projectPath, "*.jsonl"))
		if err != nil || len(jsonlFiles) == 0 {
			continue
		}

		for _, jsonlFile := range jsonlFiles {
			sessionID := strings.TrimSuffix(filepath.Base(jsonlFile), ".jsonl")

			info, err := os.Stat(jsonlFile)
			if err != nil {
				continue
			}
			mtime := info.ModTime()

			title, cwd, msgCount := parseJSONL(jsonlFile, verbose)

			if cwd == "" {
				// Fallback: decode project directory name
				decoded, err := url.PathUnescape(entry.Name())
				if err != nil || !strings.HasPrefix(decoded, "/") {
					continue
				}
				cwd = decoded
			}

			if !filter.Matches(pathFilter, strictMatch, cwd) {
				continue
			}

			cwdDisplay := abbreviateHome(cwd, home)

			sessions = append(sessions, Session{
				Client:      ClientClaude,
				ID:          sessionID,
				Title:       title,
				CWD:         cwd,
				CWDDisplay:  cwdDisplay,
				ProjectPath: projectPath,
				Time:        mtime,
				MsgCount:    msgCount,
			})
		}
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].Time.After(sessions[j].Time)
	})

	return sessions, nil
}

// parseJSONL extracts title, cwd, and message count from a JSONL session file.
func parseJSONL(path string, verbose bool) (title, cwd string, msgCount int) {
	f, err := os.Open(path)
	if err != nil {
		return "Untitled", "", 0
	}
	defer f.Close()

	var (
		lastCustomTitle      string
		firstUserMsgTitle    string
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
					lastCustomTitle = strings.TrimSpace(ct)
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

