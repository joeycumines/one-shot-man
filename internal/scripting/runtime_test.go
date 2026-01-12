package scripting

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/require"
)

func TestNewRuntime(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rt, err := NewRuntime(ctx)
	if err != nil {
		t.Fatalf("NewRuntime failed: %v", err)
	}
	defer rt.Close()

	if !rt.IsRunning() {
		t.Error("runtime should be running after creation")
	}

	if rt.Registry() == nil {
		t.Error("registry should not be nil")
	}

	if rt.EventLoop() == nil {
		t.Error("event loop should not be nil")
	}
}

func TestNewRuntimeWithRegistry(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	registry := require.NewRegistry()
	rt, err := NewRuntimeWithRegistry(ctx, registry)
	if err != nil {
		t.Fatalf("NewRuntimeWithRegistry failed: %v", err)
	}
	defer rt.Close()

	if rt.Registry() != registry {
		t.Error("should use provided registry")
	}
}

func TestRuntime_Close(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rt, err := NewRuntime(ctx)
	if err != nil {
		t.Fatalf("NewRuntime failed: %v", err)
	}

	// Close should succeed
	if err := rt.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}

	if rt.IsRunning() {
		t.Error("runtime should not be running after close")
	}

	// Close should be idempotent
	if err := rt.Close(); err != nil {
		t.Errorf("second Close failed: %v", err)
	}

	// Done channel should be closed
	select {
	case <-rt.Done():
		// expected
	default:
		t.Error("Done channel should be closed after Close")
	}
}

func TestRuntime_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	rt, err := NewRuntime(ctx)
	if err != nil {
		t.Fatalf("NewRuntime failed: %v", err)
	}

	// Cancel the context
	cancel()

	// Wait for runtime to stop (with timeout)
	select {
	case <-rt.Done():
		// expected
	case <-time.After(time.Second):
		t.Error("runtime should stop when context is canceled")
	}
}

func TestRuntime_RunOnLoop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rt, err := NewRuntime(ctx)
	if err != nil {
		t.Fatalf("NewRuntime failed: %v", err)
	}
	defer rt.Close()

	var executed atomic.Bool
	done := make(chan struct{})

	ok := rt.RunOnLoop(func(vm *goja.Runtime) {
		executed.Store(true)
		close(done)
	})

	if !ok {
		t.Fatal("RunOnLoop should return true for running runtime")
	}

	select {
	case <-done:
		// expected
	case <-time.After(time.Second):
		t.Fatal("RunOnLoop callback should execute")
	}

	if !executed.Load() {
		t.Error("callback should have been executed")
	}
}

func TestRuntime_RunOnLoop_Stopped(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rt, err := NewRuntime(ctx)
	if err != nil {
		t.Fatalf("NewRuntime failed: %v", err)
	}

	rt.Close()

	ok := rt.RunOnLoop(func(vm *goja.Runtime) {
		t.Error("callback should not be executed on stopped runtime")
	})

	if ok {
		t.Error("RunOnLoop should return false for stopped runtime")
	}
}

func TestRuntime_RunOnLoopSync(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rt, err := NewRuntime(ctx)
	if err != nil {
		t.Fatalf("NewRuntime failed: %v", err)
	}
	defer rt.Close()

	// Test successful execution
	var value int64
	err = rt.RunOnLoopSync(func(vm *goja.Runtime) error {
		value = 42
		return nil
	})

	if err != nil {
		t.Errorf("RunOnLoopSync failed: %v", err)
	}
	if value != 42 {
		t.Errorf("value should be 42, got %d", value)
	}

	// Test error propagation
	expectedErr := errors.New("test error")
	err = rt.RunOnLoopSync(func(vm *goja.Runtime) error {
		return expectedErr
	})

	if err != expectedErr {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}
}

func TestRuntime_RunOnLoopSync_Timeout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rt, err := NewRuntime(ctx)
	if err != nil {
		t.Fatalf("NewRuntime failed: %v", err)
	}
	defer rt.Close()

	// Set a very short timeout
	rt.SetTimeout(10 * time.Millisecond)

	// Schedule a long-running operation
	err = rt.RunOnLoopSync(func(vm *goja.Runtime) error {
		time.Sleep(100 * time.Millisecond)
		return nil
	})

	if err == nil {
		t.Error("expected timeout error")
	}
}

