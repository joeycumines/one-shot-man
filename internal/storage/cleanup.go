package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Cleaner enforces retention policies for session files.
type Cleaner struct {
	MaxAgeDays int
	MaxCount   int
	MaxSizeMB  int
	// MinOrphanAge is the minimum age a lock file must have before the
	// cleaner will consider it an orphan and attempt removal. If zero,
	// a default grace period is used to avoid race conditions with
	// processes that have created a lock but not yet written the session.
	MinOrphanAge time.Duration
	// DryRun when true makes ExecuteCleanup report what it would remove but
	// does not actually delete any files or locks. This is useful for CLI
	// dry-run behavior and tests that need to validate the removal list
	// without performing destructive actions.
	DryRun bool
	// Purge when true instructs the cleaner to ignore retention policies
	// and consider all non-active, non-excluded sessions for removal.
	Purge bool
}

// CleanupReport summarizes what was removed and what was skipped.
type CleanupReport struct {
	Removed []string
	Skipped []string
}

// ExecuteCleanup runs the cleanup process and returns a report.
// excludeID is a session id to never delete (e.g., current session).
func (c *Cleaner) ExecuteCleanup(excludeID string) (*CleanupReport, error) {
	// Acquire a global cleanup lock to avoid concurrent cleaners.
	// Place lock at {UserConfigDir}/one-shot-man/cleanup.lock
	sessionsDir, err := sessionDirectory()
	if err != nil {
		return nil, err
	}
	parent := filepath.Dir(sessionsDir)
	globalLockPath := filepath.Join(parent, "cleanup.lock")

	globalLock, err := acquireFileLock(globalLockPath)
	if err != nil {
		// If we can't acquire the global lock, bail out to avoid race.
		return nil, fmt.Errorf("failed to acquire global cleanup lock: %w", err)
	}
	defer releaseFileLock(globalLock)

	sessions, err := ScanSessions()
	if err != nil {
		return nil, err
	}

	now := time.Now()
	var candidates []SessionInfo
	var report CleanupReport

	// Filter out active sessions and excluded id
	for _, s := range sessions {
		if s.ID == excludeID {
			report.Skipped = append(report.Skipped, s.ID)
			continue
		}
		if s.Active {
			report.Skipped = append(report.Skipped, s.ID)
			continue
		}
		candidates = append(candidates, s)
	}

	// Age-based deletions
	var ageCandidates []SessionInfo
	if c.MaxAgeDays > 0 {
		cutoff := now.Add(-time.Duration(c.MaxAgeDays) * 24 * time.Hour)
		for _, s := range candidates {
			if s.UpdateTime.Before(cutoff) {
				ageCandidates = append(ageCandidates, s)
			}
		}
	}

	// Mark ageCandidates for removal
	toRemoveMap := map[string]SessionInfo{}
	for _, s := range ageCandidates {
		toRemoveMap[s.ID] = s
	}

	// Purge mode: select all candidates (non-active, non-excluded)
	if c.Purge {
		for _, s := range candidates {
			toRemoveMap[s.ID] = s
		}
	}

	// Count-based pruning: keep newest MaxCount
	if c.MaxCount > 0 {
		// Sort candidates by UpdateTime descending (newest first)
		sort.SliceStable(candidates, func(i, j int) bool {
			return candidates[i].UpdateTime.After(candidates[j].UpdateTime)
		})
		if len(candidates) > c.MaxCount {
			for _, s := range candidates[c.MaxCount:] {
				toRemoveMap[s.ID] = s
			}
		}
	}

	// Size-based pruning
	if c.MaxSizeMB > 0 {
		var total int64
		for _, s := range candidates {
			total += s.Size
		}
		maxBytes := int64(c.MaxSizeMB) * 1024 * 1024
		if total > maxBytes {
			// Remove oldest until under limit
			// Sort ascending (oldest first)
			sort.SliceStable(candidates, func(i, j int) bool {
				return candidates[i].UpdateTime.Before(candidates[j].UpdateTime)
			})
			for _, s := range candidates {
				if total <= maxBytes {
					break
				}
				total -= s.Size
				toRemoveMap[s.ID] = s
			}
		}
	}

	// Collect removal list
	var toRemove []SessionInfo
	for _, s := range toRemoveMap {
		toRemove = append(toRemove, s)
	}

	// Perform deletes safely: try to acquire the session lock non-blocking and
	// hold the lock while removing the session file. This avoids releasing the
	// lock and re-acquiring it (which creates a race window).
	// List candidates to inspect behavior in tests.
	for _, s := range toRemove {
		if c.DryRun {
			// In dry-run mode we do not attempt any filesystem mutations; list
			// the candidate as removed for reporting purposes but do not touch
			// locks or session files.
			report.Removed = append(report.Removed, s.ID)
			continue
		}
		// Try locking session safely
		f, ok, err := AcquireLockHandle(s.LockPath)
		if err != nil || !ok {
			if f != nil {
				_ = f.Close()
			}
			report.Skipped = append(report.Skipped, s.ID)
			continue
		}

		// We hold the lock. Attempt to remove the session file.
		if err := os.Remove(s.Path); err != nil && !os.IsNotExist(err) {
			// Failed to remove session file; Close handle to release lock
			// but DO NOT remove the lock file.
			_ = f.Close()
			report.Skipped = append(report.Skipped, s.ID)
			continue
		}

		// Session file removed successfully (or didn't exist).
		// Now release and remove the lock file.
		if err := ReleaseLockHandle(f); err != nil {
			// Technically a partial failure (session gone, lock remains),
			// but we consider the session "Removed" for the report.
		}
		report.Removed = append(report.Removed, s.ID)
	}

	// list toRemove entries

	// Orphaned locks: remove any *.lock files that have no corresponding .session.json
	// Read directory entries directly
	dir := sessionsDir
	entries, err := os.ReadDir(dir)
	if err != nil {
		// If the sessions directory doesn't exist, there is nothing to clean.
		if os.IsNotExist(err) {
			return &report, nil
		}
		return nil, fmt.Errorf("failed to read sessions directory %q: %w", dir, err)
	}
	// debug: show directory entries in logs when needed

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if filepath.Ext(name) != ".lock" {
			continue
		}
		// Only treat files that end in the expected ".session.lock" suffix as
		// session lockfiles. Some tools may create arbitrary ".lock" files
		// (e.g. "temp.lock") which would make the naive substring approach
		// panic due to negative slicing. Guard against that explicitly.
		if !strings.HasSuffix(name, ".session.lock") {
			continue
		}
		base := name[:len(name)-len(".session.lock")]
		sessionPath := filepath.Join(dir, base+".session.json")
		// Instead of checking for session existence and then trying to acquire
		// the lock (which opens a TOCTOU window), attempt to acquire the lock
		// first. If we can hold the lock, we are guaranteed nobody else owns
		// the session and can safely check whether the session file exists and
		// remove the orphaned lock without racing with a creator.
		lockPath := filepath.Join(dir, name)
		// Conservative approach: check whether a corresponding session file
		// exists first. If the session exists, we must not remove the lock
		// artifact — leave it in place. Only attempt to acquire the lock and
		// remove the artifact when the session file does not exist (an orphan).
		if _, err := os.Stat(sessionPath); err == nil {
			// Session file exists — do not remove the lock artifact.
			report.Skipped = append(report.Skipped, base)
			continue
		} else if !os.IsNotExist(err) {
			// Unexpected stat error - skip conservative.
			report.Skipped = append(report.Skipped, base)
			continue
		}

		// Session is missing — check how old the lock file is. If it's very
		// young, it's likely an embryonic session creator hasn't finished
		// initializing the session file yet. Skip recent lock files to avoid
		// the Unix inode race where a creator holds an FD on an unlinked inode.
		info, ierr := e.Info()
		if ierr != nil {
			// Fallback to stat the path if DirEntry.Info failed for any reason.
			info, ierr = os.Stat(lockPath)
		}
		if ierr != nil {
			// If we can't stat the lock file conservatively skip it.
			report.Skipped = append(report.Skipped, base)
			continue
		}

		// Do not treat very young locks as orphaned — allow a short grace
		// period for processes creating a new session to finish writing.
		minOrphanAge := c.MinOrphanAge
		if minOrphanAge == 0 {
			minOrphanAge = 5 * time.Second
		}
		if time.Since(info.ModTime()) < minOrphanAge {
			report.Skipped = append(report.Skipped, base)
			continue
		}

		// Session is missing — try to acquire the lock handle and remove
		// About to attempt locking and removal for orphan candidate
		// the orphaned artifact if we can safely lock it.
		if c.DryRun {
			// Don't modify filesystem in dry-run mode; just report the orphan
			// lock as something that would be removed.
			report.Removed = append(report.Removed, base)
			continue
		}

		if f, ok, err := AcquireLockHandle(lockPath); err == nil && ok {
			if rerr := ReleaseLockHandle(f); rerr == nil {
				report.Removed = append(report.Removed, base)
			} else {
				report.Skipped = append(report.Skipped, base)
			}
		} else {
			// Could not acquire the lock or encountered an error -> skip
			report.Skipped = append(report.Skipped, base)
		}
	}

	return &report, nil
}
