package scripting

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/dop251/goja"
)

// Engine represents a JavaScript scripting engine with deferred execution capabilities.
type Engine struct {
	vm             *goja.Runtime
	scripts        []Script
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
	deferred    []func() error
}

// ExecutionContext provides the execution environment for scripts, similar to testing.T.
type ExecutionContext struct {
	engine   *Engine
	script   *Script
	name     string
	parent   *ExecutionContext
	failed   bool
	output   strings.Builder
	deferred []func()
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
		Name:     name,
		Path:     path,
		Content:  content,
		deferred: make([]func() error, 0),
	}

	e.scripts = append(e.scripts, *script)
	return script, nil
}

// LoadScriptFromString loads a JavaScript script from a string.
func (e *Engine) LoadScriptFromString(name, content string) *Script {
	script := &Script{
		Name:     name,
		Path:     "<string>",
		Content:  content,
		deferred: make([]func() error, 0),
	}

	e.scripts = append(e.scripts, *script)
	return script
}

// ExecuteScript executes a script in the engine.
func (e *Engine) ExecuteScript(script *Script) error {
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

	// Execute the script
	_, err := e.vm.RunString(script.Content)
	if err != nil {
		return fmt.Errorf("script execution failed: %w", err)
	}

	// Execute deferred functions
	return ctx.runDeferred()
}

