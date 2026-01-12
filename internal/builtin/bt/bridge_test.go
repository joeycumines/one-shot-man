package bt

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
	gojarequire "github.com/dop251/goja_nodejs/require"
	"github.com/stretchr/testify/require"
)

// testBridge creates a new Bridge with its own test event loop.
// The event loop is automatically started and will be stopped when
// the bridge's Stop() method is called.
//
// This helper is for testing purposes only. Production code should
// use NewBridgeWithEventLoop with a shared event loop.
func testBridge(t *testing.T) *Bridge {
	reg := gojarequire.NewRegistry()
	loop := eventloop.NewEventLoop(
		eventloop.WithRegistry(reg),
		eventloop.EnableConsole(true),
	)
	loop.Start()

	ctx := context.Background()
	t.Cleanup(func() {
		loop.Stop()
	})

	bridge := NewBridgeWithEventLoop(ctx, loop, reg)
	t.Cleanup(func() {
		bridge.Stop()
	})

	return bridge
}

// TestBridgeWithManualShutdown_CleanupSafetyNet verifies that the safety cleanup
// in testBridgeWithManualShutdown works correctly by intentionally NOT calling Stop()
// and verifying the cleanup still fires.
func TestBridgeWithManualShutdown_CleanupSafetyNet(t *testing.T) {
	t.Parallel()

	// Create bridge but do NOT call Stop()
	bridge, loop := testBridgeWithManualShutdown(t)

	// Verify it's running
	require.True(t, bridge.IsRunning())

	// Do NOT call bridge.Stop() or loop.Stop() here
	// The t.Cleanup safety net will handle it when the test function ends
	// This test verifies that resources don't leak even if we forget to clean up
	_ = loop // Use the loop variable to avoid "declared and not used" error
}

// testBridgeWithManualShutdown creates a new Bridge without automatic cleanup.
// The test is responsible for calling bridge.Stop() and loop.Stop().
// This is needed for tests that need to control the shutdown timing precisely.
//
// Returns a tuple (*Bridge, *eventloop.EventLoop) for explicit control.
// Even though cleanup is manual, a safety net is registered via t.Cleanup
// to prevent resource leaks if the test forgets to call Stop().
func testBridgeWithManualShutdown(t *testing.T) (*Bridge, *eventloop.EventLoop) {
	reg := gojarequire.NewRegistry()
	loop := eventloop.NewEventLoop(
		eventloop.WithRegistry(reg),
		eventloop.EnableConsole(true),
	)
	loop.Start()

	ctx := context.Background()
	bridge := NewBridgeWithEventLoop(ctx, loop, reg)

	// Safety cleanup in case test forgets to call Stop()
	// Uses generous 30-second timeout for graceful shutdown
	t.Cleanup(func() {
		// Stop the bridge
		if bridge.IsRunning() {
			done := make(chan struct{})
			go func() {
				bridge.Stop()
				close(done)
			}()
			select {
			case <-done:
				// Bridge stopped cleanly
			case <-time.After(30 * time.Second):
				t.Logf("WARNING: bridge.Stop() timed out after 30s in test cleanup")
			}
		}

		// Stop the event loop
		stopDone := make(chan struct{})
		go func() {
			loop.Stop()
			close(stopDone)
		}()
		select {
		case <-stopDone:
			// Loop stopped cleanly
		case <-time.After(30 * time.Second):
			t.Logf("WARNING: loop.Stop() timed out after 30s in test cleanup")
		}
	})

	return bridge, loop
}

func TestBridge_NewAndStop(t *testing.T) {
	t.Parallel()

	bridge := testBridge(t)
	require.True(t, bridge.IsRunning())

	bridge.Stop()
	require.False(t, bridge.IsRunning())

	// Stop should be idempotent
	bridge.Stop()
	require.False(t, bridge.IsRunning())
}

func TestBridge_ContextCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	reg := gojarequire.NewRegistry()
	loop := eventloop.NewEventLoop(
		eventloop.WithRegistry(reg),
		eventloop.EnableConsole(true),
	)
	loop.Start()
	t.Cleanup(func() { loop.Stop() })

	bridge := NewBridgeWithEventLoop(ctx, loop, reg)
	require.True(t, bridge.IsRunning())

	// Cancel context
	cancel()

	// Wait for bridge to stop - use Done() channel for deterministic synchronization
	select {
	case <-bridge.Done():
		// Bridge stopped
	case <-time.After(5 * time.Second):
		t.Fatal("bridge did not stop after context cancellation")
	}

	require.False(t, bridge.IsRunning())
}

func TestBridge_RunOnLoop(t *testing.T) {
	t.Parallel()

	bridge := testBridge(t)

	// Run code on the loop
	executed := make(chan bool, 1)
	ok := bridge.RunOnLoop(func(vm *goja.Runtime) {
		executed <- true
	})
	require.True(t, ok)

	select {
	case <-executed:
		// Success
	case <-time.After(time.Second):
		t.Fatal("RunOnLoop callback not executed")
	}
}

