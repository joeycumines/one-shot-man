package eventlooputil

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goroutineid"
)

func newTestLoop(t *testing.T) (*goeventloop.Loop, func()) {
	t.Helper()
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("create loop: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = loop.Run(ctx)
	}()
	return loop, func() {
		cancel()
		<-done
	}
}

// TestRunner_TrySync_InlineWhenOnLoop reproduces the fork-from-loop scenario:
// calling TrySync from the loop goroutine must execute inline on the SAME
// goroutine without scheduling — completing in well under a millisecond.
func TestRunner_TrySync_InlineWhenOnLoop(t *testing.T) {
	loop, stop := newTestLoop(t)
	defer stop()

	var loopGoroutineID atomic.Int64
	initDone := make(chan struct{})
	if err := loop.Submit(func() {
		loopGoroutineID.Store(goroutineid.Get())
		close(initDone)
	}); err != nil {
		t.Fatalf("submit id capture: %v", err)
	}
	<-initDone

	r, err := NewRunner(RunnerConfig{
		Loop:          loop,
		OnLoopThread:  func() bool { return IsLoopThread(loopGoroutineID.Load()) },
		NotRunningErr: errors.New("event loop not running"),
	})
	if err != nil {
		t.Fatalf("new runner: %v", err)
	}

	err = loop.Submit(func() {
		start := time.Now()
		execGoroutine := int64(0)
		fnErr := r.TrySync(func() error {
			execGoroutine = goroutineid.Get()
			return nil
		}, 0)
		elapsed := time.Since(start)
		if fnErr != nil {
			t.Errorf("TrySync inline returned error: %v", fnErr)
		}
		if execGoroutine != loopGoroutineID.Load() {
			t.Errorf("TrySync forked off loop thread: ran on %d, loop is %d", execGoroutine, loopGoroutineID.Load())
		}
		if elapsed > time.Millisecond {
			t.Errorf("inline TrySync took %v, want sub-millisecond", elapsed)
		}
	})
	if err != nil {
		t.Fatalf("submit repro: %v", err)
	}
}

// TestRunner_TrySync_OffLoopSchedules verifies the non-inline branch routes
// through Submit and settles with the function's error.
func TestRunner_TrySync_OffLoopSchedules(t *testing.T) {
	loop, stop := newTestLoop(t)
	defer stop()

	var loopGoroutineID atomic.Int64
	_ = loop.Submit(func() { loopGoroutineID.Store(goroutineid.Get()) })

	r, err := NewRunner(RunnerConfig{
		Loop:          loop,
		OnLoopThread:  func() bool { return IsLoopThread(loopGoroutineID.Load()) },
		NotRunningErr: errors.New("event loop not running"),
	})
	if err != nil {
		t.Fatalf("new runner: %v", err)
	}

	want := errors.New("propagated")
	if got := r.TrySync(func() error { return want }, 0); !errors.Is(got, want) {
		t.Fatalf("TrySync off-loop = %v, want %v", got, want)
	}
}

func TestRunner_Sync_SettlesAndPropagates(t *testing.T) {
	loop, stop := newTestLoop(t)
	defer stop()

	r, err := NewRunner(RunnerConfig{Loop: loop})
	if err != nil {
		t.Fatalf("new runner: %v", err)
	}
	if got := r.Sync(func() error { return nil }, 0); got != nil {
		t.Fatalf("Sync success = %v, want nil", got)
	}
	want := errors.New("boom")
	if got := r.Sync(func() error { return want }, time.Second); !errors.Is(got, want) {
		t.Fatalf("Sync error propagation = %v, want %v", got, want)
	}
}

func TestRunner_Sync_Timeout(t *testing.T) {
	loop, stop := newTestLoop(t)
	defer stop()

	custom := func(d time.Duration) error { return errors.New("custom timeout") }
	r, err := NewRunner(RunnerConfig{Loop: loop, TimeoutErr: custom})
	if err != nil {
		t.Fatalf("new runner: %v", err)
	}
	got := r.Sync(func() error {
		time.Sleep(250 * time.Millisecond)
		return nil
	}, 10*time.Millisecond)
	if got == nil || got.Error() != "custom timeout" {
		t.Fatalf("Sync timeout = %v, want custom timeout error", got)
	}

	def, err := NewRunner(RunnerConfig{Loop: loop})
	if err != nil {
		t.Fatalf("new default runner: %v", err)
	}
	got = def.Sync(func() error {
		time.Sleep(250 * time.Millisecond)
		return nil
	}, 10*time.Millisecond)
	if got == nil || got.Error() != "operation timed out after 10ms" {
		t.Fatalf("default timeout message = %v", got)
	}
}

func TestRunner_Sync_NotRunning(t *testing.T) {
	notRunning := errors.New("event loop not running")
	r, err := NewRunner(RunnerConfig{
		Loop:          failingSubmitter{},
		NotRunningErr: notRunning,
	})
	if err != nil {
		t.Fatalf("new runner: %v", err)
	}
	if got := r.Sync(func() error { return nil }, 0); !errors.Is(got, notRunning) {
		t.Fatalf("Sync with failed submit = %v, want %v", got, notRunning)
	}
}

func TestRunner_Sync_StoppedWatch(t *testing.T) {
	loop, stop := newTestLoop(t)

	done := make(chan struct{})
	stopped := errors.New("owner stopped")
	r, err := NewRunner(RunnerConfig{Loop: loop, Done: done, StoppedErr: stopped})
	if err != nil {
		t.Fatalf("new runner: %v", err)
	}

	result := make(chan error, 1)
	release := make(chan struct{})
	go func() {
		result <- r.Sync(func() error {
			<-release
			return nil
		}, 0)
	}()

	time.Sleep(20 * time.Millisecond)
	close(done)
	close(release)
	stop()

	select {
	case err := <-result:
		if !errors.Is(err, stopped) {
			t.Fatalf("Sync after stop = %v, want %v", err, stopped)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Sync did not observe owner Done")
	}
}

func TestNewRunner_NilLoop(t *testing.T) {
	if _, err := NewRunner(RunnerConfig{}); err == nil {
		t.Fatal("NewRunner with nil Submitter should fail")
	}
}

type failingSubmitter struct{}

func (failingSubmitter) Submit(fn func()) error { return errors.New("closed") }

func TestRunner_Go_SchedulingFailureOnly(t *testing.T) {
	notRunning := errors.New("event loop not running")
	r, err := NewRunner(RunnerConfig{Loop: failingSubmitter{}, NotRunningErr: notRunning})
	if err != nil {
		t.Fatalf("new runner: %v", err)
	}
	if got := r.Go(func() error { return errors.New("unobserved") }); !errors.Is(got, notRunning) {
		t.Fatalf("Go scheduling failure = %v, want %v", got, notRunning)
	}

	loop, stop := newTestLoop(t)
	defer stop()
	ok, err := NewRunner(RunnerConfig{Loop: loop})
	if err != nil {
		t.Fatalf("new runner: %v", err)
	}
	if err := ok.Go(func() error { return errors.New("settlement unobserved by design") }); err != nil {
		t.Fatalf("Go on live loop = %v, want nil", err)
	}
}
