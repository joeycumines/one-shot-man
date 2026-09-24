package scripting

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime/debug"
	"slices"
	"sync"
	"sync/atomic"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
	gojaEventloop "github.com/joeycumines/goja-eventloop"
	console "github.com/joeycumines/goja_nodejs/console"
	"github.com/joeycumines/goja_nodejs/require"
	"github.com/joeycumines/one-shot-man/internal/builtin"
	"github.com/joeycumines/one-shot-man/internal/builtin/bt"
	userk8smod "github.com/joeycumines/one-shot-man/internal/builtin/userk8s"
)

// Engine represents a JavaScript scripting engine with deferred execution capabilities.
// It supports CommonJS modules via a global `require` function, which can load:
// - Native Go modules with the "osm:" prefix (e.g., require("osm:utils")).
// - Absolute file paths (e.g., require("/path/to/module.js")).
// - Relative file paths (e.g., require("./module.js")).
//
// The Engine now uses a shared Runtime with an event loop for all JavaScript execution,
// enabling proper async/Promise support and safe integration with bt.

// scriptPanicError is a structured error type for script panics.
// It implements the error interface and provides structured access to panic details
// for programmatic consumption by callers.
//
// Example usage:
//
//	if err := engine.ExecuteScript(script); err != nil {
//	    var panicErr *scriptPanicError
//	    if errors.As(err, &panicErr) {
//	        log.Printf("script %q panicked: %v", panicErr.ScriptName, panicErr.Value)
//	        // panicErr.StackTrace contains the full stack trace
//	    }
//	}
type scriptPanicError struct {
	// Value is the value passed to panic() in the script
	Value any
	// StackTrace contains the Go runtime stack trace at the time of panic
	StackTrace string
	// ScriptName is the name of the script that panicked
	ScriptName string
}

// Error returns a human-readable description of the panic error.
func (e *scriptPanicError) Error() string {
	return fmt.Sprintf("script %q panicked: %v", e.ScriptName, e.Value)
}

// Unwrap returns the underlying panic value for errors.Is/As compatibility.
// Note: The panic value may not implement the error interface, so this returns
// the value directly when it does, or wraps it in an error when it doesn't.
func (e *scriptPanicError) Unwrap() error {
	if err, ok := e.Value.(error); ok {
		return err
	}
	return nil
}

type Engine struct {
	runtime                *Runtime          // Shared runtime with event loop
	vm                     *goja.Runtime     // Direct VM reference (for sync operations)
	registry               *require.Registry // CommonJS require registry
	scripts                []*Script
	ctx                    context.Context
	stdout                 io.Writer
	stderr                 io.Writer
	globals                map[string]any
	globalsMu              sync.RWMutex // Protects globals map access (C5 fix)
	testMode               bool
	closed                 atomic.Bool
	tuiManager             *TUIManager
	contextManager         *ContextManager
	logger                 *TUILogger
	tuiOutputActive        atomic.Bool
	terminalIO             *TerminalIO               // Shared terminal I/O for all TUI subsystems
	bubbleteaManager       builtin.BubbleteaManager  // For sending state refresh messages to running TUI
	btBridge               *bt.Bridge                // Behavior tree bridge for JS integration
	bubblezoneManager      builtin.BubblezoneManager // Zone-based mouse hit-testing for BubbleTea
	requireModule          *require.RequireModule    // CommonJS require module for file-based script execution
	stateRefreshDispatcher *stateRefreshDispatcher
}

// stateRefreshDispatcher coalesces rapid state refreshes per key into a single
// SendStateRefresh, using one bounded goroutine. Latest wins deterministically.
type stateRefreshDispatcher struct {
	mu           sync.Mutex
	closed       bool
	pending      map[string]struct{}
	ch           chan struct{}
	stop         chan struct{}
	mgr          builtin.BubbleteaManager
	stateManager builtin.StateManager
	listenerID   int
	listenerSet  bool
	done         chan struct{}
	closeOnce    sync.Once
}

func newStateRefreshDispatcher(mgr builtin.BubbleteaManager, stateManager builtin.StateManager) *stateRefreshDispatcher {
	d := &stateRefreshDispatcher{
		pending:      make(map[string]struct{}),
		ch:           make(chan struct{}, 1),
		stop:         make(chan struct{}),
		done:         make(chan struct{}),
		mgr:          mgr,
		stateManager: stateManager,
	}
	if stateManager != nil {
		d.listenerID = stateManager.AddListener(func(key string) {
			d.Enqueue(key)
		})
		d.listenerSet = true
	}
	go d.loop()
	return d
}

func (d *stateRefreshDispatcher) Enqueue(key string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return
	}
	d.pending[key] = struct{}{}
	select {
	case d.ch <- struct{}{}:
	default:
	}
}

