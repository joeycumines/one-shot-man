package bt

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	bt "github.com/joeycumines/go-behaviortree"
	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
	gojaeventloop "github.com/joeycumines/goja-eventloop"
	"github.com/joeycumines/goja_nodejs/require"
)

// Bridge manages the behavior tree integration between Go and JavaScript.
// It provides a safe interface for Go code to interact with JavaScript, ensuring
// all JavaScript operations happen on the event loop goroutine.
//
// Key Constraints:
//   - goja.Runtime is NOT goroutine-safe; all access must happen via Run/RunSync
//   - Promise resolve/reject functions must be called on the event loop goroutine
//   - The event loop must be started before any JavaScript operations
//
// The Bridge uses an external event loop. The caller is responsible for
// starting and stopping the event loop. The Bridge's Stop() method only stops
// the internal bt.Manager, not the event loop.
type Bridge struct {
	loop    *goeventloop.Loop
	vm      *goja.Runtime
	adapter *gojaeventloop.Adapter

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
}

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
	if ctx == nil {
		ctx = context.Background()
	}
	// Keep lifecycle cancellation under Bridge.Stop so stopped state is published
	// before Done closes. Parent cancellation is forwarded through stopParentCtx
	// below, preserving that ordering invariant.
	childCtx, cancel := context.WithCancel(context.Background())

	b := &Bridge{
		loop:    loop,
		vm:      vm,
		ctx:     childCtx,
		cancel:  cancel,
		manager: bt.NewManager(),
	}

	// Mark as started (event loop should already be running)
	b.mu.Lock()
	b.started = true
	b.mu.Unlock()

	// Initialize the VM within the event loop FIRST
	if err := b.RunSync(ctx, func(vm *goja.Runtime) error {
		return b.initializeJS()
	}); err != nil {
		cancel()
		b.manager.Stop()
		panic(fmt.Sprintf("failed to initialize JavaScript environment: %v", err))
	}

	// NOW register the osm:bt module
	if reg != nil {
		reg.RegisterNativeModule("osm:bt", b.ModuleLoader(childCtx))
	}

	if ctx.Done() != nil {
		stopParentCtx := context.AfterFunc(ctx, func() {
			b.Stop()
		})
		b.mu.Lock()
		stopped := b.stopped
		if !stopped {
			b.stopParentCtx = stopParentCtx
		}
		b.mu.Unlock()
		if stopped {
			// Stop may have won the cancellation race before the callback
			// handle was installed. Do not retain an orphaned callback.
			stopParentCtx()
		}
	}

	return b
}

// initializeJS sets up the JavaScript environment with behavior tree helpers.
func (b *Bridge) initializeJS() error {
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
// Strict microtask ordering is always-on, so Promise microtasks drain after
// each macrotask. runLeaf calls the tick function synchronously and
// detects Promise returns via thenable checks, bridging both sync and async
// results through the callback. Async Promise resolution is driven by the
// event loop's microtask drain, which is guaranteed to run.
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
//  4. Cancel the context — closes Done() and unblocks RunSync waiters. This
//     must happen before stopping the manager so managed tickers blocked in
//     bridge operations can observe cancellation and finish.
//  5. Stop the internal bt.Manager while the event loop remains alive, so
//     settled ticker promises can dispatch their callbacks. This remains
//     synchronous so Manager().Add cannot admit a ticker after Stop returns.
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
	stopParentCtx := b.stopParentCtx
	b.stopParentCtx = nil
	b.mu.Unlock()

	// Detach the parent cancellation callback once the bridge has begun
	// stopping. This prevents a stopped bridge from retaining its parent
	// context and callback indefinitely. If parent cancellation won the race,
	// the callback may already be running; its reentrant Stop call observes
	// stopped=true and returns safely.
	if stopParentCtx != nil {
		stopParentCtx()
	}

	// Cancel first so in-flight ticker work observes bridge shutdown and
	// unblocks any bridge-mediated synchronous calls before manager.Stop joins
	// the manager's ticker handlers.
	b.cancel()

	// Stop the internal bt.Manager while the event loop remains alive so
	// promise callbacks can be dispatched. Cancellation above releases any
	// bridge-mediated ticker work before Manager.Stop is entered. Keep this
	// admission barrier synchronous: Manager() exposes the raw manager, and
	// Stop must not return while it can still admit a new ticker.
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

// IsRunning returns true if the bridge is running (started and not stopped).
func (b *Bridge) IsRunning() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.started && !b.stopped
}

// GetLifecycleSnapshot returns a snapshot of both lifecycle state atomically.
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

