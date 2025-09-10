package scripting

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestEngine_BasicExecution(t *testing.T) {
	var stdout, stderr bytes.Buffer
	ctx := context.Background()
	
	engine := NewEngine(ctx, &stdout, &stderr)
	engine.SetTestMode(true)
	
	script := engine.LoadScriptFromString("test", `
		ctx.Log("Hello from JavaScript");
		ctx.Logf("Number: %d", 42);
	`)
	
	err := engine.ExecuteScript(script)
	if err != nil {
		t.Fatalf("Script execution failed: %v", err)
	}
	
	output := stdout.String()
	if !strings.Contains(output, "Hello from JavaScript") {
		t.Errorf("Expected log message not found in output: %s", output)
	}
	if !strings.Contains(output, "Number: 42") {
		t.Errorf("Expected formatted log message not found in output: %s", output)
	}
}

func TestEngine_DeferredExecution(t *testing.T) {
	var stdout, stderr bytes.Buffer
	ctx := context.Background()
	
	engine := NewEngine(ctx, &stdout, &stderr)
	engine.SetTestMode(true)
	
	script := engine.LoadScriptFromString("test_defer", `
		ctx.Log("Before defer");
		ctx.Defer(function() {
			ctx.Log("Deferred 2");
		});
		ctx.Defer(function() {
			ctx.Log("Deferred 1");
		});
		ctx.Log("After defer");
	`)
	
	err := engine.ExecuteScript(script)
	if err != nil {
		t.Fatalf("Script execution failed: %v", err)
	}
	
	output := stdout.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")
	
	expected := []string{
		"Before defer",
		"After defer", 
		"Deferred 1", // Should execute in reverse order
		"Deferred 2",
	}
	
	for i, expectedLine := range expected {
		if i >= len(lines) || !strings.Contains(lines[i], expectedLine) {
			t.Errorf("Expected line %d to contain '%s', got: %s", i, expectedLine, lines[i])
		}
	}
}

func TestEngine_SubTests(t *testing.T) {
	var stdout, stderr bytes.Buffer
	ctx := context.Background()
	
	engine := NewEngine(ctx, &stdout, &stderr)
	engine.SetTestMode(true)
	
	script := engine.LoadScriptFromString("test_subtests", `
		ctx.Run("subtest1", function() {
			ctx.Log("In subtest 1");
		});
		
		ctx.Run("subtest2", function() {
			ctx.Log("In subtest 2");
			ctx.Run("nested", function() {
				ctx.Log("In nested test");
			});
		});
	`)
	
	err := engine.ExecuteScript(script)
	if err != nil {
		t.Fatalf("Script execution failed: %v", err)
	}
	
	output := stdout.String()
	if !strings.Contains(output, "In subtest 1") {
		t.Errorf("Subtest 1 output not found: %s", output)
	}
	if !strings.Contains(output, "In subtest 2") {
		t.Errorf("Subtest 2 output not found: %s", output)
	}
	if !strings.Contains(output, "In nested test") {
		t.Errorf("Nested test output not found: %s", output)
	}
}

func TestEngine_ErrorHandling(t *testing.T) {
	var stdout, stderr bytes.Buffer
	ctx := context.Background()
	
	engine := NewEngine(ctx, &stdout, &stderr)
	engine.SetTestMode(true)
	
	script := engine.LoadScriptFromString("test_error", `
		ctx.Error("This is an error");
		ctx.Errorf("Formatted error: %s", "test");
	`)
	
	err := engine.ExecuteScript(script)
	if err == nil {
		t.Error("Expected script execution to fail")
	}
	
	errorOutput := stderr.String()
	if !strings.Contains(errorOutput, "This is an error") {
		t.Errorf("Expected error message not found: %s", errorOutput)
	}
	if !strings.Contains(errorOutput, "Formatted error: test") {
		t.Errorf("Expected formatted error message not found: %s", errorOutput)
	}
}

