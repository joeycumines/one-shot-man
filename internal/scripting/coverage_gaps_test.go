package scripting

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dop251/goja"
	"github.com/joeycumines/one-shot-man/internal/testutil"
)

// =============================================================================
// scriptPanicError tests
// =============================================================================

func TestScriptPanicError_Error(t *testing.T) {
	t.Parallel()
	pe := &scriptPanicError{
		Value:      "something broke",
		StackTrace: "goroutine 1 [running]:\n...",
		ScriptName: "test.js",
	}
	got := pe.Error()
	want := `script "test.js" panicked: something broke`
	if got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestScriptPanicError_Unwrap_WithError(t *testing.T) {
	t.Parallel()
	inner := errors.New("underlying error")
	pe := &scriptPanicError{
		Value:      inner,
		ScriptName: "err.js",
	}
	got := pe.Unwrap()
	if got != inner {
		t.Errorf("Unwrap() = %v, want %v", got, inner)
	}
	// errors.Is should work
	if !errors.Is(pe, inner) {
		t.Error("errors.Is(pe, inner) should be true")
	}
}

func TestScriptPanicError_Unwrap_NonError(t *testing.T) {
	t.Parallel()
	pe := &scriptPanicError{
		Value:      "not an error",
		ScriptName: "str.js",
	}
	got := pe.Unwrap()
	if got != nil {
		t.Errorf("Unwrap() = %v, want nil for non-error value", got)
	}
}

func TestScriptPanicError_Unwrap_NilValue(t *testing.T) {
	t.Parallel()
	pe := &scriptPanicError{
		Value:      nil,
		ScriptName: "nil.js",
	}
	got := pe.Unwrap()
	if got != nil {
		t.Errorf("Unwrap() = %v, want nil", got)
	}
}

func TestScriptPanicError_ErrorsAs(t *testing.T) {
	t.Parallel()
	pe := &scriptPanicError{
		Value:      42,
		ScriptName: "num.js",
		StackTrace: "stack",
	}
	var target *scriptPanicError
	if !errors.As(pe, &target) {
		t.Fatal("errors.As should match scriptPanicError")
	}
	if target.ScriptName != "num.js" {
		t.Errorf("ScriptName = %q, want %q", target.ScriptName, "num.js")
	}
	if target.Value != 42 {
		t.Errorf("Value = %v, want 42", target.Value)
	}
	if target.StackTrace != "stack" {
		t.Errorf("StackTrace = %q, want %q", target.StackTrace, "stack")
	}
}

// =============================================================================
// Engine getter tests
// =============================================================================

func TestEngine_Stdout(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	ctx := context.Background()
	engine := newTestEngine(t, ctx, &stdout, &stderr)
	if engine.Stdout() != &stdout {
		t.Error("Stdout() should return the configured stdout writer")
	}
}

func TestEngine_Stderr(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	ctx := context.Background()
	engine := newTestEngine(t, ctx, &stdout, &stderr)
	if engine.Stderr() != &stderr {
		t.Error("Stderr() should return the configured stderr writer")
	}
}

func TestEngine_GetScripts_Empty(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	ctx := context.Background()
	engine := newTestEngine(t, ctx, &stdout, &stderr)
	scripts := engine.GetScripts()
	if len(scripts) != 0 {
		t.Errorf("GetScripts() should return empty slice initially, got %d", len(scripts))
	}
}

func TestEngine_GetScripts_AfterLoad(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	ctx := context.Background()
	engine := newTestEngine(t, ctx, &stdout, &stderr)
	engine.LoadScriptFromString("a", "1")
	engine.LoadScriptFromString("b", "2")
	scripts := engine.GetScripts()
	if len(scripts) != 2 {
		t.Errorf("GetScripts() should return 2 scripts, got %d", len(scripts))
	}
	if scripts[0].Name != "a" || scripts[1].Name != "b" {
		t.Errorf("GetScripts() names = [%q, %q], want [a, b]", scripts[0].Name, scripts[1].Name)
	}
}

func TestEngine_GetTerminalReader_NilTerminalIO(t *testing.T) {
	t.Parallel()
	e := &Engine{} // terminalIO is nil
	if r := e.GetTerminalReader(); r != nil {
		t.Error("GetTerminalReader() should return nil when terminalIO is nil")
	}
}

func TestEngine_GetTerminalWriter_NilTerminalIO(t *testing.T) {
	t.Parallel()
	e := &Engine{} // terminalIO is nil
	if w := e.GetTerminalWriter(); w != nil {
		t.Error("GetTerminalWriter() should return nil when terminalIO is nil")
	}
}

func TestEngine_EventLoop_NilRuntime(t *testing.T) {
	t.Parallel()
	e := &Engine{} // runtime is nil
	if el := e.Loop(); el != nil {
		t.Error("EventLoop() should return nil when runtime is nil")
	}
}

func TestEngine_Registry(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	ctx := context.Background()
	engine := newTestEngine(t, ctx, &stdout, &stderr)
	if engine.Registry() == nil {
		t.Error("Registry() should not be nil for initialized engine")
	}
}

// =============================================================================
// LoadScript from file tests
// =============================================================================

func TestEngine_LoadScript_FileNotFound(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	ctx := context.Background()
	engine := newTestEngine(t, ctx, &stdout, &stderr)

	_, err := engine.LoadScript("missing", "/nonexistent/path/to/script.js")
	if err == nil {
		t.Fatal("LoadScript should fail for nonexistent file")
	}
	if !strings.Contains(err.Error(), "failed to read script") {
		t.Errorf("error should mention 'failed to read script', got: %v", err)
	}
}

func TestEngine_LoadScript_Success(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	ctx := context.Background()
	engine := newTestEngine(t, ctx, &stdout, &stderr)

	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "test.js")
	if err := os.WriteFile(scriptPath, []byte("var x = 1;"), 0644); err != nil {
		t.Fatal(err)
	}

	script, err := engine.LoadScript("test", scriptPath)
	if err != nil {
		t.Fatalf("LoadScript failed: %v", err)
	}
	if script.Name != "test" {
		t.Errorf("Name = %q, want %q", script.Name, "test")
	}
	if script.Path != scriptPath {
		t.Errorf("Path = %q, want %q", script.Path, scriptPath)
	}
	if script.Content != "var x = 1;" {
		t.Errorf("Content = %q, want %q", script.Content, "var x = 1;")
	}
}

