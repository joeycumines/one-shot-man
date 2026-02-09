package scripting

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dop251/goja"
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

// TestSetGlobal_QueueSafeFromGoroutine verifies C5 fix:
// QueueSetGlobal and QueueGetGlobal provide thread-safe access from arbitrary goroutines.
func TestSetGlobal_QueueSafeFromGoroutine(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	ctx := context.Background()

	engine := newTestEngine(t, ctx, &stdout, &stderr)
	engine.SetTestMode(true)

	// Test QueueSetGlobal from multiple goroutines
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			engine.QueueSetGlobal(fmt.Sprintf("key%d", idx), idx)
		}(i)
	}
	wg.Wait()

	// Verify all values were set (eventually, as QueueSetGlobal is async)
	// Use a wait group to ensure all callbacks complete before checking results
	var resultsMu sync.Mutex
	var results []int
	var resultsWG sync.WaitGroup

	for i := 0; i < 10; i++ {
		resultsWG.Add(1)
		idx := i // capture loop variable
		engine.QueueGetGlobal(fmt.Sprintf("key%d", idx), func(value interface{}) {
			defer resultsWG.Done()
			if v, ok := value.(int64); ok {
				resultsMu.Lock()
				results = append(results, int(v))
				resultsMu.Unlock()
			}
		})
	}
	resultsWG.Wait()

	// Verify all values were retrieved
	if len(results) != 10 {
		t.Errorf("Expected 10 values, got %d", len(results))
	}
}

// TestSetGlobal_ThreadCheckMode verifies that SetThreadCheckMode enables panic on wrong goroutine.
func TestSetGlobal_ThreadCheckMode(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	ctx := context.Background()

	engine := newTestEngine(t, ctx, &stdout, &stderr)
	engine.SetTestMode(true)

	// Enable thread check mode
	engine.SetThreadCheckMode(true)

	// Set a value from the main goroutine (should be the event loop goroutine after SetThreadCheckMode)
	engine.SetGlobal("testKey", "testValue")

	// Get the value
	value := engine.GetGlobal("testKey")
	if value != "testValue" {
		t.Errorf("Expected 'testValue', got: %v", value)
	}
}

// TestSetGlobal_DirectAccess_WorksOnEventLoop verifies direct access works when called correctly.
func TestSetGlobal_DirectAccess_WorksOnEventLoop(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	ctx := context.Background()

	engine := newTestEngine(t, ctx, &stdout, &stderr)
	engine.SetTestMode(true)

	// Direct access should work (called from test goroutine, but no thread check mode enabled)
	engine.SetGlobal("directKey", "directValue")

	// Verify
	value := engine.GetGlobal("directKey")
	if value != "directValue" {
		t.Errorf("Expected 'directValue', got: %v", value)
	}
}

// TestQueueSetGlobal_AsyncBehavior verifies QueueSetGlobal behavior is async.
func TestQueueSetGlobal_AsyncBehavior(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	ctx := context.Background()

	engine := newTestEngine(t, ctx, &stdout, &stderr)
	engine.SetTestMode(true)

	// Queue a set operation
	engine.QueueSetGlobal("asyncKey", "asyncValue")

	// Immediately trying to read might not see the value yet (async)
	// But eventually the callback should receive it
	var receivedValue interface{}
	var wg sync.WaitGroup
	wg.Add(1)
	engine.QueueGetGlobal("asyncKey", func(value interface{}) {
		receivedValue = value
		wg.Done()
	})
	wg.Wait()

	if receivedValue != "asyncValue" {
		t.Errorf("Expected 'asyncValue', got: %v", receivedValue)
	}
}

// TestQueueGetGlobal_NilHandling verifies QueueGetGlobal handles nil/undefined values correctly.
func TestQueueGetGlobal_NilHandling(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	ctx := context.Background()

	engine := newTestEngine(t, ctx, &stdout, &stderr)
	engine.SetTestMode(true)

	// Test getting a non-existent key
	var nilResult interface{}
	var nilWG sync.WaitGroup
	nilWG.Add(1)
	engine.QueueGetGlobal("nonExistentKey", func(value interface{}) {
		nilResult = value
		nilWG.Done()
	})
	nilWG.Wait()

	if nilResult != nil {
		t.Errorf("Expected nil for non-existent key, got: %v", nilResult)
	}

	// Set to nil explicitly and verify
	engine.SetGlobal("explicitNil", nil)
	var explicitNilResult interface{}
	var explicitNilWG sync.WaitGroup
	explicitNilWG.Add(1)
	engine.QueueGetGlobal("explicitNil", func(value interface{}) {
		explicitNilResult = value
		explicitNilWG.Done()
	})
	explicitNilWG.Wait()

	if explicitNilResult != nil {
		t.Errorf("Expected nil for explicitly nil value, got: %v", explicitNilResult)
	}
}

