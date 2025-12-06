package storage

import (
	"os"
	"path/filepath"
	"testing"
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
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
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
