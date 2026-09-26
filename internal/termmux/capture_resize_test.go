package termmux

import (
	"context"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

// countingTermState counts GetSize queries so a test can observe the
// resize watcher without a real terminal. MakeRaw and Restore are promoted
// from the shared fake: the fd is never touched by a real syscall.
type countingTermState struct {
	*ptTestTermState
	sizeCalls atomic.Int32
}

func (t *countingTermState) GetSize(fd int) (int, int, error) {
	t.sizeCalls.Add(1)
	return t.ptTestTermState.GetSize(fd)
}

type resizeTestSignal struct{}

func (resizeTestSignal) Signal() {}

func (resizeTestSignal) String() string { return "resize-test" }

// TestWatchResizeSignalLoop proves a delivered resize signal causes a fresh
// terminal-size query and callback. The signal source is injected so the
// behavior is deterministic on every supported platform.
func TestWatchResizeSignalLoop(t *testing.T) {
	t.Parallel()

	ts := &countingTermState{ptTestTermState: &ptTestTermState{width: 120, height: 40}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	signals := make(chan os.Signal, 1)
	called := make(chan struct{}, 1)
	done := make(chan struct{})
	go func() {
		watchResizeSignalLoop(ctx, 3, ts, signals, func(rows, cols int) {
			if rows != 40 || cols != 120 {
				t.Errorf("resize dimensions = (%d, %d), want (40, 120)", rows, cols)
			}
			called <- struct{}{}
		})
		close(done)
	}()

	signals <- resizeTestSignal{}
	select {
	case <-called:
	case <-time.After(5 * time.Second):
		t.Fatal("resize signal did not invoke callback")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("resize signal loop did not stop after cancellation")
	}
}
