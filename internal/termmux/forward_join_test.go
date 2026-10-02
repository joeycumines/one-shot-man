package termmux

// forward_join_test.go — the stdin forwarder join must never hang.
//
// Cancellation cannot always interrupt the forwarder's blocking read:
// ultraviolet falls back to a non-cancellable reader for /dev/tty and for
// descriptors at or above FD_SETSIZE, and NewCancelReader fails outright for a
// regular file on Linux. Before the join was bounded, a passthrough over such
// a reader blocked forever instead of returning.

import (
	"context"
	"io"
	"sync"
	"testing"
	"time"
)

// TestJoinForwarder_ReturnsWhenForwarderExits is the ordinary path: a
// forwarder that honours cancellation is joined immediately.
func TestJoinForwarder_ReturnsWhenForwarderExits(t *testing.T) {
	done := make(chan struct{})
	close(done)
	start := time.Now()
	if !joinForwarder(done) {
		t.Fatal("joinForwarder reported abandonment for an exited forwarder")
	}
	if elapsed := time.Since(start); elapsed > forwardJoinGrace {
		t.Errorf("join took %s for an already-exited forwarder", elapsed)
	}
}

// TestJoinForwarder_GivesUpOnStuckForwarder is the regression guard: a
// forwarder blocked in an uninterruptible read must be abandoned after the
// grace period, not waited on forever.
func TestJoinForwarder_GivesUpOnStuckForwarder(t *testing.T) {
	stuck := make(chan struct{})
	defer close(stuck)

	// The assertion happens on the test goroutine: calling t.Error from a
	// goroutine that outlives the test panics and takes the whole binary with
	// it, which would mask every other test in the package.
	resultCh := make(chan bool, 1)
	go func() { resultCh <- joinForwarder(stuck) }()

	select {
	case ok := <-resultCh:
		if ok {
			t.Error("joinForwarder reported success for a stuck forwarder")
		}
	case <-time.After(forwardJoinGrace * 10):
		t.Fatal("joinForwarder blocked on a forwarder that never exits")
	}
}

// TestPassthrough_UninterruptibleStdinStillReturns exercises the real path: a
// stdin reader that ignores both context cancellation and Close must not
// prevent Passthrough from returning.
func TestPassthrough_UninterruptibleStdinStillReturns(t *testing.T) {
	if testing.Short() {
		t.Skip("slow test skipped in -short mode")
	}
	t.Parallel()
	skipIfWindows(t)

	cs := NewCaptureSession(CaptureConfig{Command: buildIdleProgram(t)})
	if err := cs.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer cs.Close()

	stdin := &uninterruptibleReader{entered: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	resultCh := make(chan struct {
		reason ExitReason
		err    error
	}, 1)
	go func() {
		reason, err := cs.Passthrough(ctx, PassthroughConfig{
			Stdin:  stdin,
			Stdout: io.Discard,
			TermFd: -1,
		})
		resultCh <- struct {
			reason ExitReason
			err    error
		}{reason, err}
	}()

	select {
	case <-stdin.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("passthrough did not enter the blocking read")
	}
	cancel()

	select {
	case result := <-resultCh:
		// Cancellation is the requested outcome; the point of this test is
		// that the call returns at all rather than blocking on the join.
		if result.reason != ExitContext {
			t.Errorf("reason = %v, want ExitContext", result.reason)
		}
	case <-time.After(forwardJoinGrace * 40):
		t.Fatal("passthrough hung on an uninterruptible stdin reader")
	}
}

// uninterruptibleReader blocks forever in Read and ignores Close, modelling
// the non-cancellable fallback readers ultraviolet can return for /dev/tty
// and for descriptors at or above FD_SETSIZE.
type uninterruptibleReader struct {
	entered chan struct{}
	once    sync.Once
}

func (r *uninterruptibleReader) Read([]byte) (int, error) {
	r.once.Do(func() { close(r.entered) })
	select {}
}

func (r *uninterruptibleReader) Close() error { return nil }
