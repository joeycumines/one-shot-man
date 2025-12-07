package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// FileSystemBackend implements StorageBackend using the local file system.
type FileSystemBackend struct {
	sessionID string
	lockFile  *os.File
}

// NewFileSystemBackend creates a new file system storage backend.
// It acquires an exclusive lock on the session to prevent concurrent access.
func NewFileSystemBackend(sessionID string) (*FileSystemBackend, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("sessionID cannot be empty")
	}

	// Ensure the session directory exists
	sessionDir, err := SessionDirectory()
	if err != nil {
		return nil, fmt.Errorf("failed to get session directory: %w", err)
	}
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create session directory: %w", err)
	}

	// Acquire exclusive lock on the session
	lockPath, err := SessionLockFilePath(sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get lock file path: %w", err)
	}

	lockFile, err := acquireFileLock(lockPath)
	if err != nil {
		return nil, fmt.Errorf("failed to acquire session lock: %w", err)
	}

	backend := &FileSystemBackend{
		sessionID: sessionID,
		lockFile:  lockFile,
	}

	return backend, nil
}

// LoadSession retrieves a session by its unique ID.
// It returns (nil, nil) if the session does not exist.
func (b *FileSystemBackend) LoadSession(sessionID string) (*Session, error) {
	if sessionID != b.sessionID {
		return nil, fmt.Errorf("session ID mismatch: backend is locked for %q, requested %q", b.sessionID, sessionID)
	}

	sessionPath, err := SessionFilePath(sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get session file path: %w", err)
	}

	// Check if the session file exists
	data, err := os.ReadFile(sessionPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read session file: %w", err)
	}

	// Deserialize the session
	var session Session
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("failed to unmarshal session: %w", err)
	}

	// Version validation is handled by StateManager to allow graceful recovery

	return &session, nil
}

// SaveSession atomically persists the entire session state.
func (b *FileSystemBackend) SaveSession(session *Session) error {
	if session.ID != b.sessionID {
		return fmt.Errorf("session ID mismatch: backend is locked for %q, session has %q", b.sessionID, session.ID)
	}

	sessionPath, err := SessionFilePath(session.ID)
	if err != nil {
		return fmt.Errorf("failed to get session file path: %w", err)
	}

	// Update the session metadata
	session.Version = CurrentSchemaVersion
	session.UpdateTime = time.Now()

	// Serialize the session
	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal session: %w", err)
	}

	// Write atomically
	if err := AtomicWriteFile(sessionPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write session file: %w", err)
	}

	return nil
}

// ArchiveSession safely archives an existing session file to a destination path.
// Uses atomic os.Rename when both files are on the same filesystem.
// If the source session file doesn't exist, returns nil (no-op).
func (b *FileSystemBackend) ArchiveSession(sessionID string, destPath string) error {
	if sessionID != b.sessionID {
		return fmt.Errorf("session ID mismatch: backend is locked for %q, archive requested for %q", b.sessionID, sessionID)
	}

	if destPath == "" {
		return fmt.Errorf("destPath cannot be empty")
	}

	sessionPath, err := SessionFilePath(sessionID)
	if err != nil {
		return fmt.Errorf("failed to get session file path: %w", err)
	}

	// Check if source session file exists
	if _, err := os.Stat(sessionPath); err != nil {
		if os.IsNotExist(err) {
			return nil // No-op: session doesn't exist
		}
		return fmt.Errorf("failed to stat session file: %w", err)
	}

	// Ensure destination directory exists
	destDir := filepath.Dir(destPath)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("failed to create archive directory: %w", err)
	}

	// Ensure destination does not already exist. We must guarantee that
	// we never silently overwrite an existing archive file; using a
	// non-atomic check-then-rename would open a TOCTOU window on POSIX
	// platforms. Instead we rely on an exclusive-create copy path which
	// the kernel enforces atomically.
	if _, err := os.Stat(destPath); err == nil {
		return os.ErrExist
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to stat destination: %w", err)
	}

	// Read the session contents (session files are small so in-memory copy
	// is acceptable).
	data, err := os.ReadFile(sessionPath)
	if err != nil {
		return fmt.Errorf("failed to read session for archive: %w", err)
	}

	// Try to create the destination atomically with O_EXCL to ensure it
	// doesn't already exist. If it does exist, surface the error so callers
	// can retry with a different counter.
	dstFile, err := os.OpenFile(destPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		if os.IsExist(err) {
			return os.ErrExist
		}
		return fmt.Errorf("failed to create archive destination: %w", err)
	}

	if err := func() (returnedErr error) {
		var shouldntRemoveDest bool
		defer func() {
			if !shouldntRemoveDest {
				_ = os.Remove(destPath)
			}
		}()

		defer func() {
			if err := dstFile.Close(); err != nil {
				shouldntRemoveDest = false
				if returnedErr == nil {
					returnedErr = fmt.Errorf("failed to close archive destination: %w", err)
				}
			}
		}()

		// Write contents and ensure durable write before removing source.
		// We keep the destination file if subsequent removal of the source
		// fails so we do not destroy the user's data.
		// Write contents and ensure durable write before removing source. On
		// Windows the file cannot be removed while the handle is still open, so
		// ensure we close the destination before attempting any cleanup.
		if _, err := dstFile.Write(data); err != nil {
			return fmt.Errorf("failed to write archive destination: %w", err)
		}
		if err := dstFile.Sync(); err != nil {
			return fmt.Errorf("failed to sync archive destination: %w", err)
		}

		shouldntRemoveDest = true

		return nil
	}(); err != nil {
		return err
	}

	// Attempt to remove original session file. If removal fails we return an
	// error but leave the newly created archive intact to avoid destroying
	// the user's previous session state.
	if err := os.Remove(sessionPath); err != nil {
		return fmt.Errorf("failed to remove original session after archive: %w", err)
	}

	return nil
}

// Close releases the session lock and performs cleanup.
func (b *FileSystemBackend) Close() error {
	if b.lockFile == nil {
		return nil
	}

	if err := releaseFileLock(b.lockFile); err != nil {
		return fmt.Errorf("failed to release session lock: %w", err)
	}

	b.lockFile = nil
	return nil
}

// Ensure FileSystemBackend implements StorageBackend at compile time
var _ StorageBackend = (*FileSystemBackend)(nil)
