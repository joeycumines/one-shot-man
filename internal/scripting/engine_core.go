package scripting

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/dop251/goja"
)

// Engine represents a JavaScript scripting engine with deferred execution capabilities.
type Engine struct {
	vm             *goja.Runtime
	scripts        []*Script
	ctx            context.Context
	stdout         io.Writer
	stderr         io.Writer
	globals        map[string]interface{}
	testMode       bool
	tuiManager     *TUIManager
	contextManager *ContextManager
	logger         *TUILogger
}

// Script represents a JavaScript script with metadata.
type Script struct {
	Name        string
	Path        string
	Content     string
	Description string
}

// NewEngine creates a new JavaScript scripting engine.
func NewEngine(ctx context.Context, stdout, stderr io.Writer) *Engine {
	// Get current working directory for context manager
	workingDir, err := os.Getwd()
	if err != nil {
		workingDir = "."
	}

	engine := &Engine{
		vm:             goja.New(),
		ctx:            ctx,
		stdout:         stdout,
		stderr:         stderr,
		globals:        make(map[string]interface{}),
		contextManager: NewContextManager(workingDir),
		logger:         NewTUILogger(stdout, 1000),
	}

	// Create TUI manager
	engine.tuiManager = NewTUIManager(ctx, engine, os.Stdin, os.Stdout)

	// Set up the global context and APIs
	engine.setupGlobals()

	// Interrupt JS execution when context is cancelled
	go func() {
		<-ctx.Done()
		// It's safe to call Interrupt from another goroutine.
		if engine.vm != nil {
			engine.vm.Interrupt(ctx.Err())
		}
	}()

	return engine
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
	contextObj := map[string]interface{}{
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
	}
	e.vm.Set("ctx", contextObj)

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
			err = fmt.Errorf("script panicked (fatal error): %v", r)
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

// GetScripts returns all loaded scripts.
func (e *Engine) GetScripts() []*Script {
	return e.scripts
}

// Close cleans up the engine resources.
func (e *Engine) Close() error {
	// Clean up any resources
	e.vm = nil
	e.scripts = nil
	return nil
}

// setupGlobals sets up the global JavaScript environment.
func (e *Engine) setupGlobals() {
	// Utility functions
	e.vm.Set("sleep", func(ms int) {
		time.Sleep(time.Duration(ms) * time.Millisecond)
	})

	e.vm.Set("env", func(key string) string {
		return getEnv(key)
	})

	// Context management functions
	e.vm.Set("context", map[string]interface{}{
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
	e.vm.Set("log", map[string]interface{}{
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
	e.vm.Set("output", map[string]interface{}{
		"print":  e.jsOutputPrint,
		"printf": e.jsOutputPrintf,
	})

	// TUI and Mode management functions
	e.vm.Set("tui", map[string]interface{}{
		"registerMode":         e.tuiManager.jsRegisterMode,
		"switchMode":           e.tuiManager.jsSwitchMode,
		"getCurrentMode":       e.tuiManager.jsGetCurrentMode,
		"setState":             e.tuiManager.jsSetState,
		"getState":             e.tuiManager.jsGetState,
		"registerCommand":      e.tuiManager.jsRegisterCommand,
		"listModes":            e.tuiManager.jsListModes,
		"createPromptBuilder":  e.jsCreatePromptBuilder,
		"createAdvancedPrompt": e.tuiManager.jsCreateAdvancedPrompt,
		"runPrompt":            e.tuiManager.jsRunPrompt,
		"registerCompleter":    e.tuiManager.jsRegisterCompleter,
		"setCompleter":         e.tuiManager.jsSetCompleter,
		"registerKeyBinding":   e.tuiManager.jsRegisterKeyBinding,
	})

	// System utilities: exec, editor, clipboard
	e.vm.Set("system", map[string]interface{}{
		"exec":          e.jsSystemExec,
		"execv":         e.jsSystemExecv,
		"parseArgv":     e.jsSystemParseArgv,
		"openEditor":    e.jsSystemOpenEditor,
		"clipboardCopy": e.jsSystemClipboardCopy,
		"readFile":      e.jsSystemReadFile,
		"fileExists":    e.jsSystemFileExists,
	})
}

// jsSystemFileExists checks whether a file or directory exists at the given path.
func (e *Engine) jsSystemFileExists(path string) bool {
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

// readFile reads a file and returns its content as a string.
func readFile(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

// getEnv gets an environment variable.
func getEnv(key string) string {
	return os.Getenv(key)
}
