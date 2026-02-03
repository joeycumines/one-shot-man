package storage

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// Verifies that the cleaner does not remove a lock file when a valid
// session file exists. The cleaner must only remove orphan lock files.
func TestCleaner_DoesNotRemoveLockForActiveSession(t *testing.T) {
	dir := t.TempDir()
	SetTestPaths(dir)

	// Create a session file and a lock file (but don't hold any lock).
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}
	sessionID := "active-session"
	sessionPath, _ := sessionFilePath(sessionID)
	lockPath, _ := sessionLockFilePath(sessionID)

	if err := os.WriteFile(sessionPath, []byte(`{"version":"`+CurrentSchemaVersion+`"}`), 0644); err != nil {
		t.Fatalf("failed to write session: %v", err)
	}
	// Create and acquire a lock so the session is actively locked.
	lf, err := acquireFileLock(lockPath)
	if err != nil {
		t.Fatalf("failed to acquire lock for active session: %v", err)
	}
	defer func() { _ = releaseFileLock(lf) }()

	cleaner := &Cleaner{MaxAgeDays: 0, MaxCount: 0, MaxSizeMB: 0}

	// Log directory contents before cleanup for debugging
	if entries, err := os.ReadDir(dir); err == nil {
		for _, e := range entries {
			t.Logf("before: %s (isdir=%v)", e.Name(), e.IsDir())
		}
	}

	report, err := cleaner.ExecuteCleanup("")

	// Log directory contents after cleanup for debugging
	if entries, err := os.ReadDir(dir); err == nil {
		for _, e := range entries {
			t.Logf("after: %s (isdir=%v)", e.Name(), e.IsDir())
		}
	}
	if err != nil {
		t.Fatalf("cleanup failed: %v", err)
	}

	// The lockfile should still exist and the session should be skipped.
	if _, err := os.Stat(lockPath); os.IsNotExist(err) {
		t.Fatalf("expected lock file to still exist for active session")
	}
	for _, id := range report.Removed {
		if id == sessionID {
			t.Fatalf("expected active session not to be removed")
		}
	}
}

// Verifies that the cleaner removes a lock file with no corresponding session
// (an orphan) when it can acquire the lock and confirm the session file is missing.
func TestCleaner_RemovesOrphanLock(t *testing.T) {
	dir := t.TempDir()
	SetTestPaths(dir)

	sessionID := "orphan-session"
	_, _ = sessionFilePath(sessionID)
	lockPath, _ := sessionLockFilePath(sessionID)

	// Create only the lock file (no session file) and make it older than
	// the safety window so the cleaner will remove it.
	lf, err := os.Create(lockPath)
	if err != nil {
		t.Fatalf("failed to create lock file: %v", err)
	}
	_ = lf.Close()

	// Make the lock older than the min orphan age used by cleaner.
	old := time.Now().Add(-10 * time.Second)
	if err := os.Chtimes(lockPath, old, old); err != nil {
		t.Fatalf("failed to set lock mtime: %v", err)
	}

	cleaner := &Cleaner{MaxAgeDays: 0, MaxCount: 0, MaxSizeMB: 0}

	// Debug listing before cleanup
	if entries, err := os.ReadDir(dir); err == nil {
		for _, e := range entries {
			t.Logf("before: %s (isdir=%v)", e.Name(), e.IsDir())
		}
	}

	report, err := cleaner.ExecuteCleanup("")

	// Debug listing after cleanup
	if entries, err := os.ReadDir(dir); err == nil {
		for _, e := range entries {
			t.Logf("after: %s (isdir=%v)", e.Name(), e.IsDir())
		}
	}
	if err != nil {
		t.Fatalf("cleanup failed: %v", err)
	}

	// The lockfile should no longer exist and the id should be in Removed.
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Fatalf("expected lock file to be removed for orphan session")
	}

	found := false
	for _, id := range report.Removed {
		if id == sessionID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected orphan session id in report.Removed")
	}
}

