package scripting

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"runtime/debug"
	"sync"
	"sync/atomic"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
	"github.com/dop251/goja_nodejs/require"
	"github.com/joeycumines/one-shot-man/internal/builtin"
	"github.com/joeycumines/one-shot-man/internal/builtin/bt"
	tviewmod "github.com/joeycumines/one-shot-man/internal/builtin/tview"
	"github.com/joeycumines/one-shot-man/internal/goroutineid"
)

// Engine represents a JavaScript scripting engine with deferred execution capabilities.
// It supports CommonJS modules via a global `require` function, which can load:
// - Native Go modules with the "osm:" prefix (e.g., require("osm:utils")).
// - Absolute file paths (e.g., require("/path/to/module.js")).
// - Relative file paths (e.g., require("./module.js")).
//
// The Engine now uses a shared Runtime with an event loop for all JavaScript execution,
// enabling proper async/Promise support and safe integration with bt.

// ScriptPanicError is a structured error type for script panics.
// It implements the error interface and provides structured access to panic details
// for programmatic consumption by callers.
//
// Example usage:
//
//	if err := engine.ExecuteScript(script); err != nil {
//	    var panicErr *ScriptPanicError
//	    if errors.As(err, &panicErr) {
//	        log.Printf("script %q panicked: %v", panicErr.ScriptName, panicErr.Value)
//	        // panicErr.StackTrace contains the full stack trace
//	    }
//	}
type ScriptPanicError struct {
	// Value is the value passed to panic() in the script
	Value any
	// StackTrace contains the Go runtime stack trace at the time of panic
	StackTrace string
	// ScriptName is the name of the script that panicked
	ScriptName string
}

// Error returns a human-readable description of the panic error.
func (e *ScriptPanicError) Error() string {
	return fmt.Sprintf("script %q panicked: %v", e.ScriptName, e.Value)
}

// Unwrap returns the underlying panic value for errors.Is/As compatibility.
// Note: The panic value may not implement the error interface, so this returns
// the value directly when it does, or wraps it in an error when it doesn't.
func (e *ScriptPanicError) Unwrap() error {
	if err, ok := e.Value.(error); ok {
		return err
	}
	return nil
}

type Engine struct {
	runtime              *Runtime          // Shared runtime with event loop
	vm                   *goja.Runtime     // Direct VM reference (for sync operations)
	registry             *require.Registry // CommonJS require registry
	scripts              []*Script
	ctx                  context.Context
	stdout               io.Writer
	stderr               io.Writer
	globals              map[string]interface{}
	globalsMu            sync.RWMutex // Protects globals map access (C5 fix)
	testMode             bool
	threadCheckMode      bool  // If true, SetGlobal/GetGlobal panic on wrong goroutine
	eventLoopGoroutineID int64 // Captured at initialization for thread checking (atomic)
	tuiManager           *TUIManager
	tviewManager         *tviewmod.Manager
	contextManager       *ContextManager
	logger               *TUILogger
	terminalIO           *TerminalIO               // Shared terminal I/O for all TUI subsystems
	bubbleteaManager     builtin.BubbleteaManager  // For sending state refresh messages to running TUI
	btBridge             *bt.Bridge                // Behavior tree bridge for JS integration
	bubblezoneManager    builtin.BubblezoneManager // Zone-based mouse hit-testing for BubbleTea
}

// Script represents a JavaScript script with metadata.
type Script struct {
	Name        string
	Path        string
	Content     string
	Description string
}

// NewEngine creates a new JavaScript scripting engine.
// For test isolation and to avoid data races, use NewEngineWithConfig instead.
func NewEngine(ctx context.Context, stdout, stderr io.Writer) (*Engine, error) {
	return NewEngineWithConfig(ctx, stdout, stderr, "", "")
}

// NewEngineWithConfig creates a new JavaScript scripting engine with explicit session configuration.
// sessionID and store parameters override environment-based discovery and avoid data races.
func NewEngineWithConfig(ctx context.Context, stdout, stderr io.Writer, sessionID, store string) (*Engine, error) {
	return NewEngineDetailed(ctx, stdout, stderr, sessionID, store, nil, 0, slog.LevelInfo)
}

