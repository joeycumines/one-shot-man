package scripting

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ==========================================================================
// context.go — AddRelativePath Lstat error branch (line 147-149)
// ==========================================================================

// TestAddRelativePath_NonExistentPath exercises the os.Lstat error branch
// in AddRelativePath when the target does not exist.
func TestAddRelativePath_NonExistentPath(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	cm, err := NewContextManager(base)
	if err != nil {
		t.Fatal(err)
	}

	_, err = cm.AddRelativePath("nonexistent-file.txt")
	if err == nil {
		t.Fatal("expected error for non-existent path")
	}
	if !strings.Contains(err.Error(), "failed to stat path") {
		t.Errorf("expected 'failed to stat path' error, got: %v", err)
	}
}

// ==========================================================================
// context.go — addPathWithOwnerLocked unsupported type branch (line 178)
// See context_coverage_unix_test.go for TestAddPath_UnsupportedFileType
// (requires syscall.Mkfifo, Unix only).
// ==========================================================================

// ==========================================================================
// context.go — normalizeOwnerPath ".." branch (line 62)
// ==========================================================================

// TestNormalizeOwnerPath_ParentDir exercises the branch where the relative
// path resolves to exactly ".." (the parent directory of basePath).
func TestNormalizeOwnerPath_ParentDir(t *testing.T) {
	t.Parallel()

	// Create base/child/ — basePath is child
	base := t.TempDir()
	child := filepath.Join(base, "child")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}

	cm, err := NewContextManager(child)
	if err != nil {
		t.Fatal(err)
	}

	// normalizeOwnerPath(base) should return base (absolute) because
	// Rel(child, base) == ".." which triggers the special case.
	result := cm.normalizeOwnerPath(base)
	if result != base {
		t.Errorf("expected %q for parent-dir input, got %q", base, result)
	}
}

// TestNormalizeOwnerPath_DeepParent exercises the Rel leading-".." branch
// where the path is a grandparent or further ancestor.
func TestNormalizeOwnerPath_DeepParent(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	deep := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}

	cm, err := NewContextManager(deep)
	if err != nil {
		t.Fatal(err)
	}

	// normalizeOwnerPath(root) should return root (absolute) because
	// Rel(deep, root) starts with "../.."
	result := cm.normalizeOwnerPath(root)
	if result != root {
		t.Errorf("expected %q for deep-parent input, got %q", root, result)
	}
}

// ==========================================================================
// context.go — walkDirectory symlink-to-broken error branch (line 271)
// See context_coverage_unix_test.go for TestWalkDirectory_BrokenSymlink
// (requires symlinks, Unix only).
// ==========================================================================

// ==========================================================================
// context.go — AddPath Lstat error branch (line 98)
// ==========================================================================

// TestAddPath_NonExistentFile exercises the Lstat error branch in AddPath.
func TestAddPath_NonExistentFile(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	cm, err := NewContextManager(base)
	if err != nil {
		t.Fatal(err)
	}

	err = cm.AddPath(filepath.Join(base, "does-not-exist.txt"))
	if err == nil {
		t.Fatal("expected error for non-existent file")
	}
	if !strings.Contains(err.Error(), "failed to stat path") {
		t.Errorf("expected 'failed to stat path' error, got: %v", err)
	}
}

// ==========================================================================
// context.go — AddPath file re-add behavior (idempotency)
// ==========================================================================

// TestAddPath_UpdateExistingFile verifies that re-adding an already-tracked
// file updates its content. addPathWithOwnerLocked calls removeOwnerLocked
// before addFileLocked, so the paths entry is recreated — this tests the
// full add-remove-readd cycle for a single-owner file.
func TestAddPath_UpdateExistingFile(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	file := filepath.Join(base, "test.txt")
	if err := os.WriteFile(file, []byte("version 1"), 0o644); err != nil {
		t.Fatal(err)
	}

	cm, err := NewContextManager(base)
	if err != nil {
		t.Fatal(err)
	}

	// First add
	if err := cm.AddPath(file); err != nil {
		t.Fatal(err)
	}

	// Modify file
	if err := os.WriteFile(file, []byte("version 2"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Re-add (should update, not duplicate)
	if err := cm.AddPath(file); err != nil {
		t.Fatal(err)
	}

	// Verify updated content
	owner := cm.normalizeOwnerPath(file)
	cp, ok := cm.GetPath(owner)
	if !ok {
		t.Fatal("expected path to be tracked after re-add")
	}
	if cp.Content != "version 2" {
		t.Errorf("expected 'version 2', got %q", cp.Content)
	}
}

// ==========================================================================
// engine_core.go — Adapter nil-runtime branch (line 592-593)
// ==========================================================================

// TestEngine_Adapter_NilRuntime exercises the nil check in Adapter().
func TestEngine_Adapter_NilRuntime(t *testing.T) {
	t.Parallel()

	// Create a bare Engine with runtime=nil.
	e := &Engine{}
	result := e.Adapter()
	if result != nil {
		t.Errorf("expected nil adapter for nil runtime, got %v", result)
	}
}
