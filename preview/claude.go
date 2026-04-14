package preview

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// RenderClaude writes a preview of a Claude Code session to w.
func RenderClaude(w io.Writer, sessionID, projectPath, workingDir string) {
	jsonlFile := filepath.Join(projectPath, sessionID+".jsonl")

	var timeStr string
	if info, err := os.Stat(jsonlFile); err == nil {
		timeStr = info.ModTime().Format("2006-01-02 15:04:05")
	}

	title, msgCount, recentMsgs := parseJSONLPreview(jsonlFile)

	fmt.Fprintf(w, "%s\n", previewHeader.Render("━━━ SESSION INFO ━━━"))
	fmt.Fprintf(w, "%s     %s\n", previewLabelTitle.Render("Title:"), title)
	fmt.Fprintf(w, "%s      %s\n", previewLabelTime.Render("Time:"), timeStr)
	fmt.Fprintf(w, "%s  %d\n", previewLabelMsg.Render("Messages:"), msgCount)
	fmt.Fprintf(w, "%s %s\n", previewLabelDir.Render("Directory:"), workingDir)

	if len(recentMsgs) > 0 {
		fmt.Fprintf(w, "%s\n", previewHeader.Render("━━━ RECENT MESSAGES ━━━"))
		for _, msg := range recentMsgs {
			fmt.Fprintf(w, "%s %s\n", previewBullet.Render("•"), msg)
		}
	}

	fmt.Fprintf(w, "%s\n\n", previewHeader.Render("━━━ DIRECTORY LIST ━━━"))
	listDir(w, workingDir)
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
		lastCustomTitle string
		firstUserTitle  string
		allUserMsgs     []string
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

	if lastCustomTitle != "" {
		title = lastCustomTitle
	} else if firstUserTitle != "" {
		title = firstUserTitle
	} else {
		title = "Untitled"
	}

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

	var s string
	if json.Unmarshal(contentRaw, &s) == nil {
		return filterPreviewMsg(s)
	}
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
	if idx := strings.Index(s, "\n"); idx >= 0 {
		s = s[:idx]
	}
	return strings.TrimSpace(s)
}

// ClaudeInfo returns the session info fields (Title/Time/Messages/Directory)
// as a styled string for the info viewport section.
// No section header is included; the caller provides the header via lipgloss.
func ClaudeInfo(sessionID, projectPath, workingDir string) string {
	jsonlFile := filepath.Join(projectPath, sessionID+".jsonl")

	var timeStr string
	if info, err := os.Stat(jsonlFile); err == nil {
		timeStr = info.ModTime().Format("2006-01-02 15:04:05")
	}

	title, msgCount, _ := parseJSONLPreview(jsonlFile)

	var sb strings.Builder
	fmt.Fprintf(&sb, "%s     %s\n", previewLabelTitle.Render("Title:"), title)
	fmt.Fprintf(&sb, "%s      %s\n", previewLabelTime.Render("Time:"), timeStr)
	fmt.Fprintf(&sb, "%s  %d\n", previewLabelMsg.Render("Messages:"), msgCount)
	fmt.Fprintf(&sb, "%s %s\n", previewLabelDir.Render("Directory:"), workingDir)
	return sb.String()
}

// ClaudeMsgs returns the recent user messages as a styled bullet list.
// Returns empty string when the JSONL file is missing or has no user messages.
func ClaudeMsgs(sessionID, projectPath string) string {
	jsonlFile := filepath.Join(projectPath, sessionID+".jsonl")
	_, _, recentMsgs := parseJSONLPreview(jsonlFile)
	if len(recentMsgs) == 0 {
		return ""
	}
	var sb strings.Builder
	for _, msg := range recentMsgs {
		fmt.Fprintf(&sb, "%s %s\n", previewBullet.Render("•"), msg)
	}
	return sb.String()
}
