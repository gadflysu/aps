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

func TestDetectActive_guessLimitedByUnmappedProcCount(t *testing.T) {
	// 2 unmapped procs for /foo, 3 sessions → only 2 guessed
	now := time.Now()
	sessions := []Session{
		{Client: ClientOpencode, ID: "s1", CWD: "/foo", Time: now.Add(-1 * time.Second)},
		{Client: ClientOpencode, ID: "s2", CWD: "/foo", Time: now.Add(-2 * time.Second)},
		{Client: ClientOpencode, ID: "s3", CWD: "/foo", Time: now.Add(-3 * time.Second)},
	}
	procs := []ProcInfo{
		{PID: "1", LStart: "t1", CWD: "/foo"},
		{PID: "2", LStart: "t2", CWD: "/foo"},
	}
	result := DetectActive(sessions, procs, nil)
	if len(result.Guessed) != 2 {
		t.Errorf("expected 2 guessed (= unmapped proc count), got %d: %v", len(result.Guessed), result.Guessed)
	}
}

func TestDetectActive_guessReducedByConfirmedProcs(t *testing.T) {
	// 2 procs for /foo, 1 confirmed via cache → 1 unmapped slot → only 1 guessed
	now := time.Now()
	sessions := []Session{
		{Client: ClientOpencode, ID: "s-conf", CWD: "/foo", Time: now.Add(-1 * time.Second)},
		{Client: ClientOpencode, ID: "s2",     CWD: "/foo", Time: now.Add(-2 * time.Second)},
		{Client: ClientOpencode, ID: "s3",     CWD: "/foo", Time: now.Add(-3 * time.Second)},
	}
	proc1 := ProcInfo{PID: "1", LStart: "t1", CWD: "/foo"}
	proc2 := ProcInfo{PID: "2", LStart: "t2", CWD: "/foo"}
	cache := &PIDCache{entries: map[string]string{proc1.key(): "s-conf"}}

	result := DetectActive(sessions, []ProcInfo{proc1, proc2}, cache)

	if !result.Confirmed["s-conf"] {
		t.Error("s-conf should be confirmed via cache")
	}
	// proc2 is unmapped → 1 slot → 1 guessed among the non-confirmed sessions
	if len(result.Guessed) != 1 {
		t.Errorf("expected 1 guessed (n=2 procs, k=1 confirmed → 1 slot), got %d: %v", len(result.Guessed), result.Guessed)
	}
}

func TestDetectActive_guessPrioritizesMostRecent(t *testing.T) {
	// 1 unmapped proc for /foo, 3 sessions → only most recently active session guessed
	now := time.Now()
	sessions := []Session{
		{Client: ClientOpencode, ID: "s-old",    CWD: "/foo", Time: now.Add(-10 * time.Second)},
		{Client: ClientOpencode, ID: "s-recent", CWD: "/foo", Time: now.Add(-1 * time.Second)},
		{Client: ClientOpencode, ID: "s-middle", CWD: "/foo", Time: now.Add(-5 * time.Second)},
	}
	procs := []ProcInfo{{PID: "1", LStart: "t1", CWD: "/foo"}}

	result := DetectActive(sessions, procs, nil)

	if len(result.Guessed) != 1 {
		t.Errorf("expected 1 guessed session, got %d: %v", len(result.Guessed), result.Guessed)
	}
	if !result.Guessed["s-recent"] {
		t.Errorf("most recent session should be guessed, got %v", result.Guessed)
	}
}