// NewEngineDetailed creates a new JavaScript scripting engine with full configuration options.
// logFile: optional writer for log output (JSON).
// logBufferSize: size of the in-memory log buffer (default 1000 if <= 0).
// logLevel: minimum log level to capture (e.g. slog.LevelDebug).
func NewEngineDetailed(ctx context.Context, stdout, stderr io.Writer, sessionID, store string, logFile io.Writer, logBufferSize int, logLevel slog.Level) (*Engine, error) {
	// Get current working directory for context manager
	workingDir, err := os.Getwd()
	if err != nil {
		workingDir = "."
	}

	contextManager, err := NewContextManager(workingDir)
	if err != nil {
		return nil, fmt.Errorf("failed to create context manager: %w", err)
	}

	// Create shared terminal I/O for all TUI subsystems.
	// This is the single source of truth for terminal state management.
	terminalIO := NewTerminalIOStdio()

	// Create the require registry first - it will be shared
	registry := require.NewRegistry()

	// Create the shared Runtime with event loop
	// The Runtime owns the event loop and provides thread-safe JS execution
	runtime, err := NewRuntimeWithRegistry(ctx, registry)
	if err != nil {
		return nil, fmt.Errorf("failed to create runtime: %w", err)
	}

	// Get a direct reference to the VM for sync operations
	// Note: All script execution should still go through the event loop for async safety
	var vm *goja.Runtime
	err = runtime.RunOnLoopSync(func(r *goja.Runtime) error {
		vm = r
		return nil
	})
	if err != nil {
		runtime.Close()
		return nil, fmt.Errorf("failed to get VM reference: %w", err)
	}

	engine := &Engine{
		runtime:        runtime,
		vm:             vm,
		registry:       registry,
		ctx:            ctx,
		stdout:         stdout,
		stderr:         stderr,
		globals:        make(map[string]interface{}),
		contextManager: contextManager,
		logger:         NewTUILogger(stdout, logFile, logBufferSize, logLevel),
		terminalIO:     terminalIO,
	}

	// Capture event loop goroutine ID using atomic store for thread-safe access (C5 fix)
	atomic.StoreInt64(&engine.eventLoopGoroutineID, runtime.eventLoopGoroutineID.Load())

	// Create tview manager WITHOUT terminal ops - let tview/tcell manage its own TTY.
	// Using TerminalIO here conflicts with go-prompt's reader which is already consuming
	// stdin. tcell expects raw os.Stdin access, not go-prompt's buffered reader.
	engine.tviewManager = tviewmod.NewManagerWithTerminal(ctx, nil, nil, nil, nil)

	// Register native Go modules. These are all prefixed with "osm:".
	// Pass through the engine's context and a TUI sink for modules that need them.
	// Pass 'engine' as terminalProvider so bubbletea uses the unified TerminalIO
	// instead of defaulting to raw os.Stdin (which would violate Single Source of Truth).
	// Pass 'engine' as eventLoopProvider so bt shares the event loop.
	registerResult := builtin.Register(ctx, func(msg string) { engine.logger.PrintToTUI(msg) }, engine.registry, engine, engine, engine)
	engine.bubbleteaManager = registerResult.BubbleteaManager
	engine.btBridge = registerResult.BTBridge
	engine.bubblezoneManager = registerResult.BubblezoneManager

	// Enable the `require` function in the runtime (must be done on event loop)
	err = runtime.RunOnLoopSync(func(r *goja.Runtime) error {
		registry.Enable(r)
		return nil
	})
	if err != nil {
		runtime.Close()
		return nil, fmt.Errorf("failed to enable require: %w", err)
	}

	// Create TUI manager with explicit configuration (this also initializes StateManager).
	// Pass the shared terminal reader/writer from terminalIO.
	engine.tuiManager = NewTUIManagerWithConfig(ctx, engine, terminalIO.TUIReader, terminalIO.TUIWriter, sessionID, store)

	// Wire StateManager to send state refresh messages to bubbletea when state changes.
	// This enables the TUI to automatically re-render when external code modifies state.
	// NOTE: The listener is invoked asynchronously (in a goroutine) to avoid blocking
	// the update loop or causing issues with p.Send() being called from within updates.
	if engine.bubbleteaManager != nil && engine.tuiManager.stateManager != nil {
		bubbleteaMgr := engine.bubbleteaManager
		engine.tuiManager.stateManager.AddListener(func(key string) {
			// Send state refresh asynchronously to avoid blocking
			go bubbleteaMgr.SendStateRefresh(key)
		})
	}

	// Register the shared symbols module properly through the require registry
	engine.registry.RegisterNativeModule("osm:sharedStateSymbols", builtin.GetSharedSymbolsLoader(engine.tuiManager))

	// Set up the global context and APIs
	engine.setupGlobals()

	// Interrupt JS execution when context is canceled
	context.AfterFunc(ctx, func() {
		if engine.vm != nil {
			// N.B. It's safe to call Interrupt from another goroutine.
			engine.vm.Interrupt(ctx.Err())
		}
	})

	return engine, nil
}

// SetTestMode enables test mode for the engine.
func (e *Engine) SetTestMode(enabled bool) {
	e.testMode = enabled
}

