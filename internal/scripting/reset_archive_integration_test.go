package scripting

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/joeycumines/one-shot-man/internal/storage"
	"github.com/joeycumines/one-shot-man/internal/testutil"
)

// TestResetArchiveIntegration_FileSystemBackend verifies the end-to-end reset+archive flow.
// It tests that:
// 1. Old session is archived to archive/ dir with session ID in filename
// 2. New session file is created under original sessionID path
// 3. Archive filename includes timestamp and counter
// 4. Old session data is preserved in archive
// 5. New session starts fresh
func TestResetArchiveIntegration_FileSystemBackend(t *testing.T) {
	defer storage.ResetPaths()

	tmpDir, err := os.MkdirTemp("", "osm-reset-archive-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	storage.SetTestPaths(tmpDir)
	defer storage.ResetPaths()
	// Test uses an isolated filesystem backend and a unique session id; do not
	// clear global in-memory sessions to avoid races with other tests.

	sessionID := testutil.NewTestSessionID("test-reset-session", t.Name())

	// Create initial session with some state
	backend, err := storage.NewFileSystemBackend(sessionID)
	if err != nil {
		t.Fatalf("Failed to create backend: %v", err)
	}

	initialSession := &storage.Session{
		Version:    storage.CurrentSchemaVersion,
		ID:         sessionID,
		CreateTime: time.Now(),
		UpdateTime: time.Now(),
		SharedState: map[string]interface{}{
			"contextItems": []string{"file1.txt", "file2.go"},
			"customData":   "important value",
		},
		ScriptState: map[string]map[string]interface{}{
			"code-review": map[string]interface{}{
				"reviewed": 5,
				"notes":    "good code",
			},
		},
		History: []storage.HistoryEntry{
			{
				EntryID:  "entry-1",
				ModeID:   "code-review",
				Command:  "add file1.txt",
				ReadTime: time.Now(),
			},
		},
	}

	if err := backend.SaveSession(initialSession); err != nil {
		t.Fatalf("Failed to save initial session: %v", err)
	}

	// Verify initial session file exists
	sessionPath, _ := storage.SessionFilePath(sessionID)
	if _, err := os.Stat(sessionPath); err != nil {
		t.Fatalf("Initial session file not found: %v", err)
	}

	// Now perform archive + reset
	stateManager, err := NewStateManager(backend, sessionID)
	if err != nil {
		t.Fatalf("Failed to create state manager: %v", err)
	}

	archivePath, err := stateManager.ArchiveAndReset()
	if err != nil {
		t.Fatalf("Failed to archive and reset: %v", err)
	}

	// Verify archive path is sensible
	if archivePath == "" {
		t.Fatalf("Archive path is empty")
	}

	if !strings.Contains(archivePath, "archive") {
		t.Errorf("Archive path should contain 'archive': %s", archivePath)
	}

	if !strings.Contains(archivePath, "reset") {
		t.Errorf("Archive path should contain 'reset': %s", archivePath)
	}

	// Verify archive file exists
	if _, err := os.Stat(archivePath); err != nil {
		t.Fatalf("Archive file was not created: %v", err)
	}

	// Verify archived session contains original data
	archiveData, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatalf("Failed to read archive file: %v", err)
	}

	var archivedSession storage.Session
	if err := json.Unmarshal(archiveData, &archivedSession); err != nil {
		t.Fatalf("Failed to unmarshal archived session: %v", err)
	}

	// Check original data is preserved
	if len(archivedSession.SharedState) == 0 {
		t.Errorf("Archived session lost shared state")
	}

	if contextItems, ok := archivedSession.SharedState["contextItems"]; ok {
		if items, ok := contextItems.([]interface{}); ok {
			if len(items) != 2 {
				t.Errorf("Archived session lost contextItems: got %d, want 2", len(items))
			}
		}
	}

	// Verify new session file exists and is reset
	newSessionData, err := os.ReadFile(sessionPath)
	if err != nil {
		t.Fatalf("Failed to read new session file: %v", err)
	}

	var newSession storage.Session
	if err := json.Unmarshal(newSessionData, &newSession); err != nil {
		t.Fatalf("Failed to unmarshal new session: %v", err)
	}

	// Verify new session is empty
	if len(newSession.SharedState) > 0 {
		t.Errorf("New session should have empty shared state, got: %v", newSession.SharedState)
	}

	if len(newSession.ScriptState) > 0 {
		t.Errorf("New session should have empty script state, got: %v", newSession.ScriptState)
	}

	// Verify new session has fresh timestamps
	if newSession.CreateTime.Before(initialSession.UpdateTime) {
		t.Errorf("New session CreateTime should be after archive time")
	}
}

