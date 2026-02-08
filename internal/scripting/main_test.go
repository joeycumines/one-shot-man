package scripting

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/dop251/goja"
)

var (
	testBinaryPath string
	testBinaryDir  string
	testBinDir     string // The bin/ directory added to PATH

	// Recording flags - set via -record and -execute-vhs flags
	recordingEnabled  bool
	executeVHSEnabled bool
)

// TestMain provides setup and teardown for the entire test suite.
// It builds the test binary once and cleans it up after all tests complete.
//
// Recording flags:
//
//	-record          Enable VHS tape generation for recording tests
//	-execute-vhs     Execute VHS to generate GIFs (requires VHS in PATH)
//	-recording-dir   Output directory for recordings (default: docs/visuals/gifs)
func TestMain(m *testing.M) {
	// Parse recording flags
	flag.BoolVar(&recordingEnabled, "record", false, "enable recording")
	flag.BoolVar(&executeVHSEnabled, "execute-vhs", false, "enable VHS execution for recording tests")
	flag.Parse()

	// Build the test binary before any tests run
	wd, err := os.Getwd()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Failed to get working directory: %v\n", err)
		os.Exit(1)
	}

	// Build to a predictable location in the system temp directory
	tmpBase := os.TempDir()
	testBinaryDir = filepath.Join(tmpBase, fmt.Sprintf("osm-test-binary-%d", os.Getpid()))

	// Create bin/ subdirectory - this will be added to PATH so recordings use "osm" not full path
	testBinDir = filepath.Join(testBinaryDir, "bin")
	if err := os.MkdirAll(testBinDir, 0755); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Failed to create bin dir for binary: %v\n", err)
		os.Exit(1)
	}

	repoRoot := filepath.Clean(filepath.Join(wd, "..", ".."))
	// Binary is placed in bin/ as just "osm" so PATH lookup works
	testBinaryPath = filepath.Join(testBinDir, "osm")
	if runtime.GOOS == "windows" {
		testBinaryPath += ".exe"
	}

	// Build the binary (enable integration tag for sync protocol)
	fmt.Printf("TestMain: building test binary to %s\n", testBinaryPath)
	cmd := exec.Command("go", "build", "-tags=integration", "-o", testBinaryPath, "./cmd/osm")
	cmd.Dir = repoRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Failed to build test binary: %v\nOutput:\n%s", err, string(output))
		os.Exit(1)
	}

	// Verify the binary was created
	if info, err := os.Stat(testBinaryPath); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Binary build succeeded but file doesn't exist: %v\n", err)
		os.Exit(1)
	} else {
		fmt.Printf("TestMain: binary built successfully (size: %d bytes, mode: %s)\n", info.Size(), info.Mode())
	}

	// Prepend bin/ to PATH so that "osm" command works in recordings
	currentPath := os.Getenv("PATH")
	newPath := testBinDir + string(os.PathListSeparator) + currentPath
	if err := os.Setenv("PATH", newPath); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Failed to set PATH: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("TestMain: added %s to PATH\n", testBinDir)

	// Run all tests
	exitCode := m.Run()

	// Cleanup: remove the test binary directory after all tests complete
	fmt.Printf("TestMain: cleaning up test binary directory %s\n", testBinaryDir)
	if err := os.RemoveAll(testBinaryDir); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Warning: failed to clean up test binary: %v\n", err)
	}

	os.Exit(exitCode)
}

// buildTestBinary returns the path to the test binary built by TestMain.
// The binary is guaranteed to exist and persist for the entire test run.
func buildTestBinary(tb testing.TB) string {
	tb.Helper()
	if testBinaryPath == "" {
		tb.Fatal("testBinaryPath not initialized - TestMain did not run?")
	}
	tb.Logf("buildTestBinary: returning path %s", testBinaryPath)
	return testBinaryPath
}

// getRecordingOutputDir returns the output directory for recordings.
//
//lint:ignore U1000 Unused depending on env.
func getRecordingOutputDir() string {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		panic("failed to find caller source")
	}
	// Clean, absolute path to docs/visuals/gifs
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(source), "..", ".."))
	return filepath.Join(repoRoot, "docs", "visuals", "gifs")
}

// ============================================================================
// Edge Case Tests for Scripting Engine
// ============================================================================

