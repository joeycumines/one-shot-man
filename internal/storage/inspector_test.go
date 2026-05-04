package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// --- T207: Comprehensive unit tests for inspector.go ---
//
// These tests modify package-level variables via SetTestPaths / acquireFileLock,
// so they must NOT use t.Parallel(). This matches the pattern used in
// cleanup_test.go for the same reason.

func TestScanSessions_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	SetTestPaths(dir)
	defer ResetPaths()

	infos, err := ScanSessions()
	if err != nil {
		t.Fatalf("ScanSessions returned error for empty dir: %v", err)
	}
	if len(infos) != 0 {
		t.Fatalf("expected 0 sessions, got %d", len(infos))
	}
}

func TestScanSessions_SingleSession(t *testing.T) {
	dir := t.TempDir()
	SetTestPaths(dir)
	defer ResetPaths()

	data := []byte(`{"key":"value"}`)
	sessionPath := filepath.Join(dir, "my-session.session.json")
	if err := os.WriteFile(sessionPath, data, 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	infos, err := ScanSessions()
	if err != nil {
		t.Fatalf("ScanSessions returned error: %v", err)
	}
	if len(infos) != 1 {
		t.Fatalf("expected 1 session, got %d", len(infos))
	}

	info := infos[0]
	if info.ID != "my-session" {
		t.Errorf("expected ID 'my-session', got %q", info.ID)
	}
	if info.Path != sessionPath {
		t.Errorf("expected Path %q, got %q", sessionPath, info.Path)
	}
	if info.Size != int64(len(data)) {
		t.Errorf("expected Size %d, got %d", len(data), info.Size)
	}
	if info.UpdateTime.IsZero() {
		t.Error("expected non-zero UpdateTime")
	}
	if info.CreateTime.IsZero() {
		t.Error("expected non-zero CreateTime")
	}
	expectedLockPath := filepath.Join(dir, "my-session.session.lock")
	if info.LockPath != expectedLockPath {
		t.Errorf("expected LockPath %q, got %q", expectedLockPath, info.LockPath)
	}
}

func TestScanSessions_MultipleSessions(t *testing.T) {
	dir := t.TempDir()
	SetTestPaths(dir)
	defer ResetPaths()

	ids := []string{"alpha", "bravo", "charlie"}
	for _, id := range ids {
		if err := os.WriteFile(filepath.Join(dir, id+".session.json"), []byte("{}"), 0644); err != nil {
			t.Fatalf("write %s: %v", id, err)
		}
	}

	infos, err := ScanSessions()
	if err != nil {
		t.Fatalf("ScanSessions returned error: %v", err)
	}
	if len(infos) != 3 {
		t.Fatalf("expected 3 sessions, got %d", len(infos))
	}

	found := make(map[string]bool)
	for _, info := range infos {
		found[info.ID] = true
	}
	for _, id := range ids {
		if !found[id] {
			t.Errorf("expected session %q in results", id)
		}
	}
}

func TestScanSessions_SkipsDirectories(t *testing.T) {
	dir := t.TempDir()
	SetTestPaths(dir)
	defer ResetPaths()

	// Subdirectory should be ignored.
	if err := os.Mkdir(filepath.Join(dir, "subdir"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Even if it has .session.json suffix.
	if err := os.Mkdir(filepath.Join(dir, "tricky.session.json"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Real session file.
	if err := os.WriteFile(filepath.Join(dir, "real.session.json"), []byte("{}"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	infos, err := ScanSessions()
	if err != nil {
		t.Fatalf("ScanSessions returned error: %v", err)
	}
	if len(infos) != 1 {
		t.Fatalf("expected 1 session, got %d", len(infos))
	}
	if infos[0].ID != "real" {
		t.Errorf("expected ID 'real', got %q", infos[0].ID)
	}
}

func TestScanSessions_SkipsNonJSONFiles(t *testing.T) {
	dir := t.TempDir()
	SetTestPaths(dir)
	defer ResetPaths()

	nonSessionFiles := []string{
		"notes.txt",
		"test.session.lock",
		"readme.md",
		"data.csv",
	}
	for _, f := range nonSessionFiles {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("x"), 0644); err != nil {
			t.Fatalf("write %s: %v", f, err)
		}
	}
	// Valid session file.
	if err := os.WriteFile(filepath.Join(dir, "valid.session.json"), []byte("{}"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	infos, err := ScanSessions()
	if err != nil {
		t.Fatalf("ScanSessions returned error: %v", err)
	}
	if len(infos) != 1 {
		t.Fatalf("expected 1 session, got %d", len(infos))
	}
}

func TestScanSessions_SkipsNonSessionJSONFiles(t *testing.T) {
	dir := t.TempDir()
	SetTestPaths(dir)
	defer ResetPaths()

	// JSON files that are NOT session files should be skipped.
	// This previously caused a panic because the old code used
	// filepath.Ext check (.json) then subtracted ".session.json" length.
	nonSessionJSON := []string{
		"notes.json",
		"config.json",
		"a.json",
		"x.json",
	}
	for _, f := range nonSessionJSON {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("{}"), 0644); err != nil {
			t.Fatalf("write %s: %v", f, err)
		}
	}
	// Valid session file.
	if err := os.WriteFile(filepath.Join(dir, "ok.session.json"), []byte("{}"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	infos, err := ScanSessions()
	if err != nil {
		t.Fatalf("ScanSessions returned error: %v", err)
	}
	if len(infos) != 1 {
		t.Fatalf("expected 1 session (skipping non-session .json), got %d", len(infos))
	}
	if infos[0].ID != "ok" {
		t.Errorf("expected ID 'ok', got %q", infos[0].ID)
	}
}

func TestInspector_ScanSessions_NonExistentDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "deep", "nested", "nope")
	SetTestPaths(dir)
	defer ResetPaths()

	infos, err := ScanSessions()
	if err != nil {
		t.Fatalf("ScanSessions should not error for non-existent dir: %v", err)
	}
	if len(infos) != 0 {
		t.Fatalf("expected empty slice, got %d entries", len(infos))
	}
}

func TestInspector_ScanSessions_SessionDirError(t *testing.T) {
	orig := sessionDirectory
	defer func() { sessionDirectory = orig }()
	sessionDirectory = func() (string, error) {
		return "", fmt.Errorf("injected session dir error")
	}

	_, err := ScanSessions()
	if err == nil {
		t.Fatal("expected error when sessionDirectory fails")
	}
}

func TestScanSessions_StatError(t *testing.T) {
	dir := t.TempDir()
	SetTestPaths(dir)
	defer ResetPaths()

	// Create a session file.
	sessionPath := filepath.Join(dir, "stat-fail.session.json")
	if err := os.WriteFile(sessionPath, []byte("{}"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Remove the file after ReadDir but before Stat by making the file
	// unreadable (won't work on all platforms) or by simply verifying
	// that Stat errors cause the entry to be skipped silently.
	// Instead, we remove the file to trigger a Stat error.
	if err := os.Remove(sessionPath); err != nil {
		t.Fatalf("remove: %v", err)
	}

	// Put back an empty to make ReadDir find it, but remove immediately.
	// This is a race, so instead, we use a different approach:
	// Create a symlink to a non-existent target. Stat will fail.
	if err := os.Symlink("/nonexistent-target-xyz", sessionPath); err != nil {
		t.Skip("symlinks not supported, skipping Stat error test")
	}

	infos, err := ScanSessions()
	if err != nil {
		t.Fatalf("ScanSessions should not error on Stat failure: %v", err)
	}
	// The broken symlink file should be skipped (Stat error → continue).
	if len(infos) != 0 {
		t.Fatalf("expected 0 sessions (stat error skipped), got %d", len(infos))
	}
}

func TestScanSessions_InactiveSession(t *testing.T) {
	dir := t.TempDir()
	SetTestPaths(dir)
	defer ResetPaths()

	// Create session file without holding a lock.
	if err := os.WriteFile(filepath.Join(dir, "idle.session.json"), []byte("{}"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	infos, err := ScanSessions()
	if err != nil {
		t.Fatalf("ScanSessions returned error: %v", err)
	}
	if len(infos) != 1 {
		t.Fatalf("expected 1 session, got %d", len(infos))
	}
	if infos[0].Active {
		t.Error("expected Active=false for unlocked session")
	}
}

func TestScanSessions_ActiveSession(t *testing.T) {
	dir := t.TempDir()
	SetTestPaths(dir)
	defer ResetPaths()

	sessionID := "active-test"
	if err := os.WriteFile(filepath.Join(dir, sessionID+".session.json"), []byte("{}"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Acquire a lock to make the session active.
	lockPath := filepath.Join(dir, sessionID+".session.lock")
	f, ok, err := AcquireLockHandle(lockPath)
	if err != nil {
		t.Fatalf("AcquireLockHandle error: %v", err)
	}
	if !ok {
		t.Fatal("expected lock acquisition to succeed")
	}
	defer func() { _ = ReleaseLockHandle(f) }()

	infos, err := ScanSessions()
	if err != nil {
		t.Fatalf("ScanSessions returned error: %v", err)
	}
	if len(infos) != 1 {
		t.Fatalf("expected 1 session, got %d", len(infos))
	}
	if !infos[0].Active {
		t.Error("expected Active=true for locked session")
	}
}

func TestInspector_ScanSessions_AcquireLockError(t *testing.T) {
	dir := t.TempDir()
	SetTestPaths(dir)
	defer ResetPaths()

	if err := os.WriteFile(filepath.Join(dir, "lockerr.session.json"), []byte("{}"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Stub acquireFileLock to return a non-errWouldBlock error.
	origLock := acquireFileLock
	defer func() { acquireFileLock = origLock }()
	acquireFileLock = func(path string) (*os.File, error) {
		return nil, fmt.Errorf("injected lock error")
	}

	infos, err := ScanSessions()
	if err != nil {
		t.Fatalf("ScanSessions should not propagate lock errors: %v", err)
	}
	if len(infos) != 1 {
		t.Fatalf("expected 1 session, got %d", len(infos))
	}
	if infos[0].Active {
		t.Error("expected Active=false when AcquireLockHandle returns error")
	}
}

func TestScanSessions_SessionInfoFields(t *testing.T) {
	dir := t.TempDir()
	SetTestPaths(dir)
	defer ResetPaths()

	data := []byte(`{"some":"data","with":"content"}`)
	sessionPath := filepath.Join(dir, "field-check.session.json")
	if err := os.WriteFile(sessionPath, data, 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	infos, err := ScanSessions()
	if err != nil {
		t.Fatalf("ScanSessions returned error: %v", err)
	}
	if len(infos) != 1 {
		t.Fatalf("expected 1 session, got %d", len(infos))
	}

	info := infos[0]

	// ID
	if info.ID != "field-check" {
		t.Errorf("ID: expected 'field-check', got %q", info.ID)
	}

	// Path
	if info.Path != sessionPath {
		t.Errorf("Path: expected %q, got %q", sessionPath, info.Path)
	}

	// LockPath
	expectedLock := filepath.Join(dir, "field-check.session.lock")
	if info.LockPath != expectedLock {
		t.Errorf("LockPath: expected %q, got %q", expectedLock, info.LockPath)
	}

	// Size
	if info.Size != int64(len(data)) {
		t.Errorf("Size: expected %d, got %d", len(data), info.Size)
	}

	// Timestamps should be non-zero.
	if info.UpdateTime.IsZero() {
		t.Error("UpdateTime should not be zero")
	}
	if info.CreateTime.IsZero() {
		t.Error("CreateTime should not be zero")
	}

	// Active should be false (no lock held).
	if info.Active {
		t.Error("Active: expected false for unlocked session")
	}
}

func TestScanSessions_ReadDirError(t *testing.T) {
	// Use a file as the sessions directory — ReadDir will fail with a
	// non-IsNotExist error.
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "not-a-dir")
	if err := os.WriteFile(filePath, []byte("x"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	pathsMu.Lock()
	orig := sessionDirectory
	sessionDirectory = func() (string, error) { return filePath, nil }
	pathsMu.Unlock()
	defer func() {
		pathsMu.Lock()
		sessionDirectory = orig
		pathsMu.Unlock()
	}()

	_, err := ScanSessions()
	if err == nil {
		t.Fatal("expected error when sessions directory is a file")
	}
}