// QueueSetGlobal queues a SetGlobal operation to be executed on the event loop.
// This is the thread-safe alternative to SetGlobal for use from arbitrary goroutines.
//
// The operation is asynchronous - it will be executed on the event loop but
// this method returns immediately. If you need to wait for completion,
// use Runtime.SetGlobal instead.
//
// For testing/debugging, you can enable strict thread-checking mode which
// will cause SetGlobal/GetGlobal to panic if called from the wrong goroutine.
// See SetThreadCheckMode.
func (e *Engine) QueueSetGlobal(name string, value interface{}) {
	// Queue the VM and local cache update to the event loop for thread safety
	e.runtime.loop.RunOnLoop(func(vm *goja.Runtime) {
		e.globals[name] = value
		vm.Set(name, value)
	})
}

// QueueGetGlobal queues a GetGlobal operation to be executed on the event loop.
// This is the thread-safe alternative to GetGlobal for use from arbitrary goroutines.
//
// The operation is asynchronous - the callback is invoked with the result
// once the operation completes on the event loop.
// If you need synchronous access, use Runtime.GetGlobal instead.
func (e *Engine) QueueGetGlobal(name string, callback func(value interface{})) {
	// Queue the VM read to the event loop for thread safety
	e.runtime.loop.RunOnLoop(func(vm *goja.Runtime) {
		val := vm.Get(name)
		var result interface{}
		if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
			result = nil
		} else {
			result = val.Export()
		}
		callback(result)
	})
}

// SetThreadCheckMode enables or disables strict thread-checking mode.
// When enabled, SetGlobal and GetGlobal will panic if called from a goroutine
// other than the event loop goroutine. This helps catch threading bugs early.
//
// Default is disabled for performance. Enable during testing or debugging.
func (e *Engine) SetThreadCheckMode(enabled bool) {
	e.threadCheckMode = enabled
	if enabled {
		// Capture the event loop goroutine ID using atomic store
		atomic.StoreInt64(&e.eventLoopGoroutineID, goroutineid.Get())
	}
}

// checkEventLoopGoroutine panics if called from the wrong goroutine (when thread checking is enabled).
func (e *Engine) checkEventLoopGoroutine(methodName string) {
	currentID := goroutineid.Get()
	// Use atomic load for thread-safe read of eventLoopGoroutineID
	storedID := atomic.LoadInt64(&e.eventLoopGoroutineID)
	if currentID != storedID {
		panic(fmt.Sprintf("%s called from wrong goroutine: expected %d, got %d. "+
			"Use QueueSetGlobal/QueueGetGlobal or Runtime.SetGlobal/Runtime.GetGlobal for thread-safe access.",
			methodName, storedID, currentID))
	}
}

// SetGlobal sets a global variable in the JavaScript runtime.
//
// THREADING: This method directly accesses the goja.Runtime without going through
// the event loop. It must ONLY be called:
//   - During engine initialization (before any async operations)
//   - From within script execution context (already on event loop goroutine)
//
// For thread-safe global access from arbitrary goroutines, use QueueSetGlobal
// or Runtime.SetGlobal instead.
//
// PANIC: In debug mode (when ThreadCheckMode is enabled), this will panic
// if called from a goroutine other than the event loop goroutine.
func (e *Engine) SetGlobal(name string, value interface{}) {
	if e.threadCheckMode {
		e.checkEventLoopGoroutine("SetGlobal")
	}
	// Use mutex to protect globals map and VM access (C5 fix)
	e.globalsMu.Lock()
	e.globals[name] = value
	e.vm.Set(name, value)
	e.globalsMu.Unlock()
}

// GetGlobal retrieves a global variable from the JavaScript runtime.
// Returns nil if the variable is not defined or is undefined.
//
// THREADING: This method directly accesses the goja.Runtime without going through
// the event loop. It must ONLY be called:
//   - During engine initialization (before any async operations)
//   - From within script execution context (already on event loop goroutine)
//
// For thread-safe global access from arbitrary goroutines, use QueueGetGlobal
// or Runtime.GetGlobal instead.
//
// PANIC: In debug mode (when ThreadCheckMode is enabled), this will panic
// if called from a goroutine other than the event loop goroutine.
func (e *Engine) GetGlobal(name string) interface{} {
	if e.threadCheckMode {
		e.checkEventLoopGoroutine("GetGlobal")
	}
	// Use mutex to protect globals map access (C5 fix)
	e.globalsMu.RLock()
	val := e.vm.Get(name)
	e.globalsMu.RUnlock()
	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		return nil
	}
	return val.Export()
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

	e.scripts = append(e.scripts, script)
	return script, nil
}