func (d *stateRefreshDispatcher) loop() {
	defer close(d.done)
	for {
		select {
		case <-d.ch:
			d.mu.Lock()
			if d.closed {
				d.mu.Unlock()
				return
			}
			keys := make([]string, 0, len(d.pending))
			for k := range d.pending {
				keys = append(keys, k)
				delete(d.pending, k)
			}
			d.mu.Unlock()
			for _, k := range keys {
				d.mgr.SendStateRefresh(k)
			}
			// Drain coalesced signals that arrived while sending.
			d.mu.Lock()
			hasPending := !d.closed && len(d.pending) > 0
			d.mu.Unlock()
			if hasPending {
				select {
				case d.ch <- struct{}{}:
				default:
				}
			}
		case <-d.stop:
			return
		}
	}
}

func (d *stateRefreshDispatcher) Close() {
	d.closeOnce.Do(func() {
		d.mu.Lock()
		d.closed = true
		clear(d.pending)
		if d.stateManager != nil && d.listenerSet {
			d.stateManager.RemoveListener(d.listenerID)
		}
		d.mu.Unlock()
		close(d.stop)
	})
	<-d.done
}

// Script represents a JavaScript script with metadata.
type Script struct {
	Name        string
	Path        string
	Content     string
	Description string
}

// engineOptions holds optional configuration for engine creation.
type engineOptions struct {
	modulePaths []string
	userK8s     userk8smod.Options
}

// EngineOption configures optional Engine settings.
type EngineOption func(*engineOptions)

// WithUserK8sOptions supplies the resolved osm:userk8s configuration, so the
// model catalog the module exposes comes from the caller's configuration
// rather than this package's defaults.
func WithUserK8sOptions(options userk8smod.Options) EngineOption {
	return func(o *engineOptions) { o.userK8s = options }
}

// WithModulePaths configures additional module search paths for require().
// These paths are searched when a bare module name is used (e.g., require('mylib')),
// similar to NODE_PATH in Node.js. Relative and absolute paths are supported.
func WithModulePaths(paths ...string) EngineOption {
	return func(o *engineOptions) {
		o.modulePaths = append(o.modulePaths, paths...)
	}
}