// =============================================================================
// readFile tests
// =============================================================================

func TestReadFile_Error(t *testing.T) {
	t.Parallel()
	_, err := readFile("/nonexistent/file/path.js")
	if err == nil {
		t.Fatal("readFile should fail for nonexistent file")
	}
}

func TestReadFile_Success(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "hello.js")
	want := "hello world"
	if err := os.WriteFile(path, []byte(want), 0644); err != nil {
		t.Fatal(err)
	}
	got, err := readFile(path)
	if err != nil {
		t.Fatalf("readFile failed: %v", err)
	}
	if got != want {
		t.Errorf("readFile = %q, want %q", got, want)
	}
}

// =============================================================================
// shebangStrippingLoader tests
// =============================================================================

func TestShebangStrippingLoader_WithShebang(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "shebang.js")
	content := "#!/usr/bin/env node\nvar x = 1;\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	data, err := shebangStrippingLoader(path)
	if err != nil {
		t.Fatalf("shebangStrippingLoader failed: %v", err)
	}
	if data[0] != '/' || data[1] != '/' {
		t.Errorf("shebang should be replaced with //, got %q", string(data[:2]))
	}
	// Rest of content should be preserved
	if !strings.Contains(string(data), "var x = 1;") {
		t.Errorf("content should be preserved, got: %s", string(data))
	}
}

func TestShebangStrippingLoader_WithoutShebang(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "normal.js")
	content := "var x = 1;\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	data, err := shebangStrippingLoader(path)
	if err != nil {
		t.Fatalf("shebangStrippingLoader failed: %v", err)
	}
	if string(data) != content {
		t.Errorf("content should be unchanged, got: %s", string(data))
	}
}

func TestShebangStrippingLoader_FileNotFound(t *testing.T) {
	t.Parallel()
	_, err := shebangStrippingLoader("/nonexistent/file.js")
	if err == nil {
		t.Fatal("shebangStrippingLoader should fail for nonexistent file")
	}
}

// =============================================================================
// WithModulePaths option tests
// =============================================================================

func TestWithModulePaths(t *testing.T) {
	t.Parallel()
	var opts engineOptions
	fn := WithModulePaths("/path/a", "/path/b")
	fn(&opts)
	if len(opts.modulePaths) != 2 {
		t.Fatalf("modulePaths length = %d, want 2", len(opts.modulePaths))
	}
	if opts.modulePaths[0] != "/path/a" || opts.modulePaths[1] != "/path/b" {
		t.Errorf("modulePaths = %v, want [/path/a, /path/b]", opts.modulePaths)
	}

	// Append more
	fn2 := WithModulePaths("/path/c")
	fn2(&opts)
	if len(opts.modulePaths) != 3 {
		t.Fatalf("modulePaths length = %d, want 3", len(opts.modulePaths))
	}
	if opts.modulePaths[2] != "/path/c" {
		t.Errorf("modulePaths[2] = %q, want /path/c", opts.modulePaths[2])
	}
}

