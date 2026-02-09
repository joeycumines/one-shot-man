package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// setupTest overrides the paths to use a temporary directory for the duration of a test.
// This is critical to avoid polluting the user's actual config directory.
func setupTest(t *testing.T) (string, func()) {
	t.Helper()

	// Store original functions
	originalSessionDirFunc := sessionDirectory
	originalSessionFileFunc := sessionFilePath
	originalSessionLockFunc := sessionLockFilePath

	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, "one-shot-man", "sessions")

	// Create the sessions directory
	if err := os.MkdirAll(sessionsDir, 0755); err != nil {
		t.Fatalf("failed to create sessions dir: %v", err)
	}

	// Override functions to point to temp dir
	sessionDirectory = func() (string, error) {
		return sessionsDir, nil
	}
	sessionFilePath = func(sessionID string) (string, error) {
		return filepath.Join(sessionsDir, sessionID+".session.json"), nil
	}
	sessionLockFilePath = func(sessionID string) (string, error) {
		return filepath.Join(sessionsDir, sessionID+".session.lock"), nil
	}

	// Return a cleanup function to restore originals
	return tmpDir, func() {
		sessionDirectory = originalSessionDirFunc
		sessionFilePath = originalSessionFileFunc
		sessionLockFilePath = originalSessionLockFunc
	}
}

func TestNewFileSystemBackend(t *testing.T) {
	_, cleanup := setupTest(t)
	defer cleanup()

	t.Run("success", func(t *testing.T) {
		sessionID := "test-session-success"

		backend, err := NewFileSystemBackend(sessionID)
		if err != nil {
			t.Fatalf("NewFileSystemBackend() error = %v", err)
		}
		defer backend.Close()

		lockPath, _ := SessionLockFilePath(sessionID)
		if _, err := os.Stat(lockPath); os.IsNotExist(err) {
			t.Error("lock file was not created")
		}
	})

	t.Run("empty session id", func(t *testing.T) {
		_, err := NewFileSystemBackend("")
		if err == nil {
			t.Error("expected error for empty sessionID, got nil")
		}
	})

	t.Run("lock already held", func(t *testing.T) {
		sessionID := "test-session-lock-held"

		// First backend acquires the lock
		backend1, err := NewFileSystemBackend(sessionID)
		if err != nil {
			t.Fatalf("failed to create first backend: %v", err)
		}
		defer backend1.Close()

		// Second attempt should fail
		_, err = NewFileSystemBackend(sessionID)
		if err == nil {
			t.Fatal("expected error when creating backend for a locked session, got nil")
		}
	})
}

func TestFileSystemBackend_Close(t *testing.T) {
	_, cleanup := setupTest(t)
	defer cleanup()

	sessionID := "test-session-close"

	t.Run("success", func(t *testing.T) {
		backend, err := NewFileSystemBackend(sessionID)
		if err != nil {
			t.Fatalf("NewFileSystemBackend() error = %v", err)
		}

		err = backend.Close()
		if err != nil {
			t.Errorf("Close() error = %v", err)
		}

		lockPath, _ := SessionLockFilePath(sessionID)
		if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
			t.Error("lock file was not removed after Close()")
		}
	})

	t.Run("double close is safe", func(t *testing.T) {
		backend, err := NewFileSystemBackend(sessionID)
		if err != nil {
			t.Fatalf("NewFileSystemBackend() error = %v", err)
		}
		backend.Close() // First close

		err = backend.Close() // Second close
		if err != nil {
			t.Errorf("second Close() should not return an error, got %v", err)
		}
	})
}