// NewEngine creates a new JavaScript scripting engine with full configuration options.
// logFile: optional writer for log output (JSON).
// logBufferSize: size of the in-memory log buffer (default 1000 if <= 0).
// logLevel: minimum log level to capture (e.g. slog.LevelDebug).
// opts: optional engine configuration (e.g., WithModulePaths).
func NewEngine(
	ctx context.Context,
	stdout, stderr io.Writer,
	sessionID, store string,
	logFile io.Writer,
	logBufferSize int,
	logLevel slog.Level,
	opts ...EngineOption) (*Engine, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	// Apply options
	var eopts engineOptions
	for _, o := range opts {
		o(&eopts)
	}
	// Get current working directory for context manager
	workingDir, err := os.Getwd()
	if err != nil {
		// Fallback to temp dir when CWD is unavailable (can happen in
		// containers or when parallel tests delete their working directory).
		workingDir = os.TempDir()
	}

	contextManager, err := NewContextManager(workingDir)
	if err != nil {
		return nil, fmt.Errorf("failed to create context manager: %w", err)
	}

	// Create shared terminal I/O for all TUI subsystems.
	// This is the single source of truth for terminal state management.
	terminalIO := NewTerminalIOStdio()

	// Create a temporary slog.Logger for startup validation (the TUILogger isn't
	// constructed yet). Warnings go to stderr so they're visible immediately.
	startupLogger := slog.New(slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))

	// Create the require registry first - it will be shared.
	// If module paths are configured, they are registered as global folders
	// for bare module name resolution (similar to NODE_PATH).
	// A custom source loader strips shebang lines (#!/...) from scripts,
	// matching Node.js behavior so scripts with shebangs work through require().
	var registryOpts []require.Option

	if len(eopts.modulePaths) > 0 {
		// Validate paths at startup: drop invalid/missing/non-directory entries
		// with a warning rather than failing hard.
		validPaths := validateModulePaths(eopts.modulePaths, startupLogger)

		// Use a hardened loader that adds symlink escape security and better
		// error messages, and a hardened path resolver for path-traversal security.
		registryOpts = append(registryOpts,
			require.WithLoader(newHardenedSourceLoader(validPaths, eopts.modulePaths)),
		)

		if len(validPaths) > 0 {
			registryOpts = append(registryOpts, require.WithGlobalFolders(validPaths...))
			registryOpts = append(registryOpts,
				require.WithPathResolver(newHardenedPathResolver(validPaths)),
			)
		}
	} else {
		registryOpts = append(registryOpts, require.WithLoader(shebangStrippingLoader))
	}
	registry := require.NewRegistry(registryOpts...)

	// Create the shared Runtime with event loop
	// The Runtime owns the event loop and provides thread-safe JS execution
	runtime, err := NewRuntimeRegistry(ctx, registry)
	if err != nil {
		return nil, fmt.Errorf("failed to create runtime: %w", err)
	}

	vm := runtime.Runtime()

	engine := &Engine{
		runtime:        runtime,
		vm:             vm,
		registry:       registry,
		ctx:            ctx,
		stdout:         stdout,
		stderr:         stderr,
		globals:        make(map[string]any),
		contextManager: contextManager,
		logger:         NewTUILogger(stdout, logFile, logBufferSize, logLevel),
		terminalIO:     terminalIO,
	}

	// Register native Go modules. These are all prefixed with "osm:".
	// Pass through the engine's context and a TUI sink for modules that need them.
	// Pass 'engine' as terminalProvider so bubbletea uses the unified TerminalIO
	// instead of defaulting to raw os.Stdin (which would violate Single Source of Truth).
	// Pass 'engine' as eventLoopProvider so bt shares the event loop.
	registerResult := builtin.Register(ctx, func(msg string) { engine.logger.PrintToTUI(msg) }, engine.registry, engine, engine, builtin.WithUserK8sOptions(eopts.userK8s))
	engine.bubbleteaManager = registerResult.BubbleteaManager
	engine.btBridge = registerResult.BTBridge
	engine.bubblezoneManager = registerResult.BubblezoneManager

	// Enable the `require` function in the runtime (must be done on event loop).
	// Store the RequireModule so we can use it for file-based script execution,
	// which gives scripts proper __filename, __dirname, and relative require resolution.
	err = runtime.RunSync(ctx, func(r *goja.Runtime) error {
		engine.requireModule = registry.Enable(r)

		// Extend the console object (created by adapter.Bind() with timer methods)
		// with console.log/warn/error/info/debug from goja_nodejs/console module.
		// adapter.Bind() only provides console.time/timeEnd/timeLog/count/etc.
		// The goja_nodejs/console module provides the standard logging methods.
		// The console module is ALSO loaded with a PLAIN printer: the
		// registered default wraps console.log in log.LstdFlags timestamps,
		// which corrupts every machine-readable output a script emits
		// (`--list --json` was unparseable). log/info/debug print verbatim to
		// stdout; warn/error go to stderr through the same printer so they
		// carry no timestamp either.
		existingConsole := r.Get("console").ToObject(r)
		plainModule := r.NewObject()
		plainModule.Set("exports", r.NewObject())
		console.RequireWithPrinter(newPlainStdPrinter())(r, plainModule)
		plainExports := plainModule.Get("exports").ToObject(r)
		for _, method := range []string{"log", "info", "debug", "warn", "error"} {
			existingConsole.Set(method, plainExports.Get(method))
		}

		// Install circular dependency detection by wrapping the require function.
		// This must happen after Enable() since it wraps the require function that
		// Enable() installs. goja_nodejs caches modules before execution, so cycles
		// cannot be detected at the SourceLoader level—they must be detected here.
		installRequireCycleDetection(r, startupLogger)
		return nil
	})
	if err != nil {
		_ = runtime.Close()
		return nil, fmt.Errorf("failed to enable require: %w", err)
	}

	// Create TUI manager with explicit configuration (this also initializes StateManager).
	// Pass the shared terminal reader/writer from terminalIO.
	engine.tuiManager = NewTUIManagerWithConfig(ctx, engine, terminalIO.TUIReader, terminalIO.TUIWriter, sessionID, store)

	// Wire StateManager to bubbletea with a coalescing dispatcher.
	// Rapid 100 SetState calls for the same key collapse to a single
	// SendStateRefresh for the latest value, using one bounded goroutine
	// serialized per engine lifecycle. Latest wins deterministically.
	if engine.bubbleteaManager != nil && engine.tuiManager.stateManager != nil {
		engine.stateRefreshDispatcher = newStateRefreshDispatcher(engine.bubbleteaManager, engine.tuiManager.stateManager)
	}

	// Register the shared symbols module properly through the require registry
	engine.registry.RegisterNativeModule("osm:sharedStateSymbols", builtin.GetSharedSymbolsLoader(engine.tuiManager))

	// Set up the global context and APIs
	if err := engine.executeOnLoop(func(*goja.Runtime) error {
		engine.setupGlobals()
		return nil
	}); err != nil {
		if engine.stateRefreshDispatcher != nil {
			engine.stateRefreshDispatcher.Close()
		}
		_ = runtime.Close()
		return nil, fmt.Errorf("failed to set up globals: %w", err)
	}

	// Interrupt JS execution when context is canceled.
	// Capture vm locally for the context.AfterFunc callback. goja.Runtime.Interrupt
	// is documented as safe to call from any goroutine.
	vmForInterrupt := engine.vm
	context.AfterFunc(ctx, func() {
		if vmForInterrupt != nil {
			vmForInterrupt.Interrupt(ctx.Err())
		}
	})

	return engine, nil
}

// SetTestMode enables test mode for the engine.
func (e *Engine) SetTestMode(enabled bool) {
	e.testMode = enabled
}

func (e *Engine) QueueSetGlobal(name string, value any) {
	if e.Loop() != nil && e.Loop().IsCallbackOwner() {
		e.globalsMu.Lock()
		e.globals[name] = value
		e.vm.Set(name, value)
		e.globalsMu.Unlock()
		return
	}
	e.runtime.adapter.Submit(func(rt *goja.Runtime) {
		e.globalsMu.Lock()
		e.globals[name] = value
		rt.Set(name, value)
		e.globalsMu.Unlock()
	})
}

