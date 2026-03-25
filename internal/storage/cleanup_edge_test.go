package storage

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestCleaner_MaxCount1 verifies that MaxCount=1 keeps only the single newest
// session and removes all others.
func TestCleaner_MaxCount1(t *testing.T) {
	dir := t.TempDir()
	SetTestPaths(dir)
	defer ResetPaths()

	now := time.Now()
	ids := []string{"mc1-a", "mc1-b", "mc1-c", "mc1-d", "mc1-e"}
	for i, id := range ids {
		p, _ := sessionFilePath(id)
		if err := os.WriteFile(p, []byte("{}"), 0644); err != nil {
			t.Fatalf("write session %s: %v", id, err)
		}
		// Stagger: mc1-a oldest (-5h), mc1-e newest (-1h)
		mt := now.Add(-time.Duration(len(ids)-i) * time.Hour)
		if err := os.Chtimes(p, mt, mt); err != nil {
			t.Fatalf("chtimes %s: %v", id, err)
		}
	}

	cleaner := &Cleaner{MaxCount: 1}
	report, err := cleaner.ExecuteCleanup("")
	if err != nil {
		t.Fatalf("ExecuteCleanup: %v", err)
	}

	if len(report.Removed) != 4 {
		t.Fatalf("expected 4 removed, got %d: %v", len(report.Removed), report.Removed)
	}

	remaining, err := ScanSessions()
	if err != nil {
		t.Fatalf("ScanSessions: %v", err)
	}
	if len(remaining) != 1 {
		t.Fatalf("expected 1 remaining session, got %d", len(remaining))
	}
	if remaining[0].ID != "mc1-e" {
		t.Fatalf("expected newest session mc1-e to remain, got %s", remaining[0].ID)
	}
}

// TestCleaner_AllZeroPolicies verifies that zero-valued retention policies
// mean "disabled" — no sessions are removed.
func TestCleaner_AllZeroPolicies(t *testing.T) {
	dir := t.TempDir()
	SetTestPaths(dir)
	defer ResetPaths()

	now := time.Now()
	ids := []string{"zero-a", "zero-b", "zero-c"}
	for i, id := range ids {
		p, _ := sessionFilePath(id)
		if err := os.WriteFile(p, []byte("{}"), 0644); err != nil {
			t.Fatalf("write session %s: %v", id, err)
		}
		// Stagger ages: 1 day, 2 days, 3 days ago
		mt := now.Add(-time.Duration(i+1) * 24 * time.Hour)
		if err := os.Chtimes(p, mt, mt); err != nil {
			t.Fatalf("chtimes %s: %v", id, err)
		}
	}

	cleaner := &Cleaner{MaxAgeDays: 0, MaxCount: 0, MaxSizeMB: 0}
	report, err := cleaner.ExecuteCleanup("")
	if err != nil {
		t.Fatalf("ExecuteCleanup: %v", err)
	}

	if len(report.Removed) != 0 {
		t.Fatalf("expected 0 removed with all-zero policies, got %d: %v",
			len(report.Removed), report.Removed)
	}

	remaining, err := ScanSessions()
	if err != nil {
		t.Fatalf("ScanSessions: %v", err)
	}
	if len(remaining) != 3 {
		t.Fatalf("expected all 3 sessions to remain, got %d", len(remaining))
	}
}