func TestEngine_GlobalVariables(t *testing.T) {
	var stdout, stderr bytes.Buffer
	ctx := context.Background()
	
	engine := NewEngine(ctx, &stdout, &stderr)
	engine.SetTestMode(true)
	engine.SetGlobal("testVar", "test value")
	engine.SetGlobal("testNum", 123)
	
	script := engine.LoadScriptFromString("test_globals", `
		ctx.Log("testVar: " + testVar);
		ctx.Log("testNum: " + testNum);
	`)
	
	err := engine.ExecuteScript(script)
	if err != nil {
		t.Fatalf("Script execution failed: %v", err)
	}
	
	output := stdout.String()
	if !strings.Contains(output, "testVar: test value") {
		t.Errorf("Global string variable not accessible: %s", output)
	}
	if !strings.Contains(output, "testNum: 123") {
		t.Errorf("Global number variable not accessible: %s", output)
	}
}

func TestEngine_ConsoleAPI(t *testing.T) {
	var stdout, stderr bytes.Buffer
	ctx := context.Background()
	
	engine := NewEngine(ctx, &stdout, &stderr)
	
	script := engine.LoadScriptFromString("test_console", `
		console.log("stdout message");
		console.error("stderr message");
	`)
	
	err := engine.ExecuteScript(script)
	if err != nil {
		t.Fatalf("Script execution failed: %v", err)
	}
	
	if !strings.Contains(stdout.String(), "stdout message") {
		t.Errorf("Console.log output not found: %s", stdout.String())
	}
	if !strings.Contains(stderr.String(), "stderr message") {
		t.Errorf("Console.error output not found: %s", stderr.String())
	}
}

// TestTerminalIntegration tests the terminal using termtest for PTY support
func TestTerminalIntegration(t *testing.T) {
	t.Skip("Skipping complex terminal integration test - requires more setup")
	
	// This would require setting up a proper test binary and command
	// For now, we'll skip this test and focus on the unit tests
}

// TestTerminalScriptLoading tests script loading and execution in terminal
func TestTerminalScriptLoading(t *testing.T) {
	t.Skip("Skipping complex terminal test - requires more setup")
	
	// This would require setting up a proper test binary and command
	// For now, we'll skip this test and focus on the unit tests
}

func TestEngine_ComplexScenario(t *testing.T) {
	var stdout, stderr bytes.Buffer
	ctx := context.Background()
	
	engine := NewEngine(ctx, &stdout, &stderr)
	engine.SetTestMode(true)
	engine.SetGlobal("config", map[string]interface{}{
		"timeout": 1000,
		"retries": 3,
	})
	
	script := engine.LoadScriptFromString("complex_test", `
		ctx.Run("setup", function() {
			ctx.Log("Setting up test environment");
			ctx.Defer(function() {
				ctx.Log("Cleaning up test environment");
			});
		});
		
		ctx.Run("main_test", function() {
			ctx.Logf("Using timeout: %d", config.timeout);
			ctx.Logf("Using retries: %d", config.retries);
			
			ctx.Run("sub_operation", function() {
				ctx.Log("Performing sub-operation");
				// Simulate some work
				sleep(10);
				ctx.Log("Sub-operation completed");
			});
		});
		
		ctx.Run("teardown", function() {
			ctx.Log("Tearing down test");
		});
	`)
	
	err := engine.ExecuteScript(script)
	if err != nil {
		t.Fatalf("Complex script execution failed: %v", err)
	}
	
	output := stdout.String()
	expectedMessages := []string{
		"Setting up test environment",
		"Using timeout: 1000",
		"Using retries: 3",
		"Performing sub-operation",
		"Sub-operation completed",
		"Tearing down test",
		"Cleaning up test environment",
	}
	
	for _, expected := range expectedMessages {
		if !strings.Contains(output, expected) {
			t.Errorf("Expected message not found: %s\nFull output: %s", expected, output)
		}
	}
}