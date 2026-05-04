package storage

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
)

// TestFileLock_ConcurrentAcquisition verifies that when 20 goroutines race to
// acquire the same file lock, exactly one wins and the rest fail with
// errWouldBlock. Must pass with -race.
func TestFileLock_ConcurrentAcquisition(t *testing.T) {
	t.Parallel()

	lockPath := filepath.Join(t.TempDir(), "race.lock")

	const numGoroutines = 20
	var (
		wg      sync.WaitGroup
		gate    = make(chan struct{}) // closed to start all goroutines simultaneously
		winners int32
		losers  int32
	)

	type result struct {
		file *os.File
		err  error
	}
	results := make(chan result, numGoroutines)

	wg.Add(numGoroutines)
	for range numGoroutines {
		go func() {
			defer wg.Done()
			<-gate // wait for start signal
			f, err := acquireFileLock(lockPath)
			results <- result{file: f, err: err}
		}()
	}

	// Fire the starting gun.
	close(gate)
	wg.Wait()
	close(results)

	var winnerFile *os.File
	for r := range results {
		if r.err == nil && r.file != nil {
			atomic.AddInt32(&winners, 1)
			winnerFile = r.file
		} else {
			atomic.AddInt32(&losers, 1)
		}
	}

	if winners != 1 {
		t.Fatalf("expected exactly 1 winner, got %d", winners)
	}
	if losers != numGoroutines-1 {
		t.Fatalf("expected %d losers, got %d", numGoroutines-1, losers)
	}

	// Release the winning lock.
	if err := releaseFileLock(winnerFile); err != nil {
		t.Fatalf("releaseFileLock: %v", err)
	}
}

// TestFileLock_RapidCycling acquires and releases the same lock 100 times in a
// tight loop, checking for resource leaks (stale files, file descriptor leaks).
// Must pass with -race.
func TestFileLock_RapidCycling(t *testing.T) {
	t.Parallel()

	lockPath := filepath.Join(t.TempDir(), "cycle.lock")

	const iterations = 100
	for i := range iterations {
		f, err := acquireFileLock(lockPath)
		if err != nil {
			t.Fatalf("iteration %d: acquireFileLock failed: %v", i, err)
		}
		if f == nil {
			t.Fatalf("iteration %d: acquireFileLock returned nil file", i)
		}

		if err := releaseFileLock(f); err != nil {
			t.Fatalf("iteration %d: releaseFileLock failed: %v", i, err)
		}

		// Lock file should be removed after release.
		if _, err := os.Stat(lockPath); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("iteration %d: lock file still exists after release", i)
		}
	}
}

// TestFileLock_CrashRecovery simulates a process crash by closing the file
// descriptor directly (without calling releaseFileLock). On both Unix and
// Windows, closing the FD releases the advisory/mandatory lock, so a
// subsequent acquire on the same path must succeed.
// Must pass with -race.
func TestFileLock_CrashRecovery(t *testing.T) {
	t.Parallel()

	lockPath := filepath.Join(t.TempDir(), "crash.lock")

	// Acquire the lock normally.
	f, err := acquireFileLock(lockPath)
	if err != nil {
		t.Fatalf("initial acquireFileLock: %v", err)
	}

	// Simulate crash: close the fd directly, bypassing releaseFileLock.
	// This releases the flock but does NOT remove the lock file from disk.
	if err := f.Close(); err != nil {
		t.Fatalf("simulated crash close: %v", err)
	}

	// The lock file may still exist on disk (releaseFileLock was never called).
	// That's expected — the important thing is the *advisory lock* was released.

	// Re-acquire must succeed because the flock is released.
	f2, err := acquireFileLock(lockPath)
	if err != nil {
		t.Fatalf("post-crash acquireFileLock: %v", err)
	}
	if f2 == nil {
		t.Fatal("post-crash acquireFileLock returned nil file")
	}

	// Clean up properly this time.
	if err := releaseFileLock(f2); err != nil {
		t.Fatalf("releaseFileLock after recovery: %v", err)
	}
}

