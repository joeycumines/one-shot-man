//go:build windows

package storage

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

// acquireFileLock attempts to acquire an exclusive lock on the given file.
// Returns the file handle on success, or an error if the lock cannot be acquired.
var acquireFileLock = func(path string) (*os.File, error) {
	// Create or open the lock file
	lockFile, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open lock file: %w", err)
	}

	// Try to acquire an exclusive lock
	err = lockFileWindows(lockFile)
	if err != nil {
		lockFile.Close()
		return nil, fmt.Errorf("session is locked by another active process: %w", err)
	}

	return lockFile, nil
}

// releaseFileLock releases the lock and removes the lock file.
// The removal is best-effort: if another process has the file open (which can
// happen under concurrent contention), the error is silently ignored because
// the lock itself has already been released, which is the correctness-critical
// operation.  Windows does not allow deleting a file that is currently open by
// any process, so a "file in use" error here indicates a benign race.
func releaseFileLock(lockFile *os.File) error {
	if lockFile == nil {
		return nil
	}

	path := lockFile.Name()

	err1 := unlockFileWindows(lockFile)
	err2 := lockFile.Close()
	err3 := os.Remove(path)

	// Ignore "file does not exist" — a concurrent releaser already cleaned up.
	// Ignore "access denied" / "being used by another process" — a concurrent
	// acquirer has the file open; the lock was already released above, so the
	// caller's session is correctly unlocked.
	if err3 != nil {
		if errors.Is(err3, os.ErrNotExist) {
			err3 = nil
		} else if isWindowsFileBusy(err3) {
			err3 = nil
		}
	}

	return errors.Join(err1, err2, err3)
}

// lockFileWindows acquires an exclusive lock using LockFileEx.
func lockFileWindows(f *os.File) error {
	// Get the file handle
	handle := windows.Handle(f.Fd())

	// Create an overlapped structure
	var overlapped windows.Overlapped

	// Try to lock the file (non-blocking)
	err := windows.LockFileEx(
		handle,
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0,
		1, // Lock 1 byte
		0,
		&overlapped,
	)
	if err != nil {
		if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
			return errWouldBlock
		}
		return fmt.Errorf("LockFileEx failed: %w", err)
	}
	return nil
}

// unlockFileWindows releases the lock using UnlockFileEx.
func unlockFileWindows(f *os.File) error {
	handle := windows.Handle(f.Fd())
	var overlapped windows.Overlapped
	err := windows.UnlockFileEx(handle, 0, 1, 0, &overlapped)
	if err != nil {
		return fmt.Errorf("UnlockFileEx failed: %w", err)
	}
	return nil
}

// isWindowsFileBusy reports whether the error indicates the file is in use by
// another process and therefore cannot be deleted.  This covers:
//   - ERROR_SHARING_VIOLATION (0x20): "The process cannot access the file
//     because it is being used by another process."
//   - ERROR_ACCESS_DENIED (0x5): returned by some overlayfs/NTFS combinations
//     when a file handle is still open.
func isWindowsFileBusy(err error) bool {
	var errno windows.Errno
	if errors.As(err, &errno) {
		return errno == windows.ERROR_SHARING_VIOLATION || errno == windows.ERROR_ACCESS_DENIED
	}
	return false
}
