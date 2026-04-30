package scripting

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/require"
	goeventloop "github.com/joeycumines/go-eventloop"
	gojaEventloop "github.com/joeycumines/goja-eventloop"
	"github.com/joeycumines/goroutineid"
)

// Runtime wraps a goja.Runtime with an integrated event loop and module registry.
// It provides thread-safe execution of JavaScript by running all JS code
// on a single dedicated event-loop goroutine.
type Runtime struct {
	loop    *goeventloop.Loop
	adapter *gojaEventloop.Adapter
	vm      *goja.Runtime

	// registry is the CommonJS require registry for native modules.
	registry *require.Registry

	// timeout is the maximum duration to wait for RunOnLoopSync operations.
	// Default is defaultSyncTimeout. Set to 0 to disable timeout (not recommended).
	timeout time.Duration

	// eventLoopGoroutineID is captured at initialization for deadlock prevention.
	// Parsing goroutine ID from runtime.Stack() happens ONCE at startup.
	eventLoopGoroutineID atomic.Int64

	// loopCancel cancels the context passed to loop.Run()
	loopCancel context.CancelFunc

	// done is closed when the event loop returns from Run()
	done chan struct{}

	// bootstrapDone is closed when natural auto-exit is allowed to proceed.
	// We hold a Promisify token until this is closed to prevent premature shutdown.
	bootstrapDone chan struct{}

	// mu protects started/stopped state
	mu      sync.RWMutex
	started bool
	stopped bool

	// ctx is the lifecycle context for Done() channel
	ctx    context.Context
	cancel context.CancelFunc
}

// defaultSyncTimeout is the maximum duration to wait for RunOnLoopSync operations.
const defaultSyncTimeout = 5 * time.Second

// NewRuntime creates a new Runtime with an initialized event loop.
// The event loop is automatically started and runs in a background goroutine.
// Call Close() when done to clean up resources.
//
// The provided context controls lifecycle - when canceled, the runtime stops.
func NewRuntime(ctx context.Context) (*Runtime, error) {
	return NewRuntimeRegistry(ctx, nil)
}

// NewRuntimeRegistry creates a new Runtime with an existing require.Registry.
// If registry is nil, a new one is created.
// This allows sharing module registrations across multiple components.
func NewRuntimeRegistry(ctx context.Context, registry *require.Registry) (*Runtime, error) {
	if registry == nil {
		registry = require.NewRegistry()
	}

	// Create the Go event loop.
	// WithStrictMicrotaskOrdering ensures Promise .then() callbacks
	// (microtasks) are drained after EVERY macrotask, matching standard
	// JavaScript event-loop semantics.
	//
	// WithAutoExit(true) allows the loop to exit naturally when no tasks,
	// timers, or Promisify tokens remain. This is the primary shutdown
	// signal for the application.
	loop, err := goeventloop.New(
		goeventloop.WithStrictMicrotaskOrdering(true),
		goeventloop.WithAutoExit(true),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create event loop: %w", err)
	}

	vm := goja.New()
	registry.Enable(vm)

	loopCtx, loopCancel := context.WithCancel(context.Background())

	// Create internal lifecycle context
	childCtx, cancel := context.WithCancel(context.Background())

	rt := &Runtime{
		loop:          loop,
		vm:            vm,
		registry:      registry,
		ctx:           childCtx,
		cancel:        cancel,
		loopCancel:    loopCancel,
		done:          make(chan struct{}),
		bootstrapDone: make(chan struct{}),
		timeout:       defaultSyncTimeout,
	}

	// Use Promisify to keep the loop alive until natural exit is requested.
	// This prevents the loop from auto-exiting during the registration phase.
	loop.Promisify(context.Background(), func(ctx context.Context) (any, error) {
		<-rt.bootstrapDone
		return nil, nil
	})

	// Create goja adapter and bind JS globals (setTimeout, Promise, etc.)
	// This must happen on the event loop goroutine.
	// We submit this work BEFORE starting the loop goroutine to avoid the
	// auto-exit race where an empty loop exits immediately.
	errCh := make(chan error, 1)
	err = loop.Submit(func() {
		// Capture event loop goroutine ID for deadlock prevention
		rt.eventLoopGoroutineID.Store(goroutineid.Get())

		var bindErr error
		rt.adapter, bindErr = gojaEventloop.New(loop, vm)
		if bindErr != nil {
			errCh <- fmt.Errorf("failed to create goja adapter: %w", bindErr)
			return
		}

		if bindErr = rt.adapter.Bind(); bindErr != nil {
			errCh <- fmt.Errorf("failed to bind JS globals: %w", bindErr)
			return
		}

		errCh <- nil
	})
	if err != nil {
		close(rt.bootstrapDone)
		loopCancel()
		return nil, fmt.Errorf("failed to submit initialization task: %w", err)
	}

	// Start the event loop in background goroutine
	go func() {
		defer close(rt.done)
		// Run loop on its own goroutine
		if err := rt.loop.Run(loopCtx); err != nil && err != context.Canceled {
			// Report unexpected loop exit
			log.Printf("ERROR: eventloop: loop terminated unexpectedly: %v", err)
		}
	}()

	rt.mu.Lock()
	rt.started = true
	rt.mu.Unlock()

	// Wait for the initialization task to complete
	if initErr := <-errCh; initErr != nil {
		_ = rt.Close()
		return nil, initErr
	}

	// Handle external context cancellation
	if ctx.Done() != nil {
		context.AfterFunc(ctx, func() {
			_ = rt.Close()
		})
	}

	return rt, nil
}

