package scripting

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
	gojaEventloop "github.com/joeycumines/goja-eventloop"
	"github.com/joeycumines/goja_nodejs/require"
	"github.com/joeycumines/logiface"
)

// runtimeLogEvent bridges go-eventloop structured diagnostics (Loop.Log) to
// the host's slog. It implements logiface.Event via UnimplementedEvent so
// future logiface fields remain compatible. Panics in callbacks and promise
// jobs that would otherwise silently hang are now observed as structured
// errors on this logger's writer, which forwards to slog at LevelError.
type runtimeLogEvent struct {
	logiface.UnimplementedEvent
	level   logiface.Level
	message string
	err     error
	fields  map[string]any
}

func (e *runtimeLogEvent) Level() logiface.Level { return e.level }
func (e *runtimeLogEvent) AddField(key string, val any) {
	if e.fields == nil {
		e.fields = make(map[string]any)
	}
	e.fields[key] = val
}
func (e *runtimeLogEvent) AddMessage(msg string) bool            { e.message = msg; return true }
func (e *runtimeLogEvent) AddError(err error) bool               { e.err = err; return true }
func (e *runtimeLogEvent) AddString(key string, val string) bool { e.AddField(key, val); return true }

type runtimeLogEventFactory struct{}

func (runtimeLogEventFactory) NewEvent(level logiface.Level) logiface.Event {
	return &runtimeLogEvent{level: level}
}

func newRuntimeLogger() *logiface.Logger[logiface.Event] {
	return logiface.New[logiface.Event](
		logiface.WithEventFactory[logiface.Event](runtimeLogEventFactory{}),
		logiface.WithWriter[logiface.Event](logiface.NewWriterFunc(func(e logiface.Event) error {
			// e is *runtimeLogEvent in practice; extract via type assertion.
			if re, ok := e.(*runtimeLogEvent); ok {
				args := []any{"level", re.level.String()}
				if re.message != "" {
					args = append(args, "msg", re.message)
				}
				if re.err != nil {
					args = append(args, "error", re.err)
				}
				for k, v := range re.fields {
					args = append(args, k, v)
				}
				// Use slog at appropriate level; Loop.Log already filters by level.
				// Map logiface.Level to slog level roughly; default to Error.
				slog.Error("eventloop diagnostic", args...)
			} else {
				slog.Error("eventloop diagnostic (unknown event type)", "event", e)
			}
			return nil
		})),
	).Logger()
}

// RegisterFD is deliberately unused. The event loop's RegisterFD is a
// channel-based readiness API that on Windows, Plan 9, js/wasm and wasip1/wasm
// returns ErrReadinessUnsupported by design (see go-eventloop README and
// adapter docs). Our bindings are channel and timer based, not FD based, so
// the lack of use is intentional and legitimate.

// Runtime wraps a goja.Runtime with an integrated event loop and module registry.
// It provides thread-safe execution of JavaScript by running all JS code
// on a single dedicated event-loop goroutine.
type Runtime struct {
	loop    *goeventloop.Loop
	adapter *gojaEventloop.Adapter
	vm      *goja.Runtime

	// registry is the CommonJS require registry for native modules.
	registry *require.Registry

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
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Create the Go event loop. Strict microtask ordering (microtasks drained after
	// every macrotask, per Node.js semantics) is now always-on in the 20260823
	// surface — no option needed.
	//
	// WithAutoExit(true) allows the loop to exit naturally when no tasks,
	// timers, or Promisify tokens remain. This is the primary shutdown
	// signal for the application.
	loop, err := goeventloop.New(
		goeventloop.WithAutoExit(true),
		goeventloop.WithLogger(newRuntimeLogger()),
		goeventloop.WithMetrics(true),
		goeventloop.WithDebugMode(true),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create event loop: %w", err)
	}

	vm := goja.New()
	registry.Enable(vm)

	loopCtx, loopCancel := context.WithCancel(ctx)

	// Create internal lifecycle context from the caller context so cancellation
	// propagates to bindings and lifecycle work without waiting for AfterFunc.
	childCtx, cancel := context.WithCancel(ctx)

	rt := &Runtime{
		loop:          loop,
		vm:            vm,
		registry:      registry,
		ctx:           childCtx,
		cancel:        cancel,
		loopCancel:    loopCancel,
		done:          make(chan struct{}),
		bootstrapDone: make(chan struct{}),
	}

	// Use Promisify to keep the loop alive until natural exit is requested.
	// This prevents the loop from auto-exiting during the registration phase.
	loop.Promisify(ctx, func(ctx context.Context) (any, error) {
		<-rt.bootstrapDone
		return nil, nil
	})

	// Create goja adapter and bind JS globals before starting the loop.
	// New in 20260823: New/Bind must be called while the loop is awake (before Run),
	// not from inside a Submit callback (which would be Running).
	var err2 error
	rt.adapter, err2 = gojaEventloop.New(loop, vm)
	if err2 != nil {
		close(rt.bootstrapDone)
		loopCancel()
		return nil, fmt.Errorf("failed to create goja adapter: %w", err2)
	}
	if err2 = rt.adapter.Bind(); err2 != nil {
		close(rt.bootstrapDone)
		loopCancel()
		return nil, fmt.Errorf("failed to bind JS globals: %w", err2)
	}
	rt.adapter.SetConsoleOutput(os.Stderr)

	// Start the event loop in background goroutine
	go func() {
		defer close(rt.done)
		// Run loop on its own goroutine
		if err := rt.loop.Run(loopCtx); err != nil && err != context.Canceled {
			// Report unexpected loop exit
			slog.Error("eventloop terminated unexpectedly", "error", err)
		}
	}()

	rt.mu.Lock()
	rt.started = true
	rt.mu.Unlock()

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

	// Release the bootstrap token under the same mutex used by Wait. Close and
	// Wait may race, but the channel must be closed exactly once.
	select {
	case <-rt.bootstrapDone:
	default:
		close(rt.bootstrapDone)
	}
	rt.mu.Unlock()

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
	if rt.adapter != nil {
		<-rt.adapter.Done()
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
	if rt.adapter != nil {
		<-rt.adapter.Done()
	}
	// Natural auto-exit is terminal just like Close. Publish the stopped state
	// after all loop callbacks have drained so IsRunning cannot report a live
	// runtime after Wait returns.
	rt.mu.Lock()
	rt.stopped = true
	rt.mu.Unlock()
}

// Done returns the terminal completion signal from the adapter when bound,
// otherwise the internal lifecycle context. The adapter signal closes only
// after terminal cleanup when no callback accepted can still execute.
func (rt *Runtime) Done() <-chan struct{} {
	if rt.adapter != nil {
		return rt.adapter.Done()
	}
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

// Registry returns the require.Registry for native modules.
func (rt *Runtime) Registry() *require.Registry {
	return rt.registry
}

// Adapter returns the goja-eventloop adapter.
func (rt *Runtime) Adapter() *gojaEventloop.Adapter {
	return rt.adapter
}

// Promisify implements EventLoopProvider. It wraps a Go function in a
// Future-like lifecycle that keeps the event loop alive until completion.
func (rt *Runtime) Promisify(ctx context.Context, fn func(ctx context.Context) (any, error)) goeventloop.Future {
	return rt.loop.Promisify(ctx, fn)
}

// IsRunning returns true if the runtime is running (started and not stopped).
func (rt *Runtime) IsRunning() bool {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	return rt.started && !rt.stopped
}

// Run schedules a function to run on the event loop goroutine.
// Returns true if the function was successfully scheduled.
func (rt *Runtime) Run(fn func(vm *goja.Runtime)) bool {
	rt.mu.RLock()
	if !rt.started || rt.stopped {
		rt.mu.RUnlock()
		return false
	}
	rt.mu.RUnlock()

	vm := rt.vm
	return rt.loop.Submit(func() {
		fn(vm)
	}) == nil
}

// RunSync schedules a function on the event loop and waits for completion.
// If already on the event loop callback owner goroutine, it executes inline.
func (rt *Runtime) RunSync(ctx context.Context, fn func(vm *goja.Runtime) error) error {
	return rt.TryRunSync(ctx, nil, fn)
}

func runSyncCallback(fn func(*goja.Runtime) error, vm *goja.Runtime) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("synchronous runtime callback panicked: %v", recovered)
		}
	}()
	return fn(vm)
}