// Run executes a sub-test, similar to testing.T.Run() (Go-style method for internal use).
func (ctx *ExecutionContext) Run(name string, fn goja.Callable) bool {
	subCtx := &ExecutionContext{
		engine: ctx.engine,
		script: ctx.script,
		name:   fmt.Sprintf("%s/%s", ctx.name, name),
		parent: ctx,
	}

	// Set up the sub-context in JavaScript with both Go-style and JS-style methods
	contextObj := map[string]interface{}{
		// JavaScript-style methods (camelCase)
		"run":    subCtx.Run,
		"defer":  subCtx.Defer,
		"log":    subCtx.Log,
		"logf":   subCtx.Logf,
		"error":  subCtx.Error,
		"errorf": subCtx.Errorf,
		"fatal":  subCtx.Fatal,
		"fatalf": subCtx.Fatalf,
		"failed": subCtx.Failed,
		"name":   subCtx.Name,
	}
	ctx.engine.vm.Set("ctx", contextObj)

	// Execute the test function
	_, err := fn(goja.Undefined())
	if err != nil {
		subCtx.failed = true
		subCtx.Errorf("Test failed: %v", err)
	}

	// Run deferred functions for sub-context
	if err := subCtx.runDeferred(); err != nil {
		subCtx.failed = true
		subCtx.Errorf("Deferred function failed: %v", err)
	}

	// Restore parent context
	if ctx.parent != nil {
		parentObj := map[string]interface{}{
			// JavaScript-style methods (camelCase)
			"run":    ctx.parent.Run,
			"defer":  ctx.parent.Defer,
			"log":    ctx.parent.Log,
			"logf":   ctx.parent.Logf,
			"error":  ctx.parent.Error,
			"errorf": ctx.parent.Errorf,
			"fatal":  ctx.parent.Fatal,
			"fatalf": ctx.parent.Fatalf,
			"failed": ctx.parent.Failed,
			"name":   ctx.parent.Name,
		}
		ctx.engine.vm.Set("ctx", parentObj)
	} else {
		currentObj := map[string]interface{}{
			// JavaScript-style methods
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
		ctx.engine.vm.Set("ctx", currentObj)
	}

	// Report result
	if subCtx.failed {
		ctx.Errorf("Sub-test %s failed", name)
		return false
	}

	ctx.Logf("Sub-test %s passed", name)
	return true
}

// Defer schedules a function to be executed when the current context completes.
func (ctx *ExecutionContext) Defer(fn goja.Callable) {
	ctx.deferred = append(ctx.deferred, func() {
		_, err := fn(goja.Undefined())
		if err != nil {
			ctx.Errorf("Deferred function failed: %v", err)
		}
	})
}

// Log logs a message to the test output (Go-style method for internal use).
func (ctx *ExecutionContext) Log(args ...interface{}) {
	fmt.Fprintf(&ctx.output, "[%s] %s\n", ctx.name, fmt.Sprint(args...))
	if ctx.engine.testMode {
		fmt.Fprintf(ctx.engine.stdout, "[%s] %s\n", ctx.name, fmt.Sprint(args...))
	}
}

// Logf logs a formatted message to the test output (Go-style method for internal use).
func (ctx *ExecutionContext) Logf(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(&ctx.output, "[%s] %s\n", ctx.name, msg)
	if ctx.engine.testMode {
		fmt.Fprintf(ctx.engine.stdout, "[%s] %s\n", ctx.name, msg)
	}
}

// Error marks the current test as failed and logs an error message.
func (ctx *ExecutionContext) Error(args ...interface{}) {
	ctx.failed = true
	msg := fmt.Sprint(args...)
	fmt.Fprintf(&ctx.output, "[%s] ERROR: %s\n", ctx.name, msg)
	fmt.Fprintf(ctx.engine.stderr, "[%s] ERROR: %s\n", ctx.name, msg)
}

// Errorf marks the current test as failed and logs a formatted error message.
func (ctx *ExecutionContext) Errorf(format string, args ...interface{}) {
	ctx.failed = true
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(&ctx.output, "[%s] ERROR: %s\n", ctx.name, msg)
	fmt.Fprintf(ctx.engine.stderr, "[%s] ERROR: %s\n", ctx.name, msg)
}

// Fatal marks the current test as failed and stops execution.
func (ctx *ExecutionContext) Fatal(args ...interface{}) {
	ctx.Error(args...)
	panic("test failed")
}

// Fatalf marks the current test as failed and stops execution with a formatted message.
func (ctx *ExecutionContext) Fatalf(format string, args ...interface{}) {
	ctx.Errorf(format, args...)
	panic("test failed")
}

// Failed reports whether the current test has failed.
func (ctx *ExecutionContext) Failed() bool {
	return ctx.failed
}

// Name returns the name of the current test.
func (ctx *ExecutionContext) Name() string {
	return ctx.name
}

// runDeferred executes all deferred functions for this context.
func (ctx *ExecutionContext) runDeferred() error {
	// Execute deferred functions in reverse order (LIFO)
	for i := len(ctx.deferred) - 1; i >= 0; i-- {
		func() {
			defer func() {
				if r := recover(); r != nil {
					ctx.Errorf("Deferred function panicked: %v", r)
				}
			}()
			ctx.deferred[i]()
		}()
	}

	if ctx.failed {
		return fmt.Errorf("test context failed")
	}
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
		"openEditor":    e.jsSystemOpenEditor,
		"clipboardCopy": e.jsSystemClipboardCopy,
		"readFile":      e.jsSystemReadFile,
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

// getEnv gets an environment variable.
func getEnv(key string) string {
	return os.Getenv(key)
}

// GetTUIManager returns the TUI manager for this engine.
func (e *Engine) GetTUIManager() *TUIManager {
	return e.tuiManager
}

// GetScripts returns all loaded scripts.
func (e *Engine) GetScripts() []Script {
	return e.scripts
}

// Close cleans up the engine resources.
func (e *Engine) Close() error {
	// Clean up any resources
	e.vm = nil
	e.scripts = nil
	return nil
}

// JavaScript API functions for context management

// jsContextAddPath adds a path to the context manager.
func (e *Engine) jsContextAddPath(path string) error {
	return e.contextManager.AddPath(path)
}

// jsContextRemovePath removes a path from the context manager.
func (e *Engine) jsContextRemovePath(path string) error {
	return e.contextManager.RemovePath(path)
}

// jsContextListPaths returns all tracked paths.
func (e *Engine) jsContextListPaths() []string {
	return e.contextManager.ListPaths()
}

// jsContextGetPath returns information about a tracked path.
func (e *Engine) jsContextGetPath(path string) interface{} {
	contextPath, exists := e.contextManager.GetPath(path)
	if !exists {
		return nil
	}
	return contextPath
}

// jsContextRefreshPath refreshes a tracked path.
func (e *Engine) jsContextRefreshPath(path string) error {
	return e.contextManager.RefreshPath(path)
}

// jsContextToTxtar returns the context as a txtar-formatted string.
func (e *Engine) jsContextToTxtar() string {
	return e.contextManager.GetTxtarString()
}

// jsContextFromTxtar loads context from a txtar-formatted string.
func (e *Engine) jsContextFromTxtar(data string) error {
	return e.contextManager.LoadFromTxtarString(data)
}

// jsContextGetStats returns context statistics.
func (e *Engine) jsContextGetStats() map[string]interface{} {
	return e.contextManager.GetStats()
}

// jsContextFilterPaths returns paths matching a pattern.
func (e *Engine) jsContextFilterPaths(pattern string) ([]string, error) {
	return e.contextManager.FilterPaths(pattern)
}

// jsContextGetFilesByExtension returns files with a given extension.
func (e *Engine) jsContextGetFilesByExtension(ext string) []string {
	return e.contextManager.GetFilesByExtension(ext)
}

// JavaScript API functions for logging

// jsLogDebug logs a debug message.
func (e *Engine) jsLogDebug(msg string, attrs ...map[string]interface{}) {
	e.logger.Debug(msg)
}

// jsLogInfo logs an info message.
func (e *Engine) jsLogInfo(msg string, attrs ...map[string]interface{}) {
	e.logger.Info(msg)
}

// jsLogWarn logs a warning message.
func (e *Engine) jsLogWarn(msg string, attrs ...map[string]interface{}) {
	e.logger.Warn(msg)
}

// jsLogError logs an error message.
func (e *Engine) jsLogError(msg string, attrs ...map[string]interface{}) {
	e.logger.Error(msg)
}

// jsLogPrintf logs a formatted message.
func (e *Engine) jsLogPrintf(format string, args ...interface{}) {
	e.logger.Printf(format, args...)
}

// jsGetLogs returns log entries.
func (e *Engine) jsGetLogs(count ...int) interface{} {
	if len(count) > 0 && count[0] > 0 {
		return e.logger.GetRecentLogs(count[0])
	}
	return e.logger.GetLogs()
}

// jsLogClear clears all logs.
func (e *Engine) jsLogClear() {
	e.logger.ClearLogs()
}

// jsLogSearch searches logs for a query.
func (e *Engine) jsLogSearch(query string) interface{} {
	return e.logger.SearchLogs(query)
}

// JavaScript API functions for terminal output

// jsOutputPrint prints to terminal output.
func (e *Engine) jsOutputPrint(msg string) {
	e.logger.PrintToTUI(msg)
}

// jsOutputPrintf prints formatted text to terminal output.
func (e *Engine) jsOutputPrintf(format string, args ...interface{}) {
	e.logger.PrintfToTUI(format, args...)
}

// jsCreatePromptBuilder creates a new prompt builder for JavaScript.
func (e *Engine) jsCreatePromptBuilder(title, description string) map[string]interface{} {
	pb := NewPromptBuilder(title, description)

	return map[string]interface{}{
		// Core methods
		"setTemplate": pb.SetTemplate,
		"setVariable": pb.SetVariable,
		"getVariable": pb.GetVariable,
		"build":       pb.Build,

		// Version management
		"saveVersion":    pb.SaveVersion,
		"getVersion":     pb.GetVersion,
		"restoreVersion": pb.RestoreVersion,
		"listVersions":   pb.ListVersions,

		// Export/Import
		"export": pb.Export,

		// Properties (read-only)
		"getTitle":       func() string { return pb.Title },
		"getDescription": func() string { return pb.Description },
		"getTemplate":    func() string { return pb.Template },
		"getVariables":   func() map[string]interface{} { return pb.Variables },

		// Utility methods
		"preview": func() string {
			return fmt.Sprintf("Title: %s\nDescription: %s\n\nCurrent Prompt:\n%s",
				pb.Title, pb.Description, pb.Build())
		},

		"stats": func() map[string]interface{} {
			return map[string]interface{}{
				"title":       pb.Title,
				"description": pb.Description,
				"versions":    len(pb.History),
				"variables":   len(pb.Variables),
				"hasTemplate": pb.Template != "",
				"lastUpdated": time.Now().Format(time.RFC3339),
			}
		},
	}
}

// ------------------- System JS API -------------------

// jsSystemExec executes a system command and returns an object with stdout, stderr, and exit code.
func (e *Engine) jsSystemExec(cmd string, args ...string) map[string]interface{} {
	c := exec.CommandContext(e.ctx, cmd, args...)
	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr
	// Use process stdio for input to allow interactive commands (e.g., git editor)
	c.Stdin = os.Stdin
	err := c.Run()
	code := 0
	errStr := ""
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			code = exitErr.ExitCode()
		} else {
			code = -1
		}
		errStr = err.Error()
	}
	return map[string]interface{}{
		"stdout":  stdout.String(),
		"stderr":  stderr.String(),
		"code":    code,
		"error":   err != nil,
		"message": errStr,
	}
}