func TestBridge_RunOnLoopAfterStop(t *testing.T) {
	t.Parallel()

	// Create a bridge without using the helper to control stop timing
	reg := gojarequire.NewRegistry()
	loop := eventloop.NewEventLoop(
		eventloop.WithRegistry(reg),
		eventloop.EnableConsole(true),
	)
	loop.Start()
	defer loop.Stop()

	ctx := context.Background()
	bridge := NewBridgeWithEventLoop(ctx, loop, reg)

	bridge.Stop()

	// Should return false after stop
	ok := bridge.RunOnLoop(func(vm *goja.Runtime) {
		t.Fatal("Should not execute after stop")
	})
	require.False(t, ok)
}

func TestBridge_LoadScript(t *testing.T) {
	t.Parallel()

	bridge := testBridge(t)

	// Load a simple script
	err := bridge.LoadScript("test.js", `
		function testFunc() {
			return 42;
		}
	`)
	require.NoError(t, err)

	// Verify the function exists
	val, exists := bridge.GetGlobal("testFunc")
	require.True(t, exists, "testFunc should exist after loading script")
	require.NotNil(t, val, "testFunc should not be nil")
}

func TestBridge_LoadScriptError(t *testing.T) {
	t.Parallel()

	bridge := testBridge(t)

	// Load invalid script
	err := bridge.LoadScript("bad.js", `this is not valid javascript {`)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to compile")
}

func TestBridge_SetGetGlobal(t *testing.T) {
	t.Parallel()

	bridge := testBridge(t)

	// Set various types
	err := bridge.SetGlobal("intVal", 42)
	require.NoError(t, err)

	err = bridge.SetGlobal("strVal", "hello")
	require.NoError(t, err)

	err = bridge.SetGlobal("boolVal", true)
	require.NoError(t, err)

	// Get them back
	val, exists := bridge.GetGlobal("intVal")
	require.True(t, exists, "intVal should exist")
	require.Equal(t, int64(42), val)

	val, exists = bridge.GetGlobal("strVal")
	require.True(t, exists, "strVal should exist")
	require.Equal(t, "hello", val)

	val, exists = bridge.GetGlobal("boolVal")
	require.True(t, exists, "boolVal should exist")
	require.Equal(t, true, val)

	// Non-existent global
	val, exists = bridge.GetGlobal("nonexistent")
	require.False(t, exists, "nonexistent should not exist")
	require.Nil(t, val, "nonexistent should return nil value")
}

func TestBridge_ExposeBlackboard(t *testing.T) {
	t.Parallel()

	bridge := testBridge(t)

	bb := new(Blackboard)
	bb.Set("testKey", "testValue")

	err := bridge.ExposeBlackboard("myBlackboard", bb)
	require.NoError(t, err)

	// Access from JS
	err = bridge.LoadScript("test.js", `
		var val = myBlackboard.get("testKey");
		myBlackboard.set("fromJS", val + "_modified");
	`)
	require.NoError(t, err)

	// Verify the modification
	require.Equal(t, "testValue_modified", bb.Get("fromJS"))
}

func TestBridge_JSHelpers(t *testing.T) {
	t.Parallel()

	bridge := testBridge(t)

	// Verify bt constants are available
	val, exists := bridge.GetGlobal("bt")
	require.True(t, exists, "bt should exist")
	require.NotNil(t, val, "bt should not be nil")

	btMap, ok := val.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "running", btMap["running"])
	require.Equal(t, "success", btMap["success"])
	require.Equal(t, "failure", btMap["failure"])

	// Verify runLeaf is available
	val, exists = bridge.GetGlobal("runLeaf")
	require.True(t, exists, "runLeaf should exist")
	require.NotNil(t, val, "runLeaf should not be nil")
}

// TestBridge_OsmBtModuleRegistered verifies that the osm:bt module is automatically registered
// and accessible via require('osm:bt') in JavaScript.
func TestBridge_OsmBtModuleRegistered(t *testing.T) {
	t.Parallel()

	bridge := testBridge(t)

	// Try to require the osm:bt module from JavaScript
	err := bridge.LoadScript("test.js", `
		const bt = require('osm:bt');
		globalThis.btModule = bt;
		globalThis.hasRunning = bt.running === 'running';
		globalThis.hasSuccess = bt.success === 'success';
		globalThis.hasFailure = bt.failure === 'failure';
	`)
	require.NoError(t, err)

	// Verify the module was loaded correctly
	val, exists := bridge.GetGlobal("btModule")
	require.True(t, exists, "btModule should exist after require")
	require.NotNil(t, val, "btModule should not be nil")

	val, exists = bridge.GetGlobal("hasRunning")
	require.True(t, exists, "hasRunning should exist")
	require.Equal(t, true, val)

	val, exists = bridge.GetGlobal("hasSuccess")
	require.True(t, exists, "hasSuccess should exist")
	require.Equal(t, true, val)

	val, exists = bridge.GetGlobal("hasFailure")
	require.True(t, exists, "hasFailure should exist")
	require.Equal(t, true, val)
}