// Verifies that the cleaner does NOT remove an orphan lock file that is very
// young (i.e. created within the grace period). This prevents the Unix inode
// race where a creator's open FD on an unlinked file would allow multiple
// processes to believe they hold the lock.
func TestCleaner_SkipsYoungOrphanLock(t *testing.T) {
	dir := t.TempDir()
	SetTestPaths(dir)

	sessionID := "young-orphan"
	_, _ = sessionFilePath(sessionID)
	lockPath, _ := sessionLockFilePath(sessionID)

	// Create a fresh lock file (mtime == now)
	lf, err := os.Create(lockPath)
	if err != nil {
		t.Fatalf("failed to create lock file: %v", err)
	}
	_ = lf.Close()

	cleaner := &Cleaner{MaxAgeDays: 0, MaxCount: 0, MaxSizeMB: 0}
	report, err := cleaner.ExecuteCleanup("")
	if err != nil {
		t.Fatalf("cleanup failed: %v", err)
	}

	// Lock should still exist
	if _, err := os.Stat(lockPath); os.IsNotExist(err) {
		t.Fatalf("expected young orphan lock file to remain")
	}

	// Should be reported as skipped
	found := false
	for _, id := range report.Skipped {
		if id == sessionID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected young orphan session id in report.Skipped")
	}
}

func TestCleaner_CustomMinOrphanAge_RemovesWhenSmaller(t *testing.T) {
	dir := t.TempDir()
	SetTestPaths(dir)

	sessionID := "custom-age-remove"
	_, _ = sessionFilePath(sessionID)
	lockPath, _ := sessionLockFilePath(sessionID)

	// Create a lock file with mtime 3 seconds ago.
	lf, err := os.Create(lockPath)
	if err != nil {
		t.Fatalf("failed to create lock file: %v", err)
	}
	_ = lf.Close()
	old := time.Now().Add(-3 * time.Second)
	if err := os.Chtimes(lockPath, old, old); err != nil {
		t.Fatalf("failed to set lock mtime: %v", err)
	}

	// Set cleaner to require an orphan age of 2s (so file is old enough).
	cleaner := &Cleaner{MinOrphanAge: 2 * time.Second}
	report, err := cleaner.ExecuteCleanup("")
	if err != nil {
		t.Fatalf("cleanup failed: %v", err)
	}

	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Fatalf("expected lock file to be removed for custom-age-remove")
	}
	found := false
	for _, id := range report.Removed {
		if id == sessionID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected custom-age-remove id in report.Removed")
	}
}

func TestCleaner_CustomMinOrphanAge_SkipsWhenLarger(t *testing.T) {
	dir := t.TempDir()
	SetTestPaths(dir)

	sessionID := "custom-age-skip"
	_, _ = sessionFilePath(sessionID)
	lockPath, _ := sessionLockFilePath(sessionID)

	// Create a lock file with mtime 3 seconds ago.
	lf, err := os.Create(lockPath)
	if err != nil {
		t.Fatalf("failed to create lock file: %v", err)
	}
	_ = lf.Close()
	old := time.Now().Add(-3 * time.Second)
	if err := os.Chtimes(lockPath, old, old); err != nil {
		t.Fatalf("failed to set lock mtime: %v", err)
	}

	// Set cleaner to require an orphan age of 10s (so file is too new).
	cleaner := &Cleaner{MinOrphanAge: 10 * time.Second}
	report, err := cleaner.ExecuteCleanup("")
	if err != nil {
		t.Fatalf("cleanup failed: %v", err)
	}

	// Lock should still exist
	if _, err := os.Stat(lockPath); os.IsNotExist(err) {
		t.Fatalf("expected lock file to remain for custom-age-skip")
	}

	found := false
	for _, id := range report.Skipped {
		if id == sessionID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected custom-age-skip id in report.Skipped")
	}
}

