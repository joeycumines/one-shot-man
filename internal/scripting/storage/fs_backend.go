package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

const currentSchemaVersion = "1.0.0"

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
	if session.SessionID != b.sessionID {
		return fmt.Errorf("session ID mismatch: backend is locked for %q, session has %q", b.sessionID, session.SessionID)
	}

	sessionPath, err := SessionFilePath(session.SessionID)
	if err != nil {
		return fmt.Errorf("failed to get session file path: %w", err)
	}

	// Update the session metadata
	session.Version = currentSchemaVersion
	session.UpdatedAt = time.Now()

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