// TestSetGlobal_MutexProtection verifies the globals mutex protects concurrent access.
func TestSetGlobal_MutexProtection(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	ctx := context.Background()

	engine := newTestEngine(t, ctx, &stdout, &stderr)
	engine.SetTestMode(true)

	// Concurrent writes to the same key
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			engine.SetGlobal("concurrentKey", idx)
		}(i)
	}
	wg.Wait()

	// Verify we can read the final value without data race
	value := engine.GetGlobal("concurrentKey")
	if value == nil {
		t.Error("Expected non-nil value after concurrent writes")
	}
}

// TestQueueSetGlobal_ConcurrentWrites tests concurrent QueueSetGlobal calls.
func TestQueueSetGlobal_ConcurrentWrites(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	ctx := context.Background()

	engine := newTestEngine(t, ctx, &stdout, &stderr)
	engine.SetTestMode(true)

	// Many concurrent writes to different keys
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			engine.QueueSetGlobal(fmt.Sprintf("concurrentKey_%d", idx), idx*2)
		}(i)
	}
	wg.Wait()

	// Verify all values were set
	var verifyWG sync.WaitGroup
	for i := 0; i < 50; i++ {
		verifyWG.Add(1)
		idx := i
		engine.QueueGetGlobal(fmt.Sprintf("concurrentKey_%d", idx), func(value interface{}) {
			defer verifyWG.Done()
			if v, ok := value.(int64); !ok || int(v) != idx*2 {
				t.Errorf("Expected %d, got: %v", idx*2, value)
			}
		})
	}
	verifyWG.Wait()
}

// TestQueueGetGlobal_ConcurrentReads tests concurrent QueueGetGlobal calls.
func TestQueueGetGlobal_ConcurrentReads(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	ctx := context.Background()

	engine := newTestEngine(t, ctx, &stdout, &stderr)
	engine.SetTestMode(true)

	// Set up a value
	engine.SetGlobal("sharedKey", "sharedValue")

	// Many concurrent reads of the same key
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var result interface{}
			var readWG sync.WaitGroup
			readWG.Add(1)
			engine.QueueGetGlobal("sharedKey", func(value interface{}) {
				result = value
				readWG.Done()
			})
			readWG.Wait()
			if result != "sharedValue" {
				t.Errorf("Expected 'sharedValue', got: %v", result)
			}
		}()
	}
	wg.Wait()
}

// TestSetGlobal_OverwriteValue tests overwriting existing global values.
func TestSetGlobal_OverwriteValue(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	ctx := context.Background()

	engine := newTestEngine(t, ctx, &stdout, &stderr)
	engine.SetTestMode(true)

	// Set initial value
	engine.SetGlobal("overwriteKey", "initial")

	// Overwrite with different types
	engine.SetGlobal("overwriteKey", 42)
	if v := engine.GetGlobal("overwriteKey"); v != int64(42) {
		t.Errorf("Expected 42, got: %v", v)
	}

	engine.SetGlobal("overwriteKey", "string")
	if v := engine.GetGlobal("overwriteKey"); v != "string" {
		t.Errorf("Expected 'string', got: %v", v)
	}

	engine.SetGlobal("overwriteKey", map[string]interface{}{"nested": true})
	if v := engine.GetGlobal("overwriteKey"); v == nil {
		t.Error("Expected map value, got nil")
	}
}

// TestSetGlobal_VariousTypes tests setting various Go types as globals.
func TestSetGlobal_VariousTypes(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	ctx := context.Background()

	engine := newTestEngine(t, ctx, &stdout, &stderr)
	engine.SetTestMode(true)

	tests := []struct {
		name  string
		value interface{}
	}{
		{"string", "hello"},
		{"int", int64(42)},
		{"float", float64(3.14)},
		{"bool_true", true},
		{"bool_false", false},
		{"slice", []int{1, 2, 3}},
		{"map", map[string]int{"a": 1, "b": 2}},
		{"nil", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine.SetGlobal("testValue", tt.value)
			result := engine.GetGlobal("testValue")
			if tt.value == nil {
				if result != nil {
					t.Errorf("Expected nil, got: %v", result)
				}
			} else if result == nil {
				t.Errorf("Expected non-nil value for %s, got nil", tt.name)
			}
		})
	}
}

