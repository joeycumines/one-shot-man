package command

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/joeycumines/goja"

	"github.com/joeycumines/one-shot-man/internal/builtin/mcpcallbackmod"
	"github.com/joeycumines/one-shot-man/internal/config"
	"github.com/joeycumines/one-shot-man/internal/scripting"
)

// TestPipelineFile describes a file to create in the git repo.
type TestPipelineFile struct {
	Path    string
	Content string
}

// TestPipeline provides a complete setup for pr-split integration testing:
// temp git repo with configurable files, the Goja engine loaded with
// pr-split chunk files (00-13), and a result directory for mock MCP responses.
type TestPipeline struct {
	Dir         string                       // git repo directory
	ResultDir   string                       // MCP result directory
	Stdout      *safeBuffer                  // captured stdout (thread-safe)
	Dispatch    func(string, []string) error // TUI command dispatch
	EvalJS      func(string) (any, error)    // evaluate JS in engine
	EvalJSAsync func(string) (any, error)    // evaluate async JS (await)
	Runtime     *goja.Runtime                // this pipeline's JS runtime
}

// MCPInjection is one tool result to inject when the pipeline's MCP callback
// completes init.
type MCPInjection struct {
	ToolName string
	Data     json.RawMessage
}

// injectOnMCPInit registers a watcher that injects the given tool results,
// in order, when THIS pipeline's MCP callback completes init, so a test can
// supply payloads while its own JS waits. The runtime scoping is what keeps
// parallel tests from injecting each other's payloads; register before
// starting the pipeline. The watcher goroutine is bounded by test cleanup
// (done channel) and deregistered from the global registry (cancel), so it
// never leaks and never logs through the testing machinery after completion.
// Zero injections is valid: the watcher then simply drains init without
// injecting, for tests asserting the no-injection timeout path. Injection
// errors go to stderr because the goroutine may outlive the test.
func (tp *TestPipeline) injectOnMCPInit(t *testing.T, injections ...MCPInjection) {
	t.Helper()
	watchCh, cancel := mcpcallbackmod.WatchForInit(tp.Runtime)
	done := make(chan struct{})
	t.Cleanup(func() {
		close(done)
		cancel()
	})
	go func() {
		select {
		case h := <-watchCh:
			for _, inj := range injections {
				if err := h.InjectToolResult(inj.ToolName, inj.Data); err != nil {
					fmt.Fprintf(os.Stderr, "[mcp-init-inject] %s failed: %v\n", inj.ToolName, err)
				}
			}
		case <-done:
		}
	}()
}