// Close gracefully shuts down the runtime and event loop.
// It cancels the loop context and waits for the loop goroutine to return.
func (rt *Runtime) Close() error {
	rt.mu.Lock()
	if rt.stopped {
		rt.mu.Unlock()
		return nil
	}
	rt.stopped = true
	rt.mu.Unlock()

	// Release the bootstrap token if not already released
	select {
	case <-rt.bootstrapDone:
	default:
		close(rt.bootstrapDone)
	}

	// Cancel the lifecycle context
	rt.cancel()

	// Stop the event loop
	if rt.loopCancel != nil {
		rt.loopCancel()
	}

	// Wait for the loop goroutine to exit
	if rt.done != nil {
		<-rt.done
	}

	return nil
}

// Wait blocks until the event loop naturally exits (via auto-exit or cancellation).
// It releases the bootstrap token to allow natural auto-exit to proceed.
func (rt *Runtime) Wait() {
	rt.mu.Lock()
	select {
	case <-rt.bootstrapDone:
	default:
		close(rt.bootstrapDone)
	}
	rt.mu.Unlock()
	<-rt.done
}

// Done returns a channel that is closed when the runtime is stopped.
func (rt *Runtime) Done() <-chan struct{} {
	return rt.ctx.Done()
}

// Loop returns the underlying Go event loop.
func (rt *Runtime) Loop() *goeventloop.Loop {
	return rt.loop
}

// Runtime returns the underlying goja.Runtime.
func (rt *Runtime) Runtime() *goja.Runtime {
	return rt.vm
}

// VM returns the underlying goja.Runtime.
func (rt *Runtime) VM() *goja.Runtime {
	return rt.vm
}

// Registry returns the require.Registry for native modules.
func (rt *Runtime) Registry() *require.Registry {
	return rt.registry
}

// Adapter returns the goja-eventloop adapter.
func (rt *Runtime) Adapter() *gojaEventloop.Adapter {
	return rt.adapter
}

// Promisify implements EventLoopProvider. It wraps a Go function in a
// Promise-like lifecycle that keeps the event loop alive until completion.
func (rt *Runtime) Promisify(ctx context.Context, fn func(ctx context.Context) (any, error)) goeventloop.Promise {
	return rt.loop.Promisify(ctx, fn)
}

// IsRunning returns true if the runtime is running (started and not stopped).
func (rt *Runtime) IsRunning() bool {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	return rt.started && !rt.stopped
}

// SetTimeout sets the timeout for RunOnLoopSync operations.
func (rt *Runtime) SetTimeout(timeout time.Duration) {
	rt.mu.Lock()
	rt.timeout = timeout
	rt.mu.Unlock()
}

// GetTimeout returns the current timeout duration.
func (rt *Runtime) GetTimeout() time.Duration {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	return rt.timeout
}