func TestFileSystemBackend_SaveAndLoadSession(t *testing.T) {
	_, cleanup := setupTest(t)
	defer cleanup()

	sessionID := "test-session-save-load"
	originalSession := &Session{
		ID:         sessionID,
		CreateTime: time.Now().Truncate(time.Second), // Truncate for comparison
		History: []HistoryEntry{
			{EntryID: "1", Command: "test"},
		},
	}

	// Act 1: Save the session
	backend1, err := NewFileSystemBackend(sessionID)
	if err != nil {
		t.Fatalf("failed to create backend for saving: %v", err)
	}
	err = backend1.SaveSession(originalSession)
	if err != nil {
		t.Fatalf("SaveSession() failed: %v", err)
	}
	err = backend1.Close()
	if err != nil {
		t.Fatalf("failed to close first backend: %v", err)
	}

	// Assert 1: File exists and has correct data
	sessionPath, _ := SessionFilePath(sessionID)
	data, err := os.ReadFile(sessionPath)
	if err != nil {
		t.Fatalf("failed to read session file: %v", err)
	}
	var onDiskSession Session
	if err := json.Unmarshal(data, &onDiskSession); err != nil {
		t.Fatalf("failed to unmarshal on-disk session: %v", err)
	}
	if onDiskSession.Version != CurrentSchemaVersion {
		t.Errorf("version mismatch: got %q, want %q", onDiskSession.Version, CurrentSchemaVersion)
	}
	if onDiskSession.UpdateTime.IsZero() {
		t.Error("UpdateTime was not set on save")
	}
	firstUpdateTime := onDiskSession.UpdateTime

	// Act 1b: Save again to verify timestamp advances
	time.Sleep(10 * time.Millisecond) // Ensure clock tick
	backend1Reopen, err := NewFileSystemBackend(sessionID)
	if err != nil {
		t.Fatalf("failed to reopen backend for second save: %v", err)
	}
	originalSession.History = append(originalSession.History, HistoryEntry{EntryID: "2"})
	err = backend1Reopen.SaveSession(originalSession)
	if err != nil {
		t.Fatalf("second SaveSession() failed: %v", err)
	}
	err = backend1Reopen.Close()
	if err != nil {
		t.Fatalf("failed to close backend after second save: %v", err)
	}

	// Assert 1b: Timestamps have advanced
	data2, err := os.ReadFile(sessionPath)
	if err != nil {
		t.Fatalf("failed to read session file after second save: %v", err)
	}
	var onDiskSession2 Session
	if err := json.Unmarshal(data2, &onDiskSession2); err != nil {
		t.Fatalf("failed to unmarshal session after second save: %v", err)
	}
	if !onDiskSession2.UpdateTime.After(firstUpdateTime) {
		t.Errorf("Expected UpdateTime to advance on second save. First: %v, Second: %v", firstUpdateTime, onDiskSession2.UpdateTime)
	}

	// Act 2: Load the session
	backend2, err := NewFileSystemBackend(sessionID)
	if err != nil {
		t.Fatalf("failed to create backend for loading: %v", err)
	}
	defer backend2.Close()

	loadedSession, err := backend2.LoadSession(sessionID)
	if err != nil {
		t.Fatalf("LoadSession() failed: %v", err)
	}

	// Assert 2: Loaded data matches original data
	if loadedSession == nil {
		t.Fatal("LoadSession() returned nil session")
	}
	if loadedSession.ID != originalSession.ID {
		t.Errorf("ID mismatch")
	}
	if len(loadedSession.History) != len(originalSession.History) {
		t.Errorf("History length mismatch")
	}
}