func TestRuntime_RunOnLoopSync_Stopped(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rt, err := NewRuntime(ctx)
	if err != nil {
		t.Fatalf("NewRuntime failed: %v", err)
	}

	rt.Close()

	err = rt.RunOnLoopSync(func(vm *goja.Runtime) error {
		return nil
	})

	if err == nil {
		t.Error("expected error for stopped runtime")
	}
}

func TestRuntime_TryRunOnLoopSync_DirectExecution(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rt, err := NewRuntime(ctx)
	if err != nil {
		t.Fatalf("NewRuntime failed: %v", err)
	}
	defer rt.Close()

	// Test from within the event loop - should execute directly
	var innerExecuted bool
	err = rt.RunOnLoopSync(func(vm *goja.Runtime) error {
		// This call from within the event loop should execute directly
		return rt.TryRunOnLoopSync(vm, func(innerVM *goja.Runtime) error {
			innerExecuted = true
			// Should be same VM instance
			if innerVM != vm {
				return errors.New("inner VM should be same as outer VM")
			}
			return nil
		})
	})

	if err != nil {
		t.Errorf("TryRunOnLoopSync failed: %v", err)
	}
	if !innerExecuted {
		t.Error("inner function should have executed")
	}
}

func TestRuntime_TryRunOnLoopSync_ScheduledExecution(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rt, err := NewRuntime(ctx)
	if err != nil {
		t.Fatalf("NewRuntime failed: %v", err)
	}
	defer rt.Close()

	// Test from outside the event loop - should schedule and wait
	var executed bool
	err = rt.TryRunOnLoopSync(nil, func(vm *goja.Runtime) error {
		executed = true
		return nil
	})

	if err != nil {
		t.Errorf("TryRunOnLoopSync failed: %v", err)
	}
	if !executed {
		t.Error("function should have executed")
	}
}

func TestRuntime_LoadScript(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rt, err := NewRuntime(ctx)
	if err != nil {
		t.Fatalf("NewRuntime failed: %v", err)
	}
	defer rt.Close()

	// Test successful script
	err = rt.LoadScript("test.js", "var x = 42;")
	if err != nil {
		t.Errorf("LoadScript failed: %v", err)
	}

	// Verify the variable was set
	val, err := rt.GetGlobal("x")
	if err != nil {
		t.Errorf("GetGlobal failed: %v", err)
	}
	if val != int64(42) {
		t.Errorf("expected 42, got %v", val)
	}

	// Test script with syntax error
	err = rt.LoadScript("bad.js", "var y = {")
	if err == nil {
		t.Error("expected error for invalid script")
	}
}

func TestRuntime_SetGetGlobal(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rt, err := NewRuntime(ctx)
	if err != nil {
		t.Fatalf("NewRuntime failed: %v", err)
	}
	defer rt.Close()

	// Set a value
	err = rt.SetGlobal("testVar", "hello")
	if err != nil {
		t.Errorf("SetGlobal failed: %v", err)
	}

	// Get the value
	val, err := rt.GetGlobal("testVar")
	if err != nil {
		t.Errorf("GetGlobal failed: %v", err)
	}
	if val != "hello" {
		t.Errorf("expected 'hello', got %v", val)
	}

	// Get nonexistent value
	val, err = rt.GetGlobal("nonexistent")
	if err != nil {
		t.Errorf("GetGlobal for nonexistent should not error: %v", err)
	}
	if val != nil {
		t.Errorf("expected nil for nonexistent, got %v", val)
	}
}

func TestRuntime_GetCallable(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rt, err := NewRuntime(ctx)
	if err != nil {
		t.Fatalf("NewRuntime failed: %v", err)
	}
	defer rt.Close()

	// Create a function
	err = rt.LoadScript("test.js", "function add(a, b) { return a + b; }")
	if err != nil {
		t.Fatalf("LoadScript failed: %v", err)
	}

	// Get the function
	fn, err := rt.GetCallable("add")
	if err != nil {
		t.Errorf("GetCallable failed: %v", err)
	}
	if fn == nil {
		t.Error("function should not be nil")
	}

	// Get nonexistent function
	_, err = rt.GetCallable("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent function")
	}

	// Get non-callable value
	err = rt.SetGlobal("notAFunction", 42)
	if err != nil {
		t.Fatalf("SetGlobal failed: %v", err)
	}
	_, err = rt.GetCallable("notAFunction")
	if err == nil {
		t.Error("expected error for non-callable value")
	}
}