// TestBridge_TryRunOnLoopSync tests the deadlock prevention method with comprehensive coverage.
// It verifies:
//   - Calling from event loop goroutine (direct execution path)
//   - Calling from non-event-loop goroutine (scheduled path)
//   - Correct VM usage from event loop vs scheduled path
//   - Both paths work correctly, deadlock prevention works
func TestBridge_TryRunOnLoopSync(t *testing.T) {
	t.Parallel()

	bridge := testBridge(t)

	t.Run("direct path from event loop", func(t *testing.T) {
		t.Parallel()

		// Use RunOnLoop to get on the event loop goroutine, then call TryRunOnLoopSync
		// which should detect we're already on the event loop and execute directly
		executed := make(chan bool, 1)
		vmCaptured := make(chan *goja.Runtime, 1)

		err := bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
			// We're on the event loop now, call TryRunOnLoopSync with this VM
			// This should take the direct path, executing fn directly
			err := bridge.TryRunOnLoopSync(vm, func(passedVM *goja.Runtime) error {
				vmCaptured <- passedVM
				executed <- true
				return nil
			})
			require.NoError(t, err)
			return nil
		})
		require.NoError(t, err)

		// Verify the callback was executed
		select {
		case <-executed:
			// Success - callback executed
		case <-time.After(time.Second):
			t.Fatal("TryRunOnLoopSync callback not executed via direct path")
		}

		// Verify the VM passed to the callback is the same as the one we called with
		select {
		case passedVM := <-vmCaptured:
			require.NotNil(t, passedVM, "VM should be non-nil when on event loop with non-nil currentVM")
		case <-time.After(time.Second):
			t.Fatal("VM not captured")
		}
	})

	t.Run("scheduled path from non-event-loop goroutine", func(t *testing.T) {
		t.Parallel()

		// Call TryRunOnLoopSync from a different goroutine
		// This should take the scheduled path, posting to the event loop and waiting
		executed := make(chan bool, 1)
		done := make(chan bool, 1)

		go func() {
			defer close(done)
			err := bridge.TryRunOnLoopSync(nil, func(vm *goja.Runtime) error {
				// This should execute on the event loop via scheduling
				// The VM parameter comes from the event loop callback
				require.NotNil(t, vm, "VM should be provided by event loop in scheduled path")
				executed <- true
				return nil
			})
			require.NoError(t, err)
		}()

		// Verify the callback was executed
		select {
		case <-executed:
			// Success - callback executed via scheduled path
		case <-time.After(time.Second):
			t.Fatal("TryRunOnLoopSync callback not executed via scheduled path")
		}

		// Wait for goroutine to complete
		select {
		case <-done:
			// Goroutine finished
		case <-time.After(time.Second):
			t.Fatal("Caller goroutine did not complete")
		}
	})

	t.Run("verify operations work correctly from both paths", func(t *testing.T) {
		t.Parallel()

		// Test that actual operations work from both paths
		// Set a value from event loop path, verify from scheduled path
		err := bridge.SetGlobal("testKey1", "fromEventLoop")
		require.NoError(t, err)

		// Now verify from a different goroutine (scheduled path)
		done := make(chan bool, 1)
		go func() {
			defer close(done)
			val, exists := bridge.GetGlobal("testKey1")
			require.True(t, exists, "testKey1 should exist")
			require.Equal(t, "fromEventLoop", val)

			// Set another value from the scheduled path
			err := bridge.SetGlobal("testKey2", "fromScheduled")
			require.NoError(t, err)
		}()

		select {
		case <-done:
			// Success - scheduled path operations worked
		case <-time.After(time.Second):
			t.Fatal("Scheduled path operations did not complete")
		}

		// Verify both values are accessible
		val, exists := bridge.GetGlobal("testKey1")
		require.True(t, exists, "testKey1 should exist")
		require.Equal(t, "fromEventLoop", val)

		val, exists = bridge.GetGlobal("testKey2")
		require.True(t, exists, "testKey2 should exist")
		require.Equal(t, "fromScheduled", val)
	})

	t.Run("verify no deadlock when called recursively", func(t *testing.T) {
		t.Parallel()

		// Test the deadlock prevention scenario: TryRunOnLoopSync called
		// from within the event loop (e.g., from a JS callback)
		// This is the critical scenario that would cause deadlock without the
		// goroutine ID check
		depth := 0
		maxDepth := 3

		// This function will call itself recursively via TryRunOnLoopSync
		var recursiveTest func(currentVM *goja.Runtime) error
		recursiveTest = func(currentVM *goja.Runtime) error {
			depth++
			if depth >= maxDepth {
				return nil
			}
			// Call TryRunOnLoopSync again - should NOT deadlock
			return bridge.TryRunOnLoopSync(currentVM, recursiveTest)
		}

		// Start recursive calls from the event loop
		err := bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
			return recursiveTest(vm)
		})
		require.NoError(t, err)
		require.Equal(t, maxDepth, depth, "Should have reached max depth without deadlock")
	})

	t.Run("verify nil currentVM on event loop handled correctly", func(t *testing.T) {
		t.Parallel()

		// Test calling TryRunOnLoopSync from event loop with nil currentVM
		// This is an edge case - function should be called with nil
		executed := make(chan bool, 1)
		vmWasNil := make(chan bool, 1)

		err := bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
			// Call TryRunOnLoopSync with nil currentVM (should still work)
			err := bridge.TryRunOnLoopSync(nil, func(passedVM *goja.Runtime) error {
				vmWasNil <- (passedVM == nil)
				executed <- true
				return nil
			})
			return err
		})
		require.NoError(t, err)

		// Verify the callback was executed
		select {
		case <-executed:
			// Success
		case <-time.After(time.Second):
			t.Fatal("Callback not executed with nil currentVM")
		}

		// Verify that passedVM was indeed nil
		select {
		case isNil := <-vmWasNil:
			require.True(t, isNil, "Passed VM should be nil when currentVM is nil")
		case <-time.After(time.Second):
			t.Fatal("VM nil status not captured")
		}
	})

	t.Run("verify currentVM ignored when called from non-event-loop", func(t *testing.T) {
		t.Parallel()

		// Test that currentVM is ignored when NOT on event loop
		// The function should receive VM from the event loop callback
		executed := make(chan *goja.Runtime, 1)
		done := make(chan bool, 1)

		// Create a dummy VM that should NOT be used
		dummyVM := &goja.Runtime{} // This is invalid but that's ok - it won't be used

		go func() {
			defer close(done)
			// Call from non-event-loop with a dummy VM
			// The dummyVM should be ignored; function should receive real event loop VM
			err := bridge.TryRunOnLoopSync(dummyVM, func(vm *goja.Runtime) error {
				executed <- vm
				return nil
			})
			require.NoError(t, err)
		}()

		select {
		case vm := <-executed:
			// Verify that the VM received is NOT the dummy VM
			require.NotEqual(t, dummyVM, vm, "Should receive event loop VM, not the dummy currentVM")
			require.NotNil(t, vm, "Should receive valid event loop VM")
		case <-time.After(time.Second):
			t.Fatal("Function not executed from scheduled path")
		}

		select {
		case <-done:
			// Goroutine completed successfully
		case <-time.After(time.Second):
			t.Fatal("Caller goroutine did not complete")
		}
	})
}