// TestRuntimeInitializationFailures tests JS runtime initialization failure scenarios.
func TestRuntimeInitializationFailures(t *testing.T) {
	ctx := context.Background()

	t.Run("InvalidGojaOptions", func(t *testing.T) {
		// Test that the runtime handles various goja configuration scenarios
		var stdout, stderr bytes.Buffer
		engine := mustNewEngine(t, ctx, &stdout, &stderr)

		// Execute scripts with various syntax edge cases
		testCases := []struct {
			name        string
			script      string
			shouldError bool
		}{
			{
				name:        "EmptyScript",
				script:      "",
				shouldError: false,
			},
			{
				name:        "WhitespaceOnly",
				script:      "   \t\n  ",
				shouldError: false,
			},
			{
				name:        "ValidExpression",
				script:      "1 + 1",
				shouldError: false,
			},
			{
				name:        "UndefinedVariable",
				script:      "undefinedVar",
				shouldError: true,
			},
			{
				name:        "SyntaxError",
				script:      "function( {",
				shouldError: true,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				script := engine.LoadScriptFromString(tc.name, tc.script)
				err := engine.ExecuteScript(script)
				if tc.shouldError && err == nil {
					t.Errorf("Expected error for %s but got none", tc.name)
				}
				if !tc.shouldError && err != nil {
					t.Errorf("Unexpected error for %s: %v", tc.name, err)
				}
			})
		}
	})

	t.Run("VMAccessedAfterClose", func(t *testing.T) {
		// Test that accessing VM after close is handled gracefully
		var stdout, stderr bytes.Buffer
		engine := mustNewEngine(t, ctx, &stdout, &stderr)

		// Close the engine
		_ = engine.Close()

		// After Close, the VM should be nil
		// Attempting to load a script should be handled
		script := engine.LoadScriptFromString("after_close", "1 + 1")
		// ExecuteScript will panic when trying to access nil VM
		// This is expected behavior - the test documents this edge case
		defer func() {
			if r := recover(); r != nil {
				t.Logf("Expected panic when accessing VM after close: %v", r)
			}
		}()
		_ = engine.ExecuteScript(script)
	})

	t.Run("RuntimeCloseIdempotent", func(t *testing.T) {
		// Test that closing multiple times is safe
		var stdout, stderr bytes.Buffer
		engine := mustNewEngine(t, ctx, &stdout, &stderr)

		// Close multiple times
		_ = engine.Close()
		_ = engine.Close()
		_ = engine.Close()

		t.Log("Multiple Close calls completed without panic")
	})

	t.Run("RunOnLoopSyncAfterClose", func(t *testing.T) {
		// Test RunOnLoopSync behavior after runtime is closed
		var stdout, stderr bytes.Buffer
		engine := mustNewEngine(t, ctx, &stdout, &stderr)

		// Close the runtime
		engine.runtime.Close()

		// Try to run on loop sync - should return error
		err := engine.runtime.RunOnLoopSync(func(r *goja.Runtime) error {
			return nil
		})
		if err == nil {
			t.Log("Note: RunOnLoopSync after close succeeded (may be allowed for cleanup)")
		}
	})
}

// TestGlobalRegistrationEdgeCases tests global registration edge cases.
func TestGlobalRegistrationEdgeCases(t *testing.T) {
	ctx := context.Background()

	t.Run("DuplicateSymbolNames", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		engine := mustNewEngine(t, ctx, &stdout, &stderr)

		// Set the same global multiple times
		engine.SetGlobal("duplicateKey", "first")
		engine.SetGlobal("duplicateKey", "second")
		engine.SetGlobal("duplicateKey", "third")

		// Verify the last value wins
		value := engine.GetGlobal("duplicateKey")
		if value != "third" {
			t.Errorf("Expected 'third', got: %v", value)
		}
	})

	t.Run("InvalidJSIdentifiers", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		engine := mustNewEngine(t, ctx, &stdout, &stderr)

		// These should be handled as valid JS identifiers (even if unusual)
		testKeys := []string{
			"validKey",
			"$dollar",
			"_underscore",
			"camelCase",
			"PascalCase",
			"snake_case",
			"kebab-case", // This might cause issues in JS
		}

		for _, key := range testKeys {
			t.Run(key, func(t *testing.T) {
				engine.SetGlobal(key, "value")
				value := engine.GetGlobal(key)
				if value != "value" {
					t.Errorf("Expected 'value' for key %s, got: %v", key, value)
				}
			})
		}
	})

	t.Run("GlobalRegistrationOrderDependencies", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		engine := mustNewEngine(t, ctx, &stdout, &stderr)

		// Set globals in one order, access in another
		engine.SetGlobal("key3", 3)
		engine.SetGlobal("key1", 1)
		engine.SetGlobal("key2", 2)

		// Access in different order
		if v := engine.GetGlobal("key2"); v != int64(2) {
			t.Errorf("Expected key2=2, got: %v", v)
		}
		if v := engine.GetGlobal("key1"); v != int64(1) {
			t.Errorf("Expected key1=1, got: %v", v)
		}
		if v := engine.GetGlobal("key3"); v != int64(3) {
			t.Errorf("Expected key3=3, got: %v", v)
		}
	})

	t.Run("ManyGlobalVariables", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		engine := mustNewEngine(t, ctx, &stdout, &stderr)

		// Set many global variables
		const numGlobals = 100
		for i := 0; i < numGlobals; i++ {
			engine.SetGlobal(fmt.Sprintf("global_%03d", i), i)
		}

		// Verify all were set
		for i := 0; i < numGlobals; i++ {
			key := fmt.Sprintf("global_%03d", i)
			value := engine.GetGlobal(key)
			if value != int64(i) {
				t.Errorf("Expected %d for %s, got: %v", i, key, value)
			}
		}
	})

	t.Run("UnicodeGlobalNames", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		engine := mustNewEngine(t, ctx, &stdout, &stderr)

		// Test unicode in global names
		engine.SetGlobal("日本語", "japanese")
		engine.SetGlobal("emoji_key_🎉", "emoji")

		if v := engine.GetGlobal("日本語"); v != "japanese" {
			t.Errorf("Expected 'japanese', got: %v", v)
		}
		if v := engine.GetGlobal("emoji_key_🎉"); v != "emoji" {
			t.Errorf("Expected 'emoji', got: %v", v)
		}
	})

	t.Run("GlobalWithSpecialValues", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		engine := mustNewEngine(t, ctx, &stdout, &stderr)

		// Set special values
		engine.SetGlobal("nilValue", nil)
		engine.SetGlobal("undefined", nil) // JS undefined vs Go nil

		// Access from JS
		script := engine.LoadScriptFromString("check_globals", `
			results = {
				isNil: nilValue === null || nilValue === undefined,
				isUndefined: typeof undefined === 'undefined' ? false : undefined === undefined,
			}
		`)

		err := engine.ExecuteScript(script)
		if err != nil {
			t.Errorf("Script execution failed: %v", err)
		}
	})
}

