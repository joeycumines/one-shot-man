package btbridge

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
	"github.com/dop251/goja_nodejs/require"
)

// Bridge manages the goja runtime and event loop for JavaScript behavior trees.
// It provides a safe interface for Go code to interact with JavaScript, ensuring
// all JavaScript operations happen on the event loop goroutine.
//
// Key Constraints:
//   - goja.Runtime is NOT goroutine-safe; all access must happen via RunOnLoop
//   - Promise resolve/reject functions must be called on the event loop goroutine
//   - The event loop must be started before any JavaScript operations
type Bridge struct {
	loop *eventloop.EventLoop
	vm   *goja.Runtime // Only access within RunOnLoop callbacks

	mu      sync.RWMutex
	started bool
	stopped bool

	// Lifecycle context for Done() channel
	ctx    context.Context
	cancel context.CancelFunc
}

// NewBridge creates a new Bridge with an initialized event loop.
// The event loop is automatically started.
// Call Stop() when done to clean up resources.
func NewBridge() (*Bridge, error) {
	return NewBridgeWithContext(context.Background())
}

// NewBridgeWithContext creates a new Bridge that will stop when the context is canceled.
func NewBridgeWithContext(ctx context.Context) (*Bridge, error) {
	reg := require.NewRegistry()
	loop := eventloop.NewEventLoop(
		eventloop.WithRegistry(reg),
		eventloop.EnableConsole(true),
	)

	// Create internal lifecycle context
	childCtx, cancel := context.WithCancel(context.Background())

	b := &Bridge{
		loop:   loop,
		ctx:    childCtx,
		cancel: cancel,
	}

	// Start the event loop
	loop.Start()
	b.mu.Lock()
	b.started = true
	b.mu.Unlock()

	// Initialize the VM within the event loop
	errCh := make(chan error, 1)
	ok := loop.RunOnLoop(func(vm *goja.Runtime) {
		b.vm = vm
		errCh <- b.initializeJS(vm)
	})
	if !ok {
		cancel()
		return nil, errors.New("failed to initialize: event loop not running")
	}

	if err := <-errCh; err != nil {
		cancel()
		loop.Stop()
		return nil, fmt.Errorf("failed to initialize JavaScript environment: %w", err)
	}

	// Handle external context cancellation - register if context is cancelable
	if ctx.Done() != nil {
		context.AfterFunc(ctx, func() {
			b.Stop()
		})
	}

	return b, nil
}

// initializeJS sets up the JavaScript environment with behavior tree helpers.
func (b *Bridge) initializeJS(vm *goja.Runtime) error {
	// Set up the runLeaf helper which bridges async JS functions to callbacks
	// Note: The status strings in jsHelpers MUST match JSStatusRunning, JSStatusSuccess, JSStatusFailure
	_, err := vm.RunString(jsHelpers)
	return err
}

// jsHelpers contains the JavaScript helper code for the bridge.
// IMPORTANT: Status strings here MUST match the JSStatus* constants in adapter.go
const jsHelpers = `
// runLeaf executes a JS leaf function and calls the callback with the result.
// This bridges the Promise-based JS world to the callback-based Go world.
globalThis.runLeaf = function(fn, ctx, args, callback) {
	Promise.resolve()
		.then(() => fn(ctx, args))
		.then(
			(status) => callback(String(status), null),
			(err) => callback("failure", err instanceof Error ? err.message : String(err))
		);
};

// Status constants matching go-behaviortree (must match JSStatus* constants)
globalThis.bt = {
	running: "running",
	success: "success",
	failure: "failure"
};
`

// Stop gracefully stops the event loop.
// It's safe to call multiple times.
// After Stop is called, Done() channel will be closed.
func (b *Bridge) Stop() {
	b.mu.Lock()
	if b.stopped {
		b.mu.Unlock()
		return
	}
	b.stopped = true
	b.mu.Unlock()

	// Cancel the lifecycle context BEFORE stopping the loop
	// This ensures any goroutines waiting on Done() will unblock
	b.cancel()
	b.loop.Stop()
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
func (b *Bridge) RunOnLoopSync(fn func(*goja.Runtime) error) error {
	b.mu.RLock()
	if !b.started || b.stopped {
		b.mu.RUnlock()
		return errors.New("event loop not running")
	}
	b.mu.RUnlock()

	errCh := make(chan error, 1)
	ok := b.loop.RunOnLoop(func(vm *goja.Runtime) {
		errCh <- fn(vm)
	})
	if !ok {
		return errors.New("event loop not running")
	}

	// Wait with cancellation support to avoid deadlock if bridge stops
	select {
	case err := <-errCh:
		return err
	case <-b.Done():
		return errors.New("bridge stopped before completion")
	}
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
// Returns nil if the variable doesn't exist.
func (b *Bridge) GetGlobal(name string) (any, error) {
	var result any
	err := b.RunOnLoopSync(func(vm *goja.Runtime) error {
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

// ExposeBlackboard exposes a Blackboard to JavaScript with the given name.
func (b *Bridge) ExposeBlackboard(name string, bb *Blackboard) error {
	return b.RunOnLoopSync(func(vm *goja.Runtime) error {
		return vm.Set(name, bb.ExposeToJS(vm))
	})
}
