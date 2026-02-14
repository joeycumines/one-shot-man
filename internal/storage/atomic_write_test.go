package storage

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// longDir is an excessively long path segment that, when combined
// with temp directory path components, exceeds PATH_MAX,
// causing MkdirAll to fail regardless of user permissions or root status.
var longDir = strings.Repeat("x", 256)

func TestAtomicWriteFile(t *testing.T) {
	t.Run("successful write", func(t *testing.T) {
		// Arrange
		tempDir := t.TempDir()
		filename := filepath.Join(tempDir, "test.txt")
		data := []byte("hello world")
		perm := os.FileMode(0644)

		// Act
		err := AtomicWriteFile(filename, data, perm)

		// Assert
		if err != nil {
			t.Fatalf("AtomicWriteFile failed: %v", err)
		}

		// Verify file content
		readData, err := os.ReadFile(filename)
		if err != nil {
			t.Fatalf("Failed to read back file: %v", err)
		}
		if string(readData) != string(data) {
			t.Errorf("File content mismatch: got %q, want %q", string(readData), string(data))
		}
	})

	t.Run("directory creation failure", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("Skipping directory-permission failure test on Windows")
		}
		// Arrange: Use a path with a null byte to simulate directory creation error.
		// This causes MkdirAll to fail regardless of user permissions, even for root.
		tempDir := t.TempDir()

		// Create a subdirectory that we'll make into a file to sabotage MkdirAll
		subdir := filepath.Join(tempDir, "parent", "child")
		if err := os.MkdirAll(subdir, 0755); err != nil {
			t.Fatal(err)
		}

		// Replace the subdirectory with a file - MkdirAll will fail when trying
		// to reuse this path in the atomic write operation
		if err := os.RemoveAll(subdir); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(subdir, []byte("file"), 0644); err != nil {
			t.Fatal(err)
		}

		filename := filepath.Join(subdir, "test.txt")
		data := []byte("data")

		// Act
		err := AtomicWriteFile(filename, data, 0644)

		// Assert
		if err == nil {
			t.Fatal("Expected an error but got none")
		}
		// The target file should not have been created
		// os.Stat on the directory path we sabotaged will also fail,
		// which is acceptable (file doesn't exist in any usable way)
		_, statErr := os.Stat(filename)
		if statErr == nil {
			t.Error("File should not have been created, but exists")
		}
	})

	t.Run("temp file creation failure", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("Skipping temp-file-creation failure test on Windows")
		}
		// Arrange: Create a file where we need a directory.
		// This causes MkdirAll to fail regardless of user permissions.
		tempDir := t.TempDir()
		targetDir := filepath.Join(tempDir, "target")
		// Create a file with the target directory name
		if err := os.WriteFile(targetDir, []byte(""), 0644); err != nil {
			t.Fatal(err)
		}
		filename := filepath.Join(tempDir, longDir, "test.txt")
		data := []byte("data")

		// Act
		err := AtomicWriteFile(filename, data, 0644)

		// Assert
		if err == nil {
			t.Fatal("Expected an error but got none")
		}
		// The target file should not have been created
		// For extremely long paths, os.Stat might also fail ENAMETOOLONG,
		// which is acceptable (file doesn't exist in any usable way)
		_, statErr := os.Stat(filename)
		if statErr == nil {
			t.Error("File should not have been created, but exists")
		}
		// Either IsNotExist (file doesn't exist) or any other error (path too long) is OK
	})

	t.Run("rename failure and cleanup", func(t *testing.T) {
		// Arrange: Create a directory where the target file should be, causing Rename to fail.
		tempDir := t.TempDir()
		filename := filepath.Join(tempDir, "test.txt")
		if err := os.Mkdir(filename, 0755); err != nil {
			t.Fatalf("Failed to create conflicting directory: %v", err)
		}
		data := []byte("data")

		// Act
		err := AtomicWriteFile(filename, data, 0644)

		// Assert
		if err == nil {
			t.Fatal("Expected an error but got none")
		}

		// NEW, MORE ROBUST CHECK:
		// This assumes the error returned from AtomicWriteFile can be inspected
		// to find the temporary file path.
		var renameErr RenameError
		if !errors.As(err, &renameErr) {
			t.Fatalf("Expected error to be of type RenameError, but got %T: %v", err, err)
		}

		// Now that we have the specific error type, check for cleanup.
		tempPath := renameErr.TempPath()
		if _, statErr := os.Stat(tempPath); !os.IsNotExist(statErr) {
			t.Errorf("Temporary file %q was not cleaned up after rename failure", tempPath)
		}
	})

	t.Run("write to nested directory", func(t *testing.T) {
		// Arrange
		tempDir := t.TempDir()
		filename := filepath.Join(tempDir, "a", "b", "c", "test.txt")
		data := []byte("nested hello")
		perm := os.FileMode(0644)

		// Act
		err := AtomicWriteFile(filename, data, perm)

		// Assert
		if err != nil {
			t.Fatalf("AtomicWriteFile with nested dirs failed: %v", err)
		}
		readData, err := os.ReadFile(filename)
		if err != nil {
			t.Fatalf("Failed to read back file: %v", err)
		}
		if string(readData) != string(data) {
			t.Errorf("File content mismatch: got %q, want %q", string(readData), string(data))
		}
	})

	t.Run("crash hook is invoked before rename", func(t *testing.T) {
		tempDir := t.TempDir()
		filename := filepath.Join(tempDir, "hook-test.txt")

		hookCalled := false
		SetTestHookCrashBeforeRename(func() {
			hookCalled = true
		})
		defer SetTestHookCrashBeforeRename(nil)

		err := AtomicWriteFile(filename, []byte("hooked"), 0644)
		if err != nil {
			t.Fatalf("AtomicWriteFile with hook failed: %v", err)
		}
		if !hookCalled {
			t.Fatal("expected crash hook to be called")
		}

		// File should still be written successfully.
		data, err := os.ReadFile(filename)
		if err != nil {
			t.Fatalf("ReadFile failed: %v", err)
		}
		if string(data) != "hooked" {
			t.Errorf("file content = %q, want %q", string(data), "hooked")
		}
	})
}