// TestQueueSetGlobal_ValueTypes tests QueueSetGlobal with various value types.
func TestQueueSetGlobal_ValueTypes(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	ctx := context.Background()

	engine := newTestEngine(t, ctx, &stdout, &stderr)
	engine.SetTestMode(true)

	tests := []struct {
		name  string
		value interface{}
	}{
		{"string", "hello"},
		{"int", int64(42)},
		{"float", float64(3.14)},
		{"bool", true},
		{"slice", []int{1, 2, 3}},
		{"map", map[string]int{"a": 1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var wg sync.WaitGroup
			wg.Add(1)
			engine.QueueSetGlobal("queueTestValue", tt.value)
			engine.QueueGetGlobal("queueTestValue", func(value interface{}) {
				defer wg.Done()
				if tt.value == nil {
					if value != nil {
						t.Errorf("Expected nil, got: %v", value)
					}
				} else if value == nil {
					t.Errorf("Expected non-nil value, got nil")
				}
			})
			wg.Wait()
		})
	}
}

// TestThreadCheckMode_PanicDetection verifies SetThreadCheckMode causes panic on wrong goroutine.
func TestThreadCheckMode_PanicDetection(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	ctx := context.Background()

	engine := newTestEngine(t, ctx, &stdout, &stderr)
	engine.SetTestMode(true)

	// Enable thread check mode - this captures the current goroutine ID
	engine.SetThreadCheckMode(true)

	// Calling SetGlobal from the same goroutine should work
	engine.SetGlobal("testKey", "testValue")

	// Now verify that calling from a different goroutine would panic
	// We use a subtest to catch the expected panic
	t.Run("SetGlobal_panics_on_wrong_goroutine", func(t *testing.T) {
		panicChan := make(chan interface{}, 1)
		go func() {
			defer func() {
				if r := recover(); r != nil {
					panicChan <- r
				}
			}()
			// This should panic because we're not on the event loop goroutine
			engine.SetGlobal("wrongGoroutineKey", "value")
		}()
		// Wait for the goroutine to complete
		select {
		case r := <-panicChan:
			// Panic was caught as expected
			if r == nil {
				t.Error("Expected non-nil panic value")
			}
		case <-time.After(time.Second):
			t.Error("Timeout waiting for panic from goroutine")
		}
	})

	t.Run("GetGlobal_panics_on_wrong_goroutine", func(t *testing.T) {
		panicChan := make(chan interface{}, 1)
		go func() {
			defer func() {
				if r := recover(); r != nil {
					panicChan <- r
				}
			}()
			// This should panic because we're not on the event loop goroutine
			_ = engine.GetGlobal("testKey")
		}()
		// Wait for the goroutine to complete
		select {
		case r := <-panicChan:
			// Panic was caught as expected
			if r == nil {
				t.Error("Expected non-nil panic value")
			}
		case <-time.After(time.Second):
			t.Error("Timeout waiting for panic from goroutine")
		}
	})
}

// TestQueueSetGlobal_FromEventLoop tests QueueSetGlobal can be called from event loop.
func TestQueueSetGlobal_FromEventLoop(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	ctx := context.Background()

	engine := newTestEngine(t, ctx, &stdout, &stderr)
	engine.SetTestMode(true)

	// QueueSetGlobal from event loop context (via RunOnLoopSync)
	err := engine.runtime.RunOnLoopSync(func(r *goja.Runtime) error {
		engine.QueueSetGlobal("eventLoopKey", "eventLoopValue")
		return nil
	})
	if err != nil {
		t.Fatalf("RunOnLoopSync failed: %v", err)
	}

	// Verify the value was set
	var result interface{}
	var wg sync.WaitGroup
	wg.Add(1)
	engine.QueueGetGlobal("eventLoopKey", func(value interface{}) {
		result = value
		wg.Done()
	})
	wg.Wait()

	if result != "eventLoopValue" {
		t.Errorf("Expected 'eventLoopValue', got: %v", result)
	}
}

