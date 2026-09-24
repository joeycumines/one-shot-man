package userk8s

import (
	"context"
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
	wrapped := &serialRunner{inner: inner}

	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := wrapped.Run(context.Background(), []string{"resolver"}, time.Minute); err != nil {
				t.Errorf("Run: %v", err)
			}
		}()
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