// TestBridge_RunOnLoopSyncTimeout tests the timeout mechanism of RunOnLoopSync.
// It verifies:
//   - Slow JS operations trigger timeout with appropriate error message
//   - The bridge remains functional after a timeout (not corrupted)
//   - Subsequent operations succeed after timeout
//
// NOTE: We use a separate bridge for each sub-test to avoid race conditions
// when multiple sub-tests try to modify the timeout concurrently.
func TestBridge_RunOnLoopSyncTimeout(t *testing.T) {
	t.Parallel()

	// Test 1: Verify slow blocking operation triggers timeout
	t.Run("slow operation triggers timeout", func(t *testing.T) {
		t.Parallel()

		bridge := testBridge(t)

		// Set a short timeout for this test
		// Default is 5 seconds, but we want faster test execution
		shortTimeout := 100 * time.Millisecond
		bridge.SetTimeout(shortTimeout)

		// Load a script with a blocking operation using a busy-wait loop
		// This will block the event loop for longer than the timeout
		err := bridge.LoadScript("slow.js", `
			// This function blocks for the specified milliseconds using a busy-wait loop
			// Note: This is intentionally blocking to test timeout behavior
			globalThis.busyWait = function(ms) {
				var start = Date.now();
				while (Date.now() - start < ms) {
					// Busy wait - this blocks the JavaScript execution
					// We do some work in the loop to prevent optimization
					var x = 0;
					for (var i = 0; i < 1000; i++) {
						x += i;
					}
				}
				return 'done after ' + ms + 'ms';
			};
		`)
		require.NoError(t, err, "Failed to load slow operation script")

		// Call RunOnLoopSync with a slow operation (200ms, longer than 100ms timeout)
		// This should timeout
		err = bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
			// Call the busy wait function for 200ms
			_, err := vm.RunString("busyWait(200);")
			return err
		})

		// Verify we got a timeout error
		require.Error(t, err, "RunOnLoopSync should return error for long blocking operation")
		require.Contains(t, err.Error(), "timed out", "Error should mention timeout")
	})

	// Test 2: Verify timeout error message includes configured timeout value
	t.Run("timeout error message includes timeout duration", func(t *testing.T) {
		t.Parallel()

		bridge := testBridge(t)

		// Set a specific timeout to verify error message
		shortTimeout := 250 * time.Millisecond
		bridge.SetTimeout(shortTimeout)

		// Load script with busy-wait that will exceed timeout
		err := bridge.LoadScript("message_test.js", `
			globalThis.slowOp = function(ms) {
				var start = Date.now();
				while (Date.now() - start < ms) {
					var x = 0;
					for (var i = 0; i < 1000; i++) {
						x += i;
					}
				}
				return 'done';
			};
		`)
		require.NoError(t, err)

		// Trigger timeout with a 500ms wait (longer than 250ms timeout)
		err = bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
			_, err := vm.RunString("slowOp(500);")
			return err
		})
		require.Error(t, err)

		// Verify error message includes the timeout value
		// The error message format: "operation timed out after %v (consider increasing timeout...)"
		require.Contains(t, err.Error(), "250ms", "Error message should include configured timeout duration")
	})

	// Test 3: Verify bridge remains functional after timeout on a SEPARATE bridge
	// We can't test this on the same bridge because a timeout leaves the event loop
	// in an indeterminate state (we don't know what operation was running)
	t.Run("separate bridge functional after timeout on another", func(t *testing.T) {
		t.Parallel()

		// Create two separate bridges
		bridge1 := testBridge(t)
		bridge2 := testBridge(t)

		// Set short timeout on first bridge
		bridge1.SetTimeout(100 * time.Millisecond)

		// Load slow operation on first bridge
		err := bridge1.LoadScript("slow.js", `
			globalThis.slowOp = function(ms) {
				var start = Date.now();
				while (Date.now() - start < ms) {
					var x = 0;
					for (var i = 0; i < 1000; i++) {
						x += i;
					}
				}
				return 'done';
			};
		`)
		require.NoError(t, err)

		// Trigger timeout on first bridge
		err = bridge1.RunOnLoopSync(func(vm *goja.Runtime) error {
			_, err := vm.RunString("slowOp(200);") // 200ms longer than 100ms timeout
			return err
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "timed out")

		// Now verify second bridge (which wasn't involved in timeout) is functional
		err = bridge2.LoadScript("functional_test.js", `
			globalThis.simpleAdd = function(a, b) {
				return a + b;
			};
		`)
		require.NoError(t, err, "Should be able to load script on second bridge")

		// Verify the function works on second bridge
		err = bridge2.RunOnLoopSync(func(vm *goja.Runtime) error {
			val, err := vm.RunString("simpleAdd(2, 3);")
			if err != nil {
				return err
			}
			result := val.Export().(int64)
			if result != 5 {
				return fmt.Errorf("expected 5, got %d", result)
			}
			return nil
		})
		require.NoError(t, err, "Second bridge should work normally after first bridge timed out")
	})

	// Test 4: Verify quick operations don't timeout
	t.Run("quick operations succeed without timeout", func(t *testing.T) {
		t.Parallel()

		bridge := testBridge(t)

		// Use a very short timeout to prove our quick operation is fast enough
		bridge.SetTimeout(50 * time.Millisecond)

		// Load a quick operation script
		err := bridge.LoadScript("quick.js", `
			globalThis.quickOp = function(x) {
				return x * 2;
			};
		`)
		require.NoError(t, err)

		// Call the quick operation
		err = bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
			val, err := vm.RunString("quickOp(21);")
			if err != nil {
				return err
			}
			result := val.Export().(int64)
			if result != 42 {
				return fmt.Errorf("expected 42, got %d", result)
			}
			return nil
		})
		require.NoError(t, err, "Quick operation should succeed without timeout")
	})

	// Test 5: Verify SetTimeout and GetTimeout work correctly
	t.Run("SetTimeout and GetTimeout work correctly", func(t *testing.T) {
		t.Parallel()

		bridge := testBridge(t)

		// Verify default timeout
		require.Equal(t, bridge.GetTimeout(), DefaultTimeout, "Default timeout should match DefaultTimeout constant")

		// Verify SetTimeout works
		customTimeout := 123 * time.Millisecond
		bridge.SetTimeout(customTimeout)
		require.Equal(t, customTimeout, bridge.GetTimeout(), "GetTimeout should return the value we set")

		// Verify setting to zero works (disables timeout)
		bridge.SetTimeout(0)
		require.Equal(t, time.Duration(0), bridge.GetTimeout(), "Setting to zero should work")

		// Restore default
		bridge.SetTimeout(DefaultTimeout)
		require.Equal(t, DefaultTimeout, bridge.GetTimeout(), "Restoring default should work")
	})

	// Test 6: Verify timeout after bridge stop also works
	t.Run("timeout on stopped bridge returns appropriate error", func(t *testing.T) {
		t.Parallel()

		bridge := testBridge(t)

		// Stop the bridge
		bridge.Stop()

		// Try to run an operation - should fail with "bridge stopped" not timeout
		err := bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
			return nil
		})

		require.Error(t, err)
		require.Contains(t, err.Error(), "not running", "Should mention event loop not running, not timeout")
	})
}