// Verify that when a lock file exists but is not currently held by any
// process, and a corresponding session file exists, the cleaner will not
// remove the lock file even if it can acquire the lock temporarily.
func TestCleaner_DoesNotRemoveLockWhenAcquirableAndSessionExists(t *testing.T) {
	dir := t.TempDir()
	SetTestPaths(dir)

	sessionID := "acquirable-session"
	sessionPath, _ := sessionFilePath(sessionID)
	lockPath, _ := sessionLockFilePath(sessionID)

	if err := os.WriteFile(sessionPath, []byte(`{"version":"`+CurrentSchemaVersion+`"}`), 0644); err != nil {
		t.Fatalf("failed to write session: %v", err)
	}

	// Create the lock file but do not hold it; the cleaner may acquire it.
	lf, err := os.Create(lockPath)
	if err != nil {
		t.Fatalf("failed to create lock file: %v", err)
	}
	_ = lf.Close()

	// Debug: ensure the file is present before cleanup
	if entries, err := os.ReadDir(dir); err == nil {
		for _, e := range entries {
			t.Logf("created: %s (isdir=%v)", e.Name(), e.IsDir())
		}
	}

	cleaner := &Cleaner{MaxAgeDays: 0, MaxCount: 0, MaxSizeMB: 0}
	report, err := cleaner.ExecuteCleanup("")
	if err != nil {
		t.Fatalf("cleanup failed: %v", err)
	}

	if _, err := os.Stat(lockPath); os.IsNotExist(err) {
		t.Fatalf("expected lock file to still exist for session even if acquirable")
	}

	for _, id := range report.Removed {
		if id == sessionID {
			t.Fatalf("expected session not to have its lock removed")
		}
	}
}

// (merged into top-level package/imports)

func TestGlobalMetadataLoadUpdate(t *testing.T) {
	dir := t.TempDir()
	SetTestPaths(dir)
	// No metadata -> zero value
	m, err := loadGlobalMetadataTest()
	if err != nil {
		t.Fatalf("LoadGlobalMetadata failed: %v", err)
	}
	if !m.LastCleanupRun.IsZero() {
		t.Fatalf("expected zero LastCleanupRun, got %v", m.LastCleanupRun)
	}

	now := time.Now().UTC().Truncate(time.Second)
	if err := updateLastCleanupTest(now); err != nil {
		t.Fatalf("UpdateLastCleanup failed: %v", err)
	}

	m2, err := loadGlobalMetadataTest()
	if err != nil {
		t.Fatalf("LoadGlobalMetadata failed: %v", err)
	}
	if !m2.LastCleanupRun.Equal(now) {
		t.Fatalf("expected %v got %v", now, m2.LastCleanupRun)
	}
}

func TestAcquireLockHandleAndScan(t *testing.T) {
	dir := t.TempDir()
	SetTestPaths(dir)

	// Create a session file and lock
	sessionID := "sess-A"
	sessionPath, _ := sessionFilePath(sessionID)
	lockPath, _ := sessionLockFilePath(sessionID)

	if err := os.WriteFile(sessionPath, []byte("{}"), 0644); err != nil {
		t.Fatalf("write session error: %v", err)
	}

	// TryAcquire should succeed initially (use AcquireLockHandle)
	f, ok, err := AcquireLockHandle(lockPath)
	if err != nil {
		t.Fatalf("AcquireLockHandle error: %v", err)
	}
	if !ok {
		t.Fatalf("expected to acquire lock first time")
	}
	if err := f.Close(); err != nil {
		t.Fatalf("unlock failed: %v", err)
	}

	// Acquire underlying lock to simulate active session
	lf, err := acquireFileLock(lockPath)
	if err != nil {
		t.Fatalf("acquireFileLock failed: %v", err)
	}
	defer releaseFileLock(lf)

	// Now TryAcquire should report not acquired
	f2, ok2, _ := AcquireLockHandle(lockPath)
	if f2 != nil {
		_ = f2.Close()
	}
	if ok2 {
		t.Fatalf("expected not to acquire when already locked")
	}

	// ScanSessions should report the session as active
	infos, err := ScanSessions()
	if err != nil {
		t.Fatalf("ScanSessions failed: %v", err)
	}
	var found bool
	for _, si := range infos {
		if si.ID == sessionID {
			found = true
			if !si.Active {
				t.Fatalf("expected Active true for locked session")
			}
		}
	}
	if !found {
		t.Fatalf("session not found in scan")
	}
}

