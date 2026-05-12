package source

import (
	"testing"
	"time"
)

func TestDetectActive_emptySessionsReturnsEmptyMap(t *testing.T) {
	result := DetectActive(nil)
	if result == nil {
		t.Fatal("expected non-nil map, got nil")
	}
	if len(result) != 0 {
		t.Fatalf("expected empty map, got %v", result)
	}
}

func TestDetectActive_sessionWithNoMatchingCWDIsNotActive(t *testing.T) {
	s := Session{
		Client:    ClientClaude,
		ID:        "test-id-1",
		CWD:       "/nonexistent/path/that/no/process/uses",
		Time:      time.Now(),
		jsonlPath: "", // no file
	}
	result := DetectActive([]Session{s})
	if result["test-id-1"] {
		t.Fatal("session with unmatched CWD should not be active")
	}
}

func TestDetectActive_opencodeSessionNotTodayIsNotActive(t *testing.T) {
	// Session with yesterday's time should never be active regardless of CWD
	yesterday := time.Now().AddDate(0, 0, -1)
	s := Session{
		Client: ClientOpencode,
		ID:     "oc-old",
		CWD:    "/some/path",
		Time:   yesterday,
	}
	result := DetectActive([]Session{s})
	if result["oc-old"] {
		t.Fatal("opencode session from yesterday should not be active")
	}
}
