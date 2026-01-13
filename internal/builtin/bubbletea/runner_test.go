package bubbletea_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
	"github.com/dop251/goja_nodejs/require"
	"github.com/joeycumines/one-shot-man/internal/builtin/bt"
	"github.com/joeycumines/one-shot-man/internal/builtin/bubbletea"
	"github.com/stretchr/testify/assert"
	ttRequire "github.com/stretchr/testify/require"
)

// TestJSRunner_BlocksCaller verifies that RunJSSync blocks until the callback completes.
func TestJSRunner_BlocksCaller(t *testing.T) {
	t.Parallel()

	registry := require.NewRegistry()
	loop := eventloop.NewEventLoop(eventloop.WithRegistry(registry))
	loop.Start()
	t.Cleanup(func() { loop.Stop() })

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	bridge := bt.NewBridgeWithEventLoop(ctx, loop, registry)
	t.Cleanup(bridge.Stop)

	var jsRunner bubbletea.JSRunner = bridge

	// Use a channel to verify blocking behavior
	started := make(chan struct{})
	completed := make(chan struct{})

	go func() {
		close(started)
		err := jsRunner.RunJSSync(func(vm *goja.Runtime) error {
			// Simulate some work
			time.Sleep(50 * time.Millisecond)
			return nil
		})
		assert.NoError(t, err)
		close(completed)
	}()

	// Wait for goroutine to start
	<-started

	// The goroutine should be blocked - completed should not be closed yet
	select {
	case <-completed:
		t.Fatal("RunJSSync should block until callback completes")
	case <-time.After(10 * time.Millisecond):
		// Good - still blocking
	}

	// Now wait for completion
	select {
	case <-completed:
		// Good - callback completed
	case <-time.After(500 * time.Millisecond):
		t.Fatal("RunJSSync should eventually complete")
	}
}

// TestJSRunner_PropagatesErrors verifies that errors from the callback are propagated.
func TestJSRunner_PropagatesErrors(t *testing.T) {
	t.Parallel()

	registry := require.NewRegistry()
	loop := eventloop.NewEventLoop(eventloop.WithRegistry(registry))
	loop.Start()
	t.Cleanup(func() { loop.Stop() })

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	bridge := bt.NewBridgeWithEventLoop(ctx, loop, registry)
	t.Cleanup(bridge.Stop)

	var jsRunner bubbletea.JSRunner = bridge

	expectedError := errors.New("test error from callback")

	err := jsRunner.RunJSSync(func(vm *goja.Runtime) error {
		return expectedError
	})

	ttRequire.Error(t, err)
	assert.Equal(t, expectedError, err)
}

// TestJSRunner_PropagatesNilError verifies that nil errors are propagated correctly.
func TestJSRunner_PropagatesNilError(t *testing.T) {
	t.Parallel()

	registry := require.NewRegistry()
	loop := eventloop.NewEventLoop(eventloop.WithRegistry(registry))
	loop.Start()
	t.Cleanup(func() { loop.Stop() })

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	bridge := bt.NewBridgeWithEventLoop(ctx, loop, registry)
	t.Cleanup(bridge.Stop)

	var jsRunner bubbletea.JSRunner = bridge

	err := jsRunner.RunJSSync(func(vm *goja.Runtime) error {
		return nil
	})

	assert.NoError(t, err)
}

// TestJSRunner_HandlesJSExecution verifies actual JavaScript execution works.
func TestJSRunner_HandlesJSExecution(t *testing.T) {
	t.Parallel()

	registry := require.NewRegistry()
	loop := eventloop.NewEventLoop(eventloop.WithRegistry(registry))
	loop.Start()
	t.Cleanup(func() { loop.Stop() })

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	bridge := bt.NewBridgeWithEventLoop(ctx, loop, registry)
	t.Cleanup(bridge.Stop)

	var jsRunner bubbletea.JSRunner = bridge

	var result int64

	err := jsRunner.RunJSSync(func(vm *goja.Runtime) error {
		val, err := vm.RunString("1 + 2 + 3")
		if err != nil {
			return err
		}
		result = val.ToInteger()
		return nil
	})

	ttRequire.NoError(t, err)
	assert.Equal(t, int64(6), result)
}

// TestJSRunner_HighContention verifies correct behavior under high contention.
// Run this test with -race to verify no data races.
func TestJSRunner_HighContention(t *testing.T) {
	t.Parallel()

	registry := require.NewRegistry()
	loop := eventloop.NewEventLoop(eventloop.WithRegistry(registry))
	loop.Start()
	t.Cleanup(func() { loop.Stop() })

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	bridge := bt.NewBridgeWithEventLoop(ctx, loop, registry)
	t.Cleanup(bridge.Stop)

	var jsRunner bubbletea.JSRunner = bridge

	const numGoroutines = 50
	const iterationsPerGoroutine = 20

	var counter int64
	var wg sync.WaitGroup
	var errorCount int64

	// Launch many goroutines that all call RunJSSync concurrently
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterationsPerGoroutine; j++ {
				err := jsRunner.RunJSSync(func(vm *goja.Runtime) error {
					// Access shared state safely (via event loop serialization)
					atomic.AddInt64(&counter, 1)

					// Also do some actual JS work
					val, err := vm.RunString("Math.random()")
					if err != nil {
						return err
					}
					if val.ToFloat() < 0 || val.ToFloat() > 1 {
						return errors.New("invalid random value")
					}
					return nil
				})
				if err != nil {
					atomic.AddInt64(&errorCount, 1)
				}
			}
		}(i)
	}

	wg.Wait()

	expectedCount := int64(numGoroutines * iterationsPerGoroutine)
	assert.Equal(t, expectedCount, counter, "All callbacks should have executed")
	assert.Zero(t, errorCount, "No errors should have occurred")
}

