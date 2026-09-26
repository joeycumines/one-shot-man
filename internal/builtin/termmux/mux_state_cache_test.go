package termmux

// mux_state_cache_test.go — manager-cache reconciliation and its retry bound.
//
// initializeManagerCache publishes a snapshot of the manager's sessions so the
// bindings can answer without a worker round-trip. Two properties matter:
//
//   - it must RECONCILE, not just add. A session that disappeared from the
//     manager between two reads would otherwise stay "known" forever and be
//     reported as live by cachedSessionDone.
//   - it must be bounded. The publish step is guarded by an epoch check that
//     retries on churn; an unbounded retry loop leaks a goroutine under exactly
//     the churn that causes it.

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	parent "github.com/joeycumines/one-shot-man/internal/termmux"
)

// startTestManagerWithSession starts a manager with one registered live
// session and returns the manager, the session ID, and a stop function.
func startTestManagerWithSession(t *testing.T) (*parent.SessionManager, parent.SessionID, func()) {
	t.Helper()
	m := parent.NewSessionManager(parent.WithTermSize(30, 100))
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- m.Run(ctx) }()
	<-m.Started()

	sess := &cacheTestSession{doneCh: make(chan struct{})}
	id, err := m.Register(sess, parent.SessionTarget{Name: "live", Kind: parent.SessionKindPTY})
	if err != nil {
		cancel()
		<-errCh
		t.Fatalf("Register: %v", err)
	}
	return m, id, func() {
		cancel()
		<-errCh
	}
}

type cacheTestSession struct {
	doneCh chan struct{}
}

func (s *cacheTestSession) Done() <-chan struct{}     { return s.doneCh }
func (s *cacheTestSession) Reader() <-chan []byte     { return make(chan []byte) }
func (s *cacheTestSession) Write([]byte) (int, error) { return 0, nil }
func (s *cacheTestSession) Resize(int, int) error     { return nil }
func (s *cacheTestSession) Close() error {
	select {
	case <-s.doneCh:
	default:
		close(s.doneCh)
	}
	return nil
}

// TestInitializeManagerCache_ReconcilesVanishedSession is the regression guard
// for a session that leaves the manager without a close/exit event reaching
// this cache.
func TestInitializeManagerCache_ReconcilesVanishedSession(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}

	mgr, liveID, stop := startTestManagerWithSession(t)
	defer stop()

	state := &muxState{mgr: mgr}
	// Seed a session that the manager has never heard of, as an add-only
	// publish would leave behind.
	state.knownSessions.Store(uint64(9999), struct{}{})
	state.doneSessions.Store(uint64(9999), struct{}{})

	state.initializeManagerCache()

	if _, ok := state.knownSessions.Load(uint64(9999)); ok {
		t.Error("session absent from the manager survived in knownSessions")
	}
	if _, ok := state.doneSessions.Load(uint64(9999)); ok {
		t.Error("session absent from the manager survived in doneSessions")
	}
	if _, ok := state.knownSessions.Load(uint64(liveID)); !ok {
		t.Error("live session missing from knownSessions after initialize")
	}
	if state.cachedSessionDone(uint64(liveID)) {
		t.Error("live session reported as done")
	}
	if rows, cols := state.cachedTermSize(); rows != 30 || cols != 100 {
		t.Errorf("cached term size = %dx%d, want 30x100", rows, cols)
	}
}

// TestInitializeManagerCache_RetryIsBounded covers the churn case: the epoch
// keeps moving, and the call must return instead of spinning.
func TestInitializeManagerCache_RetryIsBounded(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}

	mgr, _, stop := startTestManagerWithSession(t)
	defer stop()

	state := &muxState{mgr: mgr}

	// Churn the epoch from another goroutine so every publish attempt is
	// invalidated and the retry path is taken every time.
	stopChurn := make(chan struct{})
	var churned atomic.Bool
	go func() {
		for {
			select {
			case <-stopChurn:
				return
			default:
			}
			state.cacheEpoch.Add(1)
			churned.Store(true)
		}
	}()

	attemptsCh := make(chan int, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		attemptsCh <- state.initializeManagerCache()
	}()
	var attempts int
	select {
	case attempts = <-attemptsCh:
	case <-time.After(20 * time.Second):
		close(stopChurn)
		t.Fatal("initializeManagerCache did not return under epoch churn")
	}
	close(stopChurn)
	if !churned.Load() {
		t.Skip("churn goroutine never ran; nothing was exercised")
	}
	// The bound is the contract: whatever the churn did, the call made no
	// more than maxCacheInitAttempts passes over the publish step.
	if attempts < 1 {
		t.Errorf("attempts = %d, want at least 1", attempts)
	}
	if attempts > maxCacheInitAttempts {
		t.Errorf("attempts = %d, want <= %d", attempts, maxCacheInitAttempts)
	}
}

// TestMuxStateCachedTermSizeIsPairedAndNilSafe covers the read side the
// bindings use: a nil state and an unpopulated cache both read as 0x0, and a
// published size round-trips as a pair.
func TestMuxStateCachedTermSizeIsPairedAndNilSafe(t *testing.T) {
	var nilState *muxState
	if rows, cols := nilState.cachedTermSize(); rows != 0 || cols != 0 {
		t.Errorf("nil state term size = %dx%d, want 0x0", rows, cols)
	}
	state := &muxState{}
	if rows, cols := state.cachedTermSize(); rows != 0 || cols != 0 {
		t.Errorf("empty state term size = %dx%d, want 0x0", rows, cols)
	}
	state.cacheTermSize(24, 80)
	if rows, cols := state.cachedTermSize(); rows != 24 || cols != 80 {
		t.Errorf("term size = %dx%d, want 24x80", rows, cols)
	}
}