func (b *Bridge) executeScheduled(ctx context.Context, fn func(*goja.Runtime) error) error {
	b.mu.RLock()
	if !b.started || b.stopped {
		b.mu.RUnlock()
		return errors.New("event loop not running")
	}
	b.mu.RUnlock()

	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	errCh := make(chan error, 1)
	vm := b.vm
	var state atomic.Uint32 // 0 pending, 1 running, 2 cancelled
	submitErr := b.loop.Submit(func() {
		b.mu.Lock()
		if state.Load() != 0 || !b.started || b.stopped {
			b.mu.Unlock()
			if err := ctx.Err(); err != nil {
				errCh <- err
			} else {
				errCh <- errors.New("bridge stopped before synchronous task started")
			}
			return
		}
		if err := ctx.Err(); err != nil {
			state.Store(2)
			b.mu.Unlock()
			errCh <- err
			return
		}
		state.Store(1)
		b.mu.Unlock()
		var err error
		func() {
			defer func() {
				if recovered := recover(); recovered != nil {
					err = fmt.Errorf("synchronous bridge callback panicked: %v", recovered)
				}
			}()
			err = fn(vm)
		}()
		errCh <- err
	})
	if submitErr != nil {
		return submitErr
	}

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		// Cancellation may skip a queued callback, but once the callback has
		// acquired the VM this caller must wait for ownership to be released.
		if state.CompareAndSwap(0, 2) {
			return ctx.Err()
		}
		err := <-errCh
		if err == nil {
			return ctx.Err()
		}
		return err
	case <-b.Done():
		if state.CompareAndSwap(0, 2) {
			return errors.New("bridge stopped before completion")
		}
		err := <-errCh
		if err == nil {
			return errors.New("bridge stopped before completion")
		}
		return err
	}
}

func runSyncCallback(vm *goja.Runtime, fn func(*goja.Runtime) error) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("synchronous bridge callback panicked: %v", recovered)
		}
	}()
	return fn(vm)
}

// RunSync schedules a function on the event loop and waits for completion.
// If already on the event loop callback owner goroutine, it executes inline.
func (b *Bridge) RunSync(ctx context.Context, fn func(*goja.Runtime) error) error {
	if ctx == nil {
		ctx = context.Background()
	}
	b.mu.RLock()
	running := b.started && !b.stopped
	b.mu.RUnlock()
	if !running {
		return errors.New("event loop not running")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if b.loop.IsCallbackOwner() {
		return runSyncCallback(b.vm, fn)
	}
	return b.executeScheduled(ctx, fn)
}

// TryRunSync attempts to run a function on the event loop synchronously.
// If on the event loop goroutine (verified via loop.IsCallbackOwner()),
// fn is executed directly against currentVM (or b.vm if currentVM is nil).
// Otherwise, it schedules on the loop and waits.
func (b *Bridge) TryRunSync(ctx context.Context, currentVM *goja.Runtime, fn func(*goja.Runtime) error) error {
	if ctx == nil {
		ctx = context.Background()
	}
	b.mu.RLock()
	running := b.started && !b.stopped
	b.mu.RUnlock()
	if !running {
		return errors.New("event loop not running")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if b.loop.IsCallbackOwner() {
		vm := currentVM
		if vm == nil {
			vm = b.vm
		}
		return runSyncCallback(vm, fn)
	}
	return b.executeScheduled(ctx, fn)
}

// LoadScript loads JavaScript code into the runtime.
// Returns an error if the code fails to compile or execute.
func (b *Bridge) LoadScript(name, code string) error {
	return b.RunSync(b.ctx, func(vm *goja.Runtime) error {
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
	return b.RunSync(b.ctx, func(vm *goja.Runtime) error {
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
	err := b.RunSync(b.ctx, func(vm *goja.Runtime) error {
		val := vm.Get(name)
		if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
			if goja.IsUndefined(val) {
				exists = false
				result = nil
			} else if goja.IsNull(val) {
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

// GetCallable retrieves a global function from the JavaScript runtime as a goja.Callable.
// This is useful for getting JS functions to pass to NewJSLeafAdapter.
// Returns an error if the variable doesn't exist or is not callable.
func (b *Bridge) GetCallable(name string) (goja.Callable, error) {
	var result goja.Callable
	err := b.RunSync(b.ctx, func(vm *goja.Runtime) error {
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
	return b.RunSync(b.ctx, func(vm *goja.Runtime) error {
		return vm.Set(name, bb.ExposeToJS(vm))
	})
}
