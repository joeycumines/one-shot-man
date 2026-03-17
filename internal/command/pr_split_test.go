package command

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/joeycumines/one-shot-man/internal/config"
	"github.com/joeycumines/one-shot-man/internal/scripting"
)

// ---------------------------------------------------------------------------
// safeBuffer is a thread-safe bytes.Buffer wrapper for capturing engine output
// in tests. The JS event loop goroutine writes via TUILogger.PrintToTUI
// while the test goroutine reads for assertions and diagnostics. Without
// synchronization, -race detects concurrent access.
// ---------------------------------------------------------------------------

type safeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *safeBuffer) Write(p []byte) (n int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *safeBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

func (s *safeBuffer) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.buf.Reset()
}

// ---------------------------------------------------------------------------
// Git repo + engine helpers for end-to-end tests
// ---------------------------------------------------------------------------

// runGitCmd executes a git command in dir, failing on error.
func runGitCmd(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed in %s: %s", args, dir, string(out))
	}
	return string(out)
}

// gitBranchList returns all local branch names in the given repo directory.
func gitBranchList(t *testing.T, dir string) []string {
	t.Helper()
	raw := runGitCmd(t, dir, "branch", "--list", "--format=%(refname:short)")
	var branches []string
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			branches = append(branches, line)
		}
	}
	return branches
}

// filterPrefix returns only the strings that start with the given prefix.
func filterPrefix(ss []string, prefix string) []string {
	var out []string
	for _, s := range ss {
		if strings.HasPrefix(s, prefix) {
			out = append(out, s)
		}
	}
	return out
}