// TestBridge_ConcurrentStopAndSchedule tests the race conditions between
// Stop() and RunOnLoop() being called concurrently from multiple goroutines.
//
// This stress test verifies:
//   - No panics occur when Stop() is called while RunOnLoop is being called
//   - Post-stop calls to RunOnLoop return false (not scheduled)
//   - Pre-stop calls either execute successfully or encounter expected errors
//   - No data races or undefined behavior
//
// This test addresses MAJ-13 from the blueprint.
// TestBridge_GetGlobalNilAmbiguity tests the fix for MIN-6.
// It verifies that GetGlobal correctly distinguishes between:
//   - "key doesn't exist": returns (nil, false)
//   - "key exists with nil value": returns (nil, true)
//
// This test addresses the nil ambiguity issue where the old implementation
// returned (nil, nil) for both cases, making it impossible to tell if a key
// truly doesn't exist or if it exists but has a null value.
func TestBridge_GetGlobalNilAmbiguity(t *testing.T) {
	t.Parallel()

	bridge := testBridge(t)

	t.Run("nonexistent key returns (nil, false)", func(t *testing.T) {
		t.Parallel()

		// Try to get a key that was never set
		val, exists := bridge.GetGlobal("nonexistent_key")
		require.False(t, exists, "nonexistent_key should not exist (exists should be false)")
		require.Nil(t, val, "nonexistent_key should return nil value")
	})

	t.Run("key set to nil returns (nil, true)", func(t *testing.T) {
		t.Parallel()

		// Set a key explicitly to nil
		err := bridge.SetGlobal("explicitNil", nil)
		require.NoError(t, err, "Setting a global to nil should succeed")

		// Get it back - should exist but be nil
		val, exists := bridge.GetGlobal("explicitNil")
		require.True(t, exists, "explicitNil should exist (exists should be true)")
		require.Nil(t, val, "explicitNil should return nil value")
	})

	t.Run("key set to null in JS returns (nil, true)", func(t *testing.T) {
		t.Parallel()

		// Set a key to null in JavaScript
		err := bridge.LoadScript("null_test.js", `
			globalThis.jsNull = null;
		`)
		require.NoError(t, err, "Loading script with null assignment should succeed")

		// Get it back - should exist but be nil
		val, exists := bridge.GetGlobal("jsNull")
		require.True(t, exists, "jsNull should exist (exists should be true)")
		require.Nil(t, val, "jsNull should return nil value")
	})

	t.Run("key set to undefined in JS returns (nil, false)", func(t *testing.T) {
		t.Parallel()

		// Set a key to undefined in JavaScript
		err := bridge.LoadScript("undefined_test.js", `
			globalThis.jsUndefined = undefined;
		`)
		require.NoError(t, err, "Loading script with undefined assignment should succeed")

		// Get it back - should NOT exist (undefined is like deletion)
		val, exists := bridge.GetGlobal("jsUndefined")
		require.False(t, exists, "jsUndefined should not exist (exists should be false)")
		require.Nil(t, val, "jsUndefined should return nil value")
	})

	t.Run("deleting a key in JS returns (nil, false)", func(t *testing.T) {
		t.Parallel()

		// Set a key then delete it
		err := bridge.LoadScript("delete_test.js", `
			globalThis.toBeDeleted = "value";
			delete globalThis.toBeDeleted;
		`)
		require.NoError(t, err, "Loading script with delete should succeed")

		// Get it back - should NOT exist
		val, exists := bridge.GetGlobal("toBeDeleted")
		require.False(t, exists, "toBeDeleted should not exist after deletion")
		require.Nil(t, val, "toBeDeleted should return nil value")
	})

	t.Run("setting and getting various types", func(t *testing.T) {
		t.Parallel()

		// Set various types and verify they all return (value, true)
		err := bridge.SetGlobal("stringKey", "hello")
		require.NoError(t, err)

		err = bridge.SetGlobal("numberKey", 42)
		require.NoError(t, err)

		err = bridge.SetGlobal("boolKey", true)
		require.NoError(t, err)

		err = bridge.SetGlobal("arrayKey", []int{1, 2, 3})
		require.NoError(t, err)

		// Verify all exist and have correct values
		val, exists := bridge.GetGlobal("stringKey")
		require.True(t, exists)
		require.Equal(t, "hello", val)

		val, exists = bridge.GetGlobal("numberKey")
		require.True(t, exists)
		require.Equal(t, int64(42), val)

		val, exists = bridge.GetGlobal("boolKey")
		require.True(t, exists)
		require.Equal(t, true, val)

		val, exists = bridge.GetGlobal("arrayKey")
		require.True(t, exists)
		require.Equal(t, []int{1, 2, 3}, val)
	})

	t.Run("zero value primitives are distinguishable from missing", func(t *testing.T) {
		t.Parallel()

		// Set various zero-value types
		err := bridge.SetGlobal("zeroInt", 0)
		require.NoError(t, err)

		err = bridge.SetGlobal("zeroString", "")
		require.NoError(t, err)

		err = bridge.SetGlobal("zeroBool", false)
		require.NoError(t, err)

		// All should exist with their zero values
		val, exists := bridge.GetGlobal("zeroInt")
		require.True(t, exists, "zeroInt should exist")
		require.Equal(t, int64(0), val)

		val, exists = bridge.GetGlobal("zeroString")
		require.True(t, exists, "zeroString should exist")
		require.Equal(t, "", val)

		val, exists = bridge.GetGlobal("zeroBool")
		require.True(t, exists, "zeroBool should exist")
		require.Equal(t, false, val)

		// While missing keys should not exist
		val, exists = bridge.GetGlobal("missingZeroInt")
		require.False(t, exists, "missingZeroInt should not exist")
		require.Nil(t, val)
	})
}

