package preview

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/gadflysu/aps/source"
)

// RenderCodex writes a preview of a Codex session to w.
func RenderCodex(w io.Writer, sessionID, codexHome, directory string) {
	rolloutPath := source.FindRolloutPath(codexHome, sessionID)
	var timeStr string
	if info, err := os.Stat(rolloutPath); err == nil {
		timeStr = info.ModTime().Format("2006-01-02 15:04:05")
	}

	title, msgCount, recentMsgs := parseCodexRolloutPreview(rolloutPath)

	// Priority 1: session_index.jsonl (matches Codex CLI behavior)
	if indexTitle := source.LookupSessionIndex(codexHome, sessionID); indexTitle != "" {
		title = indexTitle
	}

	fmt.Fprintf(w, "%s\n", previewHeader.Render("━━━ SESSION INFO ━━━"))
	writePreviewInfoRow(w, previewLabelAgent, "Agent:", source.ClientCodex.String())
	writePreviewInfoRow(w, previewLabelTitle, "Title:", title)
	writePreviewInfoRow(w, previewLabelID, "Session ID:", sessionID)
	writePreviewInfoRow(w, previewLabelTime, "Time:", timeStr)
	writePreviewInfoRow(w, previewLabelMsg, "Turns:", fmt.Sprintf("%d", msgCount))
	writePreviewInfoRow(w, previewLabelDir, "Directory:", directory)
	if rolloutPath != "" {
		writePreviewInfoRow(w, previewLabelData, "Data:", rolloutPath)
	}

	if len(recentMsgs) > 0 {
		fmt.Fprintf(w, "%s\n", previewHeader.Render("━━━ RECENT MESSAGES ━━━"))
		for _, msg := range recentMsgs {
			fmt.Fprintf(w, "%s %s\n", previewBullet.Render("•"), msg)
		}
	}

	fmt.Fprintf(w, "%s\n\n", previewHeader.Render("━━━ DIRECTORY LIST ━━━"))
	listDir(w, directory)
}

// CodexInfo returns the session info fields as a styled string for the info viewport section.
func CodexInfo(sessionID, codexHome, directory string) string {
	rolloutPath := source.FindRolloutPath(codexHome, sessionID)
	var timeStr string
	if info, err := os.Stat(rolloutPath); err == nil {
		timeStr = info.ModTime().Format("2006-01-02 15:04:05")
	}

	title, msgCount, _ := parseCodexRolloutPreview(rolloutPath)

	// Priority 1: session_index.jsonl (matches Codex CLI behavior)
	if indexTitle := source.LookupSessionIndex(codexHome, sessionID); indexTitle != "" {
		title = indexTitle
	}

	var sb strings.Builder
	writePreviewInfoRow(&sb, previewLabelAgent, "Agent:", source.ClientCodex.String())
	writePreviewInfoRow(&sb, previewLabelTitle, "Title:", title)
	writePreviewInfoRow(&sb, previewLabelID, "Session ID:", sessionID)
	writePreviewInfoRow(&sb, previewLabelTime, "Time:", timeStr)
	writePreviewInfoRow(&sb, previewLabelMsg, "Turns:", fmt.Sprintf("%d", msgCount))
	writePreviewInfoRow(&sb, previewLabelDir, "Directory:", directory)
	if rolloutPath != "" {
		writePreviewInfoRow(&sb, previewLabelData, "Data:", rolloutPath)
	}
	return sb.String()
}

// CodexMsgs returns the recent user messages as a styled bullet list.
func CodexMsgs(sessionID, codexHome string) string {
	rolloutPath := source.FindRolloutPath(codexHome, sessionID)
	_, _, recentMsgs := parseCodexRolloutPreview(rolloutPath)
	if len(recentMsgs) == 0 {
		return ""
	}
	var sb strings.Builder
	for _, msg := range recentMsgs {
		fmt.Fprintf(&sb, "%s %s\n", previewBullet.Render("•"), msg)
	}
	return sb.String()
}

// parseCodexRolloutPreview parses a Codex rollout file for preview information.
func parseCodexRolloutPreview(path string) (title string, msgCount int, recent []string) {
	if path == "" {
		return "Untitled", 0, nil
	}

	f, err := os.Open(path)
	if err != nil {
		return "Untitled", 0, nil
	}
	defer f.Close()

	var (
		firstUserMsg string
		allUserMsgs  []string
	)

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, bufio.MaxScanTokenSize), 4*1024*1024)
	for scanner.Scan() {
		var event struct {
			Type    string `json:"type"`
			Payload struct {
				Type    string `json:"type"`
				Message string `json:"message"`
			} `json:"payload"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			continue
		}
		if event.Type == "event_msg" && event.Payload.Type == "user_message" && event.Payload.Message != "" {
			msgCount++
			if firstUserMsg == "" {
				firstUserMsg = event.Payload.Message
			}
			allUserMsgs = append(allUserMsgs, event.Payload.Message)
		}
	}

	// Priority 1: session_index.jsonl (matches Codex CLI behavior)
	// This is handled by the caller using codexHome and sessionID
	// For now, use first user message as fallback
	if firstUserMsg != "" {
		title = firstUserMsg
	} else {
		title = "Untitled"
	}

	// Get last 10 messages, reverse chronological
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
