//go:build !windows

package storage

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// acquireFileLock attempts to acquire an exclusive lock on the given file.
// Returns the file handle on success, or an error if the lock cannot be acquired.
var acquireFileLock = func(path string) (*os.File, error) {
	// Create or open the lock file
	lockFile, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open lock file: %w", err)
	}

	// Try to acquire an exclusive, non-blocking lock
	err = unix.Flock(int(lockFile.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if err != nil {
		lockFile.Close()
		if errors.Is(err, unix.EWOULDBLOCK) {
			return nil, ErrWouldBlock
		}
		return nil, fmt.Errorf("failed to acquire file lock: %w", err)
	}

	return lockFile, nil
}

// releaseFileLock releases the lock and removes the lock file.
func releaseFileLock(lockFile *os.File) error {
	if lockFile == nil {
		return nil
	}

	path := lockFile.Name()

	// Release the lock (Flock on unix doesn't return an error for LOCK_UN)
	unix.Flock(int(lockFile.Fd()), unix.LOCK_UN)

	err1 := lockFile.Close()
	err2 := os.Remove(path)

	// Ignore "file does not exist" for the final removal
	if err2 != nil && !os.IsNotExist(err2) {
		// keep the error
	} else {
		err2 = nil
	}

	return errors.Join(err1, err2)
}