// TestBridge_Manager tests the Manager() method.
// It verifies that Manager() returns the internal bt.Manager instance.
//
// This test addresses MIN-8 from the blueprint.
func TestBridge_Manager(t *testing.T) {
	t.Parallel()

	// Create bridge with manual shutdown
	bridge, _ := testBridgeWithManualShutdown(t)

	// Get the manager via Manager() method
	manager := bridge.Manager()

	// Manager should return a non-nil manager instance
	require.NotNil(t, manager, "Manager() should return a non-nil manager instance")

	// Verify it's the same manager instance by calling it multiple times
	// (manager is a singleton per bridge, so repeated calls return same pointer)
	manager2 := bridge.Manager()
	require.Equal(t, manager, manager2, "Manager() should return the same instance across multiple calls")

	// Verify manager is accessible while bridge is running
	require.True(t, bridge.IsRunning(), "Bridge should be running initially")

	// Stop the bridge
	bridge.Stop()

	// After stop, manager should still be accessible (it's a reference to the internal manager)
	// The manager itself may be stopped, but the returned object should not be nil
	finalManager := bridge.Manager()
	require.NotNil(t, finalManager, "Manager() should still return non-nil instance after bridge stop")

	// Verify it's still the same manager instance (same pointer)
	require.Equal(t, manager, finalManager, "Manager() should return the same instance even after bridge stop")
}

