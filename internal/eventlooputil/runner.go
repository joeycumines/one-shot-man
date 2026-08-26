package eventlooputil

import (
	"fmt"
	"time"
)

// Submitter schedules callbacks onto an event loop.
type Submitter interface {
	Submit(fn func()) error
}

// Runner centralizes the submit-and-wait discipline shared by every
// goja-runtime facade (scripting Runtime, bt Bridge, scripting Engine):
// schedule a closure on the loop goroutine, wait for settlement under an
// optional timeout and lifecycle watch, and execute inline when already on
// the loop thread so nested calls cannot deadlock.
type Runner struct {
	loop    Submitter
	onLoop  func() bool
	done    <-chan struct{}
	cfgFail error
	stopped error
	timeErr func(time.Duration) error
}

// RunnerConfig wires a Runner to a specific owner's lifecycle.
type RunnerConfig struct {
	Loop          Submitter
	OnLoopThread  func() bool
	Done          <-chan struct{}
	NotRunningErr error
	StoppedErr    error
	// TimeoutErr renders the timeout failure; nil selects the canonical
	// "operation timed out after %v" form.
	TimeoutErr func(time.Duration) error
}

// NewRunner validates the loop handle and returns a ready Runner.
func NewRunner(cfg RunnerConfig) (*Runner, error) {
	if cfg.Loop == nil {
		return nil, fmt.Errorf("eventlooputil: runner requires a non-nil Submitter")
	}
	return &Runner{
		loop:    cfg.Loop,
		onLoop:  cfg.OnLoopThread,
		done:    cfg.Done,
		cfgFail: cfg.NotRunningErr,
		stopped: cfg.StoppedErr,
		timeErr: cfg.TimeoutErr,
	}, nil
}

// TrySync runs fn inline when already on the loop thread, otherwise Sync.
func (r *Runner) TrySync(fn func() error, timeout time.Duration) error {
	return r.TrySyncBranch(fn, fn, timeout)
}

// Go schedules fire-and-forget work, reporting scheduling failure only;
// settlement of fn is not observed.
func (r *Runner) Go(fn func() error) error {
	if err := r.loop.Submit(func() { _ = fn() }); err != nil {
		if r.cfgFail != nil {
			return r.cfgFail
		}
		return err
	}
	return nil
}

// TrySyncBranch distinguishes inline from scheduled execution: some owners
// run inline against a caller-provided handle but must schedule onto their
// own canonical handle (e.g. Bridge.TryRunSync's currentVM vs b.vm).
func (r *Runner) TrySyncBranch(inline, scheduled func() error, timeout time.Duration) error {
	if r.onLoop != nil && r.onLoop() {
		return inline()
	}
	return r.Sync(scheduled, timeout)
}

// Sync schedules fn on the loop goroutine and blocks until it settles, the
// owner's Done channel fires, or the timeout elapses (timeout > 0 only).
func (r *Runner) Sync(fn func() error, timeout time.Duration) error {
	errCh := make(chan error, 1)
	if err := r.loop.Submit(func() { errCh <- fn() }); err != nil {
		if r.cfgFail != nil {
			return r.cfgFail
		}
		return err
	}
	if timeout > 0 {
		timer := time.NewTimer(timeout)
		defer timer.Stop()
		select {
		case err := <-errCh:
			return err
		case <-r.doneOrNothing():
			return r.stopped
		case <-timer.C:
			return r.timeoutFailure(timeout)
		}
	}
	select {
	case err := <-errCh:
		return err
	case <-r.doneOrNothing():
		return r.stopped
	}
}

func (r *Runner) doneOrNothing() <-chan struct{} {
	if r.done == nil {
		return nil
	}
	return r.done
}

func (r *Runner) timeoutFailure(timeout time.Duration) error {
	if r.timeErr != nil {
		return r.timeErr(timeout)
	}
	return fmt.Errorf("operation timed out after %v", timeout)
}
