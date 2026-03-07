package storage

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/testutil"
)

func TestFileLock_AcquireAndRelease(t *testing.T) {
	// Arrange
	tempDir := t.TempDir()
	lockPath := filepath.Join(tempDir, "test.lock")

	// Act: Acquire the lock
	lockFile, err := acquireFileLock(lockPath)

	// Assert: Acquisition
	if err != nil {
		t.Fatalf("acquireFileLock failed: %v", err)
	}
	if lockFile == nil {
		t.Fatal("acquireFileLock returned nil file")
	}
	defer releaseFileLock(lockFile) // Ensure cleanup

	// Act: Release the lock
	err = releaseFileLock(lockFile)

	// Assert: Release
	if err != nil {
		t.Fatalf("releaseFileLock failed: %v", err)
	}
	if _, err := os.Stat(lockPath); !errors.Is(err, os.ErrNotExist) {
		t.Error("Lock file was not removed after release")
	}
}

func TestFileLock_DoubleAcquireFails(t *testing.T) {
	// Arrange
	tempDir := t.TempDir()
	lockPath := filepath.Join(tempDir, "test.lock")

	// Act: Acquire the first lock
	lockFile1, err := acquireFileLock(lockPath)
	if err != nil {
		t.Fatalf("First acquireFileLock failed: %v", err)
	}
	defer releaseFileLock(lockFile1)

	// Act: Try to acquire the same lock again
	lockFile2, err := acquireFileLock(lockPath)

	// Assert
	if err == nil {
		t.Fatal("Second acquireFileLock should have failed, but it succeeded")
	}
	if lockFile2 != nil {
		t.Error("Second acquireFileLock should not have returned a file handle")
		releaseFileLock(lockFile2)
	}
}

func TestFileLock_ReleaseNilIsSafe(t *testing.T) {
	// Act & Assert: Should not panic
	err := releaseFileLock(nil)
	if err != nil {
		t.Errorf("releaseFileLock(nil) should not return an error, but got: %v", err)
	}
}

func TestFileLock_CannotOpenFile(t *testing.T) {
	// Arrange: Create an invalid path that cannot be a file
	tempDir := t.TempDir()
	fileAsDir := filepath.Join(tempDir, "afile")
	if err := os.WriteFile(fileAsDir, []byte("i am a file"), 0644); err != nil {
		t.Fatal(err)
	}
	invalidLockPath := filepath.Join(fileAsDir, "the.lock") // Path inside a file

	// Act
	_, err := acquireFileLock(invalidLockPath)

	// Assert
	if err == nil {
		t.Fatal("Expected an error when trying to create a lock file at an invalid path, but got none")
	}
}

func TestFileLock_ReleaseAfterFilePreDeleted(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not allow deleting a file that is still open")
	}
	// Acquire lock, then delete the lock file on disk before releasing.
	// releaseFileLock should suppress the "file not found" error from os.Remove.
	tempDir := t.TempDir()
	lockPath := filepath.Join(tempDir, "predeleted.lock")

	lockFile, err := acquireFileLock(lockPath)
	if err != nil {
		t.Fatalf("acquireFileLock: %v", err)
	}

	// Delete the file from under the lock handle.
	if err := os.Remove(lockPath); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	// Release should succeed (suppress NotExist error).
	if err := releaseFileLock(lockFile); err != nil {
		t.Errorf("releaseFileLock after pre-delete: %v", err)
	}
}

// TestFileLock_PermissionDenied verifies that acquireFileLock returns an error
// (not errWouldBlock) when the lock directory is read-only.
func TestFileLock_PermissionDenied(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows file permission model differs; skip for portability")
	}
	testutil.SkipIfRoot(t, testutil.DetectPlatform(t), "chmod restrictions bypassed by root")

	dir := t.TempDir()
	// Make directory read-only so file creation inside fails.
	if err := os.Chmod(dir, 0555); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	// Restore permissions for cleanup.
	t.Cleanup(func() { os.Chmod(dir, 0755) })

	lockPath := filepath.Join(dir, "test.lock")
	f, err := acquireFileLock(lockPath)
	if f != nil {
		releaseFileLock(f)
		t.Fatal("expected nil file when directory is read-only")
	}
	if err == nil {
		t.Fatal("expected error when directory is read-only")
	}
	// Must NOT be errWouldBlock — this is a permission error, not a contention error.
	if errors.Is(err, errWouldBlock) {
		t.Fatalf("expected permission error, got errWouldBlock")
	}
}

// TestFileLock_ReleaseAlreadyClosed verifies that releasing a file lock on a
// file that has already been closed returns an error (from Close) rather than
// panicking.
func TestFileLock_ReleaseAlreadyClosed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not allow deleting a file that is still open")
	}

	dir := t.TempDir()
	lockPath := filepath.Join(dir, "closed.lock")

	lockFile, err := acquireFileLock(lockPath)
	if err != nil {
		t.Fatalf("acquireFileLock: %v", err)
	}

	// Close the file manually, then release.
	if err := lockFile.Close(); err != nil {
		t.Fatalf("manual Close: %v", err)
	}

	// releaseFileLock should handle the already-closed state without panicking.
	// It's acceptable to return an error from the double-close.
	err = releaseFileLock(lockFile)
	// The test passes as long as it doesn't panic. An error is acceptable.
	_ = err
}

// TestAcquireLockHandle_PermissionDenied verifies that AcquireLockHandle
// returns (nil, false, non-nil-error) for permission-denied scenarios.
func TestAcquireLockHandle_PermissionDenied(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows file permission model differs; skip for portability")
	}
	testutil.SkipIfRoot(t, testutil.DetectPlatform(t), "chmod restrictions bypassed by root")

	dir := t.TempDir()
	if err := os.Chmod(dir, 0555); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0755) })

	lockPath := filepath.Join(dir, "test.lock")
	f, ok, err := AcquireLockHandle(lockPath)
	if f != nil {
		_ = f.Close()
		t.Fatal("expected nil file on permission error")
	}
	if ok {
		t.Fatal("expected ok=false on permission error")
	}
	if err == nil {
		t.Fatal("expected non-nil error on permission error")
	}
}
