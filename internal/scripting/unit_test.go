package scripting

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestTUIManagerAPI tests the TUI manager JavaScript API
func TestTUIManagerAPI(t *testing.T) {
	ctx := context.Background()
	engine := NewEngine(ctx, os.Stdout, os.Stderr)
	defer engine.Close()

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

	// Test state management
	tuiManager.SetState("test-key", "test-value")
	value := tuiManager.GetState("test-key")
	if value != "test-value" {
		t.Fatalf("Expected 'test-value', got '%v'", value)
	}
}

// TestPromptBuilder tests the prompt builder functionality
func TestPromptBuilder(t *testing.T) {
	pb := NewPromptBuilder("Test Prompt", "A test prompt for unit testing")

	// Test basic functionality
	if pb.Title != "Test Prompt" {
		t.Fatalf("Expected title 'Test Prompt', got '%s'", pb.Title)
	}

	// Test template and variable setting
	pb.SetTemplate("Hello {{name}}, you are {{age}} years old.")
	pb.SetVariable("name", "Alice")
	pb.SetVariable("age", 30)

	built := pb.Build()
	expected := "Hello Alice, you are 30 years old."
	if built != expected {
		t.Fatalf("Expected '%s', got '%s'", expected, built)
	}

	// Test version saving
	pb.SaveVersion("Initial version", []string{"test", "initial"})
	versions := pb.ListVersions()
	if len(versions) != 1 {
		t.Fatalf("Expected 1 version, got %d", len(versions))
	}

	version := versions[0]
	if version["notes"] != "Initial version" {
		t.Fatalf("Expected notes 'Initial version', got '%v'", version["notes"])
	}

	if version["content"] != expected {
		t.Fatalf("Expected content '%s', got '%v'", expected, version["content"])
	}

	// Test version restoration
	pb.SetVariable("name", "Bob")
	pb.SetVariable("age", 25)
	newBuilt := pb.Build()
	if newBuilt == built {
		t.Fatal("Expected different content after variable change")
	}

	err := pb.RestoreVersion(1)
	if err != nil {
		t.Fatalf("Version restoration failed: %v", err)
	}

	restoredBuilt := pb.Build()
	if restoredBuilt != expected {
		t.Fatalf("Expected restored content '%s', got '%s'", expected, restoredBuilt)
	}

	// Test export
	exported := pb.Export()
	if exported["title"] != "Test Prompt" {
		t.Fatalf("Expected exported title 'Test Prompt', got '%v'", exported["title"])
	}

	if exported["current"] != expected {
		t.Fatalf("Expected exported current '%s', got '%v'", expected, exported["current"])
	}
}

// TestScriptModeExecution tests running scripts that define modes
func TestScriptModeExecution(t *testing.T) {
	// Build binary for testing
	binaryPath := buildTestBinary(t)
	defer os.Remove(binaryPath)

	t.Run("DemoModeScript", func(t *testing.T) {
		cmd := exec.Command(binaryPath, "script", "--test", "scripts/demo-mode.js")
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
		cmd := exec.Command(binaryPath, "script", "--test", "scripts/llm-prompt-builder.js")
		cmd.Dir = filepath.Dir(binaryPath)
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("LLM prompt builder script execution failed: %v\nOutput: %s", err, output)
		}

		outputStr := string(output)
		if !strings.Contains(outputStr, "Welcome to LLM Prompt Builder!") {
			t.Fatalf("LLM prompt builder not found: %s", outputStr)
		}
		if !strings.Contains(outputStr, "Switched to mode: llm-prompt-builder") {
			t.Fatalf("LLM prompt builder mode switch not found: %s", outputStr)
		}
	})

	t.Run("DebugTUIScript", func(t *testing.T) {
		cmd := exec.Command(binaryPath, "script", "--test", "scripts/debug-tui.js")
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

// buildTestBinary builds the one-shot-man binary for testing
func buildTestBinary(t *testing.T) string {
	t.Helper()

	// Get the project root directory
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}

	// Navigate to project root
	projectRoot := filepath.Join(wd, "..", "..")
	tempBinary := filepath.Join(projectRoot, "one-shot-man-test")

	// Build the binary
	cmd := exec.Command("go", "build", "-o", tempBinary, "./cmd/one-shot-man")
	cmd.Dir = projectRoot
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to build test binary: %v\nOutput: %s", err, output)
	}

	return tempBinary
}