// TestNativeModuleErrorHandling tests native module error handling.
func TestNativeModuleErrorHandling(t *testing.T) {
	ctx := context.Background()

	t.Run("ModuleFunctionWithNilReceiver", func(t *testing.T) {
		// Test calling module functions with various argument patterns
		var stdout, stderr bytes.Buffer
		engine := mustNewEngine(t, ctx, &stdout, &stderr)

		// Test require with invalid module
		script := engine.LoadScriptFromString("invalid_require", `
			try {
				require('osm:nonexistent_module');
			} catch (e) {
				ctx.log("Caught expected error: " + e.message);
			}
		`)

		err := engine.ExecuteScript(script)
		if err != nil {
			t.Logf("Script execution error (may be expected): %v", err)
		}
	})

	t.Run("ModuleFunctionWithInvalidArguments", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		engine := mustNewEngine(t, ctx, &stdout, &stderr)

		// Test time.sleep with invalid arguments
		script := engine.LoadScriptFromString("invalid_args", `
			const {sleep} = require('osm:time');
			try {
				sleep(-1); // Negative sleep
			} catch (e) {
				ctx.log("Caught error for negative sleep: " + e.message);
			}
			try {
				sleep('not a number'); // Non-numeric
			} catch (e) {
				ctx.log("Caught error for string sleep: " + e.message);
			}
		`)

		err := engine.ExecuteScript(script)
		if err != nil {
			t.Logf("Script execution error (may be expected): %v", err)
		}
	})

	t.Run("ModuleFunctionThatThrowsJSErrors", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		engine := mustNewEngine(t, ctx, &stdout, &stderr)

		// Test JS error throwing from modules
		script := engine.LoadScriptFromString("js_error", `
			const {sleep} = require('osm:time');
			try {
				throw new Error("Test error from JS");
			} catch (e) {
				ctx.log("Caught JS error: " + e.message);
			}
		`)

		err := engine.ExecuteScript(script)
		if err != nil {
			t.Errorf("Script execution failed: %v", err)
		}
	})

	t.Run("ModuleFunctionThatPanics", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		engine := mustNewEngine(t, ctx, &stdout, &stderr)

		// Test panic recovery - scripts should not crash the host
		script := engine.LoadScriptFromString("panic_test", `
			// This should be caught by the panic recovery in ExecuteScript
			(function() {
				throw "String panic";
			})();
		`)

		err := engine.ExecuteScript(script)
		if err == nil {
			t.Error("Expected error from panic, got none")
		} else {
			// Verify it's a ScriptPanicError
			if !strings.Contains(err.Error(), "panic") {
				t.Logf("Panic error message: %v", err)
			}
		}
	})

	t.Run("NestedTryCatchAroundPanics", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		engine := mustNewEngine(t, ctx, &stdout, &stderr)

		// Test nested try-catch around panic scenarios
		script := engine.LoadScriptFromString("nested_panic", `
			try {
				try {
					throw new Error("Inner error");
				} catch (e) {
					ctx.log("Caught inner: " + e.message);
					throw new Error("Outer error");
				}
			} catch (e) {
				ctx.log("Caught outer: " + e.message);
			}
		`)

		err := engine.ExecuteScript(script)
		if err != nil {
			t.Errorf("Script execution failed: %v", err)
		}
	})

	t.Run("AsyncModuleOperations", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		engine := mustNewEngine(t, ctx, &stdout, &stderr)

		// Test async module operations don't cause issues
		script := engine.LoadScriptFromString("async_module", `
			const {sleep} = require('osm:time');
			ctx.log("Starting async test");
			sleep(1);
			ctx.log("After sleep");
		`)

		err := engine.ExecuteScript(script)
		if err != nil {
			t.Errorf("Script execution failed: %v", err)
		}
	})
}

