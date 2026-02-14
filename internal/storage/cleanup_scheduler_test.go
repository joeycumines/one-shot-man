package storage

import (
	"context"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

// TestCleanupScheduler_RunsOnStartup verifies that the scheduler runs cleanup
// immediately when Run is called, without waiting for a tick.
func TestCleanupScheduler_RunsOnStartup(t *testing.T) {
	dir := t.TempDir()
	SetTestPaths(dir)
	defer ResetPaths()

	// Create an old session that should be cleaned up.
	old := time.Now().Add(-48 * time.Hour)
	p, _ := SessionFilePath("startup-old")
	if err := os.WriteFile(p, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(p, old, old); err != nil {
		t.Fatal(err)
	}

	cleaner := &Cleaner{MaxAgeDays: 1}

	// Use a ticker that never fires to prove cleanup runs on startup only.
	neverTick := make(chan time.Time)
	scheduler := &CleanupScheduler{
		Cleaner:  cleaner,
		Interval: time.Hour,
		NewTicker: func(d time.Duration) (<-chan time.Time, func()) {
			return neverTick, func() {}
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		scheduler.Run(ctx)
		close(done)
	}()

	// Give the startup cleanup a moment to complete, then cancel.
	// We don't use time.Sleep because the startup cleanup runs synchronously
	// before entering the ticker loop. Instead, wait briefly and check.
	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done

	// Session should have been removed by the startup cleanup.
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatalf("expected session to be removed by startup cleanup, stat err: %v", err)
	}
}

// TestCleanupScheduler_RunsOnTick verifies that the scheduler runs cleanup
// when the ticker fires.
func TestCleanupScheduler_RunsOnTick(t *testing.T) {
	dir := t.TempDir()
	SetTestPaths(dir)
	defer ResetPaths()

	cleaner := &Cleaner{MaxAgeDays: 1}

	// Use a controllable ticker channel.
	tick := make(chan time.Time, 1)
	scheduler := &CleanupScheduler{
		Cleaner:  cleaner,
		Interval: time.Hour,
		NewTicker: func(d time.Duration) (<-chan time.Time, func()) {
			return tick, func() {}
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		scheduler.Run(ctx)
		close(done)
	}()

	// Let startup cleanup complete (no old sessions yet).
	time.Sleep(20 * time.Millisecond)

	// Create an old session AFTER startup cleanup ran.
	old := time.Now().Add(-48 * time.Hour)
	p, _ := SessionFilePath("tick-old")
	if err := os.WriteFile(p, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(p, old, old); err != nil {
		t.Fatal(err)
	}

	// Session should still exist (not yet cleaned).
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("expected session to exist before tick, err: %v", err)
	}

	// Fire the tick.
	tick <- time.Now()
	// Allow cleanup to process.
	time.Sleep(50 * time.Millisecond)

	// Session should now be removed.
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatalf("expected session to be removed after tick, stat err: %v", err)
	}

	cancel()
	<-done
}

// TestCleanupScheduler_StopsOnCancel verifies that cancelling the context
// causes Run to return promptly.
func TestCleanupScheduler_StopsOnCancel(t *testing.T) {
	dir := t.TempDir()
	SetTestPaths(dir)
	defer ResetPaths()

	cleaner := &Cleaner{MaxAgeDays: 365}

	neverTick := make(chan time.Time)
	scheduler := &CleanupScheduler{
		Cleaner:  cleaner,
		Interval: time.Hour,
		NewTicker: func(d time.Duration) (<-chan time.Time, func()) {
			return neverTick, func() {}
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		scheduler.Run(ctx)
		close(done)
	}()

	// Cancel immediately.
	cancel()

	// Run should return promptly.
	select {
	case <-done:
		// Good — Run returned.
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after context cancellation within 2s")
	}
}

// TestCleanupScheduler_ZeroInterval verifies that with Interval <= 0,
// Run performs the startup cleanup and then waits for cancellation
// without entering the ticker loop.
func TestCleanupScheduler_ZeroInterval(t *testing.T) {
	dir := t.TempDir()
	SetTestPaths(dir)
	defer ResetPaths()

	// Create an old session.
	old := time.Now().Add(-48 * time.Hour)
	p, _ := SessionFilePath("zero-int")
	if err := os.WriteFile(p, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(p, old, old); err != nil {
		t.Fatal(err)
	}

	cleaner := &Cleaner{MaxAgeDays: 1}
	scheduler := &CleanupScheduler{
		Cleaner:  cleaner,
		Interval: 0, // no recurring ticks
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		scheduler.Run(ctx)
		close(done)
	}()

	// Startup cleanup runs synchronously, so brief wait is sufficient.
	time.Sleep(50 * time.Millisecond)

	// Old session should be removed by startup cleanup.
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatalf("expected session removed by startup cleanup with 0 interval, stat err: %v", err)
	}

	cancel()
	<-done
}

// TestCleanupScheduler_ExcludeID verifies that the scheduler passes the
// ExcludeID to the cleaner, protecting a specific session from cleanup.
func TestCleanupScheduler_ExcludeID(t *testing.T) {
	dir := t.TempDir()
	SetTestPaths(dir)
	defer ResetPaths()

	old := time.Now().Add(-48 * time.Hour)

	// Create two old sessions.
	p1, _ := SessionFilePath("exclude-keep")
	p2, _ := SessionFilePath("exclude-remove")
	for _, p := range []string{p1, p2} {
		if err := os.WriteFile(p, []byte("{}"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, old, old); err != nil {
			t.Fatal(err)
		}
	}

	cleaner := &Cleaner{MaxAgeDays: 1}
	scheduler := &CleanupScheduler{
		Cleaner:   cleaner,
		ExcludeID: "exclude-keep",
		Interval:  0,
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		scheduler.Run(ctx)
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done

	// exclude-keep should still exist.
	if _, err := os.Stat(p1); err != nil {
		t.Fatalf("expected excluded session to remain, stat err: %v", err)
	}
	// exclude-remove should be deleted.
	if _, err := os.Stat(p2); !os.IsNotExist(err) {
		t.Fatalf("expected non-excluded session to be removed, stat err: %v", err)
	}
}

// TestCleanupScheduler_MultipleTicksRunMultipleCleanups verifies that each
// tick triggers a new cleanup cycle.
func TestCleanupScheduler_MultipleTicksRunMultipleCleanups(t *testing.T) {
	dir := t.TempDir()
	SetTestPaths(dir)
	defer ResetPaths()

	// Use MaxAgeDays=1 from the start so session creation and cleanup
	// don't race on the Cleaner field.
	cleaner := &Cleaner{MaxAgeDays: 1}

	tick := make(chan time.Time, 1)
	scheduler := &CleanupScheduler{
		Cleaner:  cleaner,
		Interval: time.Hour,
		NewTicker: func(d time.Duration) (<-chan time.Time, func()) {
			return tick, func() {}
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		scheduler.Run(ctx)
		close(done)
	}()

	// Wait for startup cleanup to complete (no old sessions yet).
	time.Sleep(20 * time.Millisecond)

	// Create sessions between ticks and verify they get cleaned up.
	old := time.Now().Add(-48 * time.Hour)

	// Tick 1: create and clean a session.
	p1, _ := SessionFilePath("multi-1")
	if err := os.WriteFile(p1, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(p1, old, old); err != nil {
		t.Fatal(err)
	}

	tick <- time.Now()
	time.Sleep(50 * time.Millisecond)

	if _, err := os.Stat(p1); !os.IsNotExist(err) {
		t.Fatalf("tick 1: expected session removed, stat err: %v", err)
	}

	// Tick 2: create another session and tick again.
	p2, _ := SessionFilePath("multi-2")
	if err := os.WriteFile(p2, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(p2, old, old); err != nil {
		t.Fatal(err)
	}

	tick <- time.Now()
	time.Sleep(50 * time.Millisecond)

	if _, err := os.Stat(p2); !os.IsNotExist(err) {
		t.Fatalf("tick 2: expected session removed, stat err: %v", err)
	}

	cancel()
	<-done
}

// TestCleanupScheduler_TickerStopCalled verifies that the stop function
// returned by NewTicker is called when the scheduler exits.
func TestCleanupScheduler_TickerStopCalled(t *testing.T) {
	dir := t.TempDir()
	SetTestPaths(dir)
	defer ResetPaths()

	var stopped atomic.Bool

	neverTick := make(chan time.Time)
	scheduler := &CleanupScheduler{
		Cleaner:  &Cleaner{MaxAgeDays: 365},
		Interval: time.Hour,
		NewTicker: func(d time.Duration) (<-chan time.Time, func()) {
			return neverTick, func() { stopped.Store(true) }
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		scheduler.Run(ctx)
		close(done)
	}()

	cancel()
	<-done

	if !stopped.Load() {
		t.Fatal("expected ticker stop function to be called on shutdown")
	}
}