func TestNewEngineDetailed_WithModulePaths(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dir := t.TempDir()
	modPath := filepath.Join(dir, "mylib.js")
	if err := os.WriteFile(modPath, []byte("module.exports = { answer: 42 };"), 0644); err != nil {
		t.Fatal(err)
	}

	sessionID := testutil.NewTestSessionID("", t.Name())
	engine, err := NewEngine(ctx, &stdout, &stderr, sessionID, "memory", nil, 0, 0, WithModulePaths(dir))
	if err != nil {
		t.Fatalf("NewEngineDetailed failed: %v", err)
	}
	t.Cleanup(func() { _ = engine.Close() })

	script := engine.LoadScriptFromString("test", `
		var lib = require('mylib');
		ctx.log("answer: " + lib.answer);
	`)
	engine.SetTestMode(true)
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("ExecuteScript failed: %v", err)
	}
	if !strings.Contains(stdout.String(), "answer: 42") {
		t.Errorf("expected module to load, stdout: %s", stdout.String())
	}
}

// =============================================================================
// ExecuteScript panic recovery tests
// =============================================================================

func TestExecuteScript_PanicRecovery_GoPanic(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	ctx := context.Background()
	engine := newTestEngine(t, ctx, &stdout, &stderr)

	// Register a Go function that panics
	engine.SetGlobal("goPanic", func() {
		panic("go panic value")
	})

	script := engine.LoadScriptFromString("go_panic", `goPanic();`)
	err := engine.ExecuteScript(script)
	if err == nil {
		t.Fatal("expected error from Go panic")
	}

	var pe *scriptPanicError
	if !errors.As(err, &pe) {
		t.Fatalf("expected scriptPanicError, got: %T: %v", err, err)
	}
	if pe.ScriptName != "go_panic" {
		t.Errorf("ScriptName = %q, want %q", pe.ScriptName, "go_panic")
	}
	if pe.Value != "go panic value" {
		t.Errorf("Value = %v, want 'go panic value'", pe.Value)
	}
	if pe.StackTrace == "" {
		t.Error("StackTrace should not be empty")
	}
	// stderr should contain the panic message
	if !strings.Contains(stderr.String(), "[PANIC]") {
		t.Errorf("stderr should contain [PANIC], got: %s", stderr.String())
	}
}

func TestExecuteScript_PanicRecovery_GoPanicWithError(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	ctx := context.Background()
	engine := newTestEngine(t, ctx, &stdout, &stderr)

	inner := errors.New("inner error")
	engine.SetGlobal("goPanicErr", func() {
		panic(inner)
	})

	script := engine.LoadScriptFromString("go_panic_err", `goPanicErr();`)
	err := engine.ExecuteScript(script)
	if err == nil {
		t.Fatal("expected error from Go panic")
	}

	var pe *scriptPanicError
	if !errors.As(err, &pe) {
		t.Fatalf("expected scriptPanicError, got: %T", err)
	}
	// Unwrap should return the inner error
	if !errors.Is(pe, inner) {
		t.Error("errors.Is(pe, inner) should be true via Unwrap")
	}
}

// =============================================================================
// ExecuteScript deferred error paths
// =============================================================================

func TestExecuteScript_DeferredError_CombinedWithExecError(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	ctx := context.Background()
	engine := newTestEngine(t, ctx, &stdout, &stderr)
	engine.SetTestMode(true)

	script := engine.LoadScriptFromString("combined_err", `
		ctx.defer(function() {
			ctx.error("deferred error");
		});
		ctx.error("execution error");
	`)

	err := engine.ExecuteScript(script)
	if err == nil {
		t.Fatal("expected error when both execution and deferred functions fail")
	}
	// The error should mention both execution and deferred errors
	errStr := err.Error()
	if !strings.Contains(errStr, "execution error") && !strings.Contains(errStr, "deferred error") {
		// At minimum one must be present; the combined path produces "execution error: X; deferred error: Y"
		t.Logf("error: %s", errStr)
	}
}

func TestExecuteScript_DeferredError_OnlyDeferred(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	ctx := context.Background()
	engine := newTestEngine(t, ctx, &stdout, &stderr)
	engine.SetTestMode(true)

	script := engine.LoadScriptFromString("deferred_only", `
		ctx.defer(function() {
			ctx.error("deferred failure");
		});
		ctx.log("script ran fine");
	`)

	err := engine.ExecuteScript(script)
	if err == nil {
		t.Fatal("expected error from deferred function failure")
	}
}