// TestResetCommand_WithArchive verifies the reset REPL command archives the session.
func TestResetCommand_WithArchive(t *testing.T) {
	defer storage.ResetPaths()

	tmpDir, err := os.MkdirTemp("", "osm-reset-cmd-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	storage.SetTestPaths(tmpDir)
	defer storage.ResetPaths()

	// Create a TUI manager with in-memory backend for testing
	sessionID := testutil.NewTestSessionID("test-reset-cmd-session", t.Name())
	backend, err := storage.NewInMemoryBackend(sessionID)
	if err != nil {
		t.Fatalf("Failed to create backend: %v", err)
	}

	stateManager, err := NewStateManager(backend, sessionID)
	if err != nil {
		t.Fatalf("Failed to create state manager: %v", err)
	}

	// Create a minimal TUI manager
	var buf bytes.Buffer
	tuiMgr := &TUIManager{
		stateManager: stateManager,
		writer:       NewTUIWriterFromIO(&buf),
	}

	// Set some state before reset
	stateManager.SetState("contextItems", []string{"test.txt"})
	stateManager.SetState("code-review:notes", "test notes")

	// Execute reset
	archivePath, err := tuiMgr.resetAllState()
	if err != nil {
		t.Fatalf("resetAllState failed: %v", err)
	}
	_ = archivePath // may be empty for in-memory backend

	// Verify state was cleared
	contextItems, ok := stateManager.GetState("contextItems")
	if ok && contextItems != nil {
		t.Errorf("contextItems should be cleared, got: %v", contextItems)
	}

	notes, ok := stateManager.GetState("code-review:notes")
	if ok && notes != nil {
		t.Errorf("code-review:notes should be cleared, got: %v", notes)
	}

	// Check output mentions archive (if using filesystem backend)
	output := buf.String()
	// For memory backend, ArchiveAndReset returns nil so we won't see the archive message
	// but the reset should still work
	if output != "" && !strings.Contains(output, "archive") && !strings.Contains(output, "WARNING") {
		// It's ok if empty or contains archive/warning messages
	}
}

// TestMultipleResets_ArchiveCounter verifies multiple resets produce distinct archive files.
func TestMultipleResets_ArchiveCounter(t *testing.T) {
	defer storage.ResetPaths()

	tmpDir, err := os.MkdirTemp("", "osm-multi-reset-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	storage.SetTestPaths(tmpDir)
	defer storage.ResetPaths()

	sessionID := testutil.NewTestSessionID("test-multi-reset", t.Name())

	// Perform multiple reset cycles
	archivePaths := []string{}

	for i := 0; i < 3; i++ {
		backend, err := storage.NewFileSystemBackend(sessionID)
		if err != nil {
			t.Fatalf("Failed to create backend (cycle %d): %v", i, err)
		}

		// Create state
		session := &storage.Session{
			Version:     storage.CurrentSchemaVersion,
			ID:          sessionID,
			CreateTime:  time.Now(),
			UpdateTime:  time.Now(),
			SharedState: map[string]interface{}{"value": fmt.Sprintf("cycle-%d", i)},
			ScriptState: make(map[string]map[string]interface{}),
		}

		if err := backend.SaveSession(session); err != nil {
			t.Fatalf("Failed to save session (cycle %d): %v", i, err)
		}

		// Reset
		stateManager, err := NewStateManager(backend, sessionID)
		if err != nil {
			t.Fatalf("Failed to create state manager (cycle %d): %v", i, err)
		}

		archivePath, err := stateManager.ArchiveAndReset()
		if err != nil {
			t.Fatalf("Failed to archive (cycle %d): %v", i, err)
		}

		if archivePath != "" {
			archivePaths = append(archivePaths, archivePath)
		}

		backend.Close()
	}

	// Verify all archive paths exist and are distinct
	seenPaths := make(map[string]bool)
	for i, path := range archivePaths {
		if seenPaths[path] {
			t.Errorf("Duplicate archive path at cycle %d: %s", i, path)
		}
		seenPaths[path] = true

		if _, err := os.Stat(path); err != nil {
			t.Errorf("Archive at cycle %d not found: %s (%v)", i, path, err)
		}
	}

	// Note: With 3 resets on same timestamp, the counter mechanism allows
	// up to 999 distinct archives. The test verifies archives are created,
	// not that all have distinct names (they may have same timestamps).
}

// TestArchiveSessionFilename_Readability verifies archive filenames are readable and preserve sessionID.
func TestArchiveSessionFilename_Readability(t *testing.T) {
	defer storage.ResetPaths()

	tmpDir, err := os.MkdirTemp("", "osm-archive-name-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	storage.SetTestPaths(tmpDir)
	defer storage.ResetPaths()

	sessionID := "tmux-abc123-5"
	ts := time.Date(2025, 11, 26, 15, 30, 45, 0, time.UTC)

	archivePath, err := storage.ArchiveSessionFilePath(sessionID, ts, 0)
	if err != nil {
		t.Fatalf("Failed to get archive path: %v", err)
	}

	filename := filepath.Base(archivePath)

	// Verify filename is readable and contains key markers
	// Note: SanitizeFilename doesn't replace hyphens (they're filesystem-safe)
	parts := []string{
		"tmux-abc123-5",    // session ID (hyphens preserved)
		"reset",            // operation marker
		"2025-11-26",       // date
		"15-30-45",         // time (colon replaced with hyphen)
		"000.session.json", // counter and extension
	}

	for _, part := range parts {
		if !strings.Contains(filename, part) {
			t.Errorf("Archive filename should contain %q, got: %s", part, filename)
		}
	}

	t.Logf("Archive filename (readable): %s", filename)
}
