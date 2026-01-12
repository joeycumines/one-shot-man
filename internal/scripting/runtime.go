package scripting

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
	"github.com/dop251/goja_nodejs/require"
	"github.com/joeycumines/one-shot-man/internal/goroutineid"
)

// Runtime provides a shared goja runtime and event loop for JavaScript execution.
// It is the single source of truth for all goja.Runtime access across the application,
// ensuring thread-safe execution by routing all operations through the event loop.
//
// Key Design Principles:
//   - goja.Runtime is NOT goroutine-safe; all access MUST happen via RunOnLoop
//   - The event loop is shared between scripting.Engine and bt.Bridge
//   - Lifecycle: event loop starts before any module registration, stops last
//   - Promise resolve/reject MUST happen on the event loop goroutine
//
// Usage:
//
//	rt, err := NewRuntime(ctx)
//	if err != nil { ... }
//	defer rt.Close()
//
//	// All goja operations must use RunOnLoop or RunOnLoopSync
//	err = rt.RunOnLoopSync(func(vm *goja.Runtime) error {
//	    _, err := vm.RunString("console.log('hello')")
//	    return err
//	})
type Runtime struct {
	// loop is the goja_nodejs event loop that serializes all JS execution.
	loop *eventloop.EventLoop

	// registry is the CommonJS require registry for native modules.
	registry *require.Registry

	// timeout is the maximum duration to wait for RunOnLoopSync operations.
	// Default is DefaultSyncTimeout. Set to 0 to disable timeout (not recommended).
	timeout time.Duration

	// eventLoopGoroutineID is captured at initialization for deadlock prevention.
	// Parsing goroutine ID from runtime.Stack() happens ONCE at startup.
	eventLoopGoroutineID atomic.Int64

	// mu protects started/stopped state
	mu      sync.RWMutex
	started bool
	stopped bool

	// ctx is the lifecycle context for Done() channel
	ctx    context.Context
	cancel context.CancelFunc
}

// DefaultSyncTimeout is the maximum duration to wait for RunOnLoopSync operations.
const DefaultSyncTimeout = 5 * time.Second

// NewRuntime creates a new Runtime with an initialized event loop.
// The event loop is automatically started and runs in a background goroutine.
// Call Close() when done to clean up resources.
//
// The provided context controls lifecycle - when canceled, the runtime stops.
func NewRuntime(ctx context.Context) (*Runtime, error) {
	return NewRuntimeWithRegistry(ctx, nil)
}

// NewRuntimeWithRegistry creates a new Runtime with an existing require.Registry.
// If registry is nil, a new one is created.
// This allows sharing module registrations across multiple components.
func NewRuntimeWithRegistry(ctx context.Context, registry *require.Registry) (*Runtime, error) {
	if registry == nil {
		registry = require.NewRegistry()
	}

	loop := eventloop.NewEventLoop(
		eventloop.WithRegistry(registry),
		eventloop.EnableConsole(true),
	)

	// Create internal lifecycle context (independent of parent for clean shutdown)
	childCtx, cancel := context.WithCancel(context.Background())

	rt := &Runtime{
		loop:     loop,
		registry: registry,
		ctx:      childCtx,
		cancel:   cancel,
		timeout:  DefaultSyncTimeout,
	}

	// Start the event loop
	loop.Start()
	rt.mu.Lock()
	rt.started = true
	rt.mu.Unlock()

	// Capture the event loop goroutine ID for deadlock prevention
	errCh := make(chan error, 1)
	ok := loop.RunOnLoop(func(vm *goja.Runtime) {
		id := getGoroutineID()
		rt.eventLoopGoroutineID.Store(id)
		errCh <- nil
	})
	if !ok {
		cancel()
		return nil, errors.New("failed to initialize: event loop not running")
	}

	if err := <-errCh; err != nil {
		cancel()
		loop.Stop()
		return nil, fmt.Errorf("failed to initialize runtime: %w", err)
	}

	// Handle external context cancellation
	if ctx.Done() != nil {
		context.AfterFunc(ctx, func() {
			rt.Close()
		})
	}

	return rt, nil
}

// Registry returns the require.Registry for module registration.
// Modules can be registered before or after the runtime is created,
// but must be registered before any script that uses them is executed.
func (rt *Runtime) Registry() *require.Registry {
	return rt.registry
}

// EventLoop returns the underlying event loop for advanced use cases.
// WARNING: Direct use of the event loop bypasses Runtime's lifecycle management.
// Prefer using RunOnLoop/RunOnLoopSync instead.
func (rt *Runtime) EventLoop() *eventloop.EventLoop {
	return rt.loop
}

