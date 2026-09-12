package scripting

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/joeycumines/goja"
	"github.com/joeycumines/goja_nodejs/require"
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

	if rt.Loop() == nil {
		t.Error("event loop should not be nil")
	}
}

func TestNewRuntimeRegistry(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	registry := require.NewRegistry()
	rt, err := NewRuntimeRegistry(ctx, registry)
	if err != nil {
		t.Fatalf("NewRuntimeRegistry failed: %v", err)
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

func TestRuntime_Run(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rt, err := NewRuntime(ctx)
	if err != nil {
		t.Fatalf("NewRuntime failed: %v", err)
	}
	defer rt.Close()

	var executed atomic.Bool
	done := make(chan struct{})

	ok := rt.Run(func(vm *goja.Runtime) {
		executed.Store(true)
		close(done)
	})

	if !ok {
		t.Fatal("Run should return true for running runtime")
	}

	select {
	case <-done:
		// expected
	case <-time.After(time.Second):
		t.Fatal("Run callback should execute")
	}

	if !executed.Load() {
		t.Error("callback should have been executed")
	}
}

func TestRuntime_Run_Stopped(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rt, err := NewRuntime(ctx)
	if err != nil {
		t.Fatalf("NewRuntime failed: %v", err)
	}

	rt.Close()

	ok := rt.Run(func(vm *goja.Runtime) {
		t.Error("callback should not be executed on stopped runtime")
	})

	if ok {
		t.Error("Run should return false for stopped runtime")
	}
}

func TestRuntime_RunSync(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rt, err := NewRuntime(ctx)
	if err != nil {
		t.Fatalf("NewRuntime failed: %v", err)
	}
	defer rt.Close()

	// Test successful execution
	var value int64
	err = rt.RunSync(ctx, func(vm *goja.Runtime) error {
		value = 42
		return nil
	})

	if err != nil {
		t.Errorf("RunSync failed: %v", err)
	}
	if value != 42 {
		t.Errorf("value should be 42, got %d", value)
	}

	// Test error propagation
	expectedErr := errors.New("test error")
	err = rt.RunSync(ctx, func(vm *goja.Runtime) error {
		return expectedErr
	})

	if err != expectedErr {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}
}

func TestRuntime_RunSync_Timeout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rt, err := NewRuntime(ctx)
	if err != nil {
		t.Fatalf("NewRuntime failed: %v", err)
	}
	defer rt.Close()

	ctxTimeout, cancelTimeout := context.WithTimeout(ctx, 10*time.Millisecond)
	defer cancelTimeout()

	// Schedule a long-running operation
	err = rt.RunSync(ctxTimeout, func(vm *goja.Runtime) error {
		time.Sleep(100 * time.Millisecond)
		return nil
	})

	if err == nil {
		t.Error("expected timeout error")
	}
}

func TestRuntime_RunSync_CancellationSkipsQueuedCallback(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rt, err := NewRuntime(ctx)
	if err != nil {
		t.Fatalf("NewRuntime failed: %v", err)
	}
	defer rt.Close()

	block := make(chan struct{})
	if !rt.Run(func(*goja.Runtime) { <-block }) {
		t.Fatal("failed to block event loop")
	}

	called := make(chan struct{})
	timeoutCtx, timeoutCancel := context.WithTimeout(ctx, 10*time.Millisecond)
	defer timeoutCancel()
	err = rt.RunSync(timeoutCtx, func(*goja.Runtime) error {
		close(called)
		return nil
	})
	if err == nil {
		t.Fatal("expected cancellation error")
	}

	close(block)
	select {
	case <-called:
		t.Fatal("cancelled callback executed after RunSync returned")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestRuntime_RunSync_Stopped(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rt, err := NewRuntime(ctx)
	if err != nil {
		t.Fatalf("NewRuntime failed: %v", err)
	}

	rt.Close()

	err = rt.RunSync(ctx, func(vm *goja.Runtime) error {
		return nil
	})

	if err == nil {
		t.Error("expected error for stopped runtime")
	}
}

func TestRuntime_TryRunSync_DirectExecution(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rt, err := NewRuntime(ctx)
	if err != nil {
		t.Fatalf("NewRuntime failed: %v", err)
	}
	defer rt.Close()

	// Test from within the event loop - should execute directly
	var innerExecuted bool
	err = rt.RunSync(ctx, func(vm *goja.Runtime) error {
		// This call from within the event loop should execute directly
		return rt.TryRunSync(ctx, vm, func(innerVM *goja.Runtime) error {
			innerExecuted = true
			// Should be same VM instance
			if innerVM != vm {
				return errors.New("inner VM should be same as outer VM")
			}
			return nil
		})
	})

	if err != nil {
		t.Errorf("TryRunSync failed: %v", err)
	}
	if !innerExecuted {
		t.Error("inner function should have executed")
	}
}

func TestRuntime_TryRunSync_ScheduledExecution(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rt, err := NewRuntime(ctx)
	if err != nil {
		t.Fatalf("NewRuntime failed: %v", err)
	}
	defer rt.Close()

	// Test from outside the event loop - should schedule and wait
	var executed bool
	err = rt.TryRunSync(ctx, nil, func(vm *goja.Runtime) error {
		executed = true
		return nil
	})

	if err != nil {
		t.Errorf("TryRunSync failed: %v", err)
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

	err = rt.LoadScript("test.js", `
		function add(a, b) {
			return a + b;
		}
	`)
	if err != nil {
		t.Errorf("LoadScript failed: %v", err)
	}
}

func TestRuntime_GetGlobal_SetGlobal(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rt, err := NewRuntime(ctx)
	if err != nil {
		t.Fatalf("NewRuntime failed: %v", err)
	}
	defer rt.Close()

	// Test setting and getting a primitive
	err = rt.SetGlobal("message", "hello")
	if err != nil {
		t.Errorf("SetGlobal failed: %v", err)
	}

	val, err := rt.GetGlobal("message")
	if err != nil {
		t.Errorf("GetGlobal failed: %v", err)
	}
	if val != "hello" {
		t.Errorf("expected 'hello', got %v", val)
	}

	// Test getting non-existent global
	val, err = rt.GetGlobal("nonexistent")
	if err != nil {
		t.Errorf("GetGlobal for nonexistent failed: %v", err)
	}
	if val != nil {
		t.Errorf("expected nil for nonexistent global, got %v", val)
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

	err = rt.LoadScript("test.js", `
		function multiply(a, b) {
			return a * b;
		}
		var notAFunction = 42;
	`)
	if err != nil {
		t.Fatalf("LoadScript failed: %v", err)
	}

	// Test valid callable
	fn, err := rt.GetCallable("multiply")
	if err != nil {
		t.Errorf("GetCallable failed: %v", err)
	}
	if fn == nil {
		t.Fatal("callable should not be nil")
	}

	// Test non-callable
	_, err = rt.GetCallable("notAFunction")
	if err == nil {
		t.Error("expected error for non-callable")
	}

	// Test nonexistent
	_, err = rt.GetCallable("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent function")
	}
}

func TestRuntime_Concurrent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rt, err := NewRuntime(ctx)
	if err != nil {
		t.Fatalf("NewRuntime failed: %v", err)
	}
	defer rt.Close()

	err = rt.SetGlobal("counter", int64(0))
	if err != nil {
		t.Fatalf("SetGlobal failed: %v", err)
	}

	var wg sync.WaitGroup
	numGoroutines := 10

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := rt.RunSync(ctx, func(vm *goja.Runtime) error {
				val := vm.Get("counter")
				current := val.ToInteger()
				vm.Set("counter", current+1)
				return nil
			})
			if err != nil {
				t.Errorf("concurrent RunSync failed: %v", err)
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
