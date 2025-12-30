package scripting

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime/debug"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/require"
	"github.com/joeycumines/one-shot-man/internal/builtin"
	tviewmod "github.com/joeycumines/one-shot-man/internal/builtin/tview"
)

// Engine represents a JavaScript scripting engine with deferred execution capabilities.
// It supports CommonJS modules via a global `require` function, which can load:
// - Native Go modules with the "osm:" prefix (e.g., require("osm:utils")).
// - Absolute file paths (e.g., require("/path/to/module.js")).
// - Relative file paths (e.g., require("./module.js")).

type Engine struct {
	vm               *goja.Runtime
	registry         *require.Registry
	scripts          []*Script
	ctx              context.Context
	stdout           io.Writer
	stderr           io.Writer
	globals          map[string]interface{}
	testMode         bool
	tuiManager       *TUIManager
	tviewManager     *tviewmod.Manager
	contextManager   *ContextManager
	logger           *TUILogger
	terminalIO       *TerminalIO              // Shared terminal I/O for all TUI subsystems
	bubbleteaManager builtin.BubbleteaManager // For sending state refresh messages to running TUI
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

	engine := &Engine{
		vm:             goja.New(),
		ctx:            ctx,
		stdout:         stdout,
		stderr:         stderr,
		globals:        make(map[string]interface{}),
		contextManager: contextManager,
		logger:         NewTUILogger(stdout, 1000),
		terminalIO:     terminalIO,
	}

	// Create tview manager WITHOUT terminal ops - let tview/tcell manage its own TTY.
	// Using TerminalIO here conflicts with go-prompt's reader which is already consuming
	// stdin. tcell expects raw os.Stdin access, not go-prompt's buffered reader.
	engine.tviewManager = tviewmod.NewManagerWithTerminal(ctx, nil, nil, nil, nil)

	// Set up CommonJS require support.
	{
		// TODO: Add support for configuring require.WithGlobalFolders, for instance, via an environment variable.
		engine.registry = require.NewRegistry()

		// Register native Go modules. These are all prefixed with "osm:".
		// Pass through the engine's context and a TUI sink for modules that need them.
		// Pass 'engine' as terminalProvider so bubbletea uses the unified TerminalIO
		// instead of defaulting to raw os.Stdin (which would violate Single Source of Truth).
		// This is safe because Fd() no longer triggers lazy initialization - it returns
		// os.Stdin.Fd()/os.Stdout.Fd() when the underlying reader/writer is nil.
		registerResult := builtin.Register(ctx, func(msg string) { engine.logger.PrintToTUI(msg) }, engine.registry, engine, engine)
		engine.bubbleteaManager = registerResult.BubbleteaManager

		// Enable the `require` function in the runtime.
		engine.registry.Enable(engine.vm)
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

// SetGlobal sets a global variable in the JavaScript runtime.
func (e *Engine) SetGlobal(name string, value interface{}) {
	e.globals[name] = value
	e.vm.Set(name, value)
}

// GetGlobal retrieves a global variable from the JavaScript runtime.
// Returns nil if the variable is not defined or is undefined.
func (e *Engine) GetGlobal(name string) interface{} {
	val := e.vm.Get(name)
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
			stackTrace := debug.Stack()
			err = fmt.Errorf("script panicked (fatal error): %v\n\nStack Trace:\n%s", r, string(stackTrace))
			// Also print to stderr for visibility
			_, _ = fmt.Fprintf(e.stderr, "\n[PANIC] Script execution panic:\n  %v\n\nStack Trace:\n%s\n", r, string(stackTrace))
		}
	}()

	// Execute the script
	if _, runErr := e.vm.RunString(script.Content); runErr != nil {
		return fmt.Errorf("script execution failed: %w", runErr)
	}

	return err
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

	// NOTE: We intentionally do NOT close terminalIO here.
	// TerminalIO wraps stdin/stdout which are process-owned resources.
	// Each subsystem (go-prompt, tview, bubbletea) manages its own terminal
	// state lifecycle independently. Attempting to close terminalIO causes
	// "bad file descriptor" errors because go-prompt's reader has already
	// been closed when the prompt loop exits.

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