// Close gracefully stops the event loop and releases resources.
// It's safe to call multiple times.
// After Close is called, Done() channel will be closed.
func (rt *Runtime) Close() error {
	rt.mu.Lock()
	if rt.stopped {
		rt.mu.Unlock()
		return nil
	}
	rt.stopped = true
	rt.mu.Unlock()

	// Cancel the lifecycle context BEFORE stopping the loop
	// This ensures any goroutines waiting on Done() will unblock
	rt.cancel()

	// Stop the event loop (waits for pending jobs to complete)
	rt.loop.Stop()

	return nil
}

// Done returns a channel that is closed when the runtime is stopped.
// This is useful for select statements to detect runtime shutdown.
func (rt *Runtime) Done() <-chan struct{} {
	return rt.ctx.Done()
}

// IsRunning returns true if the runtime is running (started and not stopped).
func (rt *Runtime) IsRunning() bool {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	return rt.started && !rt.stopped
}

// SetTimeout sets the timeout for RunOnLoopSync operations.
// Pass 0 to disable timeout (not recommended for production).
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
// Returns false if the event loop is not running.
//
// IMPORTANT: All goja.Runtime operations must happen inside this callback.
// The goja.Runtime passed to the callback must not be used outside the callback.
func (rt *Runtime) RunOnLoop(fn func(*goja.Runtime)) bool {
	rt.mu.RLock()
	if !rt.started || rt.stopped {
		rt.mu.RUnlock()
		return false
	}
	rt.mu.RUnlock()

	return rt.loop.RunOnLoop(fn)
}

// RunOnLoopSync schedules a function on the event loop and waits for completion.
// Returns an error if the event loop is not running or stops while waiting.
// If configured, will timeout after the Runtime's timeout duration.
func (rt *Runtime) RunOnLoopSync(fn func(*goja.Runtime) error) error {
	rt.mu.RLock()
	if !rt.started || rt.stopped {
		rt.mu.RUnlock()
		return errors.New("event loop not running")
	}
	timeout := rt.timeout
	rt.mu.RUnlock()

	errCh := make(chan error, 1)
	ok := rt.loop.RunOnLoop(func(vm *goja.Runtime) {
		errCh <- fn(vm)
	})
	if !ok {
		return errors.New("event loop not running")
	}

	// Wait with timeout and cancellation support
	if timeout > 0 {
		timer := time.NewTimer(timeout)
		defer timer.Stop()
		select {
		case err := <-errCh:
			return err
		case <-rt.Done():
			return errors.New("runtime stopped before completion")
		case <-timer.C:
			return fmt.Errorf("operation timed out after %v", timeout)
		}
	}

	// No timeout - just wait with cancellation support
	select {
	case err := <-errCh:
		return err
	case <-rt.Done():
		return errors.New("runtime stopped before completion")
	}
}

// TryRunOnLoopSync attempts to run a function on the event loop synchronously.
// If we're already on the event loop goroutine (detected via goroutine ID),
// the function is executed directly to avoid deadlock. Otherwise, it posts
// to the loop and waits like RunOnLoopSync.
//
// This is CRITICAL for code that might be called from within the event loop itself,
// such as when JS nodes call back into Go which needs to execute more JS.
//
// The currentVM parameter is used for direct execution when already on the loop.
func (rt *Runtime) TryRunOnLoopSync(currentVM *goja.Runtime, fn func(*goja.Runtime) error) error {
	// Step 1: Runtime state check
	rt.mu.RLock()
	if !rt.started || rt.stopped {
		rt.mu.RUnlock()
		return errors.New("event loop not running")
	}
	rt.mu.RUnlock()

	// Step 2: Goroutine ID check (MANDATORY - no shortcuts)
	// We MUST check if we are on the event loop goroutine.
	eventLoopID := rt.eventLoopGoroutineID.Load()
	if eventLoopID > 0 {
		currentGoroutineID := goroutineid.Get()

		if currentGoroutineID == eventLoopID {
			// We are on the event loop. Safe to run directly.
			return fn(currentVM)
		}
	}

	// Step 3: Not on event loop - schedule and wait
	return rt.RunOnLoopSync(fn)
}

// LoadScript loads and executes JavaScript code in the runtime.
// Returns an error if the code fails to compile or execute.
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
// Returns nil if the variable doesn't exist.
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

// GetCallable retrieves a global function from the JavaScript runtime as a goja.Callable.
// Returns an error if the variable doesn't exist or is not callable.
func (rt *Runtime) GetCallable(name string) (goja.Callable, error) {
	var result goja.Callable
	err := rt.RunOnLoopSync(func(vm *goja.Runtime) error {
		val := vm.Get(name)
		if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
			return fmt.Errorf("function '%s' not found", name)
		}
		fn, ok := goja.AssertFunction(val)
		if !ok {
			return fmt.Errorf("'%s' is not a callable function", name)
		}
		result = fn
		return nil
	})
	return result, err
}

// getGoroutineID captures the current goroutine ID using the shared utility.
func getGoroutineID() int64 {
	return goroutineid.Get()
}