// TestJSRunner_ConcurrentWithDifferentOperations tests mixed read/write JS operations.
func TestJSRunner_ConcurrentWithDifferentOperations(t *testing.T) {
	t.Parallel()

	registry := require.NewRegistry()
	loop := eventloop.NewEventLoop(eventloop.WithRegistry(registry))
	loop.Start()
	t.Cleanup(func() { loop.Stop() })

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	bridge := bt.NewBridgeWithEventLoop(ctx, loop, registry)
	t.Cleanup(bridge.Stop)

	var jsRunner bubbletea.JSRunner = bridge

	// Initialize a global counter in JS
	err := jsRunner.RunJSSync(func(vm *goja.Runtime) error {
		_, err := vm.RunString("globalThis.sharedCounter = 0;")
		return err
	})
	ttRequire.NoError(t, err)

	const numWriters = 10
	const numReaders = 10
	const operationsPerGoroutine = 50

	var wg sync.WaitGroup
	var writeCount int64
	var readCount int64

	// Writers increment the counter
	for i := 0; i < numWriters; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < operationsPerGoroutine; j++ {
				err := jsRunner.RunJSSync(func(vm *goja.Runtime) error {
					_, err := vm.RunString("globalThis.sharedCounter++;")
					if err != nil {
						return err
					}
					atomic.AddInt64(&writeCount, 1)
					return nil
				})
				if err != nil {
					t.Errorf("Write failed: %v", err)
				}
			}
		}()
	}

	// Readers read the counter
	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < operationsPerGoroutine; j++ {
				err := jsRunner.RunJSSync(func(vm *goja.Runtime) error {
					val, err := vm.RunString("globalThis.sharedCounter")
					if err != nil {
						return err
					}
					// Counter should be non-negative
					if val.ToInteger() < 0 {
						return errors.New("counter went negative")
					}
					atomic.AddInt64(&readCount, 1)
					return nil
				})
				if err != nil {
					t.Errorf("Read failed: %v", err)
				}
			}
		}()
	}

	wg.Wait()

	expectedWrites := int64(numWriters * operationsPerGoroutine)
	expectedReads := int64(numReaders * operationsPerGoroutine)

	assert.Equal(t, expectedWrites, writeCount, "All writes should complete")
	assert.Equal(t, expectedReads, readCount, "All reads should complete")

	// Verify final counter value matches write count
	var finalCount int64
	err = jsRunner.RunJSSync(func(vm *goja.Runtime) error {
		val, err := vm.RunString("globalThis.sharedCounter")
		if err != nil {
			return err
		}
		finalCount = val.ToInteger()
		return nil
	})
	ttRequire.NoError(t, err)
	assert.Equal(t, expectedWrites, finalCount, "Final counter should match total writes")
}

// TestJSRunner_StoppedBridgeReturnsError verifies error when bridge is stopped.
func TestJSRunner_StoppedBridgeReturnsError(t *testing.T) {
	t.Parallel()

	registry := require.NewRegistry()
	loop := eventloop.NewEventLoop(eventloop.WithRegistry(registry))
	loop.Start()
	t.Cleanup(func() { loop.Stop() })

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	bridge := bt.NewBridgeWithEventLoop(ctx, loop, registry)

	var jsRunner bubbletea.JSRunner = bridge

	// Verify it works before stopping
	err := jsRunner.RunJSSync(func(vm *goja.Runtime) error {
		return nil
	})
	ttRequire.NoError(t, err)

	// Stop the bridge
	bridge.Stop()

	// Now it should return an error
	err = jsRunner.RunJSSync(func(vm *goja.Runtime) error {
		return nil
	})
	ttRequire.Error(t, err)
}

// TestSyncJSRunner_ExecutesSynchronously tests the test helper SyncJSRunner.
func TestSyncJSRunner_ExecutesSynchronously(t *testing.T) {
	t.Parallel()

	vm := goja.New()
	runner := &bubbletea.SyncJSRunner{Runtime: vm}

	var result int64

	err := runner.RunJSSync(func(vm *goja.Runtime) error {
		val, err := vm.RunString("10 * 5")
		if err != nil {
			return err
		}
		result = val.ToInteger()
		return nil
	})

	ttRequire.NoError(t, err)
	assert.Equal(t, int64(50), result)
}

// TestSyncJSRunner_PropagatesErrors tests error propagation for the test helper.
func TestSyncJSRunner_PropagatesErrors(t *testing.T) {
	t.Parallel()

	vm := goja.New()
	runner := &bubbletea.SyncJSRunner{Runtime: vm}

	expectedErr := errors.New("sync runner test error")

	err := runner.RunJSSync(func(vm *goja.Runtime) error {
		return expectedErr
	})

	ttRequire.Error(t, err)
	assert.Equal(t, expectedErr, err)
}
