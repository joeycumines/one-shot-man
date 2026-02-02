package bt

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
	"github.com/dop251/goja_nodejs/require"
	bt "github.com/joeycumines/go-behaviortree"
	"github.com/joeycumines/one-shot-man/internal/goroutineid"
)

// Bridge manages the behavior tree integration between Go and JavaScript.
// It provides a safe interface for Go code to interact with JavaScript, ensuring
// all JavaScript operations happen on the event loop goroutine.
//
// Key Constraints:
//   - goja.Runtime is NOT goroutine-safe; all access must happen via RunOnLoop
//   - Promise resolve/reject functions must be called on the event loop goroutine
//   - The event loop must be started before any JavaScript operations
//
// The Bridge uses an external event loop. The caller is responsible for
// starting and stopping the event loop. The Bridge's Stop() method only stops
// the internal bt.Manager, not the event loop.
type Bridge struct {
	// timeout is the maximum duration to wait for RunOnLoopSync operations.
	// Default is 5 seconds. Set to 0 to disable timeout (not recommended for production).
	timeout time.Duration
	loop    *eventloop.EventLoop

	// Event loop goroutine ID (MANDATORY - fixes GAP #2)
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
}

// DefaultTimeout is the maximum duration to wait for RunOnLoopSync operations.
const DefaultTimeout = 5 * time.Second

// getGoroutineID returns the current goroutine ID using the shared utility.
func getGoroutineID() int64 {
	return goroutineid.Get()
}

// NewBridgeWithEventLoop creates a Bridge that uses an external event loop.
// The event loop must be started and managed by the caller.
//
// Panics if:
//   - loop is nil
//   - initialization fails
//
// Parameters:
//   - ctx: Context for cancellation support
//   - loop: The event loop (must already be started)
//   - registry: The require.Registry for module registration (can be nil)
//
// The Bridge will:
//   - Register the osm:bt module with the registry
//   - Initialize JavaScript helpers on the event loop
//   - Create an internal bt.Manager for ticker aggregation
func NewBridgeWithEventLoop(ctx context.Context, loop *eventloop.EventLoop, registry *require.Registry) *Bridge {
	if loop == nil {
		panic("event loop must not be nil")
	}
	return newBridgeWithLoop(ctx, loop, registry)
}

// newBridgeWithLoop is the internal constructor for Bridge.
func newBridgeWithLoop(ctx context.Context, loop *eventloop.EventLoop, reg *require.Registry) *Bridge {
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
	// TryRunOnLoopSync before ID capture, causing deadlock when already on loop.
	//
	// The happens-before guarantee:
	// 1. initializeJS runs on event loop -> captures goroutine ID (atomic.Store)
	// 2. Then RegisterNativeModule publishes module availability
	// 3. Any subsequent require sees published module AND captured ID

	// Initialize the VM within the event loop FIRST
	errCh := make(chan error, 1)
	ok := loop.RunOnLoop(func(vm *goja.Runtime) {
		errCh <- b.initializeJS(vm)
	})
	if !ok {
		cancel()
		b.manager.Stop()
		panic("failed to initialize: event loop not running")
	}

	if err := <-errCh; err != nil {
		cancel()
		b.manager.Stop()
		panic(fmt.Sprintf("failed to initialize JavaScript environment: %v", err))
	}

	// NOW register the osm:bt module (after ID is captured)
	if reg != nil {
		// CRIT-3 FIX: Use internal childCtx instead of parent ctx
		// Module loader must use bridge's internal lifecycle context (childCtx)
		// NOT the external parent context parameter, to ensure module lifecycle
		// matches bridge's lifecycle logic
		reg.RegisterNativeModule("osm:bt", b.ModuleLoader(childCtx))
	}

	// CRITICAL: External parent context cancellation handling
	// When parent ctx is cancelled, bridge should shut down cleanly.
	// We use AfterFunc to trigger Stop(), which ensures proper ordering:
	//   1. Stop() sets b.stopped=true (under mutex)
	//   2. Stop() cancels childCtx (closes Done() channel)
	// This maintains invariant: Done() closed ⇒ IsRunning() = false
	if ctx.Done() != nil {
		stop := context.AfterFunc(ctx, func() {
			b.Stop()
		})
		_ = stop // keep stop handle to prevent GC collection
	}

	return b
}