func TestRenameError_Methods(t *testing.T) {
	inner := fmt.Errorf("some rename error")
	re := RenameError{Err: inner, tempPath: "/tmp/foo"}

	if got := re.Error(); got != "some rename error" {
		t.Errorf("Error() = %q, want %q", got, "some rename error")
	}
	if got := re.TempPath(); got != "/tmp/foo" {
		t.Errorf("TempPath() = %q, want %q", got, "/tmp/foo")
	}
	if got := re.Unwrap(); got != inner {
		t.Errorf("Unwrap() returned %v, want %v", got, inner)
	}

	// Verify errors.Is works through Unwrap.
	sentinel := fmt.Errorf("sentinel")
	re2 := RenameError{Err: fmt.Errorf("wrapped: %w", sentinel), tempPath: "/tmp/bar"}
	if !errors.Is(re2, sentinel) {
		t.Error("expected errors.Is to find sentinel through Unwrap chain")
	}
}

func TestSetTestHookCrashBeforeRename(t *testing.T) {
	// Ensure we start clean.
	SetTestHookCrashBeforeRename(nil)

	// Set hook and verify it fires.
	called := false
	SetTestHookCrashBeforeRename(func() { called = true })
	if testHookCrashBeforeRename == nil {
		t.Fatal("expected hook to be set")
	}
	testHookCrashBeforeRename()
	if !called {
		t.Fatal("expected hook to be called")
	}

	// Clear and verify nil.
	SetTestHookCrashBeforeRename(nil)
	if testHookCrashBeforeRename != nil {
		t.Fatal("expected hook to be nil after clearing")
	}
}