// jsSystemExecv executes a command given as argv array, e.g., ["git","diff","--staged"].
func (e *Engine) jsSystemExecv(argv interface{}) map[string]interface{} {
	if argv == nil {
		return map[string]interface{}{"error": true, "message": "no argv"}
	}
	// Strict: only accept an array of strings
	var parts []string
	if err := e.vm.ExportTo(e.vm.ToValue(argv), &parts); err != nil || len(parts) == 0 {
		return map[string]interface{}{"error": true, "message": "execv expects an array of strings"}
	}
	cmd := parts[0]
	args := []string{}
	if len(parts) > 1 {
		args = parts[1:]
	}
	return e.jsSystemExec(cmd, args...)
}

// jsSystemOpenEditor opens the user's editor ($VISUAL, $EDITOR, fallback vi) on a temp file with initial content,
// then returns the edited content.
func (e *Engine) jsSystemOpenEditor(nameHint string, initialContent string) string {
	if nameHint == "" {
		nameHint = "oneshot"
	}
	tmpFile, err := os.CreateTemp("", fmt.Sprintf("one-shot-man-%s-*.txt", sanitizeFilename(nameHint)))
	if err != nil {
		return initialContent
	}
	path := tmpFile.Name()
	if _, err := tmpFile.WriteString(initialContent); err != nil {
		_ = tmpFile.Close()
		return initialContent
	}
	_ = tmpFile.Close()

	// Choose editor per-OS with sensible defaults
	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		if editor != "" {
			cmd = exec.CommandContext(e.ctx, editor, path)
		} else {
			// notepad blocks until closed and supports a simple CLI
			cmd = exec.CommandContext(e.ctx, "notepad", path)
		}
	default:
		if editor == "" {
			// try nano, then vi, then ed
			if _, err := exec.LookPath("nano"); err == nil {
				editor = "nano"
			} else if _, err := exec.LookPath("vi"); err == nil {
				editor = "vi"
			} else {
				editor = "ed"
			}
		}
		cmd = exec.CommandContext(e.ctx, editor, path)
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = e.stdout
	cmd.Stderr = e.stderr
	if err := cmd.Run(); err != nil {
		// Return initial content if editor failed
		data, _ := os.ReadFile(path)
		_ = os.Remove(path)
		if len(data) > 0 {
			return string(data)
		}
		return initialContent
	}

	data, readErr := os.ReadFile(path)
	_ = os.Remove(path)
	if readErr != nil {
		return initialContent
	}
	return string(data)
}