// Sanity: AcquireLockHandle -> Close should not remove the lock artifact.
func TestAcquireLockHandle_CloseLeavesFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping AcquireLockHandle_CloseLeavesFile on Windows due to file-locking semantics")
	}
	dir := t.TempDir()
	SetTestPaths(dir)

	id := "handle-close-test"
	lockPath, _ := sessionLockFilePath(id)

	if _, err := os.Create(lockPath); err != nil {
		t.Fatalf("failed to create lock file: %v", err)
	}

	f, ok, err := AcquireLockHandle(lockPath)
	if err != nil {
		t.Fatalf("AcquireLockHandle error: %v", err)
	}
	if !ok {
		t.Fatalf("expected to acquire lock on unlocked file")
	}

	// Close descriptor only
	if err := f.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}

	// File should still exist on disk
	if _, err := os.Stat(lockPath); os.IsNotExist(err) {
		t.Fatalf("expected lock artifact to remain after Close, but it was removed")
	}
}

func TestCleanerRemovesByAgeAndCount(t *testing.T) {
	dir := t.TempDir()
	SetTestPaths(dir)

	// Create 5 sessions with staggered modtimes and sizes
	ids := []string{"a", "b", "c", "d", "e"}
	now := time.Now().Add(-100 * 24 * time.Hour)
	for i, id := range ids {
		p, _ := sessionFilePath(id)
		if err := os.WriteFile(p, []byte("{}"), 0644); err != nil {
			t.Fatalf("write session: %v", err)
		}
		// Set modification time so that older files are removed
		mt := now.Add(time.Duration(i) * time.Hour)
		if err := os.Chtimes(p, mt, mt); err != nil {
			t.Fatalf("chtimes: %v", err)
		}
	}

	cleaner := Cleaner{MaxAgeDays: 30, MaxCount: 2, MaxSizeMB: 0}
	report, err := cleaner.ExecuteCleanup("")
	if err != nil {
		t.Fatalf("ExecuteCleanup failed: %v", err)
	}

	// After cleaning with MaxCount=2, many should be removed but some skipped may exist
	if len(report.Removed) == 0 {
		t.Fatalf("expected some removals, got none")
	}

	// Ensure no more than MaxCount sessions remain
	remaining, _ := ScanSessions()
	if len(remaining) > cleaner.MaxCount {
		t.Fatalf("expected at most %d remaining sessions, got %d", cleaner.MaxCount, len(remaining))
	}
}

func TestCleanup_IgnoresNonSessionLockFilesAndRemovesOrphans(t *testing.T) {
	dir := t.TempDir()
	SetTestPaths(dir)

	// Create a file that ends with .lock but is not a session lock.
	tempLock := filepath.Join(dir, "temp.lock")
	if err := os.WriteFile(tempLock, []byte("x"), 0644); err != nil {
		t.Fatalf("write temp lock: %v", err)
	}

	// Create an orphan session lock: it should be removed by cleanup.
	orphanLock := filepath.Join(dir, "orphan.session.lock")
	if err := os.WriteFile(orphanLock, []byte("l"), 0644); err != nil {
		t.Fatalf("write orphan lock: %v", err)
	}

	// Make it older than the minOrphanAge so the cleaner will remove it.
	old := time.Now().Add(-10 * time.Second)
	if err := os.Chtimes(orphanLock, old, old); err != nil {
		t.Fatalf("failed to set orphan lock mtime: %v", err)
	}

	// Run a cleaner (no sessions to remove)
	c := Cleaner{}
	_, err := c.ExecuteCleanup("")
	if err != nil {
		t.Fatalf("ExecuteCleanup failed: %v", err)
	}

	// temp.lock should remain untouched
	if _, err := os.Stat(tempLock); err != nil {
		t.Fatalf("expected temp.lock to remain, stat failed: %v", err)
	}

	// orphan.session.lock should have been removed
	if _, err := os.Stat(orphanLock); !os.IsNotExist(err) {
		t.Fatalf("expected orphan.session.lock to be removed, stat error: %v", err)
	}
}