// TestJavaScriptAPIBinding tests the core JavaScript API bindings
func TestJavaScriptAPIBinding(t *testing.T) {
	ctx := context.Background()
	engine := NewEngine(ctx, os.Stdout, os.Stderr)
	defer engine.Close()

	// Test that all API functions are available
	script := engine.LoadScriptFromString("api-binding-test", `
		// Test TUI API availability
		if (typeof tui === 'undefined') {
			throw new Error("tui object not available");
		}
		
		var requiredFunctions = [
			'registerMode', 'switchMode', 'getCurrentMode',
			'setState', 'getState', 'registerCommand', 
			'listModes', 'createPromptBuilder'
		];
		
		for (var i = 0; i < requiredFunctions.length; i++) {
			var funcName = requiredFunctions[i];
			if (typeof tui[funcName] !== 'function') {
				throw new Error("tui." + funcName + " is not available");
			}
		}
		
		ctx.log("All TUI API functions are available");
		
		// Test prompt builder creation
		var pb = tui.createPromptBuilder("Test", "Test prompt");
		if (!pb || typeof pb.setTemplate !== 'function') {
			throw new Error("Prompt builder creation failed");
		}
		
		ctx.log("Prompt builder creation successful");
	`)

	err := engine.ExecuteScript(script)
	if err != nil {
		t.Fatalf("JavaScript API binding test failed: %v", err)
	}
}

// TestCommandExecution tests command execution in different scenarios
func TestCommandExecution(t *testing.T) {
	ctx := context.Background()
	engine := NewEngine(ctx, os.Stdout, os.Stderr)
	defer engine.Close()

	tuiManager := engine.GetTUIManager()

	// Register a test mode with commands
	script := engine.LoadScriptFromString("command-test", `
		tui.registerMode({
			name: "command-test-mode",
			commands: {
				"test1": {
					description: "Test command 1",
					handler: function(args) {
						tui.setState("test1_executed", true);
						tui.setState("test1_args", args);
					}
				},
				"test2": {
					description: "Test command 2", 
					handler: function(args) {
						tui.setState("test2_executed", true);
						tui.setState("test2_args", args);
					}
				}
			}
		});
		
		// Register a global command
		tui.registerCommand({
			name: "global-test",
			description: "Global test command",
			handler: function(args) {
				tui.setState("global_executed", true);
				tui.setState("global_args", args);
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

	// Verify command was executed
	if tuiManager.GetState("test1_executed") != true {
		t.Fatal("test1 command was not executed")
	}

	// Test global command execution
	err = tuiManager.ExecuteCommand("global-test", []string{"global-arg"})
	if err != nil {
		t.Fatalf("Global command execution failed: %v", err)
	}

	if tuiManager.GetState("global_executed") != true {
		t.Fatal("global-test command was not executed")
	}

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
	engine := NewEngine(ctx, os.Stdout, os.Stderr)
	defer engine.Close()

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

	// Switch to the mode
	err = tuiManager.SwitchMode("concurrent-test")
	if err != nil {
		t.Fatalf("Mode switching failed: %v", err)
	}

	// Test concurrent state access
	done := make(chan bool, 10)

	// Start multiple goroutines accessing state
	for i := 0; i < 10; i++ {
		go func(id int) {
			defer func() { done <- true }()
			
			// Set and get state values
			for j := 0; j < 100; j++ {
				key := strings.Join([]string{"key", strings.Repeat("x", id), strings.Repeat("y", j)}, "-")
				value := strings.Join([]string{"value", strings.Repeat("a", id), strings.Repeat("b", j)}, "-")
				
				tuiManager.SetState(key, value)
				retrieved := tuiManager.GetState(key)
				
				if retrieved != value {
					t.Errorf("State mismatch in goroutine %d: expected %s, got %v", id, value, retrieved)
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