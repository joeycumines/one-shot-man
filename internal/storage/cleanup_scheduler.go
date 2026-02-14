package storage

import (
	"context"
	"time"
)

// CleanupScheduler runs periodic session cleanup in a background goroutine.
// It executes cleanup immediately on Run, then at the configured interval.
type CleanupScheduler struct {
	// Cleaner defines the retention policy to apply.
	Cleaner *Cleaner
	// ExcludeID is the session ID to exclude from cleanup (e.g., current session).
	// An empty string means no exclusion; active (locked) sessions are still
	// skipped automatically by the cleaner.
	ExcludeID string
	// Interval is the time between cleanup runs. If <= 0, only the initial
	// cleanup on startup is performed and Run returns immediately after.
	Interval time.Duration

	// NewTicker creates a ticker channel and its stop function.
	// If nil, time.NewTicker is used. Inject a custom implementation for
	// deterministic testing without real timers.
	NewTicker func(d time.Duration) (tick <-chan time.Time, stop func())
}

// Run executes cleanup immediately, then at intervals until ctx is cancelled.
// It blocks until ctx.Done() fires. Cleanup errors are silently ignored
// because cleanup is best-effort — the cleaner already handles lock contention
// and concurrent access safely via the global cleanup lock.
func (s *CleanupScheduler) Run(ctx context.Context) {
	// Run once immediately on startup.
	s.runOnce()

	if s.Interval <= 0 {
		// No recurring runs requested; wait for cancellation.
		<-ctx.Done()
		return
	}

	newTicker := s.NewTicker
	if newTicker == nil {
		newTicker = defaultNewTicker
	}

	ch, stop := newTicker(s.Interval)
	defer stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ch:
			s.runOnce()
		}
	}
}

// runOnce executes a single cleanup cycle, ignoring errors.
func (s *CleanupScheduler) runOnce() {
	_, _ = s.Cleaner.ExecuteCleanup(s.ExcludeID)
}

// defaultNewTicker wraps time.NewTicker to match the NewTicker signature.
func defaultNewTicker(d time.Duration) (<-chan time.Time, func()) {
	t := time.NewTicker(d)
	return t.C, t.Stop
}
