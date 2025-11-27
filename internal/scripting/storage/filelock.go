package storage

import (
	"errors"
	"os"
)

// ErrWouldBlock signals that a non-blocking lock attempt failed due to the
// resource being locked by another process.
var ErrWouldBlock = errors.New("file lock would block")

// AcquireLockHandle attempts to acquire an exclusive lock on path and returns
// the underlying file handle if successful. The caller may choose to call
// close only (without removing the lock file) via f.Close(), or call
// releaseFileLock(f) to both release and remove the lock artifact.
func AcquireLockHandle(path string) (*os.File, bool, error) {
	f, err := acquireFileLock(path)
	if err != nil {
		if f != nil {
			_ = f.Close()
		}
		if errors.Is(err, ErrWouldBlock) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return f, true, nil
}

// ReleaseLockHandle releases the lock represented by the provided file handle
// and removes the lock artifact. This is the counterpart to AcquireLockHandle
// for callers that need full control over the underlying file descriptor.
func ReleaseLockHandle(f *os.File) error { return releaseFileLock(f) }