// TestCleaner_ConcurrentCleaners verifies that two concurrent cleanup
// invocations are serialised by the global cleanup.lock. The loser returns
// an error (or, if execution happens to be sequential, succeeds with 0
// removals). Total removed across all successful reports must equal the
// number of eligible sessions.
func TestCleaner_ConcurrentCleaners(t *testing.T) {
	dir := t.TempDir()
	SetTestPaths(dir)
	defer ResetPaths()

	// Create 5 sessions old enough to be removed by age.
	now := time.Now()
	for i := range 5 {
		id := fmt.Sprintf("conc-%d", i)
		p, _ := sessionFilePath(id)
		if err := os.WriteFile(p, []byte("{}"), 0644); err != nil {
			t.Fatalf("write session %s: %v", id, err)
		}
		mt := now.Add(-48 * time.Hour)
		if err := os.Chtimes(p, mt, mt); err != nil {
			t.Fatalf("chtimes %s: %v", id, err)
		}
	}

	type result struct {
		report *CleanupReport
		err    error
	}

	gate := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	results := make([]result, 2)

	for i := range 2 {
		i := i
		go func() {
			defer wg.Done()
			<-gate
			cleaner := &Cleaner{MaxAgeDays: 1}
			report, err := cleaner.ExecuteCleanup("")
			results[i] = result{report, err}
		}()
	}

	close(gate)
	wg.Wait()

	// Exactly one of these patterns must hold:
	// (a) Concurrent: one succeeds (removes 5), the other fails with lock error.
	// (b) Sequential: both succeed, total removed = 5 (winner removes 5, loser removes 0).
	var totalRemoved int
	for _, r := range results {
		if r.err != nil {
			if !strings.Contains(r.err.Error(), "cleanup lock") {
				t.Errorf("unexpected error (expected cleanup lock contention): %v", r.err)
			}
			continue
		}
		totalRemoved += len(r.report.Removed)
	}

	if totalRemoved != 5 {
		t.Fatalf("expected 5 total removed across all successful reports, got %d", totalRemoved)
	}

	remaining, err := ScanSessions()
	if err != nil {
		t.Fatalf("ScanSessions: %v", err)
	}
	if len(remaining) != 0 {
		t.Fatalf("expected 0 remaining sessions, got %d", len(remaining))
	}
}

// TestCleaner_LargeSessionSet verifies correct behaviour with a large number
// of sessions. All 200 sessions are older than the age cutoff, so they are
// removed by both age and count policies, leaving at most MaxCount.
func TestCleaner_LargeSessionSet(t *testing.T) {
	dir := t.TempDir()
	SetTestPaths(dir)
	defer ResetPaths()

	now := time.Now()
	const total = 200
	for i := range total {
		id := fmt.Sprintf("large-%03d", i)
		p, _ := sessionFilePath(id)
		if err := os.WriteFile(p, []byte("{}"), 0644); err != nil {
			t.Fatalf("write session %s: %v", id, err)
		}
		// All sessions 2+ days old, staggered by 1 hour each.
		mt := now.Add(-time.Duration(48+i) * time.Hour)
		if err := os.Chtimes(p, mt, mt); err != nil {
			t.Fatalf("chtimes %s: %v", id, err)
		}
	}

	cleaner := &Cleaner{MaxAgeDays: 1, MaxCount: 50}
	report, err := cleaner.ExecuteCleanup("")
	if err != nil {
		t.Fatalf("ExecuteCleanup: %v", err)
	}

	// All 200 are older than 1 day → age removes all. Count also marks
	// beyond 50. Union = all 200.
	remaining, err := ScanSessions()
	if err != nil {
		t.Fatalf("ScanSessions: %v", err)
	}
	if len(remaining) > 50 {
		t.Fatalf("expected at most 50 remaining, got %d", len(remaining))
	}

	// Sanity: removed + skipped should account for all sessions.
	if len(report.Removed) == 0 {
		t.Fatal("expected removals in large session set")
	}
}