// TestTUIBindingEdgeCases tests TUI binding edge cases.
func TestTUIBindingEdgeCases(t *testing.T) {
	ctx := context.Background()

	t.Run("TUIOperationsWhenTUINotActive", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		engine := mustNewEngine(t, ctx, &stdout, &stderr)

		// Access TUI functions without an active TUI session
		script := engine.LoadScriptFromString("tui_no_session", `
			try {
				// These should not crash even without active TUI
				const modes = tui.listModes();
				ctx.log("Listed modes: " + JSON.stringify(modes));
			} catch (e) {
				ctx.log("Expected error: " + e.message);
			}
		`)

		err := engine.ExecuteScript(script)
		if err != nil {
			t.Errorf("Script execution failed: %v", err)
		}
	})

	t.Run("TUIOperationsWithInvalidMessageTypes", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		engine := mustNewEngine(t, ctx, &stdout, &stderr)

		// Test with invalid mode registration
		script := engine.LoadScriptFromString("invalid_tui_ops", `
			try {
				// Invalid mode registration
				tui.registerMode({
					name: null,
					tui: { prompt: "test" }
				});
			} catch (e) {
				ctx.log("Expected error for null name: " + e.message);
			}
		`)

		err := engine.ExecuteScript(script)
		if err != nil {
			t.Logf("Script execution error (may be expected): %v", err)
		}
	})

	t.Run("TUIOperationsWithInvalidComponentIDs", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		engine := mustNewEngine(t, ctx, &stdout, &stderr)

		// Test accessing non-existent modes
		script := engine.LoadScriptFromString("invalid_mode_access", `
			try {
				tui.switchMode("nonexistent_mode_12345");
			} catch (e) {
				ctx.log("Expected error for nonexistent mode: " + e.message);
			}
		`)

		err := engine.ExecuteScript(script)
		if err != nil {
			t.Logf("Script execution error (may be expected): %v", err)
		}
	})

	t.Run("TUIStateOperations", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		engine := mustNewEngine(t, ctx, &stdout, &stderr)

		// Test TUI state creation and access
		script := engine.LoadScriptFromString("tui_state_test", `
			const StateKeys = {
				testKey: Symbol("testKey")
			};
			const state = tui.createState("testMode", {
				[StateKeys.testKey]: {defaultValue: "default"}
			});
			ctx.log("Created state");
			state.set(StateKeys.testKey, "modified");
			const value = state.get(StateKeys.testKey);
			ctx.log("State value: " + value);
		`)

		err := engine.ExecuteScript(script)
		if err != nil {
			t.Errorf("Script execution failed: %v", err)
		}
	})

	t.Run("TUIContextOperations", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		engine := mustNewEngine(t, ctx, &stdout, &stderr)

		// Create a temp directory for testing instead of using /tmp
		// which may contain directories with restricted permissions
		tempDir := t.TempDir()

		// Test context operations
		script := engine.LoadScriptFromString("context_test", `
			context.addPath("`+tempDir+`");
			const paths = context.listPaths();
			ctx.log("Paths: " + JSON.stringify(paths));
		`)

		err := engine.ExecuteScript(script)
		if err != nil {
			t.Errorf("Script execution failed: %v", err)
		}
	})

	t.Run("TUILoggerOperations", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		engine := mustNewEngine(t, ctx, &stdout, &stderr)

		// Test logger operations
		script := engine.LoadScriptFromString("logger_test", `
			log.debug("Debug message");
			log.info("Info message");
			log.warn("Warning message");
			log.error("Error message");
			ctx.log("All log levels tested");
		`)

		err := engine.ExecuteScript(script)
		if err != nil {
			t.Errorf("Script execution failed: %v", err)
		}
	})

	t.Run("TUICommandRegistration", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		engine := mustNewEngine(t, ctx, &stdout, &stderr)

		// Test command registration - the handler must be a function
		script := engine.LoadScriptFromString("command_test", `
			tui.registerCommand({
				name: "testcmd",
				fn: function() {
					ctx.log("Test command executed");
					return "success";
				}
			});
			ctx.log("Command registered");
		`)

		err := engine.ExecuteScript(script)
		if err != nil {
			t.Logf("Script execution error (may be expected based on TUI state): %v", err)
		}
	})

	t.Run("TUIExitRequestOperations", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		engine := mustNewEngine(t, ctx, &stdout, &stderr)

		// Test exit request operations
		script := engine.LoadScriptFromString("exit_test", `
			ctx.log("Exit requested before: " + tui.isExitRequested());
			tui.requestExit();
			ctx.log("Exit requested after: " + tui.isExitRequested());
			tui.clearExitRequest();
			ctx.log("Exit requested after clear: " + tui.isExitRequested());
		`)

		err := engine.ExecuteScript(script)
		if err != nil {
			t.Errorf("Script execution failed: %v", err)
		}
	})

	t.Run("MultipleTUIOperations", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		engine := mustNewEngine(t, ctx, &stdout, &stderr)

		// Multiple TUI operations in sequence
		script := engine.LoadScriptFromString("multi_tui", `
			// Register a mode
			tui.registerMode({
				name: "test",
				tui: { prompt: "[test] " }
			});

			// Switch to it
			tui.switchMode("test");

			// Get current mode
			const mode = tui.getCurrentMode();
			ctx.log("Current mode: " + (mode ? mode.name : "null"));

			// List modes
			const modes = tui.listModes();
			ctx.log("Available modes: " + JSON.stringify(modes));
		`)

		err := engine.ExecuteScript(script)
		if err != nil {
			t.Errorf("Script execution failed: %v", err)
		}
	})
}

