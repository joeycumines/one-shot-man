package termmux

import (
	"context"
	"io"
	"testing"
	"time"
)

func TestCaptureSession_Passthrough_ContextCancel_JoinsForwarder(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping passthrough integration test in short mode")
	}
	t.Parallel()
	skipIfWindows(t)

	cs := NewCaptureSession(CaptureConfig{Command: buildIdleProgram(t)})
	if err := cs.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer cs.Close()

	stdin := newBlockingReadCloser()
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
	case <-stdin.started:
	case <-time.After(time.Second):
		t.Fatal("passthrough did not enter the blocking read")
	}
	cancel()

	select {
	case result := <-resultCh:
		if result.reason != ExitContext {
			t.Fatalf("reason = %v, want ExitContext", result.reason)
		}
		if !stdin.wasClosed() {
			t.Fatal("capture passthrough returned before the forwarding reader exited")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for capture passthrough to return")
	}
}

func TestSessionManager_Passthrough_ContextCancel_JoinsForwarder(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}
	t.Parallel()

	m, _, _ := passthroughTestManager(t)
	stdin := newBlockingReadCloser()
	ctx, cancel := context.WithCancel(context.Background())
	resultCh := make(chan struct {
		reason ExitReason
		err    error
	}, 1)

	go func() {
		reason, err := m.Passthrough(ctx, PassthroughConfig{
			Stdin:     stdin,
			Stdout:    io.Discard,
			TermFd:    -1,
			ToggleKey: 0x1D,
		})
		resultCh <- struct {
			reason ExitReason
			err    error
		}{reason, err}
	}()

	select {
	case <-stdin.started:
	case <-time.After(time.Second):
		t.Fatal("passthrough did not enter the blocking read")
	}
	cancel()

	select {
	case result := <-resultCh:
		if result.reason != ExitContext {
			t.Fatalf("reason = %v, want ExitContext", result.reason)
		}
		if !stdin.wasClosed() {
			t.Fatal("passthrough returned before the forwarding reader exited")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for passthrough to return")
	}
}