func (e *Engine) QueueGetGlobal(name string, callback func(value any)) {
	if e.Loop() != nil && e.Loop().IsCallbackOwner() {
		e.globalsMu.Lock()
		val := e.vm.Get(name)
		e.globalsMu.Unlock()
		var result any
		if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
			result = nil
		} else {
			result = val.Export()
		}
		callback(result)
		return
	}
	e.runtime.adapter.Submit(func(rt *goja.Runtime) {
		e.globalsMu.Lock()
		val := rt.Get(name)
		e.globalsMu.Unlock()
		var result any
		if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
			result = nil
		} else {
			result = val.Export()
		}
		callback(result)
	})
}

// SetGlobal sets a global variable in the JavaScript runtime.
//
// THREADING: This method is now owner-safe and may be called from any goroutine.
// When called on the event loop goroutine it executes inline; otherwise it
// submits to the loop via adapter.Submit and returns immediately (fire-and-forget).
// Ordering with subsequent ExecuteScript is preserved because both are queued
// to the same loop in submission order. For synchronous thread-safe access
// with result, use QueueSetGlobal or Runtime.SetGlobal.
func (e *Engine) SetGlobal(name string, value any) {
	if e.Loop() != nil && e.Loop().IsCallbackOwner() {
		e.globalsMu.Lock()
		e.globals[name] = value
		e.vm.Set(name, value)
		e.globalsMu.Unlock()
		return
	}
	e.runtime.adapter.Submit(func(rt *goja.Runtime) {
		e.globalsMu.Lock()
		e.globals[name] = value
		rt.Set(name, value)
		e.globalsMu.Unlock()
	})
}

// GetGlobal retrieves a global variable from the JavaScript runtime.
// Returns nil if the variable is not defined or is undefined.
//
// THREADING: This method is now owner-safe and may be called from any goroutine.
// When called on the event loop goroutine it executes inline; otherwise it
// submits to the loop and blocks until the result is available. For callback-
// based async access, use QueueGetGlobal.
func (e *Engine) GetGlobal(name string) any {
	if e.Loop() != nil && e.Loop().IsCallbackOwner() {
		e.globalsMu.Lock()
		val := e.vm.Get(name)
		e.globalsMu.Unlock()
		if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
			return nil
		}
		return val.Export()
	}
	var result any
	if err := e.executeOnLoop(func(rt *goja.Runtime) error {
		e.globalsMu.Lock()
		val := rt.Get(name)
		e.globalsMu.Unlock()
		if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
			result = nil
		} else {
			result = val.Export()
		}
		return nil
	}); err != nil {
		slog.Debug("engine global lookup failed", "name", name, "error", err)
		return nil
	}
	return result
}

// Stdout returns the engine's stdout writer.
func (e *Engine) Stdout() io.Writer {
	return e.stdout
}

// Stderr returns the engine's stderr writer.
func (e *Engine) Stderr() io.Writer {
	return e.stderr
}

// LoadScript loads a JavaScript script from a file.
func (e *Engine) LoadScript(name, path string) (*Script, error) {
	content, err := readFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read script %s: %w", name, err)
	}

	script := &Script{
		Name:    name,
		Path:    path,
		Content: content,
	}

	e.globalsMu.Lock()
	e.scripts = append(e.scripts, script)
	e.globalsMu.Unlock()
	return script, nil
}

// LoadScriptString loads a JavaScript script from a string.
func (e *Engine) LoadScriptString(name, content string) *Script {
	script := &Script{
		Name:    name,
		Path:    "<string>",
		Content: content,
	}

	e.globalsMu.Lock()
	e.scripts = append(e.scripts, script)
	e.globalsMu.Unlock()
	return script
}