// initializeJS sets up the JavaScript environment with behavior tree helpers.
func (b *Bridge) initializeJS(vm *goja.Runtime) error {
	// MANDATORY STEP #1: Capture event loop goroutine ID (fixes GAP #2)
	// We extract the goroutine ID from the stack trace. This parsing happens
	// ONCE at initialization, so the overhead is acceptable.
	id := getGoroutineID()
	b.eventLoopGoroutineID.Store(id)

	// Set up the runLeaf helper which bridges async JS functions to callbacks
	// Note: The status strings in jsHelpers MUST match JSStatusRunning, JSStatusSuccess, JSStatusFailure
	_, err := vm.RunString(jsHelpers)
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
// IMPORTANT: Stop does NOT wait for in-flight RunOnLoop operations to complete.
// Operations that were already scheduled may still execute after Stop returns.
// Callers should not assume that no more work will happen after Stop returns.
//
// CRITICAL FIX (C3): The correct sequence is now:
//  1. Acquire lock
//  2. Cancel context (closes Done() channel, unblocks RunOnLoopSync waiters)
//  3. Set stopped=true (atomic with cancellation, guarantees invariant)
//  4. Release lock
//  5. Stop manager (tickers can now exit cleanly)
func (b *Bridge) Stop() {
	b.mu.Lock()
	if b.stopped {
		b.mu.Unlock()
		return
	}

	// CRITICAL FIX (C3): Perform BOTH cancel and stopped update atomically under lock.
	// This guarantees the lifecycle invariant: "Once Done() is closed, IsRunning() MUST return false".
	//
	// The happens-before relationship from the mutex ensures that any goroutine that
	// observes Done() being closed will also observe stopped=true, because both
	// operations happen before we release the lock.
	//
	// Without this, there was a race window:
	//   - Thread A calls cancel() → Done() closed
	//   - Thread B observes Done() closed, checks IsRunning()
	//   - Thread B sees stopped=false (not yet set) → VIOLATION
	b.cancel()       // Close Done channel (unblocks waiters immediately)
	b.stopped = true // Update state atomically with cancellation
	b.mu.Unlock()

	// Now stop the internal bt.Manager (stops all tickers)
	// Tickers blocked in RunOnLoopSync have already been unblocked by Done() closing
	if b.manager != nil {
		b.manager.Stop()
	}
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

// SetTimeout sets the timeout for RunOnLoopSync operations.
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

// RunOnLoop schedules a function to run on the event loop goroutine.
// Returns true if the function was successfully scheduled.
// Returns false if the event loop is not running.
//
// IMPORTANT: All goja.Runtime operations must happen inside this callback.
func (b *Bridge) RunOnLoop(fn func(*goja.Runtime)) bool {
	b.mu.RLock()
	if !b.started || b.stopped {
		b.mu.RUnlock()
		return false
	}
	b.mu.RUnlock()

	return b.loop.RunOnLoop(fn)
}

// RunOnLoopSync schedules a function on the event loop and waits for completion.
// Returns an error if the event loop is not running or stops while waiting.
// If configured, will timeout after the Bridge's timeout duration.
func (b *Bridge) RunOnLoopSync(fn func(*goja.Runtime) error) error {
	b.mu.RLock()
	if !b.started || b.stopped {
		b.mu.RUnlock()
		return errors.New("event loop not running")
	}
	timeout := b.timeout
	b.mu.RUnlock()

	errCh := make(chan error, 1)
	ok := b.loop.RunOnLoop(func(vm *goja.Runtime) {
		errCh <- fn(vm)
	})
	if !ok {
		return errors.New("event loop not running")
	}

	// Wait with timeout and cancellation support
	if timeout > 0 {
		timer := time.NewTimer(timeout)
		defer timer.Stop() // Cleanup prevents goroutine leak on early return
		select {
		case err := <-errCh:
			return err
		case <-b.Done():
			return errors.New("bridge stopped before completion")
		case <-timer.C:
			// NOTE: Print to stderr so it's VISIBLE in test output (slog goes to file)
			fmt.Fprintf(os.Stderr, "\n!!! TIMEOUT: RunOnLoopSync timed out after %v - event loop may be blocked !!!\n", timeout)
			return fmt.Errorf("operation timed out after %v (consider increasing timeout or checking for infinite loops in JS code)", timeout)
		}
	}

	// No timeout - just wait with cancellation support
	select {
	case err := <-errCh:
		return err
	case <-b.Done():
		return errors.New("bridge stopped before completion")
	}
}

// RunJSSync implements bubbletea.JSRunner interface.
// This is an alias for RunOnLoopSync, provided for interface compatibility.
// It schedules a function on the event loop and waits for completion.
func (b *Bridge) RunJSSync(fn func(*goja.Runtime) error) error {
	return b.RunOnLoopSync(fn)
}

// LoadScript loads JavaScript code into the runtime.
// Returns an error if the code fails to compile or execute.
func (b *Bridge) LoadScript(name, code string) error {
	return b.RunOnLoopSync(func(vm *goja.Runtime) error {
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
	return b.RunOnLoopSync(func(vm *goja.Runtime) error {
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
	err := b.RunOnLoopSync(func(vm *goja.Runtime) error {
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

// TryRunOnLoopSync attempts to run a function on the event loop synchronously.
// If we're already on the event loop goroutine (detected via goroutine ID),
// the function is executed directly to avoid deadlock. Otherwise, it posts to the loop
// and waits like RunOnLoopSync.
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
func (b *Bridge) TryRunOnLoopSync(currentVM *goja.Runtime, fn func(*goja.Runtime) error) error {
	// STEP 1: Bridge state check
	b.mu.RLock()
	if !b.started || b.stopped {
		b.mu.RUnlock()
		return errors.New("event loop not running")
	}
	b.mu.RUnlock()

	// STEP 2: Goroutine ID check (MANDATORY - no shortcuts)
	// We MUST check if we are on the event loop goroutine.
	eventLoopID := b.eventLoopGoroutineID.Load()
	if eventLoopID > 0 {
		currentGoroutineID := goroutineid.Get()

		if currentGoroutineID == eventLoopID {
			// We are on the event loop. It is safe to run directly.
			// No locking needed for VM access as we OWN the loop.
			return fn(currentVM)
		}
	}

	// STEP 3: Not on event loop - schedule and wait
	return b.RunOnLoopSync(fn)
}

// GetCallable retrieves a global function from the JavaScript runtime as a goja.Callable.
// This is useful for getting JS functions to pass to NewJSLeafAdapter.
// Returns an error if the variable doesn't exist or is not callable.
func (b *Bridge) GetCallable(name string) (goja.Callable, error) {
	var result goja.Callable
	err := b.RunOnLoopSync(func(vm *goja.Runtime) error {
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
	return b.RunOnLoopSync(func(vm *goja.Runtime) error {
		return vm.Set(name, bb.ExposeToJS(vm))
	})
}