// TryRunSync attempts to run a function on the event loop synchronously.
// If already on the event loop callback owner goroutine, it executes directly
// against currentVM (or rt.vm if currentVM is nil). Otherwise, it posts to the loop
// and coordinates wait with ctx, interrupt, and shutdown barrier.
func (rt *Runtime) TryRunSync(ctx context.Context, currentVM *goja.Runtime, fn func(vm *goja.Runtime) error) error {
	rt.mu.RLock()
	if !rt.started || rt.stopped {
		rt.mu.RUnlock()
		return errors.New("event loop not running")
	}
	rt.mu.RUnlock()

	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	if rt.loop.IsCallbackOwner() {
		vm := currentVM
		if vm == nil {
			vm = rt.vm
		}
		return runSyncCallback(fn, vm)
	}

	errCh := make(chan error, 1)
	vm := rt.vm
	// Gate execution so cancellation can prevent a queued callback from
	// mutating the VM after RunSync has returned.
	var state atomic.Uint32 // 0 pending, 1 running, 2 cancelled
	submitErr := rt.loop.Submit(func() {
		if !state.CompareAndSwap(0, 1) {
			if err := ctx.Err(); err != nil {
				errCh <- err
			} else {
				errCh <- errors.New("runtime stopped before synchronous task started")
			}
			return
		}
		errCh <- runSyncCallback(fn, vm)
	})
	if submitErr != nil {
		return submitErr
	}

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		if state.CompareAndSwap(0, 2) {
			// The callback has not started, so it is safe to let the queued
			// trampoline observe cancellation and skip VM access.
			return ctx.Err()
		}
		// Once the callback owns Goja, RunSync must not return until that
		// callback has released the VM. Returning here would permit the
		// caller to race a still-running callback with later VM operations.
		err := <-errCh
		if err == nil {
			return ctx.Err()
		}
		return err
	case <-rt.Done():
		if state.CompareAndSwap(0, 2) {
			return errors.New("runtime stopped while waiting for synchronous task")
		}
		err := <-errCh
		if err == nil {
			return errors.New("runtime stopped while waiting for synchronous task")
		}
		return err
	}
}

// LoadScript loads and executes JavaScript code in the runtime.
func (rt *Runtime) LoadScript(name, code string) error {
	return rt.RunSync(rt.ctx, func(vm *goja.Runtime) error {
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
	return rt.RunSync(rt.ctx, func(vm *goja.Runtime) error {
		return vm.Set(name, value)
	})
}

// GetGlobal retrieves a global variable from the JavaScript runtime.
func (rt *Runtime) GetGlobal(name string) (any, error) {
	var result any
	err := rt.RunSync(rt.ctx, func(vm *goja.Runtime) error {
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
	err := rt.TryRunSync(rt.ctx, nil, func(vm *goja.Runtime) error {
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