// ExecuteScript executes a script in the engine.
//
// THREADING: This method directly accesses the goja.Runtime without going through
// the event loop. It is designed for synchronous script execution and must ONLY be called:
//   - During engine initialization (before any async operations)
//   - From within the main goroutine during startup
//   - From within existing script execution context (already on event loop goroutine)
//
// For async-safe script loading, use Runtime.LoadScript instead.
//
// All Goja VM access is serialized on the event loop goroutine. This prevents
// concurrent VM access when a script starts BubbleTea (whose goroutines also
// use the VM via RunJSSync). Panic recovery and deferred function execution
// also run on the event loop goroutine for the same reason.
//
// Uses executeOnLoop (no timeout) rather than RunSync because scripts
// may perform significant synchronous work (repeated exec.execv calls etc.)
// that exceeds the 5-second RunSync timeout.
//
// If the script starts a BubbleTea program via tea.run() (which is
// non-blocking), ExecuteScript automatically blocks on WaitForProgram()
// so the calling goroutine stays alive until the TUI exits. This wait
// happens on the CALLING goroutine (not the event loop), so the event loop
// remains free for BubbleTea's RunJSSync callbacks.
func (e *Engine) ExecuteScript(script *Script) error {
	// Create execution context for this script
	ctx := &ExecutionContext{
		engine: e,
		script: script,
		name:   script.Name,
	}

	// Execute the script on the event loop
	if err := e.executeOnLoop(func(vm *goja.Runtime) (execErr error) {
		// Always run deferred functions on exit (even if a panic occurs).
		// Runs ON the event loop goroutine so deferred JS callbacks do not
		// race with any concurrent BubbleTea VM access.
		defer func() {
			if dErr := ctx.runDeferred(); dErr != nil {
				if execErr != nil {
					execErr = fmt.Errorf("execution error: %w; deferred error: %w", execErr, dErr)
				} else {
					execErr = dErr
				}
			}
		}()

		// Recover from panics ON the event loop goroutine where script
		// execution actually happens. A recover() on the calling goroutine
		// cannot catch panics that occur here.
		defer func() {
			if r := recover(); r != nil {
				stackTrace := string(debug.Stack())
				panicErr := &scriptPanicError{
					Value:      r,
					StackTrace: stackTrace,
					ScriptName: script.Name,
				}
				execErr = panicErr
				_, _ = fmt.Fprintf(e.stderr, "\n[PANIC] Script execution panic in %q:\n  %v\n\nStack Trace:\n%s\n", script.Name, r, stackTrace)
			}
		}()

		// Set up the execution context in JavaScript
		if setErr := e.setExecutionContext(ctx); setErr != nil {
			return fmt.Errorf("failed to set script execution context: %w", setErr)
		}

		// Execute the script. For file-based scripts, compile with the absolute file path
		// as the source name. This makes goja_nodejs's getCurrentModulePath() resolve the
		// correct directory for relative require() calls, while keeping the script in global
		// scope (no module wrapper) so existing behavior is fully preserved.
		// For inline/embedded scripts, use RunString as before.
		if script.Path != "" && script.Path != "<string>" {
			absPath, absErr := filepath.Abs(script.Path)
			if absErr != nil {
				return fmt.Errorf("failed to resolve script path: %w", absErr)
			}
			content := script.Content
			if len(content) >= 2 && content[0] == '#' && content[1] == '!' {
				content = "//" + content[2:]
			}
			prg, compileErr := goja.Compile(absPath, content, false)
			if compileErr != nil {
				return fmt.Errorf("script compilation failed: %w", compileErr)
			}
			if _, runErr := vm.RunProgram(prg); runErr != nil {
				// process.exit interrupts the running program with a
				// processExitSignal sentinel carrying the Node exit code.
				// That is a clean stop, not a script failure: the code is
				// settled on the adapter and readable after Wait.
				var exitSignal gojaEventloop.ProcessExitSignal
				if errors.As(runErr, &exitSignal) {
					return nil
				}
				return fmt.Errorf("script execution failed: %w", runErr)
			}
		} else {
			_, runErr := vm.RunString(script.Content)
			if runErr != nil {
				var exitSignal gojaEventloop.ProcessExitSignal
				if errors.As(runErr, &exitSignal) {
					return nil
				}
				return fmt.Errorf("script execution failed: %w", runErr)
			}
		}
		return nil
	}); err != nil {
		return err
	}

	// If the script started a BubbleTea program (via non-blocking tea.run()),
	// block the calling goroutine until it exits. This runs on the CALLING
	// goroutine, NOT the event loop, so BubbleTea's RunJSSync callbacks
	// can still be processed. Without this, the caller returns immediately
	// and the BubbleTea TUI runs orphaned in the background.
	if mgr := e.bubbleteaManager; mgr != nil {
		if waitErr := mgr.WaitForProgram(); waitErr != nil {
			return waitErr
		}

		// After BubbleTea exits, execute any registered post-exit callback
		// on the event loop. This allows JS to defer exit/shell decisions
		// until AFTER BubbleTea processes user input (e.g., 'q' to quit
		// vs 's' to drop to shell). Without this, non-blocking tea.run()
		// returns immediately and JS can't distinguish exit reasons.
		if cbErr := e.executeOnLoop(func(vm *goja.Runtime) error {
			cb := vm.Get("__postBubbleTeaExit")
			if cb == nil || goja.IsUndefined(cb) || goja.IsNull(cb) {
				return nil
			}
			// Clear before execution to prevent re-entry on next command.
			vm.Set("__postBubbleTeaExit", goja.Undefined())
			if fn, ok := goja.AssertFunction(cb); ok {
				if _, err := fn(goja.Undefined()); err != nil {
					return fmt.Errorf("post-BubbleTea exit callback failed: %w", err)
				}
			}
			return nil
		}); cbErr != nil && !errors.Is(cbErr, goeventloop.ErrLoopTerminated) {
			// A script that settled an exit code (process.exit /
			// process.exitCode) terminated the loop by design; the
			// post-exit callback then has nothing to run on. Node's
			// process.exit behaves the same way — pending work is dropped.
			return cbErr
		}
	}

	return nil
}

