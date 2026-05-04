package scripting

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ==========================================================================
// context.go — RemovePath ambiguous basename (line 382)
// ==========================================================================

// TestRemovePath_AmbiguousBasename exercises the "ambiguous path" error
// when multiple tracked files share the same basename and RemovePath is
// called with just the basename.
func TestRemovePath_AmbiguousBasename(t *testing.T) {
	t.Parallel()

	base := t.TempDir()

	// Create two files with the same basename in different directories
	dir1 := filepath.Join(base, "src")
	dir2 := filepath.Join(base, "lib")
	if err := os.MkdirAll(dir1, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir2, 0o755); err != nil {
		t.Fatal(err)
	}
	file1 := filepath.Join(dir1, "util.go")
	file2 := filepath.Join(dir2, "util.go")
	if err := os.WriteFile(file1, []byte("package src"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file2, []byte("package lib"), 0o644); err != nil {
		t.Fatal(err)
	}

	cm, err := NewContextManager(base)
	if err != nil {
		t.Fatal(err)
	}

	// Add both files
	if err := cm.AddPath(file1); err != nil {
		t.Fatal(err)
	}
	if err := cm.AddPath(file2); err != nil {
		t.Fatal(err)
	}

	// RemovePath with just the basename should be ambiguous
	err = cm.RemovePath("util.go")
	if err == nil {
		t.Fatal("expected error for ambiguous basename removal")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("expected 'ambiguous' error, got: %v", err)
	}
}

// ==========================================================================
// context.go — RemovePath single basename match (line 388)
// ==========================================================================

// TestRemovePath_SingleBasenameMatch exercises the single-match basename
// removal logic in RemovePath — when only one tracked path matches the
// bare basename.
func TestRemovePath_SingleBasenameMatch(t *testing.T) {
	t.Parallel()

	base := t.TempDir()

	dir := filepath.Join(base, "src")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "unique.go")
	if err := os.WriteFile(file, []byte("package main"), 0o644); err != nil {
		t.Fatal(err)
	}

	cm, err := NewContextManager(base)
	if err != nil {
		t.Fatal(err)
	}

	if err := cm.AddPath(file); err != nil {
		t.Fatal(err)
	}

	// RemovePath with just the basename — only one match
	err = cm.RemovePath("unique.go")
	if err != nil {
		t.Fatalf("expected single-match basename removal to succeed, got: %v", err)
	}

	// Verify the file is no longer tracked
	normalized := cm.normalizeOwnerPath(file)
	if _, ok := cm.GetPath(normalized); ok {
		t.Error("expected file to be removed from tracking")
	}
}

// ==========================================================================
// context.go — RefreshPath untracked path (line 769-770)
// ==========================================================================

// TestRefreshPath_UntrackedPath exercises the "not a tracked owner" error
// when RefreshPath is called with a path that was never added.
func TestRefreshPath_UntrackedPath(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	cm, err := NewContextManager(base)
	if err != nil {
		t.Fatal(err)
	}

	err = cm.RefreshPath("nonexistent.txt")
	if err == nil {
		t.Fatal("expected error for untracked path")
	}
	if !strings.Contains(err.Error(), "not a tracked owner") {
		t.Errorf("expected 'not a tracked owner' error, got: %v", err)
	}
}

// ==========================================================================
// context.go — RefreshPath with deleted file (line 784)
// ==========================================================================

// TestRefreshPath_DeletedFile exercises the os.Lstat error path in
// RefreshPath when the tracked file no longer exists on disk.
func TestRefreshPath_DeletedFile(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	file := filepath.Join(base, "ephemeral.txt")
	if err := os.WriteFile(file, []byte("temp content"), 0o644); err != nil {
		t.Fatal(err)
	}

	cm, err := NewContextManager(base)
	if err != nil {
		t.Fatal(err)
	}

	if err := cm.AddPath(file); err != nil {
		t.Fatal(err)
	}

	// Delete the file from disk
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}

	// RefreshPath should fail because the file no longer exists
	err = cm.RefreshPath(file)
	if err == nil {
		t.Fatal("expected error for deleted file refresh")
	}
	if !strings.Contains(err.Error(), "failed to stat") {
		t.Errorf("expected 'failed to stat' error, got: %v", err)
	}
}

// ==========================================================================
// context.go — RefreshPath normalization fallback (lines 765-770)
// ==========================================================================

// TestRefreshPath_NormalizedPath exercises the normalization fallback in
// RefreshPath where the raw path doesn't match but its normalized form does.
func TestRefreshPath_NormalizedPath(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	file := filepath.Join(base, "target.txt")
	if err := os.WriteFile(file, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}

	cm, err := NewContextManager(base)
	if err != nil {
		t.Fatal(err)
	}

	if err := cm.AddPath(file); err != nil {
		t.Fatal(err)
	}

	// Update the file content
	if err := os.WriteFile(file, []byte("updated"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Call RefreshPath with a different form of the path (e.g., with ./)
	altPath := filepath.Join(base, ".", "target.txt")
	err = cm.RefreshPath(altPath)
	if err != nil {
		t.Fatalf("expected normalized refresh to succeed, got: %v", err)
	}

	// Verify the content was updated
	normalized := cm.normalizeOwnerPath(file)
	cp, ok := cm.GetPath(normalized)
	if !ok {
		t.Fatal("expected path to still be tracked after refresh")
	}
	if cp.Content != "updated" {
		t.Errorf("expected content 'updated', got %q", cp.Content)
	}
}
