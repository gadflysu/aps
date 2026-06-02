// Package preview renders the three-pane preview (SESSION INFO / RECENT MESSAGES / DIRECTORY) shown inside the picker.
package preview

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/gadflysu/aps/source"
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


func parseJSONLPreview(path string) (title string, msgCount int, recent []string) {
	f, err := os.Open(path)
	if err != nil {
		return "Untitled", 0, nil
	}
	defer f.Close()

	var (
		lastAgentName   string
		lastCustomTitle string
		lastAiTitle     string
		lastSummary     string
		lastPrompt      string
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
		case "agent-name":
			if raw, ok := rec["agentName"]; ok {
				var an string
				if json.Unmarshal(raw, &an) == nil && an != "" {
					lastAgentName = strings.TrimSpace(an)
				}
			}

		case "custom-title":
			if raw, ok := rec["customTitle"]; ok {
				var ct string
				if json.Unmarshal(raw, &ct) == nil && ct != "" {
					lastCustomTitle = strings.TrimSpace(ct)
				}
			}

		case "ai-title":
			if raw, ok := rec["aiTitle"]; ok {
				var at string
				if json.Unmarshal(raw, &at) == nil && at != "" {
					lastAiTitle = strings.TrimSpace(at)
				}
			}

		case "summary":
			if raw, ok := rec["summary"]; ok {
				var s string
				if json.Unmarshal(raw, &s) == nil && s != "" {
					lastSummary = strings.TrimSpace(s)
				}
			}

		case "last-prompt":
			if raw, ok := rec["lastPrompt"]; ok {
				var lp string
				if json.Unmarshal(raw, &lp) == nil && lp != "" {
					lastPrompt = strings.TrimSpace(lp)
				}
			}

		case "user":
			result := source.ClaudeUserTurnText(rec)
			if result.Countable {
				msgCount++
				if firstUserTitle == "" {
					firstUserTitle = result.Text
				}
				allUserMsgs = append(allUserMsgs, result.Text)
			}
		}
	}

	// Priority: agent-name > custom-title > ai-title > summary > last-prompt > first user text > Untitled
	switch {
	case lastAgentName != "":
		title = lastAgentName
	case lastCustomTitle != "":
		title = lastCustomTitle
	case lastAiTitle != "":
		title = lastAiTitle
	case lastSummary != "":
		title = lastSummary
	case lastPrompt != "":
		title = lastPrompt
	case firstUserTitle != "":
		title = firstUserTitle
	default:
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
	fmt.Fprintf(&sb, "%s     %d\n", previewLabelMsg.Render("Turns:"), msgCount)
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