// Wait blocks until the event loop naturally exits.
func (e *Engine) Wait() {
	if e.runtime != nil {
		e.runtime.Wait()
	}
}

// executeOnLoop submits a function to the event loop and blocks until it
// completes. Unlike RunSync this has NO timeout — scripts can take
// arbitrarily long to execute.
//
// If the caller is already on the event loop goroutine, fn is executed
// directly to prevent deadlock (Submit + blocking receive would deadlock
// the single-threaded event loop).
func (e *Engine) executeOnLoop(fn func(*goja.Runtime) error) error {
	loop := e.Loop()
	if loop == nil {
		return errors.New("event loop not available")
	}
	ctx := e.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if loop.IsCallbackOwner() {
		return executeEngineCallback(fn, e.vm)
	}

	errCh := make(chan error, 1)
	var state atomic.Uint32 // 0 pending, 1 running, 2 cancelled
	submitErr := loop.Submit(func() {
		if !state.CompareAndSwap(0, 1) {
			if err := ctx.Err(); err != nil {
				errCh <- err
			} else {
				errCh <- errors.New("engine stopped before script started")
			}
			return
		}
		errCh <- executeEngineCallback(fn, e.vm)
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
	case <-e.runtime.Done():
		if state.CompareAndSwap(0, 2) {
			return errors.New("runtime stopped before script completion")
		}
		err := <-errCh
		if err == nil {
			return errors.New("runtime stopped before script completion")
		}
		return err
	}
}

func executeEngineCallback(fn func(*goja.Runtime) error, vm *goja.Runtime) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("engine callback panicked: %v", recovered)
		}
	}()
	return fn(vm)
}

// Logger returns the engine's logger.
func (e *Engine) Logger() *slog.Logger {
	return e.logger.Logger()
}

// GetTUIManager returns the TUI manager for this engine.
func (e *Engine) GetTUIManager() *TUIManager {
	return e.tuiManager
}

// BubbleteaManager returns the BubbleTea manager for this engine.
// Used to call WaitForProgram() after starting a BubbleTea program.
func (e *Engine) BubbleteaManager() builtin.BubbleteaManager {
	return e.bubbleteaManager
}

// GetTerminalReader returns the shared terminal reader.
// This implements builtin.TerminalOpsProvider.
func (e *Engine) GetTerminalReader() io.Reader {
	if e.terminalIO == nil {
		return nil
	}
	return e.terminalIO.TUIReader
}

// GetTerminalWriter returns the shared terminal writer.
// This implements builtin.TerminalOpsProvider.
func (e *Engine) GetTerminalWriter() io.Writer {
	if e.terminalIO == nil {
		return nil
	}
	return e.terminalIO.TUIWriter
}

// Loop returns the shared event loop.
// This implements builtin.EventLoopProvider.
func (e *Engine) Loop() *goeventloop.Loop {
	if e.runtime == nil {
		return nil
	}
	return e.runtime.Loop()
}

// Runtime returns the goja.Runtime for JavaScript execution.
// This implements builtin.EventLoopProvider.
func (e *Engine) Runtime() *goja.Runtime {
	return e.vm
}

// Registry returns the require.Registry for module registration.
// This implements builtin.EventLoopProvider.
func (e *Engine) Registry() *require.Registry {
	return e.registry
}

// Adapter returns the goja-eventloop adapter for promise-based async operations.
// This implements builtin.EventLoopProvider.
func (e *Engine) Adapter() *gojaEventloop.Adapter {
	if e.runtime == nil {
		return nil
	}
	return e.runtime.Adapter()
}

// ExitCode reports the script's settled Node exit status: the code published
// by process.exit, by an assignment to process.exitCode, or by a fatal path.
// The ok result is false when the script never set an exit code, in which case
// the process should exit with whatever the caller's own outcome dictates.
// Call it after Wait.
func (e *Engine) ExitCode() (code int, ok bool) {
	adapter := e.Adapter()
	if adapter == nil {
		return 0, false
	}
	return adapter.ExitCode()
}

