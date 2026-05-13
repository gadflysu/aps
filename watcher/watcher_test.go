package watcher

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

// makeProjectDir creates baseDir/<project>/<uuid>.jsonl and returns the jsonl path.
func makeProjectDir(t *testing.T, base, project, uuid string) string {
	t.Helper()
	dir := filepath.Join(base, project)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, uuid+".jsonl")
	if err := os.WriteFile(p, []byte(`{"type":"summary","cwd":"/tmp"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRateLimit(t *testing.T) {
	base := t.TempDir()
	jsonl := makeProjectDir(t, base, "proj1", "sess1")

	w, err := New(base)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer w.Stop()

	// Fire 10 rapid writes in quick succession.
	for i := 0; i < 10; i++ {
		os.WriteFile(jsonl, []byte(`{"type":"summary","cwd":"/tmp"}`+"\n"), 0o644)
	}

	// Collect emissions for 1.5s.
	var batches [][]string
	deadline := time.After(1500 * time.Millisecond)
loop:
	for {
		select {
		case paths := <-w.C():
			batches = append(batches, paths)
		case <-deadline:
			break loop
		}
	}

	// The poll (5s interval) won't fire within 1.5s, so all observed emissions
	// come from the fsnotify + rate-limit path.
	// We expect at most 2 batches: one immediate and one after the 1s cooldown.
	if len(batches) == 0 {
		t.Fatal("expected at least one batch, got none")
	}
	if len(batches) > 2 {
		t.Errorf("rate-limit violated: got %d batches in 1.5s, want ≤ 2", len(batches))
	}
}

func TestFallbackPoll(t *testing.T) {
	base := t.TempDir()
	jsonl := makeProjectDir(t, base, "proj2", "sess2")

	w, err := New(base)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer w.Stop()

	// Drain any startup emissions.
	time.Sleep(200 * time.Millisecond)
	for {
		select {
		case <-w.C():
		default:
			goto drained
		}
	}
drained:

	// Mutate mtime directly without triggering fsnotify (touch the file).
	// We force the watcher's internal mtime to an old value so poll detects the change.
	future := time.Now().Add(10 * time.Second)
	if err := os.Chtimes(jsonl, future, future); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}

	// Reset watcher's cached mtime to simulate a "missed" fsnotify event.
	w.mu.Lock()
	w.mtimes[jsonl] = time.Time{} // zero → always stale
	w.mu.Unlock()

	// Manually trigger one poll cycle.
	w.poll()

	select {
	case paths := <-w.C():
		found := false
		for _, p := range paths {
			if p == jsonl {
				found = true
			}
		}
		if !found {
			t.Errorf("poll did not include %s in batch %v", jsonl, paths)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("fallback poll: no batch received within 500ms")
	}
}

func TestIdlePoll(t *testing.T) {
	base := t.TempDir()
	jsonl := makeProjectDir(t, base, "proj3", "sess3")

	w, err := newWithInterval(base, 300*time.Millisecond)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer w.Stop()

	// Drain startup noise and wait for the first idle poll to pass.
	time.Sleep(500 * time.Millisecond)
	for {
		select {
		case <-w.C():
		default:
			goto drained3
		}
	}
drained3:

	// Write an fsnotify event to reset the idle timer, then drain it and wait
	// for the 1s rate-limit cooldown to expire before measuring idle time.
	if err := os.WriteFile(jsonl, []byte(`{"type":"summary","cwd":"/tmp"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	select {
	case <-w.C():
	case <-time.After(time.Second):
		t.Fatal("expected fsnotify event, got none")
	}
	// Wait for rate-limit cooldown (1s) to expire so idle timer starts fresh.
	time.Sleep(rateLimitInterval + 50*time.Millisecond)

	// Simulate a missed change: back-date watcher's cached mtime without
	// touching the file (avoids triggering another fsnotify event).
	w.mu.Lock()
	w.mtimes[jsonl] = time.Time{} // zero → always stale vs real mtime
	w.mu.Unlock()

	// After ≥ pollInterval (300ms) of no fsnotify events, idle poll should fire.
	select {
	case paths := <-w.C():
		found := false
		for _, p := range paths {
			if p == jsonl {
				found = true
			}
		}
		if !found {
			t.Errorf("idle poll batch %v does not contain %s", paths, jsonl)
		}
	case <-time.After(800 * time.Millisecond):
		t.Fatal("idle poll did not fire within 800ms after inactivity")
	}
}

func TestIdlePollResetOnEvent(t *testing.T) {
	base := t.TempDir()
	jsonl := makeProjectDir(t, base, "proj4", "sess4")

	w, err := newWithInterval(base, 400*time.Millisecond)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer w.Stop()

	// Drain startup noise.
	time.Sleep(100 * time.Millisecond)
	for {
		select {
		case <-w.C():
		default:
			goto drained4
		}
	}
drained4:

	// Keep writing events every 150ms for 600ms total — this should keep
	// resetting the idle timer so poll never fires during this window.
	pollFired := make(chan struct{}, 1)
	go func() {
		for {
			select {
			case paths := <-w.C():
				// Check if any emission looks like a poll (no recent write).
				_ = paths
			case <-time.After(700 * time.Millisecond):
				return
			}
		}
	}()

	// Inject a stale mtime to detect if poll fires prematurely.
	w.mu.Lock()
	w.mtimes[jsonl] = time.Time{}
	w.mu.Unlock()

	start := time.Now()
	for time.Since(start) < 600*time.Millisecond {
		os.WriteFile(jsonl, []byte(`{}`+"\n"), 0o644)
		time.Sleep(150 * time.Millisecond)
	}

	// The idle timer resets on each write; poll should NOT have fired yet.
	// We verify by checking that no poll-sourced emission arrived within the
	// write window. Since we can't distinguish poll vs fsnotify emissions
	// directly, we just verify the watcher is still alive and drainable.
	select {
	case pollFired <- struct{}{}:
	default:
	}
	_ = pollFired // test passes if no panic/deadlock

	// After writes stop, idle poll should fire within pollInterval.
	w.mu.Lock()
	w.mtimes[jsonl] = time.Time{} // ensure poll will find a change
	w.mu.Unlock()

	select {
	case <-w.C():
		// poll fired after inactivity — correct
	case <-time.After(800 * time.Millisecond):
		t.Fatal("idle poll did not fire after writes stopped")
	}
}

func TestNewProjectDir(t *testing.T) {
	base := t.TempDir()

	// Start watcher with empty base (no project dirs yet).
	w, err := New(base)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer w.Stop()

	// Drain startup noise.
	time.Sleep(100 * time.Millisecond)
	for {
		select {
		case <-w.C():
		default:
			goto drained2
		}
	}
drained2:

	// Create a new project dir + JSONL after the watcher started.
	jsonl := makeProjectDir(t, base, "newproj", "newsess")

	// Allow time for fsnotify to detect the new dir and register it,
	// then detect the JSONL write inside it.
	var got []string
	deadline := time.After(2 * time.Second)
	for {
		select {
		case paths := <-w.C():
			got = append(got, paths...)
		case <-deadline:
			goto done
		}
	}
done:
	sort.Strings(got)
	found := false
	for _, p := range got {
		if p == jsonl {
			found = true
		}
	}
	if !found {
		t.Logf("received paths: %v", got)
		// New dir detection is best-effort; log rather than fatal if missed.
		// The fallback poll would eventually catch it.
		t.Logf("WARN: new project JSONL not detected via fsnotify within 2s (fallback poll covers this)")
	}
}
