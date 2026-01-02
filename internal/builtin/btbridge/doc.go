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

Note: This is Variant C.2 (Go-centric with JS leaves) as the baseline. Full interoperability
with bt.js composites (status translation, module compatibility, mixed Go/JS composites)
requires additional implementation work documented in the architecture specification.

# Core Components

Bridge: Manages the goja runtime and event loop. All JavaScript operations must
happen within RunOnLoop callbacks to maintain thread safety. The Done() channel
can be used to detect bridge shutdown.

Blackboard: A thread-safe key-value store for behavior tree state. Exposes accessor
methods to JavaScript for reading and writing state.

JSLeafAdapter: Bridges JavaScript async leaf functions to go-behaviortree Nodes.
Implements a state machine to handle Promise-based JS execution within the
synchronous go-behaviortree tick interface. Uses generation counting to prevent
stale callbacks from corrupting state after cancellation.

BlockingJSLeaf: A simpler blocking variant that waits for the JS Promise to resolve.
Suitable for prototyping or when interleaving isn't needed. Supports context
cancellation via BlockingJSLeafWithContext.

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
Uses generation counting to safely handle cancellation.

BlockingJSLeaf (blocking): Blocks until the Promise resolves. Simpler but
prevents interleaving of other nodes during the same tick. Uses sync.Once
to prevent double-send bugs and bridge.Done() to prevent deadlocks.

# Thread Safety

The Bridge ensures all JavaScript operations happen on the event loop goroutine.
The Blackboard uses sync.RWMutex for safe concurrent access from Go code.
JSLeafAdapter is safe for concurrent Tick() calls due to atomic state transitions
and generation-based stale callback protection.

# Context and Cancellation

NewJSLeafAdapterWithContext creates an adapter with cancellation support. Once
cancelled, that specific adapter instance cannot be reused (one-shot semantics).
Create a new adapter for retries after cancellation.

BlockingJSLeafWithContext supports context cancellation while waiting for the
JS Promise to resolve.

# Error Handling

JS errors (thrown exceptions) are mapped to Failure status with the error
message. Panics in JS callbacks are recovered and reported as Failure.

# bt.js Compatibility

This package implements Go-side primitives compatible with bt.js-style leaves
(async functions returning string status). The embedded bt.js source (accessible
via GetBTJS()) is provided for reference. Users who need bt.js composites should
use the gist source directly or bundle it for their use case.
*/
package btbridge