// setupTestGitRepo creates a temp git repo with main + feature branch for
// pr-split end-to-end tests. Returns the repo directory.
func setupTestGitRepo(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}

	dir := t.TempDir()

	// Initialize repo on main.
	runGitCmd(t, dir, "init")
	runGitCmd(t, dir, "symbolic-ref", "HEAD", "refs/heads/main")
	runGitCmd(t, dir, "config", "user.email", "test@test.com")
	runGitCmd(t, dir, "config", "user.name", "Test User")

	// Create initial files.
	for _, f := range []struct{ path, content string }{
		{"pkg/types.go", "package pkg\n\ntype Foo struct{}\n"},
		{"cmd/main.go", "package main\n\nfunc main() {}\n"},
		{"README.md", "# Test Project\n"},
	} {
		full := filepath.Join(dir, f.path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(f.content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runGitCmd(t, dir, "add", "-A")
	runGitCmd(t, dir, "commit", "-m", "initial commit")

	// Create feature branch with changes in multiple directories.
	runGitCmd(t, dir, "checkout", "-b", "feature")
	for _, f := range []struct{ path, content string }{
		{"pkg/impl.go", "package pkg\n\nfunc Bar() string { return \"bar\" }\n"},
		{"cmd/run.go", "package main\n\nfunc run() {}\n"},
		{"docs/guide.md", "# Guide\n\nUsage instructions.\n"},
		{"docs/api.md", "# API\n\nAPI reference.\n"},
	} {
		full := filepath.Join(dir, f.path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(f.content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runGitCmd(t, dir, "add", "-A")
	runGitCmd(t, dir, "commit", "-m", "feature work")

	return dir
}

// ---------------------------------------------------------------------------
// TestPipeline — configurable harness for integration tests
// ---------------------------------------------------------------------------

// chunkCompatShim is a JavaScript snippet that, when evaluated after loading
// chunks 00-13, re-exports the monolith's formerly-global symbols onto
// globalThis.  This lets the ~25 satellite test files (written for the
// monolith's flat namespace) run unchanged against the chunked architecture.
//
// For functions: Object.defineProperty with get/set proxies so that
//
//	`executeSplit = function() {...}` transparently updates prSplit.executeSplit.
//
// For state vars: same get/set proxy pointing at prSplit._state.
// For modules:    Object.defineProperty proxies so that test overrides
//
//	like `exec = newProxy` propagate to prSplit._modules.exec.
const chunkCompatShim = `
(function() {
    var ps = globalThis.prSplit;
    if (!ps) return;
    var st = ps._state || {};
    var mods = ps._modules || {};

    // --- Module proxies (get/set → prSplit._modules.*) ---
    // Tests override the entire module object (e.g. exec = mockProxy)
    // and chunks read prSplit._modules.exec; both must stay in sync.
    var modNames = ['bt', 'exec', 'osmod', 'template', 'shared', 'lip'];
    modNames.forEach(function(m) {
        if (!mods[m]) return;
        try {
            Object.defineProperty(globalThis, m, {
                get: function() { return mods[m]; },
                set: function(v) { mods[m] = v; },
                configurable: true,
                enumerable: false
            });
        } catch(e) {}
    });

    // --- Function proxies (get/set → prSplit.*) ---
    var funcNames = [
        'analyzeDiff', 'analyzeDiffStats',
        'groupByDirectory', 'groupByExtension', 'groupByPattern',
        'groupByChunks', 'groupByDependency', 'applyStrategy', 'selectStrategy',
        'parseGoImports', 'detectGoModulePath',
        'createSplitPlan', 'savePlan', 'loadPlan',
        'validateClassification', 'validatePlan', 'validateSplitPlan', 'validateResolution',
        'executeSplit',
        'verifySplit', 'verifySplits', 'verifyEquivalence', 'verifyEquivalenceDetailed',
        'cleanupBranches',
        'createPRs',
        'resolveConflicts',
        'ClaudeCodeExecutor',
        'renderClassificationPrompt', 'renderSplitPlanPrompt', 'renderConflictPrompt',
        'renderPrompt',
        'detectLanguage',
        'automatedSplit', 'heuristicFallback', 'sendToHandle', 'waitForLogged',
        'classificationToGroups',
        'assessIndependence', 'splitsAreIndependent', 'splitsAreIndependentFromMaps',
        'recordConversation', 'getConversationHistory',
        'recordTelemetry', 'getTelemetrySummary', 'saveTelemetry',
        'renderColorizedDiff', 'getSplitDiff',
        'buildDependencyGraph', 'renderAsciiGraph',
        'analyzeRetrospective',
        'cleanupExecutor',
        // T31 async versions — proxied so tests can override via bare globals.
        'analyzeDiffAsync', 'createSplitPlanAsync', 'executeSplitAsync',
        'verifySplitAsync', 'verifySplitsAsync', 'verifyEquivalenceAsync',
        'cleanupBranchesAsync'
    ];

    funcNames.forEach(function(k) {
        if (typeof ps[k] === 'undefined') return;
        try {
            Object.defineProperty(globalThis, k, {
                get: function() { return ps[k]; },
                set: function(v) { ps[k] = v; },
                configurable: true,
                enumerable: false
            });
        } catch(e) { /* skip if already defined */ }
    });

    // --- Internal helpers with _ prefix (monolith had bare names) ---
    var internalNames = {
        'gitExec':           '_gitExec',
        'shellQuote':        '_shellQuote',
        'gitAddChangedFiles':'_gitAddChangedFiles',
        'dirname':           '_dirname',
        'fileExtension':     '_fileExtension',
        'sanitizeBranchName':'_sanitizeBranchName',
        'padIndex':          '_padIndex',
        'isCancelled':       'isCancelled',
        'isPaused':          'isPaused',
        'isForceCancelled':  'isForceCancelled'
    };
    Object.keys(internalNames).forEach(function(bare) {
        var real = internalNames[bare];
        if (typeof ps[real] === 'undefined') return;
        try {
            Object.defineProperty(globalThis, bare, {
                get: function() { return ps[real]; },
                set: function(v) { ps[real] = v; },
                configurable: true,
                enumerable: false
            });
        } catch(e) {}
    });

    // --- Constants ---
    if (ps.AUTOMATED_DEFAULTS) globalThis.AUTOMATED_DEFAULTS = ps.AUTOMATED_DEFAULTS;
    if (ps.AUTO_FIX_STRATEGIES) globalThis.AUTO_FIX_STRATEGIES = ps.AUTO_FIX_STRATEGIES;
    if (ps.DEFAULT_PLAN_PATH) globalThis.DEFAULT_PLAN_PATH = ps.DEFAULT_PLAN_PATH;
    if (ps.CLASSIFICATION_PROMPT_TEMPLATE) globalThis.CLASSIFICATION_PROMPT_TEMPLATE = ps.CLASSIFICATION_PROMPT_TEMPLATE;
    if (ps.SPLIT_PLAN_PROMPT_TEMPLATE) globalThis.SPLIT_PLAN_PROMPT_TEMPLATE = ps.SPLIT_PLAN_PROMPT_TEMPLATE;
    if (ps.CONFLICT_RESOLUTION_PROMPT_TEMPLATE) globalThis.CONFLICT_RESOLUTION_PROMPT_TEMPLATE = ps.CONFLICT_RESOLUTION_PROMPT_TEMPLATE;

    // --- runtime proxy (bare global → prSplit.runtime) ---
    try {
        Object.defineProperty(globalThis, 'runtime', {
            get: function() { return ps.runtime; },
            set: function(v) { ps.runtime = v; },
            configurable: true,
            enumerable: false
        });
    } catch(e) {}

    // --- State variable proxies (get/set → prSplit._state.*) ---
    var stateNames = [
        'analysisCache', 'groupsCache', 'planCache',
        'executionResultCache', 'conversationHistory',
        'claudeExecutor', 'mcpCallbackObj'
    ];
    stateNames.forEach(function(k) {
        try {
            Object.defineProperty(globalThis, k, {
                get: function() { return st[k]; },
                set: function(v) { st[k] = v; },
                configurable: true,
                enumerable: false
            });
        } catch(e) {}
    });

    // --- _mcpCallbackObj bridge: chunks read prSplit._mcpCallbackObj,
    //     tests set mcpCallbackObj as bare global → prSplit._state ---
    try {
        Object.defineProperty(ps, '_mcpCallbackObj', {
            get: function() { return st.mcpCallbackObj; },
            set: function(v) { st.mcpCallbackObj = v; },
            configurable: true
        });
    } catch(e) {}

    // --- _extract* aliases (monolith exported with _, chunks without) ---
    if (ps.extractDirs) ps._extractDirs = ps.extractDirs;
    if (ps.extractGoPkgs) ps._extractGoPkgs = ps.extractGoPkgs;
    if (ps.extractGoImports) ps._extractGoImports = ps.extractGoImports;

    // --- Also expose verify helpers that were in monolith scope ---
    if (ps.discoverVerifyCommand) globalThis.discoverVerifyCommand = ps.discoverVerifyCommand;
    if (ps.scopedVerifyCommand) globalThis.scopedVerifyCommand = ps.scopedVerifyCommand;
})();
`

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
}

// setupTestPipeline creates a test pipeline with configurable initial files,
// feature branch files, and config overrides.
func setupTestPipeline(t *testing.T, opts TestPipelineOpts) *TestPipeline {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}

	dir := t.TempDir()
	resultDir := filepath.Join(t.TempDir(), "mcp-results")
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
	overrides := map[string]any{
		"baseBranch": "main",
		"dir":        dir,
	}
	for k, v := range opts.ConfigOverrides {
		overrides[k] = v
	}

	stdout, dispatch, evalJS, evalJSAsync := loadPrSplitEngineWithEval(t, overrides)

	return &TestPipeline{
		Dir:         dir,
		ResultDir:   resultDir,
		Stdout:      stdout,
		Dispatch:    dispatch,
		EvalJS:      evalJS,
		EvalJSAsync: evalJSAsync,
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
	for _, chunk := range prSplitChunks {
		b.WriteString(*chunk.source)
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
	for k, v := range overrides {
		jsConfig[k] = v
	}

	engine.SetGlobal("config", map[string]any{"name": "pr-split"})
	engine.SetGlobal("prSplitConfig", jsConfig)
	engine.SetGlobal("args", []string{})
	engine.SetGlobal("prSplitTemplate", prSplitTemplate)

	if err := loadChunkedScript(engine); err != nil {
		t.Fatal(err)
	}

	// Install compat shim: re-expose monolith globals for satellite tests.
	shim := engine.LoadScriptFromString("pr-split/compat-shim", chunkCompatShim)
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

func loadPrSplitEngineWithEval(t testing.TB, overrides map[string]any) (*safeBuffer, func(string, []string) error, func(string) (any, error), func(string) (any, error)) {
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
	for k, v := range overrides {
		jsConfig[k] = v
	}

	engine.SetGlobal("config", map[string]any{"name": "pr-split"})
	engine.SetGlobal("prSplitConfig", jsConfig)
	engine.SetGlobal("args", []string{})
	engine.SetGlobal("prSplitTemplate", prSplitTemplate)

	if err := loadChunkedScript(engine); err != nil {
		t.Fatal(err)
	}

	// Install compat shim: re-expose monolith globals for satellite tests.
	shim := engine.LoadScriptFromString("pr-split/compat-shim", chunkCompatShim)
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
				_ = vm.Set("__evalResult", func(val any) {
					result = val
					close(done)
				})
				_ = vm.Set("__evalError", func(msg string) {
					resultErr = errors.New(msg)
					close(done)
				})
				wrapped := "(async function() {\n\ttry {\n\t\tvar __res = " + js + ";\n\t\tif (__res && typeof __res.then === 'function') { __res = await __res; }\n\t\t__evalResult(__res);\n\t} catch(e) {\n\t\t__evalError(e.message || String(e));\n\t}\n})();"
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
				_ = vm.Set("__asyncResult", func(val any) {
					result = val
					close(done)
				})
				_ = vm.Set("__asyncError", func(msg string) {
					resultErr = errors.New(msg)
					close(done)
				})
				wrapped := `(async function() {
				try {
					var __res = ` + js + `;
					__asyncResult(__res);
				} catch(e) {
					__asyncError(e.message || String(e));
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

	return &stdout, dispatch, evalJS, evalJSAsync
}

// Compile-time assertion that scripting.Engine is used (to avoid unused import).
var _ = (*scripting.Engine)(nil)

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
func chdirTestPipeline(t *testing.T, opts TestPipelineOpts) *TestPipeline {
	t.Helper()
	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	// Register CWD restoration FIRST so it runs LAST in LIFO cleanup order.
	// This ensures the JS engine cleanup (registered by setupTestPipeline)
	// runs while CWD is still set to the temp repo directory.
	t.Cleanup(func() { _ = os.Chdir(oldDir) })
	tp := setupTestPipeline(t, opts)
	if err := os.Chdir(tp.Dir); err != nil {
		t.Fatal(err)
	}
	return tp
}

// runPlanPipeline dispatches analyze → group → plan and returns the pipeline.
func runPlanPipeline(t *testing.T, tp *TestPipeline) {
	t.Helper()
	if err := tp.Dispatch("analyze", nil); err != nil {
		t.Fatalf("analyze: %v", err)
	}
	if err := tp.Dispatch("group", nil); err != nil {
		t.Fatalf("group: %v", err)
	}
	if err := tp.Dispatch("plan", nil); err != nil {
		t.Fatalf("plan: %v", err)
	}
}

func runGitCmdAllowFail(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, _ := cmd.CombinedOutput()
	return string(out)
}

// initGitRepo creates a temporary git repo and returns its path.
// Shared across chunk-level test files that need real git repos.
func initGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	// Use init + symbolic-ref instead of init -b for compatibility
	// with git versions older than 2.28 (e.g. Windows CI).
	gitCmd(t, dir, "init")
	gitCmd(t, dir, "symbolic-ref", "HEAD", "refs/heads/main")
	gitCmd(t, dir, "config", "user.email", "test@test.com")
	gitCmd(t, dir, "config", "user.name", "Test")
	return dir
}

// writeFile creates a file with the given content, creating parent directories.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

// gitCmd runs a git command in a directory and returns combined output.
func gitCmd(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

// escapeJSPath escapes a file path for embedding in a JS string literal.
func escapeJSPath(p string) string {
	return strings.ReplaceAll(p, `\`, `\\`)
}

// jsString returns a JavaScript string literal (single-quoted, with escaping)
// for embedding a Go string into a JS expression.
func jsString(s string) string {
	escaped := strings.ReplaceAll(s, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `'`, `\'`)
	return `'` + escaped + `'`
}
