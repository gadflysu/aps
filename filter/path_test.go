package filter

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMatches_EmptyFilter(t *testing.T) {
	if !Matches("", false, "/some/path") {
		t.Error("empty pathFilter should match everything")
	}
	if !Matches("", true, "/some/path") {
		t.Error("empty pathFilter strict should match everything")
	}
}

func TestMatches_ExactMatch(t *testing.T) {
	dir := t.TempDir()
	if !Matches(dir, true, dir) {
		t.Errorf("exact match should return true for dir=%s", dir)
	}
}

func TestMatches_SubstringNonStrict(t *testing.T) {
	// Raw substring fallback: pathFilter >= 3 chars, non-strict
	if !Matches("projects", false, "/home/user/projects/foo") {
		t.Error("substring 'projects' should match in non-strict mode")
	}
}

func TestMatches_SubstringTooShort(t *testing.T) {
	// len("ab") == 2, must be > 2
	if Matches("ab", false, "/home/user/ab/something") {
		t.Error("substring of length 2 should not match (min is 3)")
	}
}

func TestMatches_SubstringExactlyThreeChars(t *testing.T) {
	if !Matches("foo", false, "/home/user/foobar") {
		t.Error("substring of length 3 should match")
	}
}

func TestMatches_StrictModeBlocksSubstringWhenPathExists(t *testing.T) {
	// Create a real directory so pathExists=true
	dir := t.TempDir()
	// cwd is a different path that contains dir as substring
	cwd := filepath.Join(dir, "subdir")
	// cwd doesn't need to exist for the filter check itself

	// In strict mode with an existing path, substring should NOT match
	// unless it's an exact or symlink match
	if Matches(dir, true, cwd) {
		t.Errorf("strict mode should block substring match when path exists: filter=%s cwd=%s", dir, cwd)
	}
}

func TestMatches_StrictModeAllowsSubstringWhenPathNotExist(t *testing.T) {
	// Non-existent path filter: strict mode falls through to raw substring
	filter := "/nonexistent/path/that/does/not/exist/anywhere"
	cwd := "/nonexistent/path/that/does/not/exist/anywhere/project"
	if !Matches(filter, true, cwd) {
		t.Error("strict mode should allow raw substring when filtered path does not exist on disk")
	}
}

func TestExpandHome_TildeSlash(t *testing.T) {
	home, _ := os.UserHomeDir()
	got := expandHome("~/foo/bar")
	want := filepath.Join(home, "foo/bar")
	if got != want {
		t.Errorf("expandHome(~/foo/bar) = %q, want %q", got, want)
	}
}

func TestExpandHome_TildeOnly(t *testing.T) {
	home, _ := os.UserHomeDir()
	got := expandHome("~")
	if got != home {
		t.Errorf("expandHome(~) = %q, want %q", got, home)
	}
}

func TestExpandHome_NoTilde(t *testing.T) {
	input := "/absolute/path"
	got := expandHome(input)
	if got != input {
		t.Errorf("expandHome(%q) = %q, want unchanged", input, got)
	}
}

func TestExpandHome_RelativePath(t *testing.T) {
	input := "relative/path"
	got := expandHome(input)
	if got != input {
		t.Errorf("expandHome(%q) = %q, want unchanged", input, got)
	}
}

func TestDirExists_Existing(t *testing.T) {
	dir := t.TempDir()
	if !dirExists(dir) {
		t.Errorf("dirExists(%q) should be true for existing dir", dir)
	}
}

func TestDirExists_NonExisting(t *testing.T) {
	if dirExists("/nonexistent/path/xyz123abc") {
		t.Error("dirExists should be false for nonexistent path")
	}
}

func TestDirExists_File(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "file")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if dirExists(f) {
		t.Errorf("dirExists(%q) should be false for a regular file", f)
	}
}

// TestMatches_FileNamedSameAsProject reproduces issue #51:
// `aps -la aps` from ~/projects.local/aps fails because filepath.EvalSymlinks("aps")
// resolves to the ./aps binary file. pathExists must NOT be set for files —
// only directories count — so raw substring fallback still fires.
//
// NOTE: cannot run in parallel — calls os.Chdir which mutates process-wide state.
func TestMatches_FileNamedSameAsProject(t *testing.T) {
	// Switch working directory to a temp dir that contains an "aps" binary file.
	dir := t.TempDir()
	binPath := filepath.Join(dir, "aps")
	if err := os.WriteFile(binPath, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(orig) })

	// pathFilter = "aps" (just the name, as the user types it).
	// EvalSymlinks("aps") resolves to ./aps — a file, not a directory.
	// cwd is the project directory that contains "aps" as a substring.
	cwd := filepath.Join(dir, "projects", "aps")

	if !Matches("aps", true, cwd) {
		t.Errorf("file-named-same-as-project: Matches(%q, true, %q) = false, want true", "aps", cwd)
	}
}

func TestMatches_ResolvedSubstringNonStrict(t *testing.T) {
	// Create a real dir; filter is its parent, cwd is a subdirectory.
	parent := t.TempDir()
	child := filepath.Join(parent, "child")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}

	// parent is > 10 chars (TempDir returns something like /tmp/TestXXXXX/001)
	// In non-strict mode, resolved substring check should catch child containing parent
	if !Matches(parent, false, child) {
		t.Errorf("non-strict: resolved substring of parent %q should match child cwd %q", parent, child)
	}
}