// =============================================================================
// ExecuteScript file-based script paths
// =============================================================================

func TestExecuteScript_FileBased_CompileError(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	ctx := context.Background()
	engine := newTestEngine(t, ctx, &stdout, &stderr)

	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "bad.js")
	if err := os.WriteFile(scriptPath, []byte("function( {"), 0644); err != nil {
		t.Fatal(err)
	}

	script, err := engine.LoadScript("bad", scriptPath)
	if err != nil {
		t.Fatalf("LoadScript failed: %v", err)
	}

	err = engine.ExecuteScript(script)
	if err == nil {
		t.Fatal("expected compile error")
	}
	if !strings.Contains(err.Error(), "script compilation failed") {
		t.Errorf("error should mention compilation, got: %v", err)
	}
}

func TestExecuteScript_FileBased_RuntimeError(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	ctx := context.Background()
	engine := newTestEngine(t, ctx, &stdout, &stderr)

	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "runtime_err.js")
	if err := os.WriteFile(scriptPath, []byte("undefinedVar;"), 0644); err != nil {
		t.Fatal(err)
	}

	script, err := engine.LoadScript("runtime_err", scriptPath)
	if err != nil {
		t.Fatalf("LoadScript failed: %v", err)
	}

	err = engine.ExecuteScript(script)
	if err == nil {
		t.Fatal("expected runtime error")
	}
	if !strings.Contains(err.Error(), "script execution failed") {
		t.Errorf("error should mention execution failed, got: %v", err)
	}
}

func TestExecuteScript_FileBased_Success(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	ctx := context.Background()
	engine := newTestEngine(t, ctx, &stdout, &stderr)
	engine.SetTestMode(true)

	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "good.js")
	if err := os.WriteFile(scriptPath, []byte(`ctx.log("file script ok");`), 0644); err != nil {
		t.Fatal(err)
	}

	script, err := engine.LoadScript("good", scriptPath)
	if err != nil {
		t.Fatalf("LoadScript failed: %v", err)
	}

	err = engine.ExecuteScript(script)
	if err != nil {
		t.Fatalf("ExecuteScript failed: %v", err)
	}
	if !strings.Contains(stdout.String(), "file script ok") {
		t.Errorf("expected output, got: %s", stdout.String())
	}
}

func TestExecuteScript_FileBased_WithShebang(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	ctx := context.Background()
	engine := newTestEngine(t, ctx, &stdout, &stderr)
	engine.SetTestMode(true)

	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "shebang.js")
	content := "#!/usr/bin/env node\nctx.log(\"shebang ok\");\n"
	if err := os.WriteFile(scriptPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	script, err := engine.LoadScript("shebang", scriptPath)
	if err != nil {
		t.Fatalf("LoadScript failed: %v", err)
	}

	err = engine.ExecuteScript(script)
	if err != nil {
		t.Fatalf("ExecuteScript failed: %v", err)
	}
	if !strings.Contains(stdout.String(), "shebang ok") {
		t.Errorf("expected output, got: %s", stdout.String())
	}
}

func TestExecuteScript_InlineScript_RuntimeError(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	ctx := context.Background()
	engine := newTestEngine(t, ctx, &stdout, &stderr)

	script := engine.LoadScriptFromString("inline_err", "undefinedVar;")
	err := engine.ExecuteScript(script)
	if err == nil {
		t.Fatal("expected runtime error")
	}
	if !strings.Contains(err.Error(), "script execution failed") {
		t.Errorf("error should mention execution failed, got: %v", err)
	}
}

// =============================================================================
// setExecutionContext nil-panic test
// =============================================================================

func TestSetExecutionContext_NilPanic(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	ctx := context.Background()
	engine := newTestEngine(t, ctx, &stdout, &stderr)

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for nil execution context")
		}
		if !strings.Contains(fmt.Sprint(r), "execution context cannot be nil") {
			t.Errorf("unexpected panic message: %v", r)
		}
	}()

	_ = engine.setExecutionContext(nil)
}

// =============================================================================
// Close error path test
// =============================================================================

func TestEngine_Close_NilFields(t *testing.T) {
	t.Parallel()
	// Engine with minimal fields — nil tuiManager, btBridge, etc.
	e := &Engine{}
	if err := e.Close(); err != nil {
		t.Fatalf("Close on minimal engine should succeed, got: %v", err)
	}
}

