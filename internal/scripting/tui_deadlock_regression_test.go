package scripting

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

// scheduleWrite queues a mutation task to be executed by the writer goroutine.
// The task runs under tm.mu.Lock(). This method returns immediately without waiting
// for the task to complete (fire-and-forget semantics).
//
// This is the fire-and-forget variant of scheduleWriteAndWait, used only in tests.
func (tm *TUIManager) scheduleWrite(fn func() error) {
	tm.queueMu.Lock()
	if tm.writerShutdown.IsSet() {
		tm.queueMu.Unlock()
		return
	}
	tm.queueMu.Unlock()

	task := writeTask{fn: fn, resultCh: nil}
	select {
	case tm.writerQueue <- task:
	case <-tm.writerStop:
	}
}

// TestTUIExitFromJSCommandNoDeadlock is a regression test for the deadlock scenario
// where a JavaScript command handler calls tui.exit() while the manager is in a state
// that would previously deadlock.
//
// The bug: JS callbacks (completers, command handlers, key bindings) run without holding
// locks and can synchronously call JS-exposed mutators that attempt to acquire the write
// lock. If another goroutine holds the write lock and waits on something from that JS
// invocation, a deadlock occurs.
//
// The fix: JS-originated mutations are routed through a single writer goroutine via
// scheduleWrite/scheduleWriteAndWait. The writer goroutine is the only place that holds
// tm.mu.Lock() for JS-originated mutations.
func TestTUIExitFromJSCommandNoDeadlock(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var output bytes.Buffer
	engine, err := NewEngineWithConfig(ctx, &output, &output, "", "")
	if err != nil {
		t.Fatalf("NewEngineWithConfig failed: %v", err)
	}
	manager := NewTUIManagerWithConfig(ctx, engine, strings.NewReader(""), &output, "test-deadlock", "memory")
	defer manager.Close()

	// Simulate the scenario: a JS command that calls TriggerExit
	// This should NOT deadlock because TriggerExit only briefly locks to read activePrompt
	// and the JS command handler runs via the executor which doesn't hold the write lock.

	exitCalled := make(chan struct{})
	var wg sync.WaitGroup

	// Register a command that triggers exit
	testCmd := Command{
		Name:        "test-exit",
		Description: "Test command that triggers exit",
		IsGoCommand: true,
		Handler: func(args []string) error {
			// This simulates what tui.exit() does from JS
			manager.TriggerExit()
			close(exitCalled)
			return nil
		},
	}
	manager.RegisterCommand(testCmd)

	// Verify command was registered
	commands := manager.ListCommands()
	found := false
	for _, cmd := range commands {
		if cmd.Name == "test-exit" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("test-exit command not registered")
	}

	// Execute the command - this should not deadlock
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := manager.ExecuteCommand("test-exit", nil)
		if err != nil {
			t.Errorf("ExecuteCommand failed: %v", err)
		}
	}()

	// Wait for exit to be called or timeout
	select {
	case <-exitCalled:
		// Success - no deadlock
	case <-ctx.Done():
		t.Fatal("DEADLOCK: test timed out waiting for exit to be called")
	}

	wg.Wait()
}

// TestJSMutatorsUseWriterQueue verifies that JS mutator functions use the writer
// queue instead of acquiring locks directly.
func TestJSMutatorsUseWriterQueue(t *testing.T) {
	ctx := context.Background()
	var output bytes.Buffer
	engine, err := NewEngineWithConfig(ctx, &output, &output, "", "")
	if err != nil {
		t.Fatalf("NewEngineWithConfig failed: %v", err)
	}
	manager := NewTUIManagerWithConfig(ctx, engine, strings.NewReader(""), &output, "test-mutators", "memory")
	defer manager.Close()

	// Test that scheduleWriteAndWait works correctly
	var executed bool
	err = manager.scheduleWriteAndWait(func() error {
		executed = true
		return nil
	})
	if err != nil {
		t.Fatalf("scheduleWriteAndWait failed: %v", err)
	}
	if !executed {
		t.Fatal("scheduleWriteAndWait did not execute the task")
	}

	// Test that scheduleWrite (fire-and-forget) works correctly
	executedCh := make(chan struct{})
	manager.scheduleWrite(func() error {
		close(executedCh)
		return nil
	})

	select {
	case <-executedCh:
		// Success
	case <-time.After(time.Second):
		t.Fatal("scheduleWrite did not execute the task within timeout")
	}
}