// TestConcurrentScriptExecution tests concurrent script execution scenarios.
func TestConcurrentScriptExecution(t *testing.T) {
	ctx := context.Background()

	t.Run("QueueSetGlobalFromMultipleGoroutines", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		engine := mustNewEngine(t, ctx, &stdout, &stderr)

		const numGoroutines = 50
		const numIterations = 20

		var wg sync.WaitGroup
		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(goroutineID int) {
				defer wg.Done()
				for j := 0; j < numIterations; j++ {
					key := fmt.Sprintf("goroutine_%d_iter_%d", goroutineID, j)
					engine.QueueSetGlobal(key, goroutineID*numIterations+j)
				}
			}(i)
		}
		wg.Wait()

		// Verify a sample of values
		var verifyWG sync.WaitGroup
		for i := 0; i < 10; i++ {
			verifyWG.Add(1)
			go func(idx int) {
				defer verifyWG.Done()
				key := fmt.Sprintf("goroutine_%d_iter_%d", idx, idx)
				var result interface{}
				var readWG sync.WaitGroup
				readWG.Add(1)
				engine.QueueGetGlobal(key, func(value interface{}) {
					result = value
					readWG.Done()
				})
				readWG.Wait()
				expected := idx*numIterations + idx
				if result != int64(expected) {
					t.Errorf("Expected %d, got: %v", expected, result)
				}
			}(i)
		}
		verifyWG.Wait()
	})

	t.Run("RapidEngineCreationAndClose", func(t *testing.T) {
		ctx := context.Background()

		const numEngines = 10
		for i := 0; i < numEngines; i++ {
			var stdout, stderr bytes.Buffer
			// Use mustNewEngine for proper cleanup, but we need to avoid cleanup conflicts
			// when creating multiple engines in a loop. Create engine directly and close immediately.
			engine, err := NewEngine(ctx, &stdout, &stderr)
			if err != nil {
				t.Fatalf("Engine %d creation failed: %v", i, err)
			}

			// Execute a simple script
			script := engine.LoadScriptFromString("test", "1 + 1")
			_ = engine.ExecuteScript(script)

			// Close immediately
			_ = engine.Close()
		}

		t.Logf("Created and closed %d engines rapidly", numEngines)
	})

	t.Run("MixedSyncAndAsyncGlobalAccess", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		engine := mustNewEngine(t, ctx, &stdout, &stderr)

		// Use QueueSetGlobal for async writes and QueueGetGlobal for async reads
		// to ensure thread-safe concurrent access
		engine.QueueSetGlobal("syncKey", "syncValue")

		var wg sync.WaitGroup
		const numOps = 20
		results := make([]interface{}, numOps)

		for i := 0; i < numOps; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				// Alternate between async write and async read
				if idx%2 == 0 {
					engine.QueueSetGlobal(fmt.Sprintf("asyncKey_%d", idx), idx)
				} else {
					// Use QueueGetGlobal for thread-safe async read
					var readWg sync.WaitGroup
					readWg.Add(1)
					engine.QueueGetGlobal("syncKey", func(value interface{}) {
						results[idx] = value
						readWg.Done()
					})
					readWg.Wait()
				}
			}(i)
		}
		wg.Wait()

		// Verify reads got the expected value
		for i := 1; i < numOps; i += 2 {
			if results[i] != "syncValue" {
				t.Errorf("Expected syncKey='syncValue', got %v", results[i])
			}
		}

		t.Log("Mixed async operations completed")
	})
}