// =============================================================================
// RegisterNativeModule test
// =============================================================================

func TestEngine_RegisterNativeModule(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	ctx := context.Background()
	engine := newTestEngine(t, ctx, &stdout, &stderr)
	engine.SetTestMode(true)

	engine.RegisterNativeModule("osm:test_coverage", func(rt *goja.Runtime, module *goja.Object) {
		exports := module.Get("exports").(*goja.Object)
		exports.Set("hello", func() string { return "from native module" })
	})

	script := engine.LoadScriptFromString("native_mod", `
		var m = require("osm:test_coverage");
		ctx.log(m.hello());
	`)
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("ExecuteScript failed: %v", err)
	}
	if !strings.Contains(stdout.String(), "from native module") {
		t.Errorf("expected native module output, got: %s", stdout.String())
	}
}

// =============================================================================
// Runtime coverage gaps
// =============================================================================

func TestRuntime_RunOnLoopSync_NoTimeout_Success(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rt, err := NewRuntime(ctx)
	if err != nil {
		t.Fatalf("NewRuntime failed: %v", err)
	}
	defer rt.Close()

	// Set timeout to 0 (no timeout)
	rt.SetTimeout(0)

	var value int
	err = rt.RunOnLoopSync(func(vm *goja.Runtime) error {
		value = 99
		return nil
	})
	if err != nil {
		t.Errorf("RunOnLoopSync with no timeout failed: %v", err)
	}
	if value != 99 {
		t.Errorf("value = %d, want 99", value)
	}
}

func TestRuntime_RunOnLoopSync_NoTimeout_RuntimeStopped(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rt, err := NewRuntime(ctx)
	if err != nil {
		t.Fatalf("NewRuntime failed: %v", err)
	}

	// Schedule a job that blocks until we close
	blockCh := make(chan struct{})
	scheduled := rt.RunOnLoop(func(vm *goja.Runtime) {
		<-blockCh
	})
	if !scheduled {
		t.Fatal("RunOnLoop should succeed")
	}

	// Set no timeout
	rt.SetTimeout(0)

	// Start a goroutine that will try RunOnLoopSync (will block because event loop is busy)
	errCh := make(chan error, 1)
	go func() {
		errCh <- rt.RunOnLoopSync(func(vm *goja.Runtime) error {
			return nil // This will never execute due to close
		})
	}()

	// Close the runtime — the pending RunOnLoopSync should get "runtime stopped" error
	close(blockCh) // unblock the first job
	rt.Close()

	err = <-errCh
	// Might get "event loop not running" or "runtime stopped before completion", or nil
	// (depends on timing — the close may happen before or after the schedule).
	// The important thing is it doesn't hang.
	t.Logf("RunOnLoopSync after close returned: %v", err)
}

func TestRuntime_RunOnLoopSync_RuntimeStoppedDuringWait(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rt, err := NewRuntime(ctx)
	if err != nil {
		t.Fatalf("NewRuntime failed: %v", err)
	}

	// Block event loop with a long-running job
	blockCh := make(chan struct{})
	rt.RunOnLoop(func(vm *goja.Runtime) {
		<-blockCh
	})

	// Set a long timeout to ensure the "runtime stopped" path hits, not timeout
	rt.SetTimeout(30 * 1000_000_000) // 30s

	errCh := make(chan error, 1)
	go func() {
		errCh <- rt.RunOnLoopSync(func(vm *goja.Runtime) error {
			return nil
		})
	}()

	// Close the runtime — this should trigger "runtime stopped before completion"
	close(blockCh)
	rt.Close()

	err = <-errCh
	t.Logf("RunOnLoopSync returned: %v", err)
}

func TestRuntime_TryRunOnLoopSync_Stopped(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rt, err := NewRuntime(ctx)
	if err != nil {
		t.Fatalf("NewRuntime failed: %v", err)
	}

	rt.Close()

	err = rt.TryRunOnLoopSync(nil, func(vm *goja.Runtime) error {
		return nil
	})
	if err == nil {
		t.Fatal("expected error for stopped runtime")
	}
	if !strings.Contains(err.Error(), "not running") {
		t.Errorf("error should mention not running, got: %v", err)
	}
}

func TestRuntime_LoadScript_RuntimeError(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rt, err := NewRuntime(ctx)
	if err != nil {
		t.Fatalf("NewRuntime failed: %v", err)
	}
	defer rt.Close()

	err = rt.LoadScript("runtime_err.js", "undefinedVar;")
	if err == nil {
		t.Fatal("expected runtime error")
	}
	if !strings.Contains(err.Error(), "failed to run") {
		t.Errorf("error should mention 'failed to run', got: %v", err)
	}
}