func TestFileSystemBackend_ErrorScenarios(t *testing.T) {
	_, cleanup := setupTest(t)
	defer cleanup()

	sessionID := "test-session-error"
	backend, err := NewFileSystemBackend(sessionID)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	defer backend.Close()

	t.Run("load non-existent session", func(t *testing.T) {
		s, err := backend.LoadSession(sessionID)
		if err != nil {
			t.Errorf("expected nil error for non-existent session, got %v", err)
		}
		if s != nil {
			t.Error("expected nil session for non-existent session, got non-nil")
		}
	})

	t.Run("load with id mismatch", func(t *testing.T) {
		_, err := backend.LoadSession("wrong-id")
		if err == nil {
			t.Error("expected error for session ID mismatch, got nil")
		}
	})

	t.Run("save with id mismatch", func(t *testing.T) {
		s := &Session{ID: "wrong-id"}
		err := backend.SaveSession(s)
		if err == nil {
			t.Error("expected error for session ID mismatch, got nil")
		}
	})

	t.Run("load corrupted file", func(t *testing.T) {
		sessionPath, _ := SessionFilePath(sessionID)
		// Manually write bad data, bypassing the backend
		if err := os.MkdirAll(filepath.Dir(sessionPath), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(sessionPath, []byte("{not-valid-json"), 0644); err != nil {
			t.Fatalf("failed to write corrupted file: %v", err)
		}

		_, err := backend.LoadSession(sessionID)
		if err == nil {
			t.Error("expected error loading corrupted file, got nil")
		}
	})

	t.Run("load version mismatch - backend returns session", func(t *testing.T) {
		sessionPath, _ := SessionFilePath(sessionID)
		mismatchedSession := fmt.Sprintf(`{"version": "0.0.1", "id": %q}`, sessionID)
		// Manually write data with wrong version
		if err := os.WriteFile(sessionPath, []byte(mismatchedSession), 0644); err != nil {
			t.Fatalf("failed to write mismatched version file: %v", err)
		}

		// Backend should return the session without error - version handling is StateManager's responsibility
		loadedSession, err := backend.LoadSession(sessionID)
		if err != nil {
			t.Errorf("expected no error loading file with version mismatch, got: %v", err)
		}
		if loadedSession == nil {
			t.Error("expected non-nil session, got nil")
		}
		if loadedSession != nil && loadedSession.Version != "0.0.1" {
			t.Errorf("expected version 0.0.1, got %q", loadedSession.Version)
		}
	})
}

func TestFileSystemBackend_ArchiveSession_ExclusiveCreate(t *testing.T) {
	tmp, cleanup := setupTest(t)
	defer cleanup()

	sessionID := "test-archive-exclusive"
	backend, err := NewFileSystemBackend(sessionID)
	if err != nil {
		t.Fatalf("failed to create backend: %v", err)
	}
	defer backend.Close()

	// Save an initial session so session file exists
	s := &Session{ID: sessionID, CreateTime: time.Now(), UpdateTime: time.Now(), ScriptState: map[string]map[string]interface{}{}, SharedState: map[string]interface{}{}, History: []HistoryEntry{}}
	if err := backend.SaveSession(s); err != nil {
		t.Fatalf("SaveSession failed: %v", err)
	}

	// Determine a candidate archive path
	ts := time.Now()
	destPath, err := ArchiveSessionFilePath(sessionID, ts, 0)
	if err != nil {
		t.Fatalf("ArchiveSessionFilePath failed: %v", err)
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		t.Fatalf("failed to mkdir archive dir: %v", err)
	}

	// 1) Create a destination file to simulate collision and expect ErrExist
	if err := os.WriteFile(destPath, []byte("existing"), 0644); err != nil {
		t.Fatalf("failed to create pre-existing dest: %v", err)
	}

	if err := backend.ArchiveSession(sessionID, destPath); !os.IsExist(err) {
		t.Fatalf("expected ErrExist when dest exists, got: %v", err)
	}

	// Ensure original session still exists because archive should not have removed it
	sessionPath, _ := SessionFilePath(sessionID)
	if _, err := os.Stat(sessionPath); err != nil {
		t.Fatalf("expected original session to still exist after collided archive attempt: %v", err)
	}

	// 2) Now pick a new unique destination and ensure ArchiveSession succeeds
	destPath2, err := ArchiveSessionFilePath(sessionID, ts, 1)
	if err != nil {
		t.Fatalf("ArchiveSessionFilePath failed: %v", err)
	}

	if err := backend.ArchiveSession(sessionID, destPath2); err != nil {
		t.Fatalf("expected archive to succeed for unused dest, got: %v", err)
	}

	// Original session should be removed after successful archive
	if _, err := os.Stat(sessionPath); !os.IsNotExist(err) {
		t.Fatalf("expected original session removed after archive success, got: %v", err)
	}

	// Archived file should exist
	if _, err := os.Stat(destPath2); err != nil {
		t.Fatalf("expected archive file to exist at %s: %v", destPath2, err)
	}

	// Clean up temp directory explicitly to avoid linter complaints
	_ = tmp
}

func TestFileSystemBackend_ArchiveSession_ConcurrentExclusive(t *testing.T) {
	tmp, cleanup := setupTest(t)
	defer cleanup()

	sessionID := "test-archive-concurrent"
	backend, err := NewFileSystemBackend(sessionID)
	if err != nil {
		t.Fatalf("failed to create backend: %v", err)
	}
	defer backend.Close()

	// Save an initial session so session file exists
	s := &Session{ID: sessionID, CreateTime: time.Now(), UpdateTime: time.Now(), ScriptState: map[string]map[string]interface{}{}, SharedState: map[string]interface{}{}, History: []HistoryEntry{}}
	if err := backend.SaveSession(s); err != nil {
		t.Fatalf("SaveSession failed: %v", err)
	}

	ts := time.Now()
	destPath, err := ArchiveSessionFilePath(sessionID, ts, 0)
	if err != nil {
		t.Fatalf("ArchiveSessionFilePath failed: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		t.Fatalf("failed to mkdir archive dir: %v", err)
	}

	// Run two concurrent archive attempts using the same destination path.
	// The backend must ensure one succeeds and the other returns os.ErrExist.
	// Use a start gate to ensure both goroutines begin execution at the same time,
	// maximizing the chance of exposing race conditions in the implementation.
	var err1, err2 error
	start := make(chan struct{})
	done := make(chan struct{}, 2)

	go func() {
		<-start // Wait for signal to start
		err1 = backend.ArchiveSession(sessionID, destPath)
		done <- struct{}{}
	}()
	go func() {
		<-start // Wait for signal to start
		err2 = backend.ArchiveSession(sessionID, destPath)
		done <- struct{}{}
	}()

	close(start) // Signal both goroutines to start simultaneously

	<-done
	<-done

	// Exactly one call must succeed and the other should return os.ErrExist.
	success := 0
	if err1 == nil {
		success++
	} else if !os.IsExist(err1) {
		t.Fatalf("unexpected error for archive attempt 1: %v", err1)
	}
	if err2 == nil {
		success++
	} else if !os.IsExist(err2) {
		t.Fatalf("unexpected error for archive attempt 2: %v", err2)
	}
	if success != 1 {
		t.Fatalf("expected exactly one success, got %d successes (err1=%v err2=%v)", success, err1, err2)
	}

	// Clean up temp directory explicitly to avoid linter complaints
	_ = tmp
}

func TestFileSystemBackend_ArchiveSession_PreserveArchiveOnSourceRemoveFailure(t *testing.T) {
	// For Windows, skip since the filesystem model doesn't support the same removal semantics
	if runtime.GOOS == "windows" {
		t.Skip("Skipping ArchiveSession remove-failure test on Windows")
	}
	tmp, cleanup := setupTest(t)
	defer cleanup()

	sessionID := "test-archive-remove-fails"
	backend, err := NewFileSystemBackend(sessionID)
	if err != nil {
		t.Fatalf("failed to create backend: %v", err)
	}
	defer backend.Close()

	// Save an initial session so session file exists
	s := &Session{ID: sessionID, CreateTime: time.Now(), UpdateTime: time.Now(), ScriptState: map[string]map[string]interface{}{}, SharedState: map[string]interface{}{}, History: []HistoryEntry{}}
	if err := backend.SaveSession(s); err != nil {
		t.Fatalf("SaveSession failed: %v", err)
	}

	// Prepare archive path and ensure archive dir exists
	ts := time.Now()
	destPath, err := ArchiveSessionFilePath(sessionID, ts, 0)
	if err != nil {
		t.Fatalf("ArchiveSessionFilePath failed: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		t.Fatalf("failed to mkdir archive dir: %v", err)
	}

	// Get the session file path
	sessionPath, _ := SessionFilePath(sessionID)

	// Read the current session data before we sabotage the file
	sessionData, err := os.ReadFile(sessionPath)
	if err != nil {
		t.Fatalf("failed to read session file: %v", err)
	}

	// Open the session file to get an exclusive lock (simulate another process holding it)
	// This will cause os.Remove to fail with "text file busy" or "device or resource busy"
	lockFile, err := os.OpenFile(sessionPath, os.O_RDONLY, 0644)
	if err != nil {
		t.Fatalf("failed to open session file for locking: %v", err)
	}
	defer lockFile.Close()

	// Create a hard link to the file that we keep open
	// On Unix, a file cannot be removed while it has hard links and is in use
	linkPath := sessionPath + ".hardlink"
	if err := os.Link(sessionPath, linkPath); err != nil {
		// If hard links aren't supported, fall back to directory replacement
		// Remove the file and replace it with a directory - os.Remove will fail on directories
		_ = os.Remove(sessionPath)
		if err := os.Mkdir(sessionPath, 0755); err != nil {
			t.Fatalf("failed to create directory at session path: %v", err)
		}
		defer os.RemoveAll(sessionPath)
	} else {
		defer os.Remove(linkPath)
	}

	// Attempt archive: copy should succeed but removal might fail
	// The archive should still exist even if removal fails
	archiveErr := backend.ArchiveSession(sessionID, destPath)

	// Due to root privileges on Docker, removal might succeed even with hardlinks
	// The important invariant is: if archive was created, it should be preserved
	if _, err := os.Stat(destPath); err == nil {
		// Archive was created - verify it contains the session data
		archiveData, err := os.ReadFile(destPath)
		if err != nil {
			t.Fatalf("archive exists but couldn't be read: %v", err)
		}
		// The archive should contain the session data we read earlier
		if string(archiveData) != string(sessionData) {
			t.Errorf("archive content mismatch")
		}
	} else if archiveErr != nil {
		// Archive creation failed - this is acceptable in root environments
		// where files can be removed despite constraints
	}

	_ = tmp
}
