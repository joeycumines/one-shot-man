package scripting

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestTUIManagerAPI tests the TUI manager JavaScript API
func TestTUIManagerAPI(t *testing.T) {
	ctx := context.Background()
	engine := mustNewEngine(t, ctx, os.Stdout, os.Stderr)

	tuiManager := engine.GetTUIManager()
	if tuiManager == nil {
		t.Fatal("TUI manager not created")
	}

	// Test mode registration
	modes := tuiManager.ListModes()
	if len(modes) != 0 {
		t.Fatalf("Expected 0 modes initially, got %d", len(modes))
	}

	// Test JavaScript API via script
	script := engine.LoadScriptFromString("api-test", `
		// Test mode registration
		tui.registerMode({
			name: "api-test-mode",
			tui: {
				title: "API Test Mode",
				prompt: "[api-test]> "
			},
			commands: {
				testcmd: {
					description: "Test command",
					handler: function(args) {
						console.log("Test command executed");
					}
				}
			}
		});

		// Test command registration
		tui.registerCommand({
			name: "global-test",
			description: "Global test command",
			handler: function(args) {
				console.log("Global test command executed");
			}
		});
	`)

	err := engine.ExecuteScript(script)
	if err != nil {
		t.Fatalf("Script execution failed: %v", err)
	}

	// Verify mode was registered
	modes = tuiManager.ListModes()
	if len(modes) != 1 {
		t.Fatalf("Expected 1 mode after registration, got %d", len(modes))
	}

	if modes[0] != "api-test-mode" {
		t.Fatalf("Expected 'api-test-mode', got '%s'", modes[0])
	}

	// Test mode switching
	err = tuiManager.SwitchMode("api-test-mode")
	if err != nil {
		t.Fatalf("Mode switching failed: %v", err)
	}

	currentMode := tuiManager.GetCurrentMode()
	if currentMode == nil {
		t.Fatal("No current mode after switching")
	}

	if currentMode.Name != "api-test-mode" {
		t.Fatalf("Expected current mode 'api-test-mode', got '%s'", currentMode.Name)
	}

	// Note: Direct state manipulation now requires state contracts.
	// State management is tested in other test functions with proper contracts.
}

// TestScriptModeExecution tests running scripts that define modes
func TestScriptModeExecution(t *testing.T) {
	// Build binary for testing
	binaryPath := buildTestBinary(t)

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}
	projectDir := filepath.Clean(filepath.Join(wd, "..", ".."))

	t.Run("DemoModeScript", func(t *testing.T) {
		cmd := exec.Command(binaryPath, "script", "--test", filepath.Join(projectDir, "scripts", "test-01-register-mode.js"))
		cmd.Dir = filepath.Dir(binaryPath)
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("Demo mode script execution failed: %v\nOutput: %s", err, output)
		}

		outputStr := string(output)
		if !strings.Contains(outputStr, "Demo mode registered!") {
			t.Fatalf("Demo mode registration not found in output: %s", outputStr)
		}

		if !strings.Contains(outputStr, "Available modes: demo") {
			t.Fatalf("Demo mode not in available modes: %s", outputStr)
		}
	})

	t.Run("LLMPromptBuilderScript", func(t *testing.T) {
		cmd := exec.Command(binaryPath, "script", "--test", filepath.Join(projectDir, "scripts", "example-01-llm-prompt-builder.js"))
		cmd.Dir = filepath.Dir(binaryPath)
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("LLM prompt builder script execution failed: %v\nOutput: %s", err, output)
		}

		outputStr := string(output)
		if !strings.Contains(outputStr, "LLM Prompt Builder mode registered!") {
			t.Fatalf("LLM prompt builder registration not found: %s", outputStr)
		}
	})

	t.Run("DebugTUIScript", func(t *testing.T) {
		cmd := exec.Command(binaryPath, "script", "--test", filepath.Join(projectDir, "scripts", "test-03-debug-tui.js"))
		cmd.Dir = filepath.Dir(binaryPath)
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("Debug TUI script execution failed: %v\nOutput: %s", err, output)
		}

		outputStr := string(output)
		if !strings.Contains(outputStr, "✓ tui object is available") {
			t.Fatalf("TUI object availability test failed: %s", outputStr)
		}

		if !strings.Contains(outputStr, "✓ Successfully registered test command") {
			t.Fatalf("Command registration test failed: %s", outputStr)
		}

		if !strings.Contains(outputStr, "✓ Successfully registered test mode") {
			t.Fatalf("Mode registration test failed: %s", outputStr)
		}
	})
}

var (
	testBinaryPath string
	testBinaryDir  string
)

// TestMain provides setup and teardown for the entire test suite.
// It builds the test binary once and cleans it up after all tests complete.
func TestMain(m *testing.M) {
	// Build the test binary before any tests run
	wd, err := os.Getwd()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Failed to get working directory: %v\n", err)
		os.Exit(1)
	}

	// Build to a predictable location in the system temp directory
	tmpBase := os.TempDir()
	testBinaryDir = filepath.Join(tmpBase, fmt.Sprintf("osm-test-binary-%d", os.Getpid()))

	// Create directory if it doesn't exist
	if err := os.MkdirAll(testBinaryDir, 0755); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Failed to create temp dir for binary: %v\n", err)
		os.Exit(1)
	}

	repoRoot := filepath.Clean(filepath.Join(wd, "..", ".."))
	testBinaryPath = filepath.Join(testBinaryDir, "osm")
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

