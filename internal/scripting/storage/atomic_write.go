package storage

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
)

// testHookCrashBeforeRename is a test-only hook to simulate a panic
// during the critical window between writing the temp file and renaming it.
var testHookCrashBeforeRename func()

// SetTestHookCrashBeforeRename sets the test hook for crash simulation.
// This is only for testing purposes.
func SetTestHookCrashBeforeRename(hook func()) {
	testHookCrashBeforeRename = hook
}

// RenameError wraps a rename error with the temporary file path for testing purposes.
type RenameError struct {
	Err      error
	tempPath string
}

func (e RenameError) Error() string    { return e.Err.Error() }
func (e RenameError) TempPath() string { return e.tempPath }
func (e RenameError) Unwrap() error    { return e.Err }

// AtomicWriteFile safely writes data by using a temporary file and an atomic rename.
func AtomicWriteFile(filename string, data []byte, perm os.FileMode) error {
	// Ensure the target directory exists
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Create temp file in the same directory to guarantee atomic rename works.
	tempFile, err := os.CreateTemp(dir, ".tmp-session-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}

	// Ensure the temp file is removed on any error path.
	// On success, the rename operation moves it, so Remove will fail harmlessly.
	var success bool
	defer func() {
		// If we haven't succeeded, clean up the temporary file
		if !success {
			if err := os.Remove(tempFile.Name()); err != nil {
				slog.Warn("failed to remove temporary file", "path", tempFile.Name(), "error", err)
			}
		}
	}()

	if _, err := tempFile.Write(data); err != nil {
		tempFile.Close()
		return fmt.Errorf("failed to write to temp file: %w", err)
	}
	if err := tempFile.Sync(); err != nil { // Ensure data is on disk.
		tempFile.Close()
		return fmt.Errorf("failed to sync temp file: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("failed to close temporary file %q: %w", tempFile.Name(), err)
	}
	if err := os.Chmod(tempFile.Name(), perm); err != nil {
		return fmt.Errorf("failed to chmod temp file: %w", err)
	}

	// Test hook for crash simulation
	if testHookCrashBeforeRename != nil {
		testHookCrashBeforeRename() // Panic will occur here if hook is set
	}

	// Perform the platform-specific atomic rename.
	var renameErr error
	if runtime.GOOS == "windows" {
		renameErr = atomicRenameWindows(tempFile.Name(), filename)
	} else {
		renameErr = os.Rename(tempFile.Name(), filename)
	}

	if renameErr == nil {
		success = true
	}
	if renameErr != nil {
		return RenameError{Err: renameErr, tempPath: tempFile.Name()}
	}
	return nil
}
