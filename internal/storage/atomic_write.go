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

// atomicFileWriter abstracts *os.File for test error injection.
type atomicFileWriter interface {
	Write([]byte) (int, error)
	Sync() error
	Close() error
}

// testHookAtomicFileWrapper, if non-nil, wraps the *os.File from CreateTemp
// so tests can inject errors for write/sync/close paths.
var testHookAtomicFileWrapper func(*os.File) atomicFileWriter

// renameError wraps a rename error with the temporary file path for testing purposes.
type renameError struct {
	Err      error
	tempPath string
}

func (e renameError) Error() string    { return e.Err.Error() }
func (e renameError) TempPath() string { return e.tempPath }
func (e renameError) Unwrap() error    { return e.Err }

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

	// Optionally wrap the file for test error injection.
	var writer atomicFileWriter = tempFile
	if testHookAtomicFileWrapper != nil {
		writer = testHookAtomicFileWrapper(tempFile)
	}

	// Capture the temp path immediately and ensure the temp file is removed
	// on any error path. Capturing the path prevents races if the underlying
	// *os.File object is changed or renamed later.
	// On success, the rename operation moves it, so Remove will fail harmlessly.
	var success bool
	tempPath := tempFile.Name()
	defer func() {
		// If we haven't succeeded, clean up the temporary file
		if !success {
			if err := os.Remove(tempPath); err != nil {
				slog.Warn("failed to remove temporary file", "path", tempPath, "error", err)
			}
		}
	}()

	if _, err := writer.Write(data); err != nil {
		writer.Close()
		return fmt.Errorf("failed to write to temp file: %w", err)
	}
	if err := writer.Sync(); err != nil { // Ensure data is on disk.
		writer.Close()
		return fmt.Errorf("failed to sync temp file: %w", err)
	}
	if err := writer.Close(); err != nil {
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
		return renameError{Err: renameErr, tempPath: tempFile.Name()}
	}
	return nil
}
