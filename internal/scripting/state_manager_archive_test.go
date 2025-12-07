package scripting

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/joeycumines/one-shot-man/internal/storage"
)

var backendAlwaysExistsCalls int32

// flakyBackend simulates a backend that reports EEXIST on the first
// ArchiveSession attempt and succeeds on the second. It stores the
// archived path so the test can verify the final archive was created.
type flakyBackend struct {
	sessionID string
	saved     *storage.Session
	mu        sync.Mutex
	calls     int
	archived  string
}

func (f *flakyBackend) LoadSession(sessionID string) (*storage.Session, error) {
	if f.saved == nil {
		// return a simple session so StateManager initializes properly
		f.saved = &storage.Session{Version: storage.CurrentSchemaVersion, ID: sessionID, CreateTime: time.Now(), UpdateTime: time.Now(), ScriptState: map[string]map[string]interface{}{}, SharedState: map[string]interface{}{}, History: []storage.HistoryEntry{}}
	}
	return f.saved, nil
}

func (f *flakyBackend) SaveSession(session *storage.Session) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.saved = session
	return nil
}

func (f *flakyBackend) ArchiveSession(sessionID string, destPath string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.calls == 1 {
		return os.ErrExist
	}
	// on success, create the destination to simulate the file existing
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(destPath, []byte("archived"), 0644); err != nil {
		return err
	}
	f.archived = destPath
	// Simulate removing the original session (as fs backend would)
	return nil
}

func (f *flakyBackend) Close() error { return nil }

func TestArchiveAndReset_RetriesOnCollision(t *testing.T) {
	// Create a temporary sessions dir and ensure path helpers use it
	dir := t.TempDir()
	storage.SetTestPaths(dir)

	fb := &flakyBackend{sessionID: "sid"}

	// Create a state manager that uses our flaky backend
	sm := &StateManager{backend: fb, sessionID: "sid", session: &storage.Session{Version: storage.CurrentSchemaVersion, ID: "sid", CreateTime: time.Now(), UpdateTime: time.Now(), ScriptState: map[string]map[string]interface{}{}, SharedState: map[string]interface{}{}, History: []storage.HistoryEntry{}}}

	archivePath, err := sm.ArchiveAndReset()
	if err != nil {
		t.Fatalf("ArchiveAndReset failed: %v", err)
	}

	if fb.calls < 2 {
		t.Fatalf("expected backend.ArchiveSession to be called at least twice (got %d)", fb.calls)
	}

	if fb.archived == "" {
		t.Fatalf("expected backend to record archived path")
	}

	if archivePath != fb.archived {
		t.Fatalf("expected returned archivePath to equal backend archived path; want %q got %q", fb.archived, archivePath)
	}

	// After successful reset, session should be empty maps and history reset
	if len(sm.session.ScriptState) != 0 {
		t.Fatalf("expected ScriptState to be cleared")
	}
	if len(sm.session.SharedState) != 0 {
		t.Fatalf("expected SharedState to be cleared")
	}
	if len(sm.session.History) != 0 {
		t.Fatalf("expected History to be cleared")
	}
}

// backendAlwaysExists simulates a backend that always returns os.ErrExist for ArchiveSession
type backendAlwaysExists struct {
	sessionID string
	saved     *storage.Session
}

func (b *backendAlwaysExists) LoadSession(sessionID string) (*storage.Session, error) {
	if b.saved == nil {
		b.saved = &storage.Session{Version: storage.CurrentSchemaVersion, ID: sessionID, CreateTime: time.Now(), UpdateTime: time.Now(), ScriptState: map[string]map[string]interface{}{"x": {}}, SharedState: map[string]interface{}{"k": "v"}, History: []storage.HistoryEntry{{EntryID: "1"}}}
	}
	return b.saved, nil
}
func (b *backendAlwaysExists) SaveSession(session *storage.Session) error {
	b.saved = session
	return nil
}
func (b *backendAlwaysExists) ArchiveSession(sessionID string, destPath string) error {
	atomic.AddInt32(&backendAlwaysExistsCalls, 1)
	fmt.Printf("backendAlwaysExists.ArchiveSession called #%d session=%s dest=%s\n", atomic.LoadInt32(&backendAlwaysExistsCalls), sessionID, destPath)
	return os.ErrExist
}
func (b *backendAlwaysExists) Close() error { return nil }