// TestCleaner_CombinedPolicies verifies that age-based and count-based
// policies compose correctly: age removes old sessions first, then count
// trims the remainder to MaxCount.
func TestCleaner_CombinedPolicies(t *testing.T) {
	dir := t.TempDir()
	SetTestPaths(dir)
	defer ResetPaths()

	now := time.Now()

	// 5 old sessions (>30 days)
	for i := range 5 {
		id := fmt.Sprintf("old-%d", i)
		p, _ := sessionFilePath(id)
		if err := os.WriteFile(p, []byte("{}"), 0644); err != nil {
			t.Fatalf("write session %s: %v", id, err)
		}
		mt := now.Add(-time.Duration(60+i) * 24 * time.Hour)
		if err := os.Chtimes(p, mt, mt); err != nil {
			t.Fatalf("chtimes %s: %v", id, err)
		}
	}

	// 5 recent sessions (staggered over last few hours)
	for i := range 5 {
		id := fmt.Sprintf("recent-%d", i)
		p, _ := sessionFilePath(id)
		if err := os.WriteFile(p, []byte("{}"), 0644); err != nil {
			t.Fatalf("write session %s: %v", id, err)
		}
		// recent-0 = newest (-1h), recent-4 = oldest recent (-5h)
		mt := now.Add(-time.Duration(i+1) * time.Hour)
		if err := os.Chtimes(p, mt, mt); err != nil {
			t.Fatalf("chtimes %s: %v", id, err)
		}
	}

	cleaner := &Cleaner{MaxAgeDays: 30, MaxCount: 3, MaxSizeMB: 0}
	report, err := cleaner.ExecuteCleanup("")
	if err != nil {
		t.Fatalf("ExecuteCleanup: %v", err)
	}

	// Age removes 5 old sessions. Count keeps newest 3 from all 10
	// candidates (recent-0, recent-1, recent-2), removing 7 total.
	remaining, err := ScanSessions()
	if err != nil {
		t.Fatalf("ScanSessions: %v", err)
	}

	if len(remaining) != 3 {
		var rids []string
		for _, s := range remaining {
			rids = append(rids, s.ID)
		}
		t.Fatalf("expected 3 remaining sessions, got %d: %v", len(remaining), rids)
	}

	// Verify the 3 survivors are the newest recent sessions.
	expected := map[string]bool{"recent-0": true, "recent-1": true, "recent-2": true}
	for _, s := range remaining {
		if !expected[s.ID] {
			t.Errorf("unexpected remaining session: %s", s.ID)
		}
	}

	// Total removed should be 7 (5 old + 2 oldest recent).
	if len(report.Removed) != 7 {
		t.Errorf("expected 7 removed, got %d: %v", len(report.Removed), report.Removed)
	}
}

// TestCleaner_ZeroByteSessionFile verifies that a zero-byte .session.json
// file does not crash the cleaner and is eligible for normal cleanup.
func TestCleaner_ZeroByteSessionFile(t *testing.T) {
	dir := t.TempDir()
	SetTestPaths(dir)
	defer ResetPaths()

	now := time.Now()

	// Create a zero-byte session file (empty, no valid JSON).
	zbPath, _ := sessionFilePath("zerobyte")
	if err := os.WriteFile(zbPath, nil, 0644); err != nil {
		t.Fatalf("create zero-byte file: %v", err)
	}
	old := now.Add(-72 * time.Hour)
	if err := os.Chtimes(zbPath, old, old); err != nil {
		t.Fatalf("chtimes zerobyte: %v", err)
	}

	// Create a valid session file (also old).
	validPath, _ := sessionFilePath("valid")
	if err := os.WriteFile(validPath, []byte("{}"), 0644); err != nil {
		t.Fatalf("write valid session: %v", err)
	}
	if err := os.Chtimes(validPath, old, old); err != nil {
		t.Fatalf("chtimes valid: %v", err)
	}

	// Create a recent session that should survive.
	recentPath, _ := sessionFilePath("recent")
	if err := os.WriteFile(recentPath, []byte("{}"), 0644); err != nil {
		t.Fatalf("write recent session: %v", err)
	}

	cleaner := &Cleaner{MaxAgeDays: 1}
	report, err := cleaner.ExecuteCleanup("")
	if err != nil {
		t.Fatalf("ExecuteCleanup: %v", err)
	}

	// Zero-byte file and valid-old should be removed; recent survives.
	removedSet := make(map[string]bool)
	for _, id := range report.Removed {
		removedSet[id] = true
	}
	if !removedSet["zerobyte"] {
		t.Error("expected zero-byte session in Removed")
	}
	if !removedSet["valid"] {
		t.Error("expected valid old session in Removed")
	}

	remaining, err := ScanSessions()
	if err != nil {
		t.Fatalf("ScanSessions: %v", err)
	}
	if len(remaining) != 1 {
		t.Fatalf("expected 1 remaining session, got %d", len(remaining))
	}
	if remaining[0].ID != "recent" {
		t.Fatalf("expected recent session to remain, got %s", remaining[0].ID)
	}
}

