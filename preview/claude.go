package preview

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Claude prints a preview of a Claude Code session to stdout.
// sessionID, projectPath, workingDir come from fzf TAB fields.
func Claude(sessionID, projectPath, workingDir string) {
	jsonlFile := filepath.Join(projectPath, sessionID+".jsonl")

	var timeStr string
	if info, err := os.Stat(jsonlFile); err == nil {
		timeStr = info.ModTime().Format("2006-01-02 15:04:05")
	}

	title, msgCount, recentMsgs := parseJSONLPreview(jsonlFile)

	fmt.Printf("\033[1;36m━━━ SESSION INFO ━━━\033[0m\n")
	fmt.Printf("\033[1;33mTitle:\033[0m     %s\n", title)
	fmt.Printf("\033[1;32mTime:\033[0m      %s\n", timeStr)
	fmt.Printf("\033[1;35mMessages:\033[0m  %d\n", msgCount)
	fmt.Printf("\033[1;90mDirectory:\033[0m %s\n", workingDir)

	if len(recentMsgs) > 0 {
		fmt.Printf("\033[1;36m━━━ RECENT MESSAGES ━━━\033[0m\n")
		for _, msg := range recentMsgs {
			fmt.Printf("\033[1;90m•\033[0m %s\n", msg)
		}
	}

	fmt.Printf("\033[1;36m━━━ DIRECTORY LIST ━━━\033[0m\n\n")
	listDir(workingDir)
}

var previewSkipPrefixes = []string{
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

func parseJSONLPreview(path string) (title string, msgCount int, recent []string) {
	f, err := os.Open(path)
	if err != nil {
		return "Untitled", 0, nil
	}
	defer f.Close()

	var (
		lastCustomTitle   string
		firstUserTitle    string
		allUserMsgs       []string
	)

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		var rec map[string]json.RawMessage
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			continue
		}

		var recType string
		if raw, ok := rec["type"]; ok {
			json.Unmarshal(raw, &recType)
		}

		switch recType {
		case "custom-title":
			if raw, ok := rec["customTitle"]; ok {
				var ct string
				if json.Unmarshal(raw, &ct) == nil && ct != "" {
					lastCustomTitle = strings.TrimSpace(ct)
				}
			}

		case "user":
			msgCount++
			text := extractUserText(rec)
			if text != "" {
				if firstUserTitle == "" {
					firstUserTitle = text
				}
				allUserMsgs = append(allUserMsgs, text)
			}
		}
	}

	// Title priority
	if lastCustomTitle != "" {
		title = lastCustomTitle
	} else if firstUserTitle != "" {
		title = firstUserTitle
	} else {
		title = "Untitled"
	}

	// Last 10 messages, newest first, capped at 80 chars
	if len(allUserMsgs) > 10 {
		allUserMsgs = allUserMsgs[len(allUserMsgs)-10:]
	}
	for i, j := 0, len(allUserMsgs)-1; i < j; i, j = i+1, j-1 {
		allUserMsgs[i], allUserMsgs[j] = allUserMsgs[j], allUserMsgs[i]
	}
	for _, m := range allUserMsgs {
		if len([]rune(m)) > 80 {
			m = string([]rune(m)[:80])
		}
		recent = append(recent, m)
	}

	return title, msgCount, recent
}

func extractUserText(rec map[string]json.RawMessage) string {
	msgRaw, ok := rec["message"]
	if !ok {
		return ""
	}
	var msg map[string]json.RawMessage
	if err := json.Unmarshal(msgRaw, &msg); err != nil {
		return ""
	}
	contentRaw, ok := msg["content"]
	if !ok {
		return ""
	}

	// Try string
	var s string
	if json.Unmarshal(contentRaw, &s) == nil {
		return filterPreviewMsg(s)
	}
	// Try array
	var items []map[string]json.RawMessage
	if json.Unmarshal(contentRaw, &items) == nil {
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
				if json.Unmarshal(textRaw, &text) == nil {
					return filterPreviewMsg(strings.TrimSpace(text))
				}
			}
		}
	}
	return ""
}

func filterPreviewMsg(s string) string {
	s = strings.TrimSpace(s)
	for _, prefix := range previewSkipPrefixes {
		if strings.HasPrefix(s, prefix) {
			return ""
		}
	}
	// First line only
	if idx := strings.Index(s, "\n"); idx >= 0 {
		s = s[:idx]
	}
	return strings.TrimSpace(s)
}