// TestJavaScriptAPIBinding tests the core JavaScript API bindings
func TestJavaScriptAPIBinding(t *testing.T) {
	ctx := context.Background()
	engine := mustNewEngine(t, ctx, os.Stdout, os.Stderr)

	// Test that all API functions are available
	script := engine.LoadScriptFromString("api-binding-test", `
		// Test TUI API availability
		if (typeof tui === 'undefined') {
			throw new Error("tui object not available");
		}

		var requiredFunctions = [
			'registerMode', 'switchMode', 'getCurrentMode',
			'registerCommand', 'listModes', 'createState'
		];

		for (var i = 0; i < requiredFunctions.length; i++) {
			var funcName = requiredFunctions[i];
			if (typeof tui[funcName] !== 'function') {
				throw new Error("tui." + funcName + " is not available");
			}
		}

		ctx.log("All TUI API functions are available");
	`)

	err := engine.ExecuteScript(script)
	if err != nil {
		t.Fatalf("JavaScript API binding test failed: %v", err)
	}
}

// TestCommandExecution tests command execution in different scenarios
func TestCommandExecution(t *testing.T) {
	ctx := context.Background()
	engine := mustNewEngine(t, ctx, os.Stdout, os.Stderr)

	tuiManager := engine.GetTUIManager()

	// Register a test mode with commands using state
	script := engine.LoadScriptFromString("command-test", `
		// Define state for test mode
		const stateKeys = {
			test1_executed: Symbol("test1_executed"),
			test1_args: Symbol("test1_args"),
			test2_executed: Symbol("test2_executed"),
			test2_args: Symbol("test2_args"),
			global_executed: Symbol("global_executed"),
			global_args: Symbol("global_args")
		};
		const state = tui.createState("command-test-mode", {
			[stateKeys.test1_executed]: {defaultValue: false},
			[stateKeys.test1_args]: {defaultValue: []},
			[stateKeys.test2_executed]: {defaultValue: false},
			[stateKeys.test2_args]: {defaultValue: []},
			[stateKeys.global_executed]: {defaultValue: false},
			[stateKeys.global_args]: {defaultValue: []}
		});

		tui.registerMode({
			name: "command-test-mode",
			commands: function() {
				return {
					"test1": {
						description: "Test command 1",
						handler: function(args) {
							state.set(stateKeys.test1_executed, true);
							state.set(stateKeys.test1_args, args);
						}
					},
					"test2": {
						description: "Test command 2",
						handler: function(args) {
							state.set(stateKeys.test2_executed, true);
							state.set(stateKeys.test2_args, args);
						}
					}
				};
			}
		});

		// Register a global command
		tui.registerCommand({
			name: "global-test",
			description: "Global test command",
			handler: function(args) {
				// This is a hack for testing: global commands can't easily access mode state
				// In a real scenario, you'd use shared state or handle this differently
				output.print("Global test command executed");
			}
		});
	`)

	err := engine.ExecuteScript(script)
	if err != nil {
		t.Fatalf("Command registration script failed: %v", err)
	}

	// Switch to test mode
	err = tuiManager.SwitchMode("command-test-mode")
	if err != nil {
		t.Fatalf("Mode switching failed: %v", err)
	}

	// Test mode-specific command execution
	err = tuiManager.ExecuteCommand("test1", []string{"arg1", "arg2"})
	if err != nil {
		t.Fatalf("Mode command execution failed: %v", err)
	}

	// Verify command was executed using test helper
	test1Executed, err := tuiManager.GetStateViaJS("command-test-mode:test1_executed")
	if err != nil {
		t.Fatalf("Failed to get test1_executed state: %v", err)
	}
	if test1Executed != true {
		t.Fatal("test1 command was not executed")
	}

	// Test global command execution
	err = tuiManager.ExecuteCommand("global-test", []string{"global-arg"})
	if err != nil {
		t.Fatalf("Global command execution failed: %v", err)
	}
	// Note: Global commands don't have direct access to mode state,
	// so we just verify the command executes without error

	// Test non-existent command
	err = tuiManager.ExecuteCommand("nonexistent", []string{})
	if err == nil {
		t.Fatal("Expected error for non-existent command")
	}

	if !strings.Contains(err.Error(), "command nonexistent not found") {
		t.Fatalf("Expected 'command not found' error, got: %v", err)
	}
}

// TestConcurrentSafety tests the thread safety of the TUI system
func TestConcurrentSafety(t *testing.T) {
	ctx := context.Background()
	engine := mustNewEngine(t, ctx, os.Stdout, os.Stderr)

	tuiManager := engine.GetTUIManager()

	// Register a test mode
	script := engine.LoadScriptFromString("concurrent-test", `
		tui.registerMode({
			name: "concurrent-test",
			commands: {}
		});
	`)

	err := engine.ExecuteScript(script)
	if err != nil {
		t.Fatalf("Script execution failed: %v", err)
	}

	// Test concurrent mode switching (basic thread safety test)
	done := make(chan bool, 10)

	// Start multiple goroutines switching modes
	for i := 0; i < 10; i++ {
		go func(id int) {
			defer func() { done <- true }()

			// Switch to the mode multiple times
			for j := 0; j < 10; j++ {
				err := tuiManager.SwitchMode("concurrent-test")
				if err != nil {
					t.Errorf("Mode switching failed in goroutine %d: %v", id, err)
					return
				}
			}
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}
}
