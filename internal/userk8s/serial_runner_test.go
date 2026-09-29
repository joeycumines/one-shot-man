package userk8s

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// concurrencyProbeRunner blocks every Run until released, recording the peak
// number of concurrent Runs so a test can assert serialization.
type concurrencyProbeRunner struct {
	release chan struct{}
	current atomic.Int32
	peak    atomic.Int32
	total   atomic.Int32
}

func (c *concurrencyProbeRunner) Run(ctx context.Context, argv []string, timeout time.Duration) (string, error) {
	cur := c.current.Add(1)
	for {
		peak := c.peak.Load()
		if cur <= peak || c.peak.CompareAndSwap(peak, cur) {
			break
		}
	}
	<-c.release
	c.current.Add(-1)
	c.total.Add(1)
	return "value", nil
}

// TestSerialRunnerSerializesResolves pins the invariant the gateway depends on:
// resolveMountCredentials fires one resolver at a time, so concurrent calls
// must never run two commands (and thus two Touch ID prompts) at once.
func TestSerialRunnerSerializesResolves(t *testing.T) {
	const callers = 4
	inner := &concurrencyProbeRunner{release: make(chan struct{})}
	wrapped := newSerialRunner(inner)

	var wg sync.WaitGroup
	for range callers {
		wg.Go(func() {
			if _, err := wrapped.Run(context.Background(), []string{"resolver"}, time.Minute); err != nil {
				t.Errorf("Run: %v", err)
			}
		})
	}

	// Wait until one caller is inside Run, then give the rest a chance to
	// pile up on the mutex. The peak must stay at one.
	deadline := time.After(5 * time.Second)
	for inner.current.Load() == 0 {
		select {
		case <-deadline:
			t.Fatal("no resolver run started")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	time.Sleep(50 * time.Millisecond)
	if got := inner.peak.Load(); got != 1 {
		t.Fatalf("peak concurrent resolver runs = %d, want 1", got)
	}

	close(inner.release)
	wg.Wait()
	if got := inner.total.Load(); got != callers {
		t.Fatalf("resolver runs = %d, want %d", got, callers)
	}
}

// TestSerialRunnerWaitsCancellably covers the context-aware gate: a caller
// cancelled while another command holds the gate returns promptly with the
// context error instead of blocking for the in-flight command's whole timeout
// (which can be 60s for a human Touch ID approval).
func TestSerialRunnerWaitsCancellably(t *testing.T) {
	inner := &concurrencyProbeRunner{release: make(chan struct{})}
	wrapped := newSerialRunner(inner)

	// Hold the gate with a first call that blocks inside the runner.
	first := make(chan error, 1)
	go func() {
		_, err := wrapped.Run(context.Background(), []string{"resolver"}, time.Minute)
		first <- err
	}()
	deadline := time.After(5 * time.Second)
	for inner.current.Load() == 0 {
		select {
		case <-deadline:
			t.Fatal("the first resolver run never started")
		default:
			time.Sleep(time.Millisecond)
		}
	}

	// A second caller with a cancellable context must return promptly.
	ctx, cancel := context.WithCancel(context.Background())
	second := make(chan error, 1)
	start := time.Now()
	go func() {
		_, err := wrapped.Run(ctx, []string{"resolver"}, time.Minute)
		second <- err
	}()
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case err := <-second:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled waiter error = %v, want context.Canceled", err)
		}
		if elapsed := time.Since(start); elapsed > 2*time.Second {
			t.Fatalf("cancelled waiter took %v to return", elapsed)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cancelled waiter did not return; the gate wait is not context-aware")
	}

	close(inner.release)
	if err := <-first; err != nil {
		t.Fatalf("first run: %v", err)
	}
}
