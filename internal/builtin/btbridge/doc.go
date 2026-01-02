/*
Package btbridge provides integration between bt.js (JavaScript behavior trees)
and go-behaviortree using the goja JavaScript runtime.

# Architecture

This package implements the Go-Centric architecture (Variant C.2 - Event-Driven Bridge)
from the bt.js integration specification:

  - go-behaviortree is the canonical BT engine
  - JavaScript is used for leaf behaviors via async bridge on goja_nodejs/eventloop
  - Go-owned Blackboard provides thread-safe state management
  - Event-driven bridge for optimal performance

# Core Components

Bridge: Manages the goja runtime and event loop. All JavaScript operations must
happen within RunOnLoop callbacks to maintain thread safety.

Blackboard: A thread-safe key-value store for behavior tree state. Exposes accessor
methods to JavaScript for reading and writing state.

JSLeafAdapter: Bridges JavaScript async leaf functions to go-behaviortree Nodes.
Implements a state machine to handle Promise-based JS execution within the
synchronous go-behaviortree tick interface.

BlockingJSLeaf: A simpler blocking variant that waits for the JS Promise to resolve.
Suitable for prototyping or when interleaving isn't needed.

# Usage

	// Create a bridge with event loop
	bridge, err := btbridge.NewBridge()
	if err != nil {
		return err
	}
	defer bridge.Stop()

	// Create and expose a blackboard for state
	bb := btbridge.NewBlackboard()
	bb.Set("targetX", 100.0)
	bb.Set("targetY", 200.0)
	bridge.ExposeBlackboard("ctx", bb)

	// Load JavaScript leaf behaviors
	bridge.LoadScript("leaves.js", `
		async function moveTo() {
			const targetX = ctx.get("targetX");
			const targetY = ctx.get("targetY");
			ctx.set("posX", targetX);
			ctx.set("posY", targetY);
			return bt.success;
		}

		async function hasTarget() {
			return ctx.has("targetX") && ctx.has("targetY") ? bt.success : bt.failure;
		}
	`)

	// Build behavior tree using go-behaviortree with JS leaves
	tree := bt.New(
		bt.Sequence,
		btbridge.BlockingJSLeaf(bridge, "hasTarget", nil),
		btbridge.BlockingJSLeaf(bridge, "moveTo", nil),
	)

	// Tick the tree
	status, err := tree.Tick()

# Non-Blocking vs Blocking Leaves

JSLeafAdapter (non-blocking): Returns Running immediately, checks for Promise
completion on subsequent ticks. Best for production with concurrent trees.

BlockingJSLeaf (blocking): Blocks until the Promise resolves. Simpler but
prevents interleaving of other nodes during the same tick.

# Thread Safety

The Bridge ensures all JavaScript operations happen on the event loop goroutine.
The Blackboard uses sync.RWMutex for safe concurrent access from Go code.

# Error Handling

JS errors (thrown exceptions) are mapped to Failure status with the error
message. Panics in JS callbacks are recovered and reported as Failure.

# Performance

Tree traversal happens in compiled Go code. Only leaf behaviors execute
JavaScript. The event-driven bridge adds minimal overhead (~50-200µs per
leaf execution).
*/
package btbridge
