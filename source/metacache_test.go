package source

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMetaCache_HitOnExactMatch(t *testing.T) {
	c := newMetaCacheWithPath(filepath.Join(t.TempDir(), "meta.gob"))
	mtime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	want := MetaEntry{
		Mtime:     mtime,
		Size:      1234,
		Title:     "My Session",
		CWD:       "/home/user/project",
		LaunchCWD: "/home/user/project",
		MsgCount:  42,
	}
	c.Store("/some/file.jsonl", want)
	got, ok := c.Lookup("/some/file.jsonl", mtime, 1234)
	if !ok {
		t.Fatal("expected cache hit, got miss")
	}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestMetaCache_MissOnMtimeChange(t *testing.T) {
	c := newMetaCacheWithPath(filepath.Join(t.TempDir(), "meta.gob"))
	mtime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	c.Store("/some/file.jsonl", MetaEntry{Mtime: mtime, Size: 1234, Title: "T"})

	differentMtime := mtime.Add(time.Second)
	_, ok := c.Lookup("/some/file.jsonl", differentMtime, 1234)
	if ok {
		t.Fatal("expected cache miss when mtime differs, got hit")
	}
}

func TestMetaCache_MissOnSizeChange(t *testing.T) {
	c := newMetaCacheWithPath(filepath.Join(t.TempDir(), "meta.gob"))
	mtime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	c.Store("/some/file.jsonl", MetaEntry{Mtime: mtime, Size: 1234, Title: "T"})

	_, ok := c.Lookup("/some/file.jsonl", mtime, 9999)
	if ok {
		t.Fatal("expected cache miss when size differs, got hit")
	}
}

func TestMetaCache_MissOnIncompleteEntry(t *testing.T) {
	c := newMetaCacheWithPath(filepath.Join(t.TempDir(), "meta.gob"))
	mtime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	c.Store("/some/file.jsonl", MetaEntry{
		Mtime:    mtime,
		Size:     1234,
		Title:    "Old Entry",
		CWD:      "/projects/foo",
		MsgCount: 3,
	})

	if _, ok := c.Lookup("/some/file.jsonl", mtime, 1234); ok {
		t.Fatal("expected miss when required cache fields are incomplete")
	}
}

func TestMetaCache_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "meta.gob")
	c1 := newMetaCacheWithPath(path)
	mtime := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	want := MetaEntry{
		Mtime:     mtime,
		Size:      5678,
		Title:     "Round Trip Session",
		CWD:       "/projects/foo/worktree",
		LaunchCWD: "/projects/foo",
		MsgCount:  7,
	}
	c1.Store("/foo/bar.jsonl", want)
	if err := c1.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	c2 := newMetaCacheWithPath(path)
	got, ok := c2.Lookup("/foo/bar.jsonl", mtime, 5678)
	if !ok {
		t.Fatal("expected cache hit after round-trip, got miss")
	}
	if got != want {
		t.Errorf("round-trip mismatch: got %+v, want %+v", got, want)
	}
}

func TestMetaCache_CorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "meta.gob")
	if err := os.WriteFile(path, []byte("this is not valid gob data!!!"), 0o600); err != nil {
		t.Fatal(err)
	}
	c := newMetaCacheWithPath(path)
	if c == nil {
		t.Fatal("LoadMetaCache returned nil on corrupt file")
	}
	// should be empty, not panic
	_, ok := c.Lookup("/any/path.jsonl", time.Now(), 1)
	if ok {
		t.Fatal("expected miss on empty cache loaded from corrupt file")
	}
}

func TestMetaCache_MissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nonexistent", "meta.gob")
	c := newMetaCacheWithPath(path)
	if c == nil {
		t.Fatal("LoadMetaCache returned nil for missing file")
	}
	_, ok := c.Lookup("/any/path.jsonl", time.Now(), 1)
	if ok {
		t.Fatal("expected miss on empty cache loaded from missing file")
	}
}

func TestLoadMetaCache_UsesHomeDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	c := LoadMetaCache()
	if c == nil {
		t.Fatal("LoadMetaCache returned nil")
	}
	// empty cache: any lookup should miss
	_, ok := c.Lookup("/any/path.jsonl", time.Now(), 1)
	if ok {
		t.Fatal("expected miss on fresh cache")
	}
	// Save should create ~/.cache/aps/ under the temp HOME
	mtime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	c.Store("/foo.jsonl", MetaEntry{Mtime: mtime, Size: 42, Title: "T", CWD: "/p", LaunchCWD: "/p"})
	if err := c.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	gobPath := filepath.Join(dir, ".cache", "aps", "session-meta.gob")
	if _, err := os.Stat(gobPath); err != nil {
		t.Fatalf("expected gob file at %s: %v", gobPath, err)
	}
	// Reload via LoadMetaCache and verify round-trip
	c2 := LoadMetaCache()
	got, ok := c2.Lookup("/foo.jsonl", mtime, 42)
	if !ok {
		t.Fatal("expected cache hit after LoadMetaCache reload")
	}
	if got.Title != "T" || got.CWD != "/p" {
		t.Errorf("unexpected entry: %+v", got)
	}
}