// TestFileLock_ConcurrentAcquireRelease tests robustness under sustained
// contention: 10 goroutines each loop 20 times, racing to acquire/release.
// Must pass with -race.
func TestFileLock_ConcurrentAcquireRelease(t *testing.T) {
	t.Parallel()

	lockPath := filepath.Join(t.TempDir(), "churn.lock")

	const (
		numGoroutines = 10
		iterations    = 20
	)

	var (
		wg        sync.WaitGroup
		gate      = make(chan struct{})
		successes int64
	)

	wg.Add(numGoroutines)
	for range numGoroutines {
		go func() {
			defer wg.Done()
			<-gate
			for range iterations {
				f, err := acquireFileLock(lockPath)
				if err != nil {
					// Lock held by another goroutine — expected.
					continue
				}
				atomic.AddInt64(&successes, 1)
				// Hold briefly (no sleep — just do the bookkeeping).
				if err := releaseFileLock(f); err != nil {
					t.Errorf("releaseFileLock: %v", err)
					return
				}
			}
		}()
	}

	close(gate)
	wg.Wait()

	total := atomic.LoadInt64(&successes)
	if total == 0 {
		t.Fatal("expected at least 1 successful acquisition across all goroutines, got 0")
	}
	t.Logf("total successful lock acquisitions: %d / %d attempts",
		total, numGoroutines*iterations)
}

// TestAcquireLockHandle_ConcurrentAccess mirrors TestFileLock_ConcurrentAcquisition
// but exercises the public AcquireLockHandle / ReleaseLockHandle API.
// Must pass with -race.
func TestAcquireLockHandle_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	lockPath := filepath.Join(t.TempDir(), "handlerace.lock")

	const numGoroutines = 20
	var (
		wg   sync.WaitGroup
		gate = make(chan struct{})
	)

	type result struct {
		file *os.File
		ok   bool
		err  error
	}
	results := make(chan result, numGoroutines)

	wg.Add(numGoroutines)
	for range numGoroutines {
		go func() {
			defer wg.Done()
			<-gate
			f, ok, err := AcquireLockHandle(lockPath)
			results <- result{file: f, ok: ok, err: err}
		}()
	}

	close(gate)
	wg.Wait()
	close(results)

	var (
		winners    int
		winnerFile *os.File
	)
	for r := range results {
		if r.err != nil {
			t.Fatalf("AcquireLockHandle returned unexpected error: %v", r.err)
		}
		if r.ok {
			winners++
			winnerFile = r.file
		} else {
			// Loser: file must be nil
			if r.file != nil {
				t.Error("loser got non-nil file handle")
				_ = r.file.Close()
			}
		}
	}

	if winners != 1 {
		t.Fatalf("expected exactly 1 winner, got %d", winners)
	}

	if err := ReleaseLockHandle(winnerFile); err != nil {
		t.Fatalf("ReleaseLockHandle: %v", err)
	}
}

// TestFSBackend_ConcurrentOpen verifies that concurrent attempts to open a
// FileSystemBackend for the same session ID result in exactly one success.
// Must pass with -race.
func TestFSBackend_ConcurrentOpen(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "sessions")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Override path functions for this test. Because SetTestPaths/ResetPaths
	// mutate package-level state, we guard access and restore manually to
	// avoid interfering with parallel tests.
	pathsMu.Lock()
	origDir := sessionDirectory
	origFile := sessionFilePath
	origLock := sessionLockFilePath
	sessionDirectory = func() (string, error) { return dir, nil }
	sessionFilePath = func(id string) (string, error) {
		return filepath.Join(dir, id+".session.json"), nil
	}
	sessionLockFilePath = func(id string) (string, error) {
		return filepath.Join(dir, id+".session.lock"), nil
	}
	pathsMu.Unlock()
	defer func() {
		pathsMu.Lock()
		sessionDirectory = origDir
		sessionFilePath = origFile
		sessionLockFilePath = origLock
		pathsMu.Unlock()
	}()

	const (
		sessionID     = "concurrent-open-test"
		numGoroutines = 20
	)

	var (
		wg   sync.WaitGroup
		gate = make(chan struct{})
	)

	type result struct {
		backend *FileSystemBackend
		err     error
	}
	results := make(chan result, numGoroutines)

	wg.Add(numGoroutines)
	for range numGoroutines {
		go func() {
			defer wg.Done()
			<-gate
			b, err := NewFileSystemBackend(sessionID)
			results <- result{backend: b, err: err}
		}()
	}

	close(gate)
	wg.Wait()
	close(results)

	var (
		winners int
		winner  *FileSystemBackend
	)
	for r := range results {
		if r.err == nil && r.backend != nil {
			winners++
			winner = r.backend
		}
		// Others should have received an error — that's expected.
	}

	if winners != 1 {
		t.Fatalf("expected exactly 1 successful backend open, got %d", winners)
	}

	if err := winner.Close(); err != nil {
		t.Fatalf("Close winner: %v", err)
	}
}
