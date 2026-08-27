package scripting

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"time"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
	gojaEventloop "github.com/joeycumines/goja-eventloop"
	"github.com/joeycumines/goja_nodejs/require"
	"github.com/joeycumines/goroutineid"
	"github.com/joeycumines/logiface"
	"github.com/joeycumines/one-shot-man/internal/eventlooputil"
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
func (e *runtimeLogEvent) AddMessage(msg string) bool { e.message = msg; return true }
func (e *runtimeLogEvent) AddError(err error) bool { e.err = err; return true }
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

	// timeout is the maximum duration to wait for RunSync operations.
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

	// loopRunner is the shared submit-and-wait substrate (lazy, see runner).
	loopRunner *eventlooputil.Runner
	runnerOnce sync.Once

	// ctx is the lifecycle context for Done() channel
	ctx    context.Context
	cancel context.CancelFunc
}

// defaultSyncTimeout is the maximum duration to wait for RunSync operations.
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

	// H0 SECURITY: neutralize dangerous process globals installed by goja-eventloop Bind.
	// Bind installs Node v26.5 process lifecycle globals (process.exit/exitCode etc.)
	// which would allow user scripts to terminate the host. The sandbox tests assert
	// these are absent, and main at 498102f proves they were absent before the fork.
	// We keep process.nextTick and event emitter methods, but delete exit-related and
	// env/pid surface. Also scrub Buffer/Deno/quit globals that must not leak.
	if procVal := vm.Get("process"); procVal != nil && !goja.IsUndefined(procVal) && !goja.IsNull(procVal) {
		if procObj, ok := procVal.(*goja.Object); ok {
			_ = procObj.Delete("exit")
			_ = procObj.Delete("exitCode")
			_ = procObj.Delete("env")
			_ = procObj.Delete("pid")
			_ = procObj.Delete("_exiting")
		}
	}
	_ = vm.Set("Buffer", goja.Undefined())
	_ = vm.Set("Deno", goja.Undefined())
	_ = vm.Set("exit", goja.Undefined())
	_ = vm.Set("quit", goja.Undefined())

	// Start the event loop in background goroutine
	go func() {
		defer close(rt.done)
		// Run loop on its own goroutine
		if err := rt.loop.Run(loopCtx); err != nil && err != context.Canceled {
			// Report unexpected loop exit
			slog.Error("eventloop terminated unexpectedly", "error", err)
		}
	}()

	// Capture event loop goroutine ID for deadlock prevention via a Submit
	// trampoline (runs on the loop goroutine once it starts).
	_ = loop.Submit(func() {
		rt.eventLoopGoroutineID.Store(goroutineid.Get())
	})

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

// GoroutineID returns the stored event-loop goroutine ID for reentrancy checks.
func (rt *Runtime) GoroutineID() int64 {
	return rt.eventLoopGoroutineID.Load()
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

// SetTimeout sets the timeout for RunSync operations.
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

// Run schedules a function to run on the event loop goroutine.
// Returns true if the function was successfully scheduled.
func (rt *Runtime) Run(fn func(vm *goja.Runtime)) bool {
	rt.mu.RLock()
	if !rt.started || rt.stopped {
		rt.mu.RUnlock()
		return false
	}
	rt.mu.RUnlock()

	return rt.runner().Go(func() error {
		fn(rt.vm)
		return nil
	}) == nil
}

// RunSync schedules a function on the event loop and waits for completion.
// Returns an error if the event loop is not running or stops while waiting.
func (rt *Runtime) RunSync(fn func(vm *goja.Runtime) error) error {
	rt.mu.RLock()
	if !rt.started || rt.stopped {
		rt.mu.RUnlock()
		return errors.New("event loop not running")
	}
	timeout := rt.timeout
	rt.mu.RUnlock()

	return rt.runner().Sync(func() error {
		return fn(rt.vm)
	}, timeout)
}

// TryRunSync attempts to run a function on the event loop synchronously.
// If we're already on the event loop goroutine, the function is executed
// directly to avoid deadlock. Otherwise, it posts to the loop and waits.
func (rt *Runtime) TryRunSync(currentVM *goja.Runtime, fn func(vm *goja.Runtime) error) error {
	rt.mu.RLock()
	if !rt.started || rt.stopped {
		rt.mu.RUnlock()
		return errors.New("event loop not running")
	}
	rt.mu.RUnlock()

	return rt.runner().TrySyncBranch(
		func() error { return fn(currentVM) },
		func() error { return fn(rt.vm) },
		rt.timeout,
	)
}

// runner lazily builds the shared submit-and-wait substrate for this runtime.
func (rt *Runtime) runner() *eventlooputil.Runner {
	rt.runnerOnce.Do(func() {
		r, err := eventlooputil.NewRunner(eventlooputil.RunnerConfig{
			Loop:          rt.loop,
			OnLoopThread:  func() bool { return eventlooputil.IsLoopThread(rt.eventLoopGoroutineID.Load()) },
			Done:          rt.ctx.Done(),
			NotRunningErr: errors.New("event loop not running"),
			StoppedErr:    errors.New("runtime stopped while waiting for synchronous task"),
		})
		if err != nil {
			panic(err)
		}
		rt.loopRunner = r
	})
	return rt.loopRunner
}

// LoadScript loads and executes JavaScript code in the runtime.
func (rt *Runtime) LoadScript(name, code string) error {
	return rt.RunSync(func(vm *goja.Runtime) error {
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
	return rt.RunSync(func(vm *goja.Runtime) error {
		return vm.Set(name, value)
	})
}

// GetGlobal retrieves a global variable from the JavaScript runtime.
func (rt *Runtime) GetGlobal(name string) (any, error) {
	var result any
	err := rt.RunSync(func(vm *goja.Runtime) error {
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
	err := rt.TryRunSync(nil, func(vm *goja.Runtime) error {
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
