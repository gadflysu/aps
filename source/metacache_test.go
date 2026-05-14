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
		Mtime:    mtime,
		Size:     1234,
		Title:    "My Session",
		CWD:      "/home/user/project",
		MsgCount: 42,
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

func TestMetaCache_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "meta.gob")
	c1 := newMetaCacheWithPath(path)
	mtime := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	want := MetaEntry{
		Mtime:    mtime,
		Size:     5678,
		Title:    "Round Trip Session",
		CWD:      "/projects/foo",
		MsgCount: 7,
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