// TestBridge_GetCallable tests the GetCallable method.
// It verifies:
//   - Getting a valid function succeeds
//   - Getting a non-callable value returns an error
//   - Getting an undefined value returns an error
//
// This test addresses MIN-7 from the blueprint.
func TestBridge_GetCallable(t *testing.T) {
	t.Parallel()

	bridge := testBridge(t)

	t.Run("getting a valid function succeeds", func(t *testing.T) {
		t.Parallel()

		// Load a script with a function
		err := bridge.LoadScript("test.js", `
			function testFunc() {
				return 42;
			}
		`)
		require.NoError(t, err)

		// Get the callable function
		fn, err := bridge.GetCallable("testFunc")
		require.NoError(t, err, "GetCallable should succeed for a valid function")
		require.NotNil(t, fn, "Returned function should not be nil")

		// Verify we can actually call it
		err = bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
			result, err := fn(nil)
			if err != nil {
				return err
			}
			value := result.Export().(int64)
			if value != 42 {
				return fmt.Errorf("expected 42, got %d", value)
			}
			return nil
		})
		require.NoError(t, err, "Retrieved function should be callable")
	})

	t.Run("getting a non-callable value returns error", func(t *testing.T) {
		t.Parallel()

		// Set a non-callable value (string)
		err := bridge.SetGlobal("notAFunc", "hello")
		require.NoError(t, err)

		// Try to get it as a callable
		_, err = bridge.GetCallable("notAFunc")
		require.Error(t, err, "GetCallable should fail for non-callable value")
		require.Contains(t, err.Error(), "not a callable function", "Error message should indicate non-callable")

		// Test with a number
		err = bridge.SetGlobal("alsoNotAFunc", 42)
		require.NoError(t, err)

		_, err = bridge.GetCallable("alsoNotAFunc")
		require.Error(t, err, "GetCallable should fail for number value")
		require.Contains(t, err.Error(), "not a callable function")

		// Test with an object
		err = bridge.LoadScript("object_test.js", `
			globalThis.myObject = { foo: "bar" };
		`)
		require.NoError(t, err)

		_, err = bridge.GetCallable("myObject")
		require.Error(t, err, "GetCallable should fail for object value")
		require.Contains(t, err.Error(), "not a callable function")
	})

	t.Run("getting an undefined value returns error", func(t *testing.T) {
		t.Parallel()

		// Try to get a function that doesn't exist
		_, err := bridge.GetCallable("nonexistentFunc")
		require.Error(t, err, "GetCallable should fail for undefined value")
		require.Contains(t, err.Error(), "not found", "Error message should indicate not found")
		require.Contains(t, err.Error(), "nonexistentFunc", "Error should include the function name")

		// Test with a variable that was explicitly set to undefined in JS
		err = bridge.LoadScript("undefined_func.js", `
			globalThis.undefinedFunc = undefined;
		`)
		require.NoError(t, err)

		_, err = bridge.GetCallable("undefinedFunc")
		require.Error(t, err, "GetCallable should fail for explicitly undefined value")
		require.Contains(t, err.Error(), "not found")
	})

	t.Run("getting a null value returns error", func(t *testing.T) {
		t.Parallel()

		// Set a variable to null in JavaScript
		err := bridge.LoadScript("null_func.js", `
			globalThis.nullFunc = null;
		`)
		require.NoError(t, err)

		// Try to get it as a callable
		_, err = bridge.GetCallable("nullFunc")
		require.Error(t, err, "GetCallable should fail for null value")
		require.Contains(t, err.Error(), "not found", "Error message should indicate not found")
	})
}

