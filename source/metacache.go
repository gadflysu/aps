package source

import (
	"encoding/gob"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// MetaEntry holds the parsed metadata for one JSONL file.
type MetaEntry struct {
	Mtime       time.Time
	Size        int64
	Title       string
	CWD         string
	MsgCount    int
	SessionTime time.Time // zero for old cache entries; fall back to mtime when zero
}

// MetaCache is an in-process cache backed by a gob file on disk.
// The backing file lives at ~/.cache/aps/session-meta.gob.
type MetaCache struct {
	path    string
	mu      sync.RWMutex
	entries map[string]MetaEntry // key: absolute file path
}

// LoadMetaCache reads ~/.cache/aps/session-meta.gob.
// Returns an empty cache (never nil, never error) if file is missing or corrupt.
func LoadMetaCache() *MetaCache {
	home, _ := os.UserHomeDir()
	path := filepath.Join(home, ".cache", "aps", "session-meta.gob")
	return newMetaCacheWithPath(path)
}

// newMetaCacheWithPath loads (or creates empty) a MetaCache backed by the given path.
// Used by LoadMetaCache and by tests.
func newMetaCacheWithPath(path string) *MetaCache {
	c := &MetaCache{
		path:    path,
		entries: make(map[string]MetaEntry),
	}
	f, err := os.Open(path)
	if err != nil {
		return c // file absent: start empty
	}
	defer f.Close()
	dec := gob.NewDecoder(f)
	var data map[string]MetaEntry
	if err := dec.Decode(&data); err != nil {
		return c // corrupt gob: start empty
	}
	c.entries = data
	return c
}

// Lookup returns the cached entry for path if mtime AND size both match.
// Returns (entry, true) on hit, (zero, false) on miss.
func (c *MetaCache) Lookup(path string, mtime time.Time, size int64) (MetaEntry, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.entries[path]
	if !ok {
		return MetaEntry{}, false
	}
	if !e.Mtime.Equal(mtime) || e.Size != size {
		return MetaEntry{}, false
	}
	return e, true
}

// Store records an entry for path.
func (c *MetaCache) Store(path string, e MetaEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[path] = e
}

// Save writes the cache to disk. Creates ~/.cache/aps/ if it does not exist.
func (c *MetaCache) Save() error {
	c.mu.RLock()
	snapshot := make(map[string]MetaEntry, len(c.entries))
	for k, v := range c.entries {
		snapshot[k] = v
	}
	c.mu.RUnlock()

	dir := filepath.Dir(c.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, "session-meta-*.gob.tmp")
	if err != nil {
		return err
	}
	enc := gob.NewEncoder(f)
	if err := enc.Encode(snapshot); err != nil {
		_ = f.Close()
		_ = os.Remove(f.Name())
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(f.Name())
		return err
	}
	return os.Rename(f.Name(), c.path)
}