// jsSystemClipboardCopy copies text to the system clipboard (supports macOS pbcopy; fallback via command)
func (e *Engine) jsSystemClipboardCopy(text string) error {
	// Allow override via env (treated as a shell-like command, basic split on spaces honoring quotes is not trivial;
	// to keep safe, require simple binary + args split by spaces with no quotes)
	if cmdStr := os.Getenv("ONESHOT_CLIPBOARD_CMD"); cmdStr != "" {
		var c *exec.Cmd
		if runtime.GOOS == "windows" {
			c = exec.CommandContext(e.ctx, "cmd", "/c", cmdStr)
		} else {
			c = exec.CommandContext(e.ctx, "/bin/sh", "-c", cmdStr)
		}
		c.Stdin = strings.NewReader(text)
		if err := c.Run(); err == nil {
			return nil
		}
	}

	switch runtime.GOOS {
	case "darwin":
		if _, err := exec.LookPath("pbcopy"); err == nil {
			c := exec.CommandContext(e.ctx, "pbcopy")
			c.Stdin = strings.NewReader(text)
			return c.Run()
		}
	case "windows":
		if _, err := exec.LookPath("clip"); err == nil {
			c := exec.CommandContext(e.ctx, "clip")
			c.Stdin = strings.NewReader(text)
			return c.Run()
		}
	default:
		// Linux / BSDs: try wl-copy, xclip, xsel in that order
		if _, err := exec.LookPath("wl-copy"); err == nil {
			c := exec.CommandContext(e.ctx, "wl-copy")
			c.Stdin = strings.NewReader(text)
			return c.Run()
		}
		if _, err := exec.LookPath("xclip"); err == nil {
			c := exec.CommandContext(e.ctx, "xclip", "-selection", "clipboard")
			c.Stdin = strings.NewReader(text)
			return c.Run()
		}
		if _, err := exec.LookPath("xsel"); err == nil {
			c := exec.CommandContext(e.ctx, "xsel", "--clipboard", "--input")
			c.Stdin = strings.NewReader(text)
			return c.Run()
		}
	}

	// Best-effort fallback: print a notice and write to stdout
	e.logger.PrintfToTUI("[clipboard] No system clipboard utility available; printing content below\n%s", text)
	return nil
}

// jsSystemReadFile reads a file from disk and returns an object with content or error info.
func (e *Engine) jsSystemReadFile(path string) map[string]interface{} {
	if path == "" {
		return map[string]interface{}{"error": true, "message": "empty path", "content": ""}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return map[string]interface{}{"error": true, "message": err.Error(), "content": ""}
	}
	return map[string]interface{}{"error": false, "content": string(data)}
}

// sanitizeFilename produces a filesystem-safe portion for temp filenames
func sanitizeFilename(s string) string {
	// Allow only alphanumeric, dash, underscore, dot; replace others with '-'
	if s == "" {
		return "untitled"
	}
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	out := b.String()
	// Collapse multiple dashes
	for strings.Contains(out, "--") {
		out = strings.ReplaceAll(out, "--", "-")
	}
	out = strings.Trim(out, "-")
	if out == "" {
		out = "untitled"
	}
	return out
}