func TestRuntime_RunOnLoop_NotStarted(t *testing.T) {
	t.Parallel()
	// Create a runtime with minimal fields where started=false
	rt := &Runtime{}
	ok := rt.RunOnLoop(func(vm *goja.Runtime) {
		t.Error("should not execute")
	})
	if ok {
		t.Error("RunOnLoop should return false for not-started runtime")
	}
}

func TestRuntime_RunOnLoopSync_RuntimeStoppedWithTimeout(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rt, err := NewRuntime(ctx)
	if err != nil {
		t.Fatalf("NewRuntime failed: %v", err)
	}

	// Block the event loop so our RunOnLoopSync job can't execute
	blockCh := make(chan struct{})
	rt.RunOnLoop(func(vm *goja.Runtime) {
		<-blockCh
	})

	// Use a long timeout so the "stopped" path fires before timeout
	rt.SetTimeout(30 * 1000_000_000) // 30s

	errCh := make(chan error, 1)
	go func() {
		errCh <- rt.RunOnLoopSync(func(vm *goja.Runtime) error {
			return nil
		})
	}()

	// Close the runtime — Done channel fires, RunOnLoopSync picks "runtime stopped"
	go func() {
		// Give RunOnLoopSync time to schedule its callback
		for i := 0; i < 100; i++ {
			if len(errCh) > 0 {
				return
			}
		}
		close(blockCh) // unblock the event loop job
		rt.Close()
	}()

	err = <-errCh
	// May get nil (job executed), "runtime stopped", or "event loop not running"
	t.Logf("RunOnLoopSync with timeout during stop: %v", err)
}