// RunOnLoop schedules a function to run on the event loop goroutine.
// Returns true if the function was successfully scheduled.
func (rt *Runtime) RunOnLoop(fn func(vm *goja.Runtime)) bool {
	rt.mu.RLock()
	if !rt.started || rt.stopped {
		rt.mu.RUnlock()
		return false
	}
	rt.mu.RUnlock()

	err := rt.loop.Submit(func() {
		fn(rt.vm)
	})
	return err == nil
}

// RunOnLoopSync schedules a function on the event loop and waits for completion.
// Returns an error if the event loop is not running or stops while waiting.
func (rt *Runtime) RunOnLoopSync(fn func(vm *goja.Runtime) error) error {
	rt.mu.RLock()
	if !rt.started || rt.stopped {
		rt.mu.RUnlock()
		return errors.New("event loop not running")
	}
	timeout := rt.timeout
	rt.mu.RUnlock()

	done := make(chan struct{})
	var resErr error
	err := rt.loop.Submit(func() {
		defer close(done)
		resErr = fn(rt.vm)
	})
	if err != nil {
		return errors.New("event loop not running")
	}

	if timeout > 0 {
		timer := time.NewTimer(timeout)
		defer timer.Stop()
		select {
		case <-done:
			return resErr
		case <-rt.ctx.Done():
			return errors.New("runtime stopped while waiting for synchronous task")
		case <-timer.C:
			return fmt.Errorf("operation timed out after %v", timeout)
		}
	}

	select {
	case <-done:
		return resErr
	case <-rt.ctx.Done():
		return errors.New("runtime stopped while waiting for synchronous task")
	}
}

// TryRunOnLoopSync attempts to run a function on the event loop synchronously.
// If we're already on the event loop goroutine, the function is executed
// directly to avoid deadlock. Otherwise, it posts to the loop and waits.
func (rt *Runtime) TryRunOnLoopSync(currentVM *goja.Runtime, fn func(vm *goja.Runtime) error) error {
	rt.mu.RLock()
	if !rt.started || rt.stopped {
		rt.mu.RUnlock()
		return errors.New("event loop not running")
	}
	rt.mu.RUnlock()

	// Capture current goroutine ID
	currentID := goroutineid.Get()
	loopID := rt.eventLoopGoroutineID.Load()

	if currentID == loopID {
		// We are already on the loop thread, run directly
		return fn(currentVM)
	}

	// Different thread, use synchronous submission
	return rt.RunOnLoopSync(fn)
}

// LoadScript loads and executes JavaScript code in the runtime.
func (rt *Runtime) LoadScript(name, code string) error {
	return rt.RunOnLoopSync(func(vm *goja.Runtime) error {
		prg, err := goja.Compile(name, code, true)
		if err != nil {
			return fmt.Errorf("failed to compile %s: %w", name, err)
		}
		_, err = vm.RunProgram(prg)
		if err != nil {
			return fmt.Errorf("failed to run %s: %w", name, err)
		}
		return nil
	})
}

// SetGlobal sets a global variable in the JavaScript runtime.
func (rt *Runtime) SetGlobal(name string, value any) error {
	return rt.RunOnLoopSync(func(vm *goja.Runtime) error {
		return vm.Set(name, value)
	})
}

// GetGlobal retrieves a global variable from the JavaScript runtime.
func (rt *Runtime) GetGlobal(name string) (any, error) {
	var result any
	err := rt.RunOnLoopSync(func(vm *goja.Runtime) error {
		val := vm.Get(name)
		if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
			result = nil
			return nil
		}
		result = val.Export()
		return nil
	})
	return result, err
}

// GetCallable retrieves a global function from the JavaScript runtime.
func (rt *Runtime) GetCallable(name string) (goja.Callable, error) {
	var result goja.Callable
	err := rt.TryRunOnLoopSync(nil, func(vm *goja.Runtime) error {
		val := vm.Get(name)
		if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
			return nil
		}
		fn, ok := goja.AssertFunction(val)
		if !ok {
			return nil
		}
		result = fn
		return nil
	})
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, fmt.Errorf("function '%s' not found or not callable", name)
	}
	return result, nil
}