// DeliverSignal delivers a POSIX-style signal name ("SIGINT", "SIGTERM") to
// the script the way Node would: it submits a loop job that calls
// process.emit(name) and reports whether any listener received it. The emit
// dispatch is synchronous, so by the time this returns the script's own
// signal handler has already run and any process.exit it performed has taken
// effect. When no listener exists Node would terminate the process by
// default — the caller implements that fallback (plus the second-signal force
// close) itself. A false result also covers a missing or unusable process
// object and submit failures, which are all no-listener cases from the
// script's point of view.
//
// A listener that calls process.exit interrupts the emit with a
// gojaEventloop.ProcessExitSignal. That is a delivered signal — the
// listener ran and decided the process should stop with its own exit code —
// so it reports true instead of the no-listener fallback, whose force
// cancel would otherwise override the script's exit status with 128+N.
func (e *Engine) DeliverSignal(name string) bool {
	delivered := false
	err := e.executeOnLoop(func(rt *goja.Runtime) error {
		procVal := rt.Get("process")
		if procVal == nil || goja.IsUndefined(procVal) || goja.IsNull(procVal) {
			return nil
		}
		procObj, ok := procVal.(*goja.Object)
		if !ok {
			return nil
		}
		emit, ok := goja.AssertFunction(procObj.Get("emit"))
		if !ok {
			return nil
		}
		result, err := emit(procObj, rt.ToValue(name))
		if err != nil {
			// The only interrupt an emit listener can raise is a
			// process.exit from inside the handler; anything else is a
			// genuine delivery failure.
			var exitSignal gojaEventloop.ProcessExitSignal
			if errors.As(err, &exitSignal) {
				delivered = true
				return nil
			}
			return err
		}
		delivered = result != nil && result.ToBoolean()
		return nil
	})
	if err != nil {
		e.Logger().Debug("signal delivery failed", "signal", name, "error", err)
		return false
	}
	return delivered
}

// Promisify executes a function in a goroutine and returns a Future.
// This is the preferred way to keep the event loop alive during async operations.
// The future resolution/rejection happens on the event loop goroutine.
// The returned future is NOT exposed to JavaScript - it's for Go-level async coordination.
// This implements builtin.EventLoopProvider.
func (e *Engine) Promisify(ctx context.Context, fn func(ctx context.Context) (any, error)) goeventloop.Future {
	if e.runtime == nil {
		panic("engine runtime is nil")
	}
	return e.runtime.Promisify(ctx, fn)
}

// GetScripts returns a snapshot of all loaded scripts. The returned slice is
// a copy — callers can safely iterate it without racing with LoadScript
// appends. Protected by globalsMu to synchronize with LoadScript/LoadScriptString.
func (e *Engine) GetScripts() []*Script {
	e.globalsMu.RLock()
	defer e.globalsMu.RUnlock()
	return slices.Clone(e.scripts)
}

// Close cleans up the engine resources. It is idempotent — subsequent calls
// return nil without re-cleaning.
func (e *Engine) Close() error {
	// Idempotency guard: ensure Close runs exactly once. Without this,
	// a double-close would re-close the TUI manager, btBridge, etc.
	if !e.closed.CompareAndSwap(false, true) {
		return nil
	}

	// Close TUI manager if it exists (this persists state)
	if e.tuiManager != nil {
		if err := e.tuiManager.Close(); err != nil {
			// Log error but continue cleanup
			if e.stderr != nil {
				_, _ = fmt.Fprintf(e.stderr, "Warning: failed to close TUI manager: %v\n", err)
			}
		}
	}

	// Stop the behavior tree bridge if it exists.
	// This stops the internal bt.Manager and any running tickers.
	if e.btBridge != nil {
		e.btBridge.Stop()
	}

	// Close() bubblezone manager if it exists to stop zone worker goroutines.
	// This prevents resource leaks which can cause tests to timeout.
	if e.bubblezoneManager != nil {
		e.bubblezoneManager.Close()
	}

	if e.stateRefreshDispatcher != nil {
		e.stateRefreshDispatcher.Close()
	}

	// NOTE: We intentionally do NOT close terminalIO here.
	// TerminalIO wraps stdin/stdout which are process-owned resources.
	// Each subsystem (go-prompt, bubbletea) manages its own terminal
	// state lifecycle independently. Attempting to close terminalIO causes
	// "bad file descriptor" errors because go-prompt's reader has already
	// been closed when the prompt loop exits.

	// Close the runtime (this stops the event loop).
	// Runtime.Close() cancels the loop context and waits for the loop
	// goroutine to exit (<-rt.done).
	if e.runtime != nil {
		if err := e.runtime.Close(); err != nil {
			if e.stderr != nil {
				_, _ = fmt.Fprintf(e.stderr, "Warning: failed to close runtime: %v\n", err)
			}
		}
	}

	// NOTE: We intentionally do NOT nil e.vm or e.scripts here. Setting
	// e.vm = nil without synchronization races with executeOnLoop's closure
	// which reads e.vm — the race detector flags this as a DATA RACE under
	// concurrent test execution. The event loop is already stopped by
	// runtime.Close(), so no new reads will occur after this point. Any
	// in-flight executeOnLoop captures e.vm locally (defense-in-depth). The
	// GC reclaims vm/scripts when the Engine is no longer referenced.
	return nil
}

// RegisterNativeModule registers a native module loader with the require registry.
func (e *Engine) RegisterNativeModule(name string, loader func(*goja.Runtime, *goja.Object)) {
	e.registry.RegisterNativeModule(name, loader)
}

const jsGlobalContextName = "ctx"

func (e *Engine) setExecutionContext(ctx *ExecutionContext) error {
	if ctx == nil {
		panic("execution context cannot be nil")
	}
	return e.vm.Set(jsGlobalContextName, map[string]any{
		"run":    ctx.Run,
		"defer":  ctx.Defer,
		"log":    ctx.Log,
		"logf":   ctx.Logf,
		"error":  ctx.Error,
		"errorf": ctx.Errorf,
		"fatal":  ctx.Fatal,
		"fatalf": ctx.Fatalf,
		"failed": ctx.Failed,
		"name":   ctx.Name,
	})
}