// Dry-run mode should report removals but must not modify the filesystem.
func TestCleaner_DryRunReportsButDoesNotDelete(t *testing.T) {
	dir := t.TempDir()
	SetTestPaths(dir)

	// Create a session file that's old enough to be eligible for age-based
	// removal and do NOT create a lock so the session is not considered
	// active. Dry-run should report it as removed but not touch disk.
	sessionID := "dryrun-session"
	sessPath, _ := sessionFilePath(sessionID)

	if err := os.WriteFile(sessPath, []byte("{}"), 0644); err != nil {
		t.Fatalf("write session: %v", err)
	}
	// Make session older than the 1-day cutoff used by the cleaner below
	old := time.Now().Add(-72 * time.Hour)
	if err := os.Chtimes(sessPath, old, old); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	cleaner := &Cleaner{MaxAgeDays: 1, MaxCount: 0, MaxSizeMB: 0, DryRun: true}
	report, err := cleaner.ExecuteCleanup("")
	if err != nil {
		t.Fatalf("dry-run ExecuteCleanup failed: %v", err)
	}

	// Files should remain on disk
	if _, err := os.Stat(sessPath); err != nil {
		t.Fatalf("expected session file to remain during dry-run: %v", err)
	}
	// No lock file was created for this session so nothing else to verify.

	// But report should contain the id(s) the cleaner would remove.
	found := false
	for _, id := range report.Removed {
		if id == sessionID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected dry-run report to include %s, got %v", sessionID, report.Removed)
	}
}

// Ensure that if removing a session file fails, the cleaner will close the
// lock handle but NOT remove the lock artifact (so there is no orphan
// session-without-lock scenario).
func TestCleaner_PreservesLockWhenRemoveFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping TestCleaner_PreservesLockWhenRemoveFails on Windows due to differing permission semantics")
	}
	dir := t.TempDir()
	SetTestPaths(dir)

	id := "delete-fails"
	sessionPath, _ := sessionFilePath(id)
	lockPath, _ := sessionLockFilePath(id)

	// Create the session file and the lock file so the session is a candidate
	if err := os.WriteFile(sessionPath, []byte(`{}`), 0644); err != nil {
		t.Fatalf("failed to write session: %v", err)
	}
	if _, err := os.Create(lockPath); err != nil {
		t.Fatalf("failed to create lock file: %v", err)
	}

	// Mark the session as old so it will be selected by age pruning
	old := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(sessionPath, old, old); err != nil {
		t.Fatalf("failed to chtimes: %v", err)
	}

	// Replace session file with a directory to cause os.Remove(sessionPath) to fail.
	// This works regardless of user permissions (even as root).
	if err := os.RemoveAll(sessionPath); err != nil {
		t.Fatalf("failed to remove session file: %v", err)
	}
	if err := os.Mkdir(sessionPath, 0755); err != nil {
		t.Fatalf("failed to create directory in place of file: %v", err)
	}
	defer func() { _ = os.RemoveAll(sessionPath) }()

	cleaner := &Cleaner{MaxAgeDays: 1, MaxCount: 0, MaxSizeMB: 0}
	report, err := cleaner.ExecuteCleanup("")
	if err != nil {
		t.Fatalf("ExecuteCleanup failed: %v", err)
	}

	// The session file removal should have failed, so session directory (now replacing file) should still exist
	if _, err := os.Stat(sessionPath); os.IsNotExist(err) {
		t.Fatalf("expected session directory (simulating file removal failure) to exist after a failed remove")
	}

	// The lock file should still exist (we should NOT have unlinked it on failure)
	if _, err := os.Stat(lockPath); os.IsNotExist(err) {
		t.Fatalf("expected lock file to remain after failed session removal")
	}

	// The ID should be listed in report.Skipped
	// Note: We replaced the session file with a directory to simulate
	// removal failure, so the cleaner will attempt to remove the directory.
	// The key is that it should be listed in report.Skipped, not Removed.
	found := false
	for _, id := range report.Skipped {
		if id == "delete-fails" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected session id in report.Skipped when remove (simulation) fails")
	}
}
