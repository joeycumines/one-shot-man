package command

import (
	"context"
	"testing"
	"time"

	"github.com/joeycumines/one-shot-man/internal/termmux"
)

// TestForceCloseSessionManager_Nil verifies that passing nil does not panic.
func TestForceCloseSessionManager_Nil(t *testing.T) {
	t.Parallel()
	forceCloseSessionManager(nil) // must not panic
}

// TestForceCloseSessionManager_RunningManager verifies that a running
// SessionManager is closed within the 5-second deadline.
func TestForceCloseSessionManager_RunningManager(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mgr := termmux.NewSessionManager(termmux.WithTermSize(24, 80))
	errCh := make(chan error, 1)
	go func() { errCh <- mgr.Run(ctx) }()
	<-mgr.Started()

	// forceCloseSessionManager should close the manager promptly.
	start := time.Now()
	forceCloseSessionManager(mgr)
	elapsed := time.Since(start)

	// Verify it completed well within the 5-second timeout.
	if elapsed > 2*time.Second {
		t.Errorf("forceCloseSessionManager took %v, expected < 2s", elapsed)
	}

	// Run should have returned after Close.
	select {
	case err := <-errCh:
		// context.Canceled or nil are both acceptable.
		_ = err
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after forceCloseSessionManager")
	}
}

// TestForceCloseSessionManager_AlreadyClosed verifies idempotent Close.
func TestForceCloseSessionManager_AlreadyClosed(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	mgr := termmux.NewSessionManager(termmux.WithTermSize(24, 80))
	errCh := make(chan error, 1)
	go func() { errCh <- mgr.Run(ctx) }()
	<-mgr.Started()

	// Normal close first.
	cancel()
	<-errCh

	// Second close via forceCloseSessionManager must not panic or hang.
	start := time.Now()
	forceCloseSessionManager(mgr)
	elapsed := time.Since(start)

	if elapsed > 1*time.Second {
		t.Errorf("double-close took %v, expected near-instant", elapsed)
	}
}
