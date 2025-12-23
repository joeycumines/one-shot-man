package scripting

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/testutil"
)

func newTestEngine(t *testing.T, ctx context.Context, stdout, stderr io.Writer) *Engine {
	t.Helper()
	engine, err := NewEngineWithConfig(ctx, stdout, stderr, testutil.NewTestSessionID("", t.Name()), "memory")
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}
	t.Cleanup(func() {
		_ = engine.Close()
	})
	return engine
}

func TestEngine_BasicExecution(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	ctx := context.Background()

	engine := newTestEngine(t, ctx, &stdout, &stderr)
	engine.SetTestMode(true)

	script := engine.LoadScriptFromString("test", `
		ctx.log("Hello from JavaScript");
		ctx.logf("Number: %d", 42);
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
	t.Parallel()
	var stdout, stderr bytes.Buffer
	ctx := context.Background()

	engine := newTestEngine(t, ctx, &stdout, &stderr)
	engine.SetTestMode(true)

	script := engine.LoadScriptFromString("test_defer", `
		ctx.log("Before defer");
		ctx.defer(function() {
			ctx.log("Deferred 2");
		});
		ctx.defer(function() {
			ctx.log("Deferred 1");
		});
		ctx.log("After defer");
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
	t.Parallel()
	var stdout, stderr bytes.Buffer
	ctx := context.Background()

	engine := newTestEngine(t, ctx, &stdout, &stderr)
	engine.SetTestMode(true)

	script := engine.LoadScriptFromString("test_subtests", `
		ctx.run("subtest1", function() {
			ctx.log("In subtest 1");
		});

		ctx.run("subtest2", function() {
			ctx.log("In subtest 2");
			ctx.run("nested", function() {
				ctx.log("In nested test");
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
	t.Parallel()
	var stdout, stderr bytes.Buffer
	ctx := context.Background()

	engine := newTestEngine(t, ctx, &stdout, &stderr)
	engine.SetTestMode(true)

	script := engine.LoadScriptFromString("test_error", `
		ctx.error("This is an error");
		ctx.errorf("Formatted error: %s", "test");
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
	t.Parallel()
	var stdout, stderr bytes.Buffer
	ctx := context.Background()

	engine := newTestEngine(t, ctx, &stdout, &stderr)
	engine.SetTestMode(true)
	engine.SetGlobal("testVar", "test value")
	engine.SetGlobal("testNum", 123)

	script := engine.LoadScriptFromString("test_globals", `
		ctx.log("testVar: " + testVar);
		ctx.log("testNum: " + testNum);
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

func TestEngine_OutputAPI(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	ctx := context.Background()

	engine := newTestEngine(t, ctx, &stdout, &stderr)

	script := engine.LoadScriptFromString("test_output", `
		output.print("stdout message");
		log.error("error message");
		output.printf("formatted: %s %d", "test", 42);
	`)

	err := engine.ExecuteScript(script)
	if err != nil {
		t.Fatalf("Script execution failed: %v", err)
	}

	if !strings.Contains(stdout.String(), "stdout message") {
		t.Errorf("Output.print output not found: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "formatted: test 42") {
		t.Errorf("Output.printf output not found: %s", stdout.String())
	}

	// Check that logs were created
	logs := engine.logger.GetLogs()
	if len(logs) == 0 {
		t.Error("Expected logs to be created, but got none")
	}
}

func TestEngine_ComplexScenario(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	ctx := context.Background()

	engine := newTestEngine(t, ctx, &stdout, &stderr)
	engine.SetTestMode(true)
	engine.SetGlobal("config", map[string]interface{}{
		"timeout": 1000,
		"retries": 3,
	})

	script := engine.LoadScriptFromString("complex_test", `
		const {sleep} = require('osm:time');

		ctx.run("setup", function() {
			ctx.log("Setting up test environment");
			ctx.defer(function() {
				ctx.log("Cleaning up test environment");
			});
		});

		ctx.run("main_test", function() {
			ctx.logf("Using timeout: %d", config.timeout);
			ctx.logf("Using retries: %d", config.retries);

			ctx.run("sub_operation", function() {
				ctx.log("Performing sub-operation");
				// Simulate some work
				sleep(10);
				ctx.log("Sub-operation completed");
			});
		});

		ctx.run("teardown", function() {
			ctx.log("Tearing down test");
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
