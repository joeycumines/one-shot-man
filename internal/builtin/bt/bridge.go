package bt

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	bt "github.com/joeycumines/go-behaviortree"
	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
	gojaeventloop "github.com/joeycumines/goja-eventloop"
	"github.com/joeycumines/goja_nodejs/require"
	"github.com/joeycumines/goroutineid"
	"github.com/joeycumines/one-shot-man/internal/eventlooputil"
)

// Bridge manages the behavior tree integration between Go and JavaScript.
// It provides a safe interface for Go code to interact with JavaScript, ensuring
// all JavaScript operations happen on the event loop goroutine.
//
// Key Constraints:
//   - goja.Runtime is NOT goroutine-safe; all access must happen via Run
//   - Promise resolve/reject functions must be called on the event loop goroutine
//   - The event loop must be started before any JavaScript operations
//
// The Bridge uses an external event loop. The caller is responsible for
// starting and stopping the event loop. The Bridge's Stop() method only stops
// the internal bt.Manager, not the event loop.
type Bridge struct {
	// timeout is the maximum duration to wait for RunSync operations.
	// Default is 5 seconds. Set to 0 to disable timeout (not recommended for production).
	timeout time.Duration
	loop    *goeventloop.Loop
	vm      *goja.Runtime
	adapter *gojaeventloop.Adapter

	// Event loop goroutine ID for deadlock prevention.
	// We extract the goroutine ID from runtime.Stack() during initialization.
	// This parsing happens ONCE at startup. The format "goroutine X" has been
	// stable since Go 1.5, making this a portable solution.
	eventLoopGoroutineID atomic.Int64

	mu      sync.RWMutex
	started bool
	stopped bool

	// Lifecycle context for Done() channel
	ctx    context.Context
	cancel context.CancelFunc

	// manager aggregates all Tickers created via newTicker.
	// It is stopped when the Bridge is stopped.
	manager bt.Manager

	// stopParentCtx keeps the context.AfterFunc stop handle alive
	// to prevent GC from collecting it before parent context cancellation.
	stopParentCtx func() bool

	// loopRunner is the shared submit-and-wait substrate, built during New.
	loopRunner *eventlooputil.Runner
}

// DefaultTimeout is the maximum duration to wait for RunSync operations.
const DefaultTimeout = 5 * time.Second

// NewBridge creates a Bridge that uses an external event loop.
// The event loop must be started and managed by the caller.
func NewBridge(ctx context.Context, loop *goeventloop.Loop, vm *goja.Runtime, registry *require.Registry, adapter *gojaeventloop.Adapter) *Bridge {
	if loop == nil {
		panic("event loop must not be nil")
	}
	if vm == nil {
		panic("goja runtime must not be nil")
	}
	if adapter == nil {
		panic("goja-eventloop adapter must not be nil")
	}
	b := newBridgeWithLoop(ctx, loop, vm, registry)
	b.adapter = adapter
	return b
}

// newBridgeWithLoop is the internal constructor for Bridge.
func newBridgeWithLoop(ctx context.Context, loop *goeventloop.Loop, vm *goja.Runtime, reg *require.Registry) *Bridge {
	// NOTE ON CONTEXT DERIVATION (addressing CRIT-2 from review-1.md):
	// Bridge's internal lifecycle context (childCtx) is NOT derived from parent ctx.
	// This is intentional to maintain the critical invariant:
	//
	//   INVARIANT: Once Done() is closed, IsRunning() MUST return false
	//
	// If childCtx were derived from parent (via context.WithCancel(ctx)), when parent
	// cancels, Go's context cascade would close childCtx.Done() BEFORE the AfterFunc
	// goroutine runs to set b.stopped=true. This creates a race where Done() is closed
	// but IsRunning() still returns true, violating the invariant.
	//
	// The correct approach is:
	// 1. childCtx is independent (from Background) for bridge lifecycle
	// 2. Parent cancellation triggers AfterFunc → Stop()
	// 3. Stop() sets b.stopped=true FIRST, then closes childCtx
	// 4. This ensures atomicity: stopped flag and Done() closure are synchronized
	//
	// This is a necessary design choice for lifecycle components requiring strict
	// state-channel consistency, not a bug.
	childCtx, cancel := context.WithCancel(context.Background())

	b := &Bridge{
		loop:    loop,
		vm:      vm,
		ctx:     childCtx,
		cancel:  cancel,
		timeout: DefaultTimeout,
		manager: bt.NewManager(),
	}

	// Mark as started (event loop should already be running)
	b.mu.Lock()
	b.started = true
	b.mu.Unlock()

	// Initialize the VM within the event loop BEFORE registering the module.
	// This ensures the event loop goroutine ID is captured BEFORE any script
	// can require the module. Otherwise, immediate require would call
	// TryRunSync before ID capture, causing deadlock when already on loop.
	//
	// The happens-before guarantee:
	// 1. initializeJS runs on event loop -> captures goroutine ID (atomic.Store)
	// 2. Then RegisterNativeModule publishes module availability
	// 3. Any subsequent require sees published module AND captured ID

	// Initialize the VM within the event loop FIRST
	initRunner, runnerErr := eventlooputil.NewRunner(eventlooputil.RunnerConfig{
		Loop:          loop,
		OnLoopThread:  func() bool { return eventlooputil.IsLoopThread(b.eventLoopGoroutineID.Load()) },
		Done:          b.ctx.Done(),
		NotRunningErr: errors.New("event loop not running"),
		StoppedErr:    errors.New("bridge stopped before completion"),
		TimeoutErr: func(d time.Duration) error {
			return fmt.Errorf("operation timed out after %v (consider increasing timeout or checking for infinite loops in JS code)", d)
		},
	})
	if runnerErr != nil {
		cancel()
		b.manager.Stop()
		panic(runnerErr)
	}
	b.loopRunner = initRunner
	if err := initRunner.Sync(func() error {
		return b.initializeJS()
	}, 0); err != nil {
		cancel()
		b.manager.Stop()
		panic(fmt.Sprintf("failed to initialize JavaScript environment: %v", err))
	}

	// NOW register the osm:bt module (after ID is captured)
	if reg != nil {
		// Module loader uses bridge's internal lifecycle context (childCtx),
		// NOT the external parent context parameter, to ensure module lifecycle
		// matches bridge's lifecycle logic.
		reg.RegisterNativeModule("osm:bt", b.ModuleLoader(childCtx))
	}

	// CRITICAL: External parent context cancellation handling
	// When parent ctx is cancelled, bridge should shut down cleanly.
	// We use AfterFunc to trigger Stop(), which ensures proper ordering:
	//   1. Stop() sets b.stopped=true (under mutex)
	//   2. Stop() cancels childCtx (closes Done() channel)
	// This maintains invariant: Done() closed ⇒ IsRunning() = false
	if ctx.Done() != nil {
		b.stopParentCtx = context.AfterFunc(ctx, func() {
			b.Stop()
		})
	}

	return b
}

// initializeJS sets up the JavaScript environment with behavior tree helpers.
func (b *Bridge) initializeJS() error {
	// Capture event loop goroutine ID. We extract the goroutine ID from the
	// stack trace. This parsing happens ONCE at initialization, so the overhead
	// is acceptable.
	b.eventLoopGoroutineID.Store(goroutineid.Get())

	// Set up the runLeaf helper which bridges async JS functions to callbacks
	// Note: The status strings in jsHelpers MUST match JSStatusRunning, JSStatusSuccess, JSStatusFailure
	_, err := b.vm.RunString(jsHelpers)
	return err
}

// jsHelpers contains the JavaScript helper code for the bridge.
// IMPORTANT: Status strings here MUST match the JSStatus* constants in adapter.go
const jsHelpers = `
// runLeaf executes a JS leaf function and calls the callback with the result.
// This bridges the JS world to the callback-based Go world.
//
// CRITICAL: This implementation calls the tick function SYNCHRONOUSLY.
// The goja_nodejs event loop only has a macrotask queue, NOT a microtask queue.
// Using Promise.resolve().then(...) would schedule microtasks that never run,
// causing the Go caller to block forever waiting for the callback.
//
// For async tick functions that return a Promise, we detect this and handle
// the Promise. But the Promise resolution still requires the event loop to
// process it, which only works if the event loop drains pending jobs.
globalThis.runLeaf = function(fn, ctx, args, callback) {
	try {
		var result = fn(ctx, args);
		// Check if result is a Promise (has a 'then' method)
		if (result && typeof result.then === 'function') {
			// Async path: result is a Promise, wait for it
			result.then(
				function(status) { callback(String(status), null); },
				function(err) { callback("failure", err instanceof Error ? err.message : String(err)); }
			);
		} else {
			// Sync path: result is immediate, call callback now
			callback(String(result), null);
		}
	} catch (err) {
		callback("failure", err instanceof Error ? err.message : String(err));
	}
};

// Status constants matching go-behaviortree (must match JSStatus* constants)
globalThis.bt = {
	running: "running",
	success: "success",
	failure: "failure"
};
`

