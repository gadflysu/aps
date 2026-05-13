package source

import (
	"testing"
	"time"
)

func TestDetectActive_emptySessionsReturnsEmptyResult(t *testing.T) {
	result := DetectActive(nil, nil, nil)
	if len(result.Confirmed) != 0 || len(result.Guessed) != 0 {
		t.Fatalf("expected empty result, got confirmed=%v guessed=%v", result.Confirmed, result.Guessed)
	}
}

func TestDetectActive_sessionWithNoMatchingCWDIsNotActive(t *testing.T) {
	s := Session{
		Client:    ClientClaude,
		ID:        "test-id-1",
		CWD:       "/nonexistent/path/that/no/process/uses",
		Time:      time.Now(),
		jsonlPath: "",
	}
	result := DetectActive([]Session{s}, nil, nil)
	if result.Confirmed["test-id-1"] || result.Guessed["test-id-1"] {
		t.Fatal("session with unmatched CWD should not be active")
	}
}

func TestDetectActive_opencodeSessionNotTodayIsNotActive(t *testing.T) {
	yesterday := time.Now().AddDate(0, 0, -1)
	s := Session{
		Client: ClientOpencode,
		ID:     "oc-old",
		CWD:    "/some/path",
		Time:   yesterday,
	}
	result := DetectActive([]Session{s}, nil, nil)
	if result.Confirmed["oc-old"] || result.Guessed["oc-old"] {
		t.Fatal("opencode session from yesterday should not be active")
	}
}