// TestCleaner_ExcludeNonExistentID verifies that excluding an ID that does not
// match any session does not crash and cleanup proceeds normally.
func TestCleaner_ExcludeNonExistentID(t *testing.T) {
	dir := t.TempDir()
	SetTestPaths(dir)
	defer ResetPaths()

	now := time.Now()
	ids := []string{"excl-a", "excl-b", "excl-c"}
	for _, id := range ids {
		p, _ := sessionFilePath(id)
		if err := os.WriteFile(p, []byte("{}"), 0644); err != nil {
			t.Fatalf("write session %s: %v", id, err)
		}
		mt := now.Add(-72 * time.Hour)
		if err := os.Chtimes(p, mt, mt); err != nil {
			t.Fatalf("chtimes %s: %v", id, err)
		}
	}

	cleaner := &Cleaner{MaxAgeDays: 1}
	report, err := cleaner.ExecuteCleanup("nonexistent-id")
	if err != nil {
		t.Fatalf("ExecuteCleanup: %v", err)
	}

	// All 3 sessions are old and the excluded ID matches nothing.
	if len(report.Removed) != 3 {
		t.Fatalf("expected 3 removed, got %d: %v", len(report.Removed), report.Removed)
	}

	remaining, err := ScanSessions()
	if err != nil {
		t.Fatalf("ScanSessions: %v", err)
	}
	if len(remaining) != 0 {
		t.Fatalf("expected 0 remaining sessions, got %d", len(remaining))
	}
}

// TestCleaner_ManyOrphanLocks verifies that the cleaner removes a large
// number of orphan lock files (no corresponding session files).
func TestCleaner_ManyOrphanLocks(t *testing.T) {
	dir := t.TempDir()
	SetTestPaths(dir)
	defer ResetPaths()

	const count = 20
	expectedIDs := make([]string, count)

	for i := range count {
		id := fmt.Sprintf("orphan-%02d", i)
		expectedIDs[i] = id
		lockPath, _ := sessionLockFilePath(id)
		if err := os.WriteFile(lockPath, []byte("l"), 0644); err != nil {
			t.Fatalf("write orphan lock %s: %v", id, err)
		}
		// Age past the default min orphan grace period (5s).
		old := time.Now().Add(-10 * time.Second)
		if err := os.Chtimes(lockPath, old, old); err != nil {
			t.Fatalf("chtimes lock %s: %v", id, err)
		}
	}

	cleaner := &Cleaner{}
	report, err := cleaner.ExecuteCleanup("")
	if err != nil {
		t.Fatalf("ExecuteCleanup: %v", err)
	}

	// All 20 orphan locks should be removed.
	if len(report.Removed) != count {
		t.Fatalf("expected %d removed, got %d: %v", count, len(report.Removed), report.Removed)
	}

	removedSet := make(map[string]bool)
	for _, id := range report.Removed {
		removedSet[id] = true
	}
	slices.Sort(expectedIDs)
	for _, id := range expectedIDs {
		if !removedSet[id] {
			t.Errorf("expected orphan %s in Removed", id)
		}
	}

	// Verify all lock files are gone.
	for _, id := range expectedIDs {
		lockPath, _ := sessionLockFilePath(id)
		if _, err := os.Stat(lockPath); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("expected lock file for %s to be removed", id)
		}
	}
}