// Stop gracefully stops the bridge and its resources.
// It's safe to call multiple times.
// After Stop is called, Done() channel will be closed.
//
// Stop only stops the internal bt.Manager (which stops all tickers).
// The event loop is managed externally by the caller.
//
// IMPORTANT: Stop does NOT wait for in-flight Run operations to complete.
// Operations that were already scheduled may still execute after Stop returns.
// Callers should not assume that no more work will happen after Stop returns.
//
// The shutdown sequence is:
//  1. Acquire lock; return early if already stopped.
//  2. Set stopped=true (so IsRunning() returns false from this point on).
//  3. Release lock.
//  4. Stop the internal bt.Manager — while the event loop is still alive, so
//     settled ticker promises can dispatch their callbacks. (bt.Manager.Stop
//     only closes its stop signal and joins its run loop; it does not block on
//     this bridge's context or the event loop, so this step cannot deadlock.)
//  5. Cancel the context — closes Done() and unblocks RunSync waiters.
//
// Ordering invariant: stopped is set true in step 2, strictly before Done()
// closes in step 5, so "Done() closed ⇒ IsRunning()==false" always holds.
func (b *Bridge) Stop() {
	b.mu.Lock()
	if b.stopped {
		b.mu.Unlock()
		return
	}

	// Set stopped=true BEFORE cancelling so that IsRunning() returns false
	// atomically with the state change. The Done() channel is closed
	// slightly later (after manager.Stop()), which is safe because
	// the lifecycle invariant only requires: "Once Done() is closed,
	// IsRunning() MUST return false" — setting stopped early satisfies this.
	b.stopped = true
	b.mu.Unlock()

	// Stop the internal bt.Manager FIRST (stops all tickers and settles
	// their done promises). This must happen while the event loop is
	// still running so promise callbacks can be dispatched.
	if b.manager != nil {
		b.manager.Stop()
	}

	// Then cancel the context to close Done() and stop the event loop.
	b.cancel()
}

// Manager returns the internal bt.Manager that aggregates all tickers.
// This can be used to monitor the aggregate state of all tickers.
//
// Note: Tickers created via newTicker are automatically registered with
// this manager. The manager is stopped when the Bridge is stopped.
func (b *Bridge) Manager() bt.Manager {
	return b.manager
}

// Done returns a channel that is closed when the bridge is stopped.
// This is useful for select statements to detect bridge shutdown.
func (b *Bridge) Done() <-chan struct{} {
	return b.ctx.Done()
}

// SetTimeout sets the timeout for RunSync operations.
// Pass 0 to disable timeout (not recommended for production).
func (b *Bridge) SetTimeout(timeout time.Duration) {
	b.mu.Lock()
	b.timeout = timeout
	b.mu.Unlock()
}

// GetTimeout returns the current timeout duration.
func (b *Bridge) GetTimeout() time.Duration {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.timeout
}

// IsRunning returns true if the bridge is running (started and not stopped).
func (b *Bridge) IsRunning() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.started && !b.stopped
}

// GetLifecycleSnapshot returns a snapshot of both lifecycle state atomicly.
// This is used by tests to verify the invariant: "If Done() is observed closed,
// IsRunning() MUST return false". By capturing both under the same lock, observers
// can check for violations without race windows.
func (b *Bridge) GetLifecycleSnapshot() (doneClosed bool, isRunning bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	// Check if Done channel is closed (non-blocking select under read lock)
	select {
	case <-b.ctx.Done():
		doneClosed = true
	default:
		doneClosed = false
	}

	// Read running state from same lock for atomic snapshot
	isRunning = b.started && !b.stopped
	return
}

// Run schedules a function to run on the event loop goroutine.
// Returns true if the function was successfully scheduled.
// Returns false if the event loop is not running.
//
// IMPORTANT: All goja.Runtime operations must happen inside this callback.
func (b *Bridge) Run(fn func(*goja.Runtime)) bool {
	b.mu.RLock()
	if !b.started || b.stopped {
		b.mu.RUnlock()
		return false
	}
	b.mu.RUnlock()

	vm := b.vm
	err := b.loop.Submit(func() {
		fn(vm)
	})
	return err == nil
}

// RunSync schedules a function on the event loop and waits for completion.
// Returns an error if the event loop is not running or stops while waiting.
// If configured, will timeout after the Bridge's timeout duration.
func (b *Bridge) RunSync(fn func(*goja.Runtime) error) error {
	b.mu.RLock()
	if !b.started || b.stopped {
		b.mu.RUnlock()
		return errors.New("event loop not running")
	}
	timeout := b.timeout
	b.mu.RUnlock()

	// The runner owns the on-loop fast path (deadlock prevention), the
	// submit, and the timeout/cancellation wait.
	vm := b.vm
	return b.loopRunner.TrySync(func() error {
		return fn(vm)
	}, timeout)
}

// RunJSSync implements bubbletea.JSRunner interface.
// This is an alias for RunSync, provided for interface compatibility.
// It schedules a function on the event loop and waits for completion.
func (b *Bridge) RunJSSync(fn func(*goja.Runtime) error) error {
	return b.RunSync(fn)
}

// LoadScript loads JavaScript code into the runtime.
// Returns an error if the code fails to compile or execute.
func (b *Bridge) LoadScript(name, code string) error {
	return b.RunSync(func(vm *goja.Runtime) error {
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
func (b *Bridge) SetGlobal(name string, value any) error {
	return b.RunSync(func(vm *goja.Runtime) error {
		return vm.Set(name, value)
	})
}

// GetGlobal retrieves a global variable from the JavaScript runtime.
// Returns the value and a boolean indicating if the variable exists.
// The boolean is true if the variable was found, false if it doesn't exist.
// Note: A variable can exist with a null/nil value, which returns (nil, true).
// This follows Go idiom consistency with map lookups.
func (b *Bridge) GetGlobal(name string) (any, bool) {
	var result any
	var exists bool
	err := b.RunSync(func(vm *goja.Runtime) error {
		val := vm.Get(name)
		if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
			// Check if the property actually exists on the global object
			// vm.Get returns nil for both nonexistent keys and keys with null value
			// We need to distinguish these cases
			// If val.ToValue() returns Undefined, it truly doesn't exist
			if goja.IsUndefined(val) {
				// Property doesn't exist
				exists = false
				result = nil
			} else if goja.IsNull(val) {
				// Property exists but is null
				exists = true
				result = nil
			}
			return nil
		}
		result = val.Export()
		exists = true
		return nil
	})
	if err != nil {
		return nil, false
	}
	return result, exists
}

// TryRunSync attempts to run a function on the event loop synchronously.
// If we're already on the event loop goroutine (detected via goroutine ID),
// the function is executed directly to avoid deadlock. Otherwise, it posts to the loop
// and waits like RunSync.
//
// This is CRITICAL for code that might be called from within the event loop itself,
// such as when JS nodes contain composites that call back into JS via tickUnwrap.
//
// IMPORTANT: currentVM parameter only used when already on event loop goroutine.
// From other goroutines, currentVM is ignored and function receives VM from event loop.
// If currentVM is nil and we're on event loop, fn(nil) will be called (caller must ensure non-nil).
//
// Behavior by calling context:
//   - On event loop goroutine: executes fn(currentVM) directly
//   - On other goroutine: schedules fn(loopVM) on event loop and waits
//
// We rely SOLELY on goroutine ID checking. This is required because
// closures capture VM references and can be called from background goroutines
// (e.g., Ticker goroutines), proving identity but NOT execution thread security.
func (b *Bridge) TryRunSync(currentVM *goja.Runtime, fn func(*goja.Runtime) error) error {
	// STEP 1: Bridge state check
	b.mu.RLock()
	if !b.started || b.stopped {
		b.mu.RUnlock()
		return errors.New("event loop not running")
	}
	b.mu.RUnlock()

	// STEP 2/3: The runner inlines when already on the event loop goroutine
	// (executing against the caller-provided currentVM) and otherwise
	// schedules onto the bridge's own VM via RunSync semantics.
	return b.loopRunner.TrySyncBranch(
		func() error { return fn(currentVM) },
		func() error { return fn(b.vm) },
		b.timeout,
	)
}

// GetCallable retrieves a global function from the JavaScript runtime as a goja.Callable.
// This is useful for getting JS functions to pass to NewJSLeafAdapter.
// Returns an error if the variable doesn't exist or is not callable.
func (b *Bridge) GetCallable(name string) (goja.Callable, error) {
	var result goja.Callable
	err := b.RunSync(func(vm *goja.Runtime) error {
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

// ExposeBlackboard exposes a Blackboard to JavaScript with the given name.
func (b *Bridge) ExposeBlackboard(name string, bb *Blackboard) error {
	return b.RunSync(func(vm *goja.Runtime) error {
		return vm.Set(name, bb.ExposeToJS(vm))
	})
}