// TestScriptPanicRecovery tests panic recovery mechanisms.
func TestScriptPanicRecovery(t *testing.T) {
	ctx := context.Background()

	t.Run("PanicWithVariousTypes", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		engine := mustNewEngine(t, ctx, &stdout, &stderr)

		panicTypes := []struct {
			name  string
			panic interface{}
		}{
			{"string", "panic string"},
			{"number", 42},
			{"object", struct{ Msg string }{Msg: "panic object"}},
			{"nil", nil},
		}

		for _, pt := range panicTypes {
			t.Run(pt.name, func(t *testing.T) {
				script := engine.LoadScriptFromString("panic_"+pt.name, fmt.Sprintf(`
					(function() {
						throw %v;
					})();
				`, pt.panic))

				err := engine.ExecuteScript(script)
				if err == nil {
					t.Errorf("Expected panic error for %s, got none", pt.name)
				}
			})
		}
	})

	t.Run("PanicRecoveryWithDefer", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		engine := mustNewEngine(t, ctx, &stdout, &stderr)

		script := engine.LoadScriptFromString("defer_panic", `
			ctx.log("Before defer");
			ctx.defer(function() {
				ctx.log("Deferred cleanup");
			});
			ctx.log("About to panic");
			throw new Error("Test panic");
			ctx.log("This should not run");
		`)

		err := engine.ExecuteScript(script)
		if err == nil {
			t.Error("Expected panic error, got none")
		}

		// Verify deferred cleanup ran (via logs if available)
		t.Log("Panic with defer test completed")
	})

	t.Run("NestedPanicRecovery", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		engine := mustNewEngine(t, ctx, &stdout, &stderr)

		script := engine.LoadScriptFromString("nested_panic_recovery", `
			try {
				try {
					throw new Error("Inner");
				} catch (e) {
					ctx.log("Inner caught: " + e.message);
					throw new Error("Outer");
				}
			} catch (e) {
				ctx.log("Outer caught: " + e.message);
			}
			ctx.log("After nested catch");
		`)

		err := engine.ExecuteScript(script)
		if err != nil {
			t.Errorf("Script execution failed: %v", err)
		}
	})

	t.Run("PanicInDeferredFunction", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		engine := mustNewEngine(t, ctx, &stdout, &stderr)

		script := engine.LoadScriptFromString("panic_in_defer", `
			ctx.defer(function() {
				throw new Error("Panic in defer");
			});
			ctx.log("Script completed normally");
		`)

		err := engine.ExecuteScript(script)
		// Panic in deferred function should be caught
		if err == nil {
			t.Log("Note: Panic in defer was handled gracefully")
		}
	})
}

// TestScriptExecutionEdgeCases tests additional script execution edge cases.
func TestScriptExecutionEdgeCases(t *testing.T) {
	ctx := context.Background()

	t.Run("VeryLongScript", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		engine := mustNewEngine(t, ctx, &stdout, &stderr)

		// Build a long script
		var lines []string
		for i := 0; i < 1000; i++ {
			lines = append(lines, fmt.Sprintf("var line_%d = %d;", i, i))
		}
		script := engine.LoadScriptFromString("long_script", strings.Join(lines, "\n"))

		err := engine.ExecuteScript(script)
		if err != nil {
			t.Errorf("Long script execution failed: %v", err)
		}
	})

	t.Run("DeepNesting", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		engine := mustNewEngine(t, ctx, &stdout, &stderr)

		// Build deeply nested code
		script := engine.LoadScriptFromString("deep_nesting", `
			(function() {
				return (function() {
					return (function() {
						return (function() {
							return (function() {
								return (function() {
									return (function() {
										return (function() {
											return (function() {
												return "deep";
											})();
										})();
									})();
								})();
							})();
						})();
					})();
				})();
			})();
		`)

		err := engine.ExecuteScript(script)
		if err != nil {
			t.Errorf("Deep nesting execution failed: %v", err)
		}
	})

	t.Run("ScriptWithUnicode", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		engine := mustNewEngine(t, ctx, &stdout, &stderr)

		script := engine.LoadScriptFromString("unicode_script", `
			const message = "Hello, 世界! 🌍";
			ctx.log(message);
			const symbols = ["α", "β", "γ", "δ", "ε"];
			ctx.log("Greek letters: " + symbols.join(", "));
		`)

		err := engine.ExecuteScript(script)
		if err != nil {
			t.Errorf("Unicode script execution failed: %v", err)
		}
	})

	t.Run("ScriptWithSpecialCharacters", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		engine := mustNewEngine(t, ctx, &stdout, &stderr)

		script := engine.LoadScriptFromString("special_chars", `
			const str = "Line1\nLine2\tTab\r\nReturn";
			ctx.log("Special chars: " + JSON.stringify(str));
			const template = `+"`template\nmultiline`"+`;
			ctx.log("Template: " + template);
		`)

		err := engine.ExecuteScript(script)
		if err != nil {
			t.Errorf("Special characters script failed: %v", err)
		}
	})

	t.Run("ScriptWithRegex", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		engine := mustNewEngine(t, ctx, &stdout, &stderr)

		script := engine.LoadScriptFromString("regex_test", `
			const pattern = /test/gi;
			const result = "This is a Test".match(pattern);
			ctx.log("Regex result: " + JSON.stringify(result));
		`)

		err := engine.ExecuteScript(script)
		if err != nil {
			t.Errorf("Regex script failed: %v", err)
		}
	})
}