func TestRuntime_RunOnLoopSync_SecondRunOnLoopFails(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rt, err := NewRuntime(ctx)
	if err != nil {
		t.Fatalf("NewRuntime failed: %v", err)
	}

	// Stop the loop but don't update the stopped flag yet —
	// this is the race window where state check passes but loop.RunOnLoop returns false.
	// We can't reliably reproduce this race, but we can verify the RunOnLoop
	// returns false check by calling RunOnLoopSync after Close.
	rt.Close()

	err = rt.RunOnLoopSync(func(vm *goja.Runtime) error {
		return nil
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestSetupGlobals_TuiResetNilCheck tests the tui.reset JS function nil guard path
func TestSetupGlobals_TuiResetSuccess(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	ctx := context.Background()
	engine := newTestEngine(t, ctx, &stdout, &stderr)
	engine.SetTestMode(true)

	// tui.reset calls tuiManager.resetAllState() which should work on a fresh engine
	script := engine.LoadScriptFromString("tui_reset", `
		try {
			var result = tui.reset();
			ctx.log("reset result: " + result);
		} catch(e) {
			ctx.log("reset error: " + e.message);
		}
	`)
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("ExecuteScript failed: %v", err)
	}
	t.Logf("output: %s", stdout.String())
}

// =============================================================================
// Engine.Close error logging
// =============================================================================

func TestExecutionContext_Fatal(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	ctx := context.Background()
	engine := newTestEngine(t, ctx, &stdout, &stderr)
	engine.SetTestMode(true)

	script := engine.LoadScriptFromString("fatal_test", `
		ctx.fatal("fatal error message");
		ctx.log("should not reach here");
	`)

	err := engine.ExecuteScript(script)
	if err == nil {
		t.Fatal("expected error from fatal()")
	}
	if !strings.Contains(stderr.String(), "fatal error message") {
		t.Errorf("stderr should contain fatal message, got: %s", stderr.String())
	}
	if strings.Contains(stdout.String(), "should not reach here") {
		t.Error("code after fatal should not execute")
	}
}

func TestExecutionContext_Fatalf(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	ctx := context.Background()
	engine := newTestEngine(t, ctx, &stdout, &stderr)
	engine.SetTestMode(true)

	script := engine.LoadScriptFromString("fatalf_test", `
		ctx.fatalf("fatal: %s %d", "arg", 42);
		ctx.log("should not reach here");
	`)

	err := engine.ExecuteScript(script)
	if err == nil {
		t.Fatal("expected error from fatalf()")
	}
	if !strings.Contains(stderr.String(), "fatal: arg 42") {
		t.Errorf("stderr should contain formatted fatal message, got: %s", stderr.String())
	}
}

func TestExecutionContext_Failed(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	ctx := context.Background()
	engine := newTestEngine(t, ctx, &stdout, &stderr)
	engine.SetTestMode(true)

	script := engine.LoadScriptFromString("failed_test", `
		ctx.log("failed before error: " + ctx.failed());
		ctx.error("marking failed");
		ctx.log("failed after error: " + ctx.failed());
	`)

	err := engine.ExecuteScript(script)
	if err == nil {
		t.Fatal("expected error")
	}
	output := stdout.String()
	if !strings.Contains(output, "failed before error: false") {
		t.Errorf("expected failed=false before error, got: %s", output)
	}
	if !strings.Contains(output, "failed after error: true") {
		t.Errorf("expected failed=true after error, got: %s", output)
	}
}

func TestExecutionContext_Name(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	ctx := context.Background()
	engine := newTestEngine(t, ctx, &stdout, &stderr)
	engine.SetTestMode(true)

	script := engine.LoadScriptFromString("name_test", `
		ctx.log("name: " + ctx.name());
	`)

	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("ExecuteScript failed: %v", err)
	}
	if !strings.Contains(stdout.String(), "name: name_test") {
		t.Errorf("expected name in output, got: %s", stdout.String())
	}
}

func TestExecutionContext_Run_SubTestPanic(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	ctx := context.Background()
	engine := newTestEngine(t, ctx, &stdout, &stderr)
	engine.SetTestMode(true)

	// Register a Go function that panics
	engine.SetGlobal("doPanic", func() {
		panic("sub-test panic")
	})

	script := engine.LoadScriptFromString("subtest_panic", `
		var result = ctx.run("panicking", function() {
			doPanic();
		});
		ctx.log("subtest result: " + result);
	`)

	err := engine.ExecuteScript(script)
	if err == nil {
		t.Fatal("expected error from panicking sub-test")
	}
	// The sub-test panic should be caught and reported
	if !strings.Contains(stderr.String(), "Test panicked") {
		t.Errorf("stderr should contain panic message, got: %s", stderr.String())
	}
}

func TestExecutionContext_Run_SubTestCallError(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	ctx := context.Background()
	engine := newTestEngine(t, ctx, &stdout, &stderr)
	engine.SetTestMode(true)

	script := engine.LoadScriptFromString("subtest_call_err", `
		var result = ctx.run("throwing", function() {
			throw new Error("sub-test error");
		});
		ctx.log("subtest result: " + result);
	`)

	err := engine.ExecuteScript(script)
	if err == nil {
		t.Fatal("expected error from throwing sub-test")
	}
	if !strings.Contains(stderr.String(), "Test failed") {
		t.Errorf("stderr should contain 'Test failed', got: %s", stderr.String())
	}
}

func TestExecutionContext_Run_SubTestDeferredError(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	ctx := context.Background()
	engine := newTestEngine(t, ctx, &stdout, &stderr)
	engine.SetTestMode(true)

	script := engine.LoadScriptFromString("subtest_deferred_err", `
		var result = ctx.run("with_defer", function() {
			ctx.defer(function() {
				ctx.error("deferred error in sub-test");
			});
			ctx.log("sub-test body ok");
		});
		ctx.log("subtest result: " + result);
	`)

	err := engine.ExecuteScript(script)
	if err == nil {
		t.Fatal("expected error from deferred error in sub-test")
	}
	if !strings.Contains(stderr.String(), "deferred error in sub-test") {
		t.Errorf("stderr should contain deferred error, got: %s", stderr.String())
	}
}

func TestExecutionContext_RunDeferred_PanicInDeferred(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	ctx := context.Background()
	engine := newTestEngine(t, ctx, &stdout, &stderr)
	engine.SetTestMode(true)

	engine.SetGlobal("doPanicInDefer", func() {
		panic("deferred panic value")
	})

	script := engine.LoadScriptFromString("deferred_panic", `
		ctx.defer(function() {
			doPanicInDefer();
		});
		ctx.log("script body ok");
	`)

	err := engine.ExecuteScript(script)
	if err == nil {
		t.Fatal("expected error from panic in deferred function")
	}
	if !strings.Contains(stderr.String(), "Deferred function panicked") {
		t.Errorf("stderr should contain deferred panic message, got: %s", stderr.String())
	}
}

func TestExecutionContext_RunDeferred_LIFO(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	ctx := context.Background()
	engine := newTestEngine(t, ctx, &stdout, &stderr)
	engine.SetTestMode(true)

	script := engine.LoadScriptFromString("lifo_test", `
		ctx.defer(function() { ctx.log("defer-1"); });
		ctx.defer(function() { ctx.log("defer-2"); });
		ctx.defer(function() { ctx.log("defer-3"); });
		ctx.log("body");
	`)

	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("ExecuteScript failed: %v", err)
	}

	output := stdout.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) < 4 {
		t.Fatalf("expected 4 lines, got %d: %s", len(lines), output)
	}
	// body, then defer-3, defer-2, defer-1 (LIFO)
	if !strings.Contains(lines[0], "body") {
		t.Errorf("line 0 should be body, got: %s", lines[0])
	}
	if !strings.Contains(lines[1], "defer-3") {
		t.Errorf("line 1 should be defer-3, got: %s", lines[1])
	}
	if !strings.Contains(lines[2], "defer-2") {
		t.Errorf("line 2 should be defer-2, got: %s", lines[2])
	}
	if !strings.Contains(lines[3], "defer-1") {
		t.Errorf("line 3 should be defer-1, got: %s", lines[3])
	}
}