// LoadScriptFromString loads a JavaScript script from a string.
func (e *Engine) LoadScriptFromString(name, content string) *Script {
	script := &Script{
		Name:    name,
		Path:    "<string>",
		Content: content,
	}

	e.scripts = append(e.scripts, script)
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
func (e *Engine) ExecuteScript(script *Script) (err error) {
	// Create execution context for this script
	ctx := &ExecutionContext{
		engine: e,
		script: script,
		name:   script.Name,
	}

	// Set up the execution context in JavaScript
	if err = e.setExecutionContext(ctx); err != nil {
		return fmt.Errorf("failed to set script execution context: %w", err)
	}

	// Always run deferred functions on exit (even if a panic occurs)
	defer func() {
		if dErr := ctx.runDeferred(); dErr != nil {
			if err != nil {
				err = fmt.Errorf("execution error: %v; deferred error: %v", err, dErr)
			} else {
				err = dErr
			}
		}
	}()

	// Recover from top-level panics so scripts cannot crash the host
	defer func() {
		if r := recover(); r != nil {
			stackTrace := string(debug.Stack())
			// Create a structured panic error for programmatic consumption
			panicErr := &ScriptPanicError{
				Value:      r,
				StackTrace: stackTrace,
				ScriptName: script.Name,
			}
			err = panicErr
			// Also print to stderr for visibility
			_, _ = fmt.Fprintf(e.stderr, "\n[PANIC] Script execution panic in %q:\n  %v\n\nStack Trace:\n%s\n", script.Name, r, stackTrace)
		}
	}()

	// Execute the script
	if _, runErr := e.vm.RunString(script.Content); runErr != nil {
		return fmt.Errorf("script execution failed: %w", runErr)
	}

	return err
}

// Logger returns the engine's logger.
func (e *Engine) Logger() *slog.Logger {
	return e.logger.Logger()
}

// GetTUIManager returns the TUI manager for this engine.
func (e *Engine) GetTUIManager() *TUIManager {
	return e.tuiManager
}

// GetTViewManager returns the TView manager for this engine.
func (e *Engine) GetTViewManager() *tviewmod.Manager {
	return e.tviewManager
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

// EventLoop returns the shared event loop.
// This implements builtin.EventLoopProvider.
func (e *Engine) EventLoop() *eventloop.EventLoop {
	if e.runtime == nil {
		return nil
	}
	return e.runtime.EventLoop()
}

// Registry returns the require.Registry for module registration.
// This implements builtin.EventLoopProvider.
func (e *Engine) Registry() *require.Registry {
	return e.registry
}

// GetScripts returns all loaded scripts.
func (e *Engine) GetScripts() []*Script {
	return e.scripts
}

// Close cleans up the engine resources.
func (e *Engine) Close() error {
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

	// NOTE: We intentionally do NOT close terminalIO here.
	// TerminalIO wraps stdin/stdout which are process-owned resources.
	// Each subsystem (go-prompt, tview, bubbletea) manages its own terminal
	// state lifecycle independently. Attempting to close terminalIO causes
	// "bad file descriptor" errors because go-prompt's reader has already
	// been closed when the prompt loop exits.

	// Close the runtime (this stops the event loop).
	// We do this after stopping btBridge so any pending JS operations complete.
	if e.runtime != nil {
		e.runtime.Close()
	}

	// Clean up any resources
	e.vm = nil
	e.scripts = nil
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
	return e.vm.Set(jsGlobalContextName, map[string]interface{}{
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
	_ = e.vm.Set("context", map[string]interface{}{
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
	_ = e.vm.Set("log", map[string]interface{}{
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
	_ = e.vm.Set("output", map[string]interface{}{
		"print":  e.jsOutputPrint,
		"printf": e.jsOutputPrintf,
	})

	// TUI and Mode management functions
	_ = e.vm.Set("tui", map[string]interface{}{
		"registerMode":         e.tuiManager.jsRegisterMode,
		"switchMode":           e.tuiManager.jsSwitchMode,
		"getCurrentMode":       e.tuiManager.jsGetCurrentMode,
		"registerCommand":      e.tuiManager.jsRegisterCommand,
		"listModes":            e.tuiManager.jsListModes,
		"createState":          e.jsCreateState,
		"createAdvancedPrompt": e.tuiManager.jsCreateAdvancedPrompt,
		"runPrompt":            e.tuiManager.jsRunPrompt,
		"registerCompleter":    e.tuiManager.jsRegisterCompleter,
		"setCompleter":         e.tuiManager.jsSetCompleter,
		"registerKeyBinding":   e.tuiManager.jsRegisterKeyBinding,
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