// setupTestPipeline creates a test pipeline with configurable initial files,
// feature branch files, and config overrides.
func setupTestPipeline(t *testing.T, opts TestPipelineOpts) *TestPipeline {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}

	dir := t.TempDir()
	t.Cleanup(func() {
		_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err == nil {
				_ = os.Chmod(path, 0o755)
			}
			return nil
		})
		_ = os.RemoveAll(dir)
	})
	resultParent := t.TempDir()
	t.Cleanup(func() {
		_ = filepath.Walk(resultParent, func(path string, info os.FileInfo, err error) error {
			if err == nil {
				_ = os.Chmod(path, 0o755)
			}
			return nil
		})
		_ = os.RemoveAll(resultParent)
	})
	resultDir := filepath.Join(resultParent, "mcp-results")
	if err := os.MkdirAll(resultDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Initialize repo on main.
	runGitCmd(t, dir, "init")
	runGitCmd(t, dir, "symbolic-ref", "HEAD", "refs/heads/main")
	runGitCmd(t, dir, "config", "user.email", "test@test.com")
	runGitCmd(t, dir, "config", "user.name", "Test User")

	// Create initial files.
	initialFiles := opts.InitialFiles
	if len(initialFiles) == 0 {
		initialFiles = []TestPipelineFile{
			{"pkg/types.go", "package pkg\n\ntype Foo struct{}\n"},
			{"cmd/main.go", "package main\n\nfunc main() {}\n"},
			{"README.md", "# Test Project\n"},
		}
	}
	for _, f := range initialFiles {
		full := filepath.Join(dir, f.Path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(f.Content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runGitCmd(t, dir, "add", "-A")
	runGitCmd(t, dir, "commit", "-m", "initial commit")

	// Create feature branch with changes.
	runGitCmd(t, dir, "checkout", "-b", "feature")
	if opts.NoFeatureFiles {
		// Empty commit — feature branch exists but has no file changes.
		runGitCmd(t, dir, "commit", "--allow-empty", "-m", "feature (no changes)")
	} else {
		featureFiles := opts.FeatureFiles
		if len(featureFiles) == 0 {
			featureFiles = []TestPipelineFile{
				{"pkg/impl.go", "package pkg\n\nfunc Bar() string { return \"bar\" }\n"},
				{"cmd/run.go", "package main\n\nfunc run() {}\n"},
				{"docs/guide.md", "# Guide\n\nUsage instructions.\n"},
			}
		}
		for _, f := range featureFiles {
			full := filepath.Join(dir, f.Path)
			if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(full, []byte(f.Content), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		runGitCmd(t, dir, "add", "-A")
		runGitCmd(t, dir, "commit", "-m", "feature work")
	}

	// Delete files on feature branch if requested.
	if len(opts.DeleteFilesOnFeature) > 0 {
		for _, delPath := range opts.DeleteFilesOnFeature {
			full := filepath.Join(dir, delPath)
			if err := os.Remove(full); err != nil {
				t.Fatalf("delete %s: %v", delPath, err)
			}
		}
		runGitCmd(t, dir, "add", "-A")
		runGitCmd(t, dir, "commit", "-m", "delete files on feature")
	}

	// Set up engine with config overrides.
	// Always include the absolute temp dir path to prevent git operations
	// from targeting the Go test package directory (B00 fix).
	stateDir := t.TempDir()
	overrides := map[string]any{
		"baseBranch":       "main",
		"dir":              dir,
		"persistStatePath": filepath.Join(stateDir, "pr-split-mux.state.json"),
		"transcriptDir":    stateDir,
	}
	maps.Copy(overrides, opts.ConfigOverrides)

	eng := loadPrSplitEngineWithRuntime(t, overrides)

	return &TestPipeline{
		Dir:         dir,
		ResultDir:   resultDir,
		Stdout:      eng.Stdout,
		Dispatch:    eng.Dispatch,
		EvalJS:      eng.EvalJS,
		EvalJSAsync: eng.EvalJSAsync,
		Runtime:     eng.Runtime,
	}
}

// TestPipelineOpts configures setupTestPipeline.
type TestPipelineOpts struct {
	InitialFiles         []TestPipelineFile // files on main (nil = default set)
	FeatureFiles         []TestPipelineFile // files on feature branch (nil = default set)
	NoFeatureFiles       bool               // if true, feature branch has no file changes (empty commit)
	DeleteFilesOnFeature []string           // file paths to delete on feature branch (after creating FeatureFiles)
	ConfigOverrides      map[string]any     // pr-split config overrides
}

// dispatchAwaitPromise dispatches a TUI command by name, calling the handler
// directly (not through ExecuteCommand which discards Promise returns).
// If the handler returns a Promise, .then/.catch is chained to properly
// await completion before signaling the Go channel. This follows the same
// pattern used by mcpmod.handlePromiseResult.
func dispatchAwaitPromise(engine *scripting.Engine, tm *scripting.TUIManager, name string, args []string) error {
	done := make(chan error, 1)
	submitErr := engine.Loop().Submit(func() {
		vm := engine.Runtime()

		// Look up the command handler from the current TUI mode.
		mode := tm.GetCurrentMode()
		if mode == nil {
			done <- fmt.Errorf("no current TUI mode")
			return
		}
		cmd, exists := mode.Commands[name]
		if !exists {
			done <- fmt.Errorf("command not found in mode %q: %s", mode.Name, name)
			return
		}

		handler, ok := cmd.Handler.(goja.Callable)
		if !ok {
			// Handler exported from JS may be func(goja.FunctionCall) goja.Value.
			// Convert via ToValue + AssertFunction to get a proper Callable.
			handlerVal := vm.ToValue(cmd.Handler)
			handler, ok = goja.AssertFunction(handlerVal)
			if !ok {
				done <- fmt.Errorf("handler for %q is not callable: %T", name, cmd.Handler)
				return
			}
		}

		// Convert Go args to a JS array (mirrors tui_manager.executeCommand).
		argsJS := vm.NewArray()
		for i, a := range args {
			_ = argsJS.Set(strconv.Itoa(i), a)
		}

		// Call the handler with panic protection.
		var result goja.Value
		var callErr error
		func() {
			defer func() {
				if r := recover(); r != nil {
					callErr = fmt.Errorf("command %q panicked: %v", name, r)
				}
			}()
			result, callErr = handler(goja.Undefined(), argsJS)
		}()
		if callErr != nil {
			done <- callErr
			return
		}

		// If the result is a Promise (duck-typed via .then), chain .then/.catch
		// to signal completion back to the Go channel.
		if result != nil && !goja.IsUndefined(result) && !goja.IsNull(result) {
			obj := result.ToObject(vm)
			if obj != nil {
				thenProp := obj.Get("then")
				if thenProp != nil && !goja.IsUndefined(thenProp) {
					if thenFn, ok := goja.AssertFunction(thenProp); ok {
						onFulfilled := vm.ToValue(func(call goja.FunctionCall) goja.Value {
							done <- nil
							return goja.Undefined()
						})
						onRejected := vm.ToValue(func(call goja.FunctionCall) goja.Value {
							reason := call.Argument(0)
							done <- fmt.Errorf("promise rejected: %v", reason.Export())
							return goja.Undefined()
						})

						thenResult, thenErr := thenFn(result, onFulfilled)
						if thenErr != nil {
							done <- thenErr
							return
						}

						// Chain .catch on the .then result for rejection handling.
						thenObj := thenResult.ToObject(vm)
						catchProp := thenObj.Get("catch")
						if catchFn, ok := goja.AssertFunction(catchProp); ok {
							if _, catchErr := catchFn(thenResult, onRejected); catchErr != nil {
								done <- catchErr
							}
						}
						return // Will be signaled by .then/.catch callback
					}
				}
			}
		}

		// Synchronous handler — signal done immediately.
		done <- nil
	})
	if submitErr != nil {
		return submitErr
	}
	select {
	case err := <-done:
		return err
	case <-time.After(60 * time.Second):
		return fmt.Errorf("dispatch %q timed out after 60s", name)
	}
}

// allChunkSources returns the concatenated source of all pr-split chunk files.
// Used by tests that need to inspect the raw JS source for expected content
// (function declarations, constant names, etc.) — the chunk equivalent of the
// former monolith prSplitScript variable.
func allChunkSources() string {
	var b strings.Builder
	for _, entry := range prSplitManifestData.Chunks {
		data, err := chunkFS.ReadFile(entry.File)
		if err != nil {
			panic("allChunkSources: failed to read " + entry.File + ": " + err.Error())
		}
		b.Write(data)
		b.WriteByte('\n')
	}
	return b.String()
}

// loadPrSplitEngine creates a scripting engine with the pr-split chunks
// loaded and ready to dispatch commands. It configures all the global
// variables that PrSplitCommand.Execute would set.
//
// After loading chunks, a compatibility shim exposes formerly-global monolith
// names (bt, exec, osmod, gitExec, executeSplit, cache vars, etc.) on
// globalThis with Object.defineProperty get/set proxies so that satellite
// tests that assign to bare names (e.g. executeSplit = function(){})
// transparently update the prSplit.* namespace used by chunk code.
func loadPrSplitEngine(t testing.TB, overrides map[string]any) (*bytes.Buffer, func(name string, args []string) error) {
	t.Helper()

	var stdout, stderr bytes.Buffer

	b := scriptCommandBase{
		config:   config.NewConfig(),
		store:    "memory",
		session:  t.Name(),
		logLevel: "info",
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	engine, cleanup, err := b.PrepareEngine(ctx, &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)

	// Set defaults — same as PrSplitCommand.Execute.
	jsConfig := map[string]any{
		"baseBranch":    "main",
		"strategy":      "directory",
		"maxFiles":      10,
		"branchPrefix":  "split/",
		"verifyCommand": "true",
		"dryRun":        false,
		"jsonOutput":    false,
	}
	maps.Copy(jsConfig, overrides)

	engine.SetGlobal("config", map[string]any{"name": "pr-split"})
	engine.SetGlobal("prSplitConfig", jsConfig)
	engine.SetGlobal("args", []string{})
	engine.SetGlobal("prSplitTemplate", prSplitTemplate)

	if err := loadChunkedScript(engine); err != nil {
		t.Fatal(err)
	}

	// Install compat shim: re-expose monolith globals for satellite tests.
	shim := engine.LoadScriptString("pr-split/compat-shim", chunkCompatShim)
	if err := engine.ExecuteScript(shim); err != nil {
		t.Fatalf("compat shim failed: %v", err)
	}

	// Return a function that dispatches mode commands.
	// Calls the handler directly (bypassing ExecuteCommand which discards
	// Promise returns) and properly awaits any returned Promise via
	// .then/.catch chaining. This is necessary because async command
	// handlers (e.g. auto-split, run, fix) return Promises.
	tm := engine.GetTUIManager()
	dispatch := func(name string, args []string) error {
		return dispatchAwaitPromise(engine, tm, name, args)
	}

	return &stdout, dispatch
}

// prSplitTestEngine bundles the handles a test needs from a loaded pr-split
// engine. Runtime is included so tests can scope test-only injection
// channels (see TestPipeline.injectOnMCPInit) to their own engine instead of a
// process-global watcher list.
type prSplitTestEngine struct {
	Stdout      *safeBuffer
	Dispatch    func(string, []string) error
	EvalJS      func(string) (any, error)
	EvalJSAsync func(string) (any, error)
	Runtime     *goja.Runtime
}

// loadPrSplitEngineWithEval keeps the historical 4-value signature used by
// satellite tests that only need to evaluate JS.
func loadPrSplitEngineWithEval(t testing.TB, overrides map[string]any) (*safeBuffer, func(string, []string) error, func(string) (any, error), func(string) (any, error)) {
	t.Helper()

	eng := loadPrSplitEngineWithRuntime(t, overrides)
	return eng.Stdout, eng.Dispatch, eng.EvalJS, eng.EvalJSAsync
}

func loadPrSplitEngineWithRuntime(t testing.TB, overrides map[string]any) prSplitTestEngine {
	t.Helper()

	// T32: Extract optional eval timeout from overrides.
	// Usage: overrides["_evalTimeout"] = 10 * time.Minute
	// Default: 60s for unit tests (fast), configurable for real AI tests.
	evalJSTimeout := 60 * time.Second
	if overrides != nil {
		if v, ok := overrides["_evalTimeout"]; ok {
			evalJSTimeout = v.(time.Duration)
			delete(overrides, "_evalTimeout")
		}
	}

	var stdout safeBuffer
	var stderr bytes.Buffer

	b := scriptCommandBase{
		config:   config.NewConfig(),
		store:    "memory",
		session:  t.Name(),
		logLevel: "info",
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	engine, cleanup, err := b.PrepareEngine(ctx, &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)

	jsConfig := map[string]any{
		"baseBranch":    "main",
		"strategy":      "directory",
		"maxFiles":      10,
		"branchPrefix":  "split/",
		"verifyCommand": "true",
		"dryRun":        false,
		"jsonOutput":    false,
	}
	maps.Copy(jsConfig, overrides)

	engine.SetGlobal("config", map[string]any{"name": "pr-split"})
	engine.SetGlobal("prSplitConfig", jsConfig)
	engine.SetGlobal("args", []string{})
	engine.SetGlobal("prSplitTemplate", prSplitTemplate)

	if err := loadChunkedScript(engine); err != nil {
		t.Fatal(err)
	}

	// Install compat shim: re-expose monolith globals for satellite tests.
	shim := engine.LoadScriptString("pr-split/compat-shim", chunkCompatShim)
	if err := engine.ExecuteScript(shim); err != nil {
		t.Fatalf("compat shim failed: %v", err)
	}

	tm := engine.GetTUIManager()
	dispatch := func(name string, args []string) error {
		return dispatchAwaitPromise(engine, tm, name, args)
	}

	evalJS := func(js string) (any, error) {
		done := make(chan struct{})
		var result any
		var resultErr error

		submitErr := engine.Loop().Submit(func() {
			vm := engine.Runtime()

			// If the JS contains 'await', it must run inside an async IIFE.
			// All await-containing calls are expressions (not statements),
			// so the `var __res = <js>` wrapping is safe for these.
			if strings.Contains(js, "await ") {
				// Convert IIFEs to async so `await` is valid inside them.
				// Only convert top-level IIFEs: (function() {...})() → (async function() {...})()
				// Do NOT convert callbacks like .map(function(...) {...}) — those must
				// remain sync to return values, not Promises.
				js = convertIIFEsToAsync(js)
				callID := atomic.AddInt64(&evalJSCallID, 1)
				resultVar := fmt.Sprintf("__evalResult_%d", callID)
				errorVar := fmt.Sprintf("__evalError_%d", callID)
				_ = vm.Set(resultVar, func(val any) {
					result = val
					close(done)
				})
				_ = vm.Set(errorVar, func(msg string) {
					resultErr = errors.New(msg)
					close(done)
				})
				wrapped := "(async function() {\n\ttry {\n\t\tvar __res = " + js + ";\n\t\tif (__res && typeof __res.then === 'function') { __res = await __res; }\n\t\t" + resultVar + "(__res);\n\t} catch(e) {\n\t\t" + errorVar + "(e.message || String(e));\n\t}\n})();"
				if _, runErr := vm.RunString(wrapped); runErr != nil {
					resultErr = runErr
					close(done)
				}
				return
			}

			// No await: run directly via RunString. This handles both
			// statement-level JS (var decls, assignments, mock setup)
			// and expression-level JS (JSON.stringify(...), function
			// calls) because RunString returns the completion value.
			val, err := vm.RunString(js)
			if err != nil {
				resultErr = err
				close(done)
				return
			}

			// Check if the result is a Promise (duck-type via .then).
			// If so, chain .then/.catch to await it properly.
			if val != nil && !goja.IsUndefined(val) && !goja.IsNull(val) {
				obj := val.ToObject(vm)
				if obj != nil {
					thenProp := obj.Get("then")
					if thenProp != nil && !goja.IsUndefined(thenProp) {
						if thenFn, ok := goja.AssertFunction(thenProp); ok {
							onFulfilled := vm.ToValue(func(call goja.FunctionCall) goja.Value {
								result = call.Argument(0).Export()
								close(done)
								return goja.Undefined()
							})
							onRejected := vm.ToValue(func(call goja.FunctionCall) goja.Value {
								resultErr = fmt.Errorf("promise rejected: %v", call.Argument(0).Export())
								close(done)
								return goja.Undefined()
							})

							thenResult, thenErr := thenFn(val, onFulfilled)
							if thenErr != nil {
								resultErr = thenErr
								close(done)
								return
							}
							// Chain .catch on the .then result.
							thenObj := thenResult.ToObject(vm)
							catchProp := thenObj.Get("catch")
							if catchFn, ok := goja.AssertFunction(catchProp); ok {
								if _, catchErr := catchFn(thenResult, onRejected); catchErr != nil {
									resultErr = catchErr
									close(done)
								}
							}
							return // Will be signaled by .then/.catch callback
						}
					}
				}
			}

			// Synchronous result.
			if val != nil {
				result = val.Export()
			}
			close(done)
		})
		if submitErr != nil {
			return nil, submitErr
		}

		select {
		case <-done:
			return result, resultErr
		case <-time.After(evalJSTimeout):
			return nil, fmt.Errorf("evalJS timed out after %s", evalJSTimeout)
		}
	}

	// evalJSAsync wraps JS in an async IIFE and awaits the result.
	// Use this for async functions like automatedSplit.
	// The JS expression should be like: "await prSplit.automatedSplit({...})"
	// or "JSON.stringify(await prSplit.automatedSplit({...}))"
	// Also handles statement-level JS (var decls, assignments) via direct execution.
	evalJSAsync := func(js string) (any, error) {
		done := make(chan struct{})
		var result any
		var resultErr error

		submitErr := engine.Loop().Submit(func() {
			vm := engine.Runtime()

			// If the JS contains 'await', wrap in async IIFE.
			if strings.Contains(js, "await ") {
				callID := atomic.AddInt64(&evalJSCallID, 1)
				resultVar := fmt.Sprintf("__asyncResult_%d", callID)
				errorVar := fmt.Sprintf("__asyncError_%d", callID)
				_ = vm.Set(resultVar, func(val any) {
					result = val
					close(done)
				})
				_ = vm.Set(errorVar, func(msg string) {
					resultErr = errors.New(msg)
					close(done)
				})
				wrapped := `(async function() {
				try {
					var __res = ` + js + `;
					` + resultVar + `(__res);
				} catch(e) {
					` + errorVar + `(e.message || String(e));
				}
			})();`
				if _, runErr := vm.RunString(wrapped); runErr != nil {
					resultErr = runErr
					close(done)
				}
				return
			}

			// No await: run directly.
			val, err := vm.RunString(js)
			if err != nil {
				resultErr = err
				close(done)
				return
			}

			// Check if result is a Promise.
			if val != nil && !goja.IsUndefined(val) && !goja.IsNull(val) {
				obj := val.ToObject(vm)
				if obj != nil {
					thenProp := obj.Get("then")
					if thenProp != nil && !goja.IsUndefined(thenProp) {
						if thenFn, ok := goja.AssertFunction(thenProp); ok {
							onFulfilled := vm.ToValue(func(call goja.FunctionCall) goja.Value {
								result = call.Argument(0).Export()
								close(done)
								return goja.Undefined()
							})
							onRejected := vm.ToValue(func(call goja.FunctionCall) goja.Value {
								resultErr = fmt.Errorf("promise rejected: %v", call.Argument(0).Export())
								close(done)
								return goja.Undefined()
							})
							thenResult, thenErr := thenFn(val, onFulfilled)
							if thenErr != nil {
								resultErr = thenErr
								close(done)
								return
							}
							thenObj := thenResult.ToObject(vm)
							catchProp := thenObj.Get("catch")
							if catchFn, ok := goja.AssertFunction(catchProp); ok {
								if _, catchErr := catchFn(thenResult, onRejected); catchErr != nil {
									resultErr = catchErr
									close(done)
								}
							}
							return
						}
					}
				}
			}

			if val != nil {
				result = val.Export()
			}
			close(done)
		})
		if submitErr != nil {
			return nil, submitErr
		}

		select {
		case <-done:
			return result, resultErr
		case <-time.After(evalJSTimeout):
			return nil, fmt.Errorf("evalJSAsync timed out after %s", evalJSTimeout)
		}
	}

	return prSplitTestEngine{
		Stdout:      &stdout,
		Dispatch:    dispatch,
		EvalJS:      evalJS,
		EvalJSAsync: evalJSAsync,
		Runtime:     engine.Runtime(),
	}
}

// Compile-time assertion that scripting.Engine is used (to avoid unused import).
var _ = (*scripting.Engine)(nil)

// evalJSCallID generates unique callback names for concurrent evalJS calls.
var evalJSCallID int64

// convertIIFEsToAsync converts leading IIFEs from (function(...) to
// (async function(...) so that `await` is valid inside them. It only
// converts IIFEs at the START of the code — callbacks like .map(function)
// are left alone so they return values, not Promises.
func convertIIFEsToAsync(js string) string {
	trimmed := strings.TrimLeft(js, " \t\n\r")
	// Only convert if the code starts with (function (not already (async function)
	if strings.HasPrefix(trimmed, "(function") && !strings.HasPrefix(trimmed, "(async function") {
		// Find the offset of (function in the original string
		idx := len(js) - len(trimmed)
		return js[:idx] + "(async function" + js[idx+9:]
	}
	return js
}

// ===========================================================================
// Vaporware audit: Tests for previously untested TUI commands
// ===========================================================================

// chdirTestPipeline is a helper that sets up a test pipeline, chdirs to
// its repo, and returns the pipeline. The chdir is undone on test cleanup.
//
// CLEANUP ORDERING: The os.Chdir restoration cleanup is registered BEFORE
// the engine cleanup (inside setupTestPipeline). Since Go's t.Cleanup is
// LIFO, the engine cleanup runs FIRST (while CWD is still the temp dir),
// then CWD restoration runs SECOND. This prevents the JS engine from
// running git operations against the test binary's package directory.
