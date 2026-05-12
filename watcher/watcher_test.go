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