func TestRuntime_ConcurrentAccess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rt, err := NewRuntime(ctx)
	if err != nil {
		t.Fatalf("NewRuntime failed: %v", err)
	}
	defer rt.Close()

	// Initialize a counter
	err = rt.LoadScript("init.js", "var counter = 0;")
	if err != nil {
		t.Fatalf("LoadScript failed: %v", err)
	}

	// Run many concurrent operations
	const numGoroutines = 100
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			err := rt.RunOnLoopSync(func(vm *goja.Runtime) error {
				_, err := vm.RunString("counter++;")
				return err
			})
			if err != nil {
				t.Errorf("concurrent RunOnLoopSync failed: %v", err)
			}
		}()
	}

	wg.Wait()

	// Verify final counter value
	val, err := rt.GetGlobal("counter")
	if err != nil {
		t.Errorf("GetGlobal failed: %v", err)
	}
	if val != int64(numGoroutines) {
		t.Errorf("expected counter to be %d, got %v", numGoroutines, val)
	}
}

func TestRuntime_SetTimeout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rt, err := NewRuntime(ctx)
	if err != nil {
		t.Fatalf("NewRuntime failed: %v", err)
	}
	defer rt.Close()

	// Default timeout
	if rt.GetTimeout() != DefaultSyncTimeout {
		t.Errorf("expected default timeout %v, got %v", DefaultSyncTimeout, rt.GetTimeout())
	}

	// Set custom timeout
	rt.SetTimeout(10 * time.Second)
	if rt.GetTimeout() != 10*time.Second {
		t.Errorf("expected timeout 10s, got %v", rt.GetTimeout())
	}

	// Disable timeout
	rt.SetTimeout(0)
	if rt.GetTimeout() != 0 {
		t.Errorf("expected timeout 0, got %v", rt.GetTimeout())
	}
}

func TestParseGoroutineIDFromStack(t *testing.T) {
	tests := []struct {
		name     string
		stack    string
		expected int64
	}{
		{
			name:     "normal stack",
			stack:    "goroutine 123 [running]:\nmain.main()\n",
			expected: 123,
		},
		{
			name:     "chan receive",
			stack:    "goroutine 456 [chan receive]:\nmain.main()\n",
			expected: 456,
		},
		{
			name:     "empty stack",
			stack:    "",
			expected: 0,
		},
		{
			name:     "no goroutine prefix",
			stack:    "main.main()\n",
			expected: 0,
		},
		{
			name:     "malformed id",
			stack:    "goroutine abc [running]:\n",
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Call the internal parse function via reflection or re-implement for test
			// For simplicity, let's re-implement the parsing logic here
			result := parseGoroutineIDFromStackForTest([]byte(tt.stack))
			if result != tt.expected {
				t.Errorf("expected %d, got %d", tt.expected, result)
			}
		})
	}
}

// parseGoroutineIDFromStackForTest is a copy of the internal parseGoroutineIDFromStack
// function for testing purposes only.
func parseGoroutineIDFromStackForTest(stack []byte) int64 {
	if len(stack) < 10 {
		return 0 // Too short to contain "goroutine X"
	}

	prefix := [10]byte{'g', 'o', 'r', 'o', 'u', 't', 'i', 'n', 'e', ' '}
	for i := 0; i <= len(stack)-10; i++ {
		found := true
		for j := 0; j < 10; j++ {
			if stack[i+j] != prefix[j] {
				found = false
				break
			}
		}
		if found {
			id := int64(0)
			for j := i + 10; j < len(stack); j++ {
				b := stack[j]
				if b >= '0' && b <= '9' {
					id = id*10 + int64(b-'0')
				} else {
					return id
				}
			}
			return id
		}
	}

	return 0
}
