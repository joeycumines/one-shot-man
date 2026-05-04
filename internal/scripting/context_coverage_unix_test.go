//go:build unix

package scripting

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// ==========================================================================
// context.go — addPathWithOwnerLocked unsupported type branch (line 178)
// ==========================================================================

// TestAddPath_UnsupportedFileType exercises the "unsupported path type"
// error returned by addPathWithOwnerLocked for non-regular, non-directory
// files (e.g., named pipes, sockets).
func TestAddPath_UnsupportedFileType(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	fifoPath := filepath.Join(base, "test.fifo")

	// Create a named pipe (FIFO). This is neither regular nor directory.
	if err := syscall.Mkfifo(fifoPath, 0o644); err != nil {
		t.Skipf("cannot create FIFO: %v", err)
	}

	cm, err := NewContextManager(base)
	if err != nil {
		t.Fatal(err)
	}

	err = cm.AddPath(fifoPath)
	if err == nil {
		t.Fatal("expected error for unsupported file type (FIFO)")
	}
	if !strings.Contains(err.Error(), "unsupported path type") {
		t.Errorf("expected 'unsupported path type' error, got: %v", err)
	}
}

// ==========================================================================
// context.go — walkDirectory symlink-to-broken error branch (line 271)
// ==========================================================================

// TestWalkDirectory_BrokenSymlink exercises the symlink resolution error
// branch inside walkDirectory where a symlink target doesn't exist.
func TestWalkDirectory_BrokenSymlink(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	dir := filepath.Join(base, "project")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create a broken symlink inside dir
	brokenLink := filepath.Join(dir, "broken.txt")
	if err := os.Symlink("/nonexistent/target", brokenLink); err != nil {
		t.Skip("symlinks not supported")
	}

	cm, err := NewContextManager(base)
	if err != nil {
		t.Fatal(err)
	}

	// AddPath on the directory should fail because the broken symlink
	// can't be resolved by os.Stat during walkDirectory.
	err = cm.AddPath(dir)
	if err == nil {
		t.Fatal("expected error for directory containing broken symlink")
	}
	if !strings.Contains(err.Error(), "failed to resolve symlink") {
		t.Errorf("expected 'failed to resolve symlink' error, got: %v", err)
	}
}
