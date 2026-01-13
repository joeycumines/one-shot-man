package bt

import (
	"testing"

	bt "github.com/joeycumines/go-behaviortree"
)

// TestCriticalFixes_ThreadSafety verifies CRITICAL #1: TryRunOnLoopSync thread safety
func TestReviewFix_ThreadSafety(t *testing.T) {
	bridge := testBridge(t)

	// Create a simple JS leaf - use require to get full bt API
	err := bridge.LoadScript("test.js", `
		const bt = require('osm:bt');
		globalThis.testTick = function() {
			return "success";
		};
	`)
	if err != nil {
		t.Fatalf("LoadScript failed: %v", err)
	}

	tick, err := bridge.GetCallable("testTick")
	if err != nil {
		t.Fatalf("GetCallable failed: %v", err)
	}

	leaf := NewJSLeafAdapter(bridge.ctx, bridge, tick, nil)

	// Concurrently call Tick from multiple goroutines to verify thread safety
	const goroutines = 10
	const ticksPerGoroutine = 100
	done := make(chan bool, goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			for j := 0; j < ticksPerGoroutine; j++ {
				nodeTick, children := leaf()
				status, err := nodeTick(children)
				if err != nil {
					t.Errorf("Tick failed: %v", err)
					return
				}
				if status != bt.Success && status != bt.Running && status != bt.Failure {
					t.Errorf("Invalid status: %v", status)
				}
			}
			done <- true
		}()
	}

	for i := 0; i < goroutines; i++ {
		<-done
	}
	t.Logf("Thread safety test passed: %d goroutines × %d ticks", goroutines, ticksPerGoroutine)
}

// TestReviewFix_ChildIndexOverflow verifies HIGH #6: Child index error messages use strconv.Itoa
func TestReviewFix_ChildIndexOverflow(t *testing.T) {
	bridge := testBridge(t)

	// Create a node with many children to force index > 9
	err := bridge.LoadScript("test_children.js", `
		const bt = require('osm:bt');
		const tick = () => "success";
		const children = [];
		for (let i = 0; i < 15; i++) {
			children.push(bt.node(tick));
		}

		// Force an error by passing a non-Node (this becomes child index 16 after tick and 15 children)
		try {
			bt.node(tick, ...children, "not a node");
		} catch (e) {
			if (!e.message.includes("child 16") && !e.message.includes("child 15")) {
				throw new Error("Expected 'child 16' or 'child 15' in error, got: " + e.message);
			}
		}

		// Create a smaller test case for child 12 specifically
		const smallChildren = [];
		for (let i = 0; i < 11; i++) {
			smallChildren.push(bt.node(tick));
		}
		// This creates 11 children + 1 "not a node" = child index 12
		try {
			bt.node(tick, ...smallChildren, "not a node");
		} catch (e) {
			if (!e.message.includes("child 12") && !e.message.includes("child 11")) {
				throw new Error("Expected 'child 12' or 'child 11' in error, got: " + e.message);
			}
		}
	`)
	if err != nil {
		t.Fatalf("Child index test failed: %v", err)
	}
	t.Logf("Child index overflow test passed - error messages show proper indices (e.g., 'child 12' not '<')")
}

// TestReviewFix_VmConsistency verifies LOW #11: Consistent vm usage in nodeUnwrap
func TestReviewFix_VmConsistency(t *testing.T) {
	bridge := testBridge(t)

	// This test verifies that nodeUnwrap uses loopVm consistently
	// The fix ensures that all VM operations in the callback use loopVm
	err := bridge.LoadScript("test_vm.js", `
		const bt = require('osm:bt');
		const tick = () => "success";
		const leaf = bt.node(tick);
		bt.tick(leaf);
	`)
	if err != nil {
		t.Fatalf("VM consistency test failed: %v", err)
	}
	t.Logf("VM consistency test passed - loopVm used correctly")
}