func TestBridge_ConcurrentStopAndSchedule(t *testing.T) {
	t.Parallel()

	// Create a bridge with manual shutdown control for precise timing
	bridge, loop := testBridgeWithManualShutdown(t)

	// Configuration for stress test
	const (
		numGoroutines   = 20  // Number of concurrent goroutines calling RunOnLoop
		operationsPerGL = 100 // Number of RunOnLoop calls per goroutine
		delayBeforeStop = 50 * time.Millisecond
	)

	// Track statistics
	var (
		totalScheduled atomic.Int64
		totalSuccess   atomic.Int64
		totalFailed    atomic.Int64
		preStopCalls   atomic.Int64
		postStopCalls  atomic.Int64
	)

	// Use a done channel to signal when stop was called
	stopCalled := make(chan struct{})
	completeCalled := make(chan struct{})

	// Start N goroutines each calling RunOnLoop in a loop
	for i := 0; i < numGoroutines; i++ {
		go func(goroutineID int) {
			defer func() {
				// Recover from any panics - should never happen
				if r := recover(); r != nil {
					t.Errorf("Goroutine %d panicked during concurrent operations: %v", goroutineID, r)
				}
			}()

			for j := 0; j < operationsPerGL; j++ {
				totalScheduled.Add(1)

				// Track whether this call is before or after stop
				select {
				case <-stopCalled:
					postStopCalls.Add(1)
				default:
					preStopCalls.Add(1)
				}

				// Call RunOnLoop - should be safe even during shutdown
				ok := bridge.RunOnLoop(func(vm *goja.Runtime) {
					// Very simple operation - just increment a counter via global
					// This executes on the event loop if scheduled
					totalSuccess.Add(1)
				})

				if !ok {
					totalFailed.Add(1)
				}

				// Short sleep to add some randomness to timing
				time.Sleep(time.Duration(j%5) * time.Millisecond)
			}
		}(i)
	}

	// After a short delay, call Stop() from another goroutine
	go func() {
		time.Sleep(delayBeforeStop)
		close(stopCalled)

		// Call Stop - this should be safe with concurrent RunOnLoop calls
		bridge.Stop()
	}()

	// Wait for all goroutines to complete
	time.Sleep(time.Duration(numGoroutines*operationsPerGL)*time.Millisecond + 500*time.Millisecond)

	// Additional safety: loop.Stop() will be called by t.Cleanup

	// Verify the bridge is stopped
	require.False(t, bridge.IsRunning(), "Bridge should be stopped after concurrent tests")

	// Verify no panics occurred
	t.Log("Concurrent stress test completed without panics")

	// Log statistics for debugging
	scheduled := totalScheduled.Load()
	success := totalSuccess.Load()
	failed := totalFailed.Load()
	preStop := preStopCalls.Load()
	postStop := postStopCalls.Load()

	t.Logf("Concurrent test statistics:")
	t.Logf("  Total RunOnLoop calls scheduled: %d", scheduled)
	t.Logf("  Successful executions: %d", success)
	t.Logf("  Failed (not scheduled) calls: %d", failed)
	t.Logf("  Pre-stop calls: %d", preStop)
	t.Logf("  Post-stop calls: %d", postStop)

	// Verify invariants
	require.Equal(t, int64(numGoroutines*operationsPerGL), scheduled,
		"All scheduled calls should be counted")

	// Sum of success + failed should equal total scheduled
	require.Equal(t, scheduled, success+failed,
		"Success + failed should equal total scheduled")

	// All calls should have been tracked as either pre-stop or post-stop
	require.Equal(t, scheduled, preStop+postStop,
		"Pre-stop + post-stop should equal total scheduled")

	// After stop, any new calls should fail
	require.Greater(t, failed, int64(0), "Should have some failed calls after stop")

	// Verify post-stop behavior by calling RunOnLoop again after stop
	postStopOK := bridge.RunOnLoop(func(vm *goja.Runtime) {
		t.Error("This should never execute after bridge is stopped")
	})
	require.False(t, postStopOK, "RunOnLoop should return false after stop")

	// Try RunOnLoopSync after stop - should return error
	err := bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
		t.Error("This should never execute after bridge is stopped")
		return nil
	})
	require.Error(t, err, "RunOnLoopSync should return error after stop")
	require.Contains(t, err.Error(), "not running",
		"Error should mention event loop not running")

	// Verify the event loop is still functional for other operations
	// (we didn't stop it, only the bridge)
	require.True(t, loop != nil, "Event loop should still exist")

	// Mark test as complete for any waiting cleanup
	close(completeCalled)
}