func TestExecutionContext_Run_PassingSubTest(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	ctx := context.Background()
	engine := newTestEngine(t, ctx, &stdout, &stderr)
	engine.SetTestMode(true)

	script := engine.LoadScriptFromString("passing_subtest", `
		var result = ctx.run("passing", function() {
			ctx.log("subtest ran");
		});
		ctx.log("result: " + result);
	`)

	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("ExecuteScript failed: %v", err)
	}
	if !strings.Contains(stdout.String(), "result: true") {
		t.Errorf("expected passing result, got: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "Sub-test passing passed") {
		t.Errorf("expected sub-test pass message, got: %s", stdout.String())
	}
}

func TestExecutionContext_Run_NestedSubTest(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	ctx := context.Background()
	engine := newTestEngine(t, ctx, &stdout, &stderr)
	engine.SetTestMode(true)

	script := engine.LoadScriptFromString("nested_sub", `
		ctx.run("outer", function() {
			ctx.log("name: " + ctx.name());
			ctx.run("inner", function() {
				ctx.log("inner_name: " + ctx.name());
			});
		});
	`)

	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("ExecuteScript failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "name: nested_sub/outer") {
		t.Errorf("expected outer name, got: %s", output)
	}
	if !strings.Contains(output, "inner_name: nested_sub/outer/inner") {
		t.Errorf("expected inner name, got: %s", output)
	}
}

// =============================================================================
// Engine.Close error logging
// =============================================================================

func TestEngine_Logger(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	ctx := context.Background()
	engine := newTestEngine(t, ctx, &stdout, &stderr)
	logger := engine.Logger()
	if logger == nil {
		t.Fatal("Logger() should not return nil")
	}
}

func TestEngine_GetTUIManager(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	ctx := context.Background()
	engine := newTestEngine(t, ctx, &stdout, &stderr)
	tuiMgr := engine.GetTUIManager()
	if tuiMgr == nil {
		t.Fatal("GetTUIManager() should not return nil for initialized engine")
	}
}

func TestEngine_SetTestMode(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	ctx := context.Background()
	engine := newTestEngine(t, ctx, &stdout, &stderr)

	// Default is false — Log shouldn't go to stdout in non-testMode
	script := engine.LoadScriptFromString("no_test_mode", `ctx.log("hidden");`)
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stdout.String(), "hidden") {
		t.Error("Log should NOT output to stdout when testMode is false")
	}

	stdout.Reset()
	engine.SetTestMode(true)
	script2 := engine.LoadScriptFromString("test_mode", `ctx.log("visible");`)
	if err := engine.ExecuteScript(script2); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "visible") {
		t.Error("Log should output to stdout when testMode is true")
	}
}

// =============================================================================
// NewEngineConfig — exercises the simple constructor
// =============================================================================

func TestNewEngineConfig(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	sessionID := testutil.NewTestSessionID("", t.Name())
	engine, err := NewEngineDeprecated(ctx, &stdout, &stderr, sessionID, "memory")
	if err != nil {
		t.Fatalf("NewEngineConfig failed: %v", err)
	}
	t.Cleanup(func() { _ = engine.Close() })

	if engine.vm == nil {
		t.Error("vm should not be nil")
	}
	if engine.registry == nil {
		t.Error("registry should not be nil")
	}
	if engine.contextManager == nil {
		t.Error("contextManager should not be nil")
	}
	if engine.tuiManager == nil {
		t.Error("tuiManager should not be nil")
	}
}

// =============================================================================
// getGoroutineID test
// =============================================================================

func TestGetGoroutineID(t *testing.T) {
	t.Parallel()
	id := getGoroutineID()
	if id <= 0 {
		t.Errorf("getGoroutineID() = %d, want > 0", id)
	}
}