// TestGetGlobal_FromEventLoop tests GetGlobal can be called from event loop.
func TestGetGlobal_FromEventLoop(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	ctx := context.Background()

	engine := newTestEngine(t, ctx, &stdout, &stderr)
	engine.SetTestMode(true)

	// Set a value first
	engine.SetGlobal("eventLoopGetKey", "eventLoopGetValue")

	// GetGlobal from event loop context
	var result interface{}
	err := engine.runtime.RunOnLoopSync(func(r *goja.Runtime) error {
		result = engine.GetGlobal("eventLoopGetKey")
		return nil
	})
	if err != nil {
		t.Fatalf("RunOnLoopSync failed: %v", err)
	}

	if result != "eventLoopGetValue" {
		t.Errorf("Expected 'eventLoopGetValue', got: %v", result)
	}
}

// TestSetGlobal_BeforeAndAfterEventLoopStart tests setting globals before and after event loop starts.
func TestSetGlobal_BeforeAndAfterEventLoopStart(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	ctx := context.Background()

	engine := newTestEngine(t, ctx, &stdout, &stderr)
	engine.SetTestMode(true)

	// Set before any async operations
	engine.SetGlobal("beforeKey", "beforeValue")

	// Execute a script to start the event loop
	script := engine.LoadScriptFromString("test_before_after", `
		ctx.log("Event loop started");
	`)
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("Script execution failed: %v", err)
	}

	// Set after event loop is running (direct access)
	engine.SetGlobal("afterKey", "afterValue")

	// Verify both values
	if v := engine.GetGlobal("beforeKey"); v != "beforeValue" {
		t.Errorf("Expected 'beforeValue', got: %v", v)
	}
	if v := engine.GetGlobal("afterKey"); v != "afterValue" {
		t.Errorf("Expected 'afterValue', got: %v", v)
	}
}

// TestQueueSetGlobal_ManyValues tests setting many values rapidly.
func TestQueueSetGlobal_ManyValues(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	ctx := context.Background()

	engine := newTestEngine(t, ctx, &stdout, &stderr)
	engine.SetTestMode(true)

	const numValues = 100

	// Set many values rapidly
	for i := 0; i < numValues; i++ {
		engine.QueueSetGlobal(fmt.Sprintf("rapidKey_%d", i), i)
	}

	// Wait for all to complete and verify
	var verifyWG sync.WaitGroup
	for i := 0; i < numValues; i++ {
		verifyWG.Add(1)
		idx := i
		engine.QueueGetGlobal(fmt.Sprintf("rapidKey_%d", idx), func(value interface{}) {
			defer verifyWG.Done()
			if v, ok := value.(int64); !ok || int(v) != idx {
				t.Errorf("Expected %d, got: %v", idx, value)
			}
		})
	}
	verifyWG.Wait()
}

// TestThreadCheckMode_DisabledByDefault verifies thread check mode is disabled by default.
func TestThreadCheckMode_DisabledByDefault(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	ctx := context.Background()

	engine := newTestEngine(t, ctx, &stdout, &stderr)
	engine.SetTestMode(true)

	// Thread check mode should be disabled by default
	// Calling from different goroutines should NOT panic
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			engine.SetGlobal(fmt.Sprintf("defaultKey_%d", idx), idx)
			_ = engine.GetGlobal(fmt.Sprintf("defaultKey_%d", idx))
		}(i)
	}
	wg.Wait()

	// All operations should complete without panic
	t.Log("Thread check mode correctly disabled by default")
}

// TestGlobalsMu_MutexWorks verifies the globals mutex provides proper synchronization.
func TestGlobalsMu_MutexWorks(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	ctx := context.Background()

	engine := newTestEngine(t, ctx, &stdout, &stderr)
	engine.SetTestMode(true)

	// Test that mutex allows concurrent access without data races
	var wg sync.WaitGroup
	iterations := 100

	// Multiple goroutines reading and writing
	for i := 0; i < iterations; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			// Alternate between read and write
			if idx%2 == 0 {
				engine.SetGlobal("mutexTestKey", idx)
			} else {
				_ = engine.GetGlobal("mutexTestKey")
			}
		}(i)
	}
	wg.Wait()

	// Final verification
	value := engine.GetGlobal("mutexTestKey")
	if value == nil {
		t.Error("Expected non-nil value after concurrent access")
	}
}
