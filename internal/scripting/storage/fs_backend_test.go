package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
		SessionID: sessionID,
		CreatedAt: time.Now().Truncate(time.Second), // Truncate for comparison
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
	if onDiskSession.Version != currentSchemaVersion {
		t.Errorf("version mismatch: got %q, want %q", onDiskSession.Version, currentSchemaVersion)
	}
	if onDiskSession.UpdatedAt.IsZero() {
		t.Error("UpdatedAt was not set on save")
	}
	firstUpdatedAt := onDiskSession.UpdatedAt

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
	if !onDiskSession2.UpdatedAt.After(firstUpdatedAt) {
		t.Errorf("Expected UpdatedAt to advance on second save. First: %v, Second: %v", firstUpdatedAt, onDiskSession2.UpdatedAt)
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
	if loadedSession.SessionID != originalSession.SessionID {
		t.Errorf("SessionID mismatch")
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
		s := &Session{SessionID: "wrong-id"}
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
		mismatchedSession := fmt.Sprintf(`{"version": "0.0.1", "session_id": %q}`, sessionID)
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