func TestArchiveAndReset_ExhaustsAndAborts(t *testing.T) {
	dir := t.TempDir()
	storage.SetTestPaths(dir)

	fb := &backendAlwaysExists{sessionID: "sid"}

	// Create a real backend and state manager so internal ring-buffer state
	// is initialized properly, then swap the backend for our failing stub.
	realBackend, err := storage.NewFileSystemBackend("sid")
	if err != nil {
		t.Fatalf("failed to construct real filesystem backend: %v", err)
	}
	// Ensure we close real backend
	defer realBackend.Close()

	sm, err := NewStateManager(realBackend, "sid")
	if err != nil {
		t.Fatalf("failed to create state manager: %v", err)
	}
	// Give it some state so we can detect preservation
	sm.SetState("x:foo", "bar")
	sm.CaptureSnapshot("mode", "cmd", json.RawMessage(`{"k":true}`))

	// Swap backend to our failing stub
	sm.backend = fb

	// Make attempt limit small for test
	sm.ArchiveAttemptsMax = 3

	// Capture state snapshot before attempting archive+reset
	beforeJSON, err := sm.SerializeCompleteState()
	if err != nil {
		t.Fatalf("failed to serialize complete state before test: %v", err)
	}

	archivePath, err := sm.ArchiveAndReset()
	if err == nil {
		t.Fatalf("expected ArchiveAndReset to fail when all candidates exist, got nil (archivePath=%s)", archivePath)
	}

	// Verify it remains the same after the failed archive attempt.
	afterJSON, err := sm.SerializeCompleteState()
	if err != nil {
		t.Fatalf("failed to serialize complete state after check: %v", err)
	}
	if string(beforeJSON) != string(afterJSON) {
		t.Fatalf("expected session JSON to be preserved; before=%s after=%s", string(beforeJSON), string(afterJSON))
	}
}

func TestArchiveAndReset_ConcurrentSafety(t *testing.T) {
	dir := t.TempDir()
	storage.SetTestPaths(dir)

	// Use a real filesystem backend to exercise the actual archive and reset logic
	fb, err := storage.NewFileSystemBackend("concurrent")
	if err != nil {
		t.Fatalf("failed to create backend: %v", err)
	}
	defer fb.Close()

	// Ensure session has some state
	sm, err := NewStateManager(fb, "concurrent")
	if err != nil {
		t.Fatalf("failed to create state manager: %v", err)
	}

	sm.SetState("x:foo", "bar")

	// Run ArchiveAndReset concurrently from several goroutines
	const writers = 5
	results := make(chan error, writers)
	for i := 0; i < writers; i++ {
		go func() {
			_, err := sm.ArchiveAndReset()
			results <- err
		}()
	}

	// Collect results
	var failed int
	for i := 0; i < writers; i++ {
		if err := <-results; err != nil {
			failed++
		}
	}

	// At least one actor must have succeeded in archiving+reset, or all may have failed
	// but in either case the code must not have corrupted state unintentionally.
	sessionPath, _ := storage.SessionFilePath("concurrent")
	// If session file exists it must be valid JSON and either empty session or preserved
	if _, err := os.Stat(sessionPath); err == nil {
		// try to load with backend to ensure it is parsable
		if sess, err := fb.LoadSession("concurrent"); err != nil || sess == nil {
			t.Fatalf("session file exists but cannot be loaded after concurrent resets: %v %v", err, sess)
		}
	}

	// Ensure at least one run completed (either success or deliberate failure)
	if failed == writers {
		t.Fatalf("expected at least one ArchiveAndReset to succeed or return a non-fatal error; all %d attempts failed", writers)
	}
}