// setupGlobals sets up the global JavaScript environment.
func (e *Engine) setupGlobals() {
	// Context management functions
	_ = e.vm.Set("context", map[string]any{
		"addPath":       e.jsContextAddPath,
		"removePath":    e.jsContextRemovePath,
		"listPaths":     e.jsContextListPaths,
		"getPath":       e.jsContextGetPath,
		"refreshPath":   e.jsContextRefreshPath,
		"toTxtar":       e.jsContextToTxtar,
		"fromTxtar":     e.jsContextFromTxtar,
		"getStats":      e.jsContextGetStats,
		"filterPaths":   e.jsContextFilterPaths,
		"getFilesByExt": e.jsContextGetFilesByExtension,
	})

	// Logging functions (application logs)
	_ = e.vm.Set("log", map[string]any{
		"debug":      e.jsLogDebug,
		"info":       e.jsLogInfo,
		"warn":       e.jsLogWarn,
		"error":      e.jsLogError,
		"printf":     e.jsLogPrintf,
		"getLogs":    e.jsGetLogs,
		"clearLogs":  e.jsLogClear,
		"searchLogs": e.jsLogSearch,
	})

	// Terminal output functions (separate from logs)
	_ = e.vm.Set("output", map[string]any{
		"print":               e.jsOutputPrint,
		"printf":              e.jsOutputPrintf,
		"_setTUIOutputActive": e.jsSetTUIOutputActive,
		"toClipboard":         e.jsOutputToClipboard,
		"fromClipboard":       e.jsOutputFromClipboard,
	})

	// TUI and Mode management functions
	_ = e.vm.Set("tui", map[string]any{
		"registerMode":       e.tuiManager.jsRegisterMode,
		"switchMode":         e.tuiManager.jsSwitchMode,
		"getCurrentMode":     e.tuiManager.jsGetCurrentMode,
		"registerCommand":    e.tuiManager.jsRegisterCommand,
		"listModes":          e.tuiManager.jsListModes,
		"createState":        e.jsCreateState,
		"createPrompt":       e.tuiManager.jsCreatePrompt,
		"runPrompt":          e.tuiManager.jsRunPrompt,
		"registerCompleter":  e.tuiManager.jsRegisterCompleter,
		"setCompleter":       e.tuiManager.jsSetCompleter,
		"registerKeyBinding": e.tuiManager.jsRegisterKeyBinding,
		// requestExit signals that the shell loop should exit after the current command completes.
		// This is checked by the exit checker configured on the prompt.
		"requestExit": func() {
			e.tuiManager.SetExitRequested(true)
		},
		// isExitRequested returns whether an exit has been requested.
		"isExitRequested": func() bool {
			return e.tuiManager.IsExitRequested()
		},
		// clearExitRequest clears the exit request flag.
		"clearExitRequest": func() {
			e.tuiManager.SetExitRequested(false)
		},
		// reset: perform an archive+reset and return the archive path (if any) or throw on error
		"reset": func() (string, error) {
			if e.tuiManager == nil {
				return "", fmt.Errorf("tui manager not available")
			}
			return e.tuiManager.resetAllState()
		},
	})
}

// readFile reads a file and returns its content as a string.
func readFile(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

// shebangStrippingLoader is a SourceLoader that strips shebang lines from JavaScript files.
// This matches Node.js behavior: if a file starts with "#!", the shebang is replaced with
// a comment ("//") to preserve line numbers while making the source valid JavaScript.
func shebangStrippingLoader(filename string) ([]byte, error) {
	data, err := require.DefaultSourceLoader(filename)
	if err != nil {
		return nil, err
	}
	if len(data) >= 2 && data[0] == '#' && data[1] == '!' {
		// Replace "#!" with "//" to turn the shebang into a JS comment.
		// This preserves the line so line numbers in error messages stay correct.
		data[0] = '/'
		data[1] = '/'
	}
	return data, nil
}

// plainStdPrinter is a console.Printer whose stdout channel writes verbatim
// (no log.LstdFlags timestamp), so machine-readable script output stays
// parseable; warnings and errors go to stderr through the default logger so
// they remain attributed. Constructed per runtime via newPlainStdPrinter —
// no package-level state.
type plainStdPrinter struct {
	log func(string)
}

func newPlainStdPrinter() console.Printer {
	return plainStdPrinter{log: func(s string) { _, _ = fmt.Fprintln(os.Stdout, s) }}
}

func (p plainStdPrinter) Log(s string)   { p.log(s) }
func (p plainStdPrinter) Warn(s string)  { _, _ = fmt.Fprintln(os.Stderr, s) }
func (p plainStdPrinter) Error(s string) { _, _ = fmt.Fprintln(os.Stderr, s) }