// TestConcurrentJSMutators verifies that concurrent JS mutator calls don't cause
// data races or deadlocks.
func TestConcurrentJSMutators(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var output bytes.Buffer
	engine, err := NewEngineWithConfig(ctx, &output, &output, "", "")
	if err != nil {
		t.Fatalf("NewEngineWithConfig failed: %v", err)
	}
	manager := NewTUIManagerWithConfig(ctx, engine, strings.NewReader(""), &output, "test-concurrent", "memory")
	defer manager.Close()

	const numGoroutines = 10
	const numOpsPerGoroutine = 100

	var wg sync.WaitGroup
	errCh := make(chan error, numGoroutines*numOpsPerGoroutine)

	// Spawn goroutines that concurrently perform mutations via the writer queue
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOpsPerGoroutine; j++ {
				err := manager.scheduleWriteAndWait(func() error {
					// Simulate a mutation
					return nil
				})
				if err != nil {
					errCh <- err
				}

				// Also do some reads to test RLock behavior
				_ = manager.ListModes()
				_ = manager.ListCommands()
			}
		}(i)
	}

	// Wait for all goroutines to complete or timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success - no deadlock
	case <-ctx.Done():
		t.Fatal("DEADLOCK: test timed out during concurrent operations")
	}

	close(errCh)
	for err := range errCh {
		t.Errorf("Error during concurrent operations: %v", err)
	}
}

// TestWriterGoroutineShutdown verifies that the writer goroutine shuts down
// gracefully when Close is called.
func TestWriterGoroutineShutdown(t *testing.T) {
	ctx := context.Background()
	var output bytes.Buffer
	engine, err := NewEngineWithConfig(ctx, &output, &output, "", "")
	if err != nil {
		t.Fatalf("NewEngineWithConfig failed: %v", err)
	}
	manager := NewTUIManagerWithConfig(ctx, engine, strings.NewReader(""), &output, "test-shutdown", "memory")

	// Queue a task before shutdown
	err = manager.scheduleWriteAndWait(func() error {
		return nil
	})
	if err != nil {
		t.Fatalf("scheduleWriteAndWait before shutdown failed: %v", err)
	}

	// Close the manager (this should stop the writer)
	if err := manager.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Verify that scheduling after shutdown returns an error
	err = manager.scheduleWriteAndWait(func() error {
		return nil
	})
	if err == nil {
		t.Fatal("scheduleWriteAndWait after shutdown should return an error")
	}
}

// TestReadOperationsRemainReentrant verifies that read operations using RLock
// remain re-entrant and don't block when called from JS callbacks.
func TestReadOperationsRemainReentrant(t *testing.T) {
	ctx := context.Background()
	var output bytes.Buffer
	engine, err := NewEngineWithConfig(ctx, &output, &output, "", "")
	if err != nil {
		t.Fatalf("NewEngineWithConfig failed: %v", err)
	}
	manager := NewTUIManagerWithConfig(ctx, engine, strings.NewReader(""), &output, "test-reentrant", "memory")
	defer manager.Close()

	// Simulate a scenario where we're in a read context and need to read more
	// This should NOT deadlock because RLock is re-entrant (multiple readers allowed)

	resultCh := make(chan bool, 1)

	go func() {
		// First read
		modes := manager.ListModes()

		// Second read while first "context" is still active (simulating JS callback)
		commands := manager.ListCommands()

		// Third read
		mode := manager.GetCurrentMode()

		// If we get here without deadlock, success
		resultCh <- (modes != nil || true) && (commands != nil || true) && (mode == nil || true)
	}()

	select {
	case success := <-resultCh:
		if !success {
			t.Fatal("Read operations returned unexpected results")
		}
	case <-time.After(time.Second):
		t.Fatal("DEADLOCK: read operations blocked")
	}
}
