package watcher

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

const (
	rateLimitInterval = 1 * time.Second
	pollInterval      = 5 * time.Second
)

// Watcher watches ~/.claude/projects/ for JSONL changes and emits batches of
// changed file paths through C(). It uses fsnotify as the primary mechanism
// and a 5s stat-only poll as a fallback. Events are rate-limited to at most
// one emission per second.
type Watcher struct {
	baseDir string
	ch      chan []string
	done    chan struct{}
	wg      sync.WaitGroup

	// mtimes tracks the last known mtime for each known JSONL path.
	mu     sync.Mutex
	mtimes map[string]time.Time
}

// New creates and starts a Watcher for baseDir (typically ~/.claude/projects).
// If fsnotify is unavailable the watcher falls back to polling only.
func New(baseDir string) (*Watcher, error) {
	w := &Watcher{
		baseDir: baseDir,
		ch:      make(chan []string, 1),
		done:    make(chan struct{}),
		mtimes:  make(map[string]time.Time),
	}

	// Seed mtimes from existing JSONL files.
	w.seedMtimes()

	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		// FSNotify unavailable: run poll-only.
		w.wg.Add(1)
		go w.runPollOnly()
		return w, nil
	}

	// Register root dir.
	_ = fsw.Add(baseDir)

	// Register existing project subdirs (depth 1 only).
	entries, _ := os.ReadDir(baseDir)
	for _, e := range entries {
		if e.IsDir() {
			_ = fsw.Add(filepath.Join(baseDir, e.Name()))
		}
	}

	w.wg.Add(2)
	go w.runFSNotify(fsw)
	go w.runPoll()

	return w, nil
}

// C returns the channel on which batches of changed JSONL paths are sent.
func (w *Watcher) C() <-chan []string {
	return w.ch
}

// Stop shuts down all background goroutines.
func (w *Watcher) Stop() {
	close(w.done)
	w.wg.Wait()
}

// seedMtimes populates w.mtimes with current mtime for every JSONL at depth 2.
func (w *Watcher) seedMtimes() {
	dirs, _ := os.ReadDir(w.baseDir)
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		projectDir := filepath.Join(w.baseDir, d.Name())
		files, _ := filepath.Glob(filepath.Join(projectDir, "*.jsonl"))
		for _, f := range files {
			if info, err := os.Stat(f); err == nil {
				w.mtimes[f] = info.ModTime()
			}
		}
	}
}

// emit sends paths on w.ch, rate-limited to 1s intervals.
// Must be called from the rate-limit goroutine only.
func emit(ch chan<- []string, paths []string) {
	if len(paths) == 0 {
		return
	}
	// Non-blocking send: if channel is full the previous batch hasn't been
	// consumed yet; drop the new batch (the 5s poll will compensate).
	select {
	case ch <- paths:
	default:
	}
}

// runFSNotify processes fsnotify events and enforces the 1s rate-limit.
func (w *Watcher) runFSNotify(fsw *fsnotify.Watcher) {
	defer w.wg.Done()
	defer fsw.Close()

	var (
		pending  = make(map[string]struct{})
		cooldown *time.Timer
		timerCh  <-chan time.Time
	)

	flush := func() {
		paths := make([]string, 0, len(pending))
		for p := range pending {
			paths = append(paths, p)
		}
		pending = make(map[string]struct{})
		emit(w.ch, paths)
		timerCh = nil
	}

	for {
		select {
		case <-w.done:
			if cooldown != nil {
				cooldown.Stop()
			}
			return

		case event, ok := <-fsw.Events:
			if !ok {
				return
			}
			path := event.Name

			// New project subdir: register it.
			if event.Has(fsnotify.Create) {
				if info, err := os.Stat(path); err == nil && info.IsDir() {
					if filepath.Dir(path) == w.baseDir {
						_ = fsw.Add(path)
					}
				}
			}

			// Only care about JSONL files at depth 2.
			if !strings.HasSuffix(path, ".jsonl") {
				continue
			}
			if filepath.Dir(filepath.Dir(path)) != w.baseDir {
				continue
			}

			// Update mtime cache.
			if info, err := os.Stat(path); err == nil {
				w.mu.Lock()
				w.mtimes[path] = info.ModTime()
				w.mu.Unlock()
			}

			pending[path] = struct{}{}

			// Rate-limit: if no cooldown active, emit immediately and start timer.
			if timerCh == nil {
				flush()
				cooldown = time.NewTimer(rateLimitInterval)
				timerCh = cooldown.C
			}

		case <-timerCh:
			// Cooldown expired: flush any accumulated pending events.
			flush()

		case err, ok := <-fsw.Errors:
			if !ok {
				return
			}
			_ = err
		}
	}
}

// runPoll is the 5s fallback stat-only poll. It runs alongside fsnotify.
func (w *Watcher) runPoll() {
	defer w.wg.Done()
	w.poll()
}

// runPollOnly is the poll path used when fsnotify is unavailable.
func (w *Watcher) runPollOnly() {
	defer w.wg.Done()
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-w.done:
			return
		case <-ticker.C:
			w.poll()
		}
	}
}

// poll scans all known JSONL files for mtime changes and emits changed paths.
func (w *Watcher) poll() {
	// Also discover new files not yet in mtimes.
	dirs, _ := os.ReadDir(w.baseDir)
	var changed []string

	w.mu.Lock()
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		projectDir := filepath.Join(w.baseDir, d.Name())
		files, _ := filepath.Glob(filepath.Join(projectDir, "*.jsonl"))
		for _, f := range files {
			info, err := os.Stat(f)
			if err != nil {
				continue
			}
			prev, known := w.mtimes[f]
			if !known || info.ModTime().After(prev) {
				w.mtimes[f] = info.ModTime()
				changed = append(changed, f)
			}
		}
	}
	w.mu.Unlock()

	emit(w.ch, changed)
}
