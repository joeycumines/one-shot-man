//go:build windows

package storage

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

// Verifies that attempting to delete a file while an open handle exists will
// fail on Windows with a sharing violation or permission error. This test is
// Windows-only and is skipped on other platforms.
func TestDeleteWhileHandleOpenFailsOnWindows(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "locked-file.txt")
	if err := os.WriteFile(path, []byte("x"), 0644); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	// Open the file without FILE_SHARE_DELETE which is the default behaviour
	// for os.OpenFile on Windows.
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatalf("open failed: %v", err)
	}
	defer f.Close()

	// Attempt to remove while the handle is open — expect an error.
	err = os.Remove(path)
	if err == nil {
		t.Fatalf("expected remove to fail on Windows when handle is open")
	}

	// The underlying error is commonly ERROR_SHARING_VIOLATION. Ensure the
	// returned error either maps to that or is a permission-like error.
	if !errors.Is(err, windows.ERROR_SHARING_VIOLATION) && !os.IsPermission(err) {
		t.Fatalf("expected sharing violation or permission denied, got: %v", err)
	}
}
