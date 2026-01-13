/*
Package bt provides integration between bt.js (JavaScript behavior trees)
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

CRITICAL CONSTRAINT: All composite nodes MUST be implemented in Go (go-behaviortree).
JavaScript nodes are ONLY for leaves. This prevents cyclic references and memory leaks.
The nodeUnwrap and tickUnwrap functions convert JS functions to bt.Node/bt.Tick ASAP,
never holding goja references longer than necessary.

# Tick Return Type Semantics

Tick functions may return either Status or Promise<Status>:

	type Tick = (children: Node[]) => Status | Promise<Status>

SYNCHRONOUS OPTIMIZATION: This runtime does NOT provide a JavaScript microtask queue.
Returning a Promise defers resolution to the macrotask queue, adding latency. Tick
implementations SHOULD return Status directly whenever the result is immediately
available, and only return Promise<Status> when genuinely async work is required.

COMPOSITE TICK BEHAVIOR: The Go-backed composite functions (sequence, fallback,
etc.) return Status synchronously. They dynamically detect when children
return Status directly (vs Promise<Status>) and propagate the immediate result,
avoiding unnecessary Promise wrapping and macrotask queue deferrals.

# Stateless Composite Design

Composite tick implementations MUST support stateless, index-based child access.
This is critical for proper lifecycle management and avoiding memory leaks:

	// CORRECT: Stateless composite - operates on children by index
	const mySequence = (children) => {
		for (let i = 0; i < children.length; i++) {
			const status = bt.tick(children[i]);
			if (status !== bt.success) return status;
		}
		return bt.success;
	};

	// WRONG: Captures direct node references - creates memory leaks
	const child1 = bt.createLeafNode(async () => bt.success);
	const child2 = bt.createLeafNode(async () => bt.success);
	const badComposite = () => {
		// Directly referencing child1, child2 - BAD!
		return bt.tick(child1) === bt.success ? bt.tick(child2) : bt.failure;
	};

# Core Components

Bridge: Manages the goja runtime and event loop. All JavaScript operations must
happen within RunOnLoop callbacks to maintain thread safety. The Done() channel
can be used to detect bridge shutdown. Use GetCallable() to retrieve JS functions
for use with the adapters.

Blackboard: A thread-safe key-value store for behavior tree state. Create with
new(Blackboard) - the internal map is lazily initialized on first write.
Exposes accessor methods to JavaScript for reading and writing state.

JSLeafAdapter: Bridges JavaScript async leaf functions to go-behaviortree Nodes.
Implements a state machine to handle Promise-based JS execution within the
synchronous go-behaviortree tick interface. Uses generation counting to prevent
stale callbacks from corrupting state after cancellation.

BlockingJSLeaf: A simpler blocking variant that waits for the JS Promise to resolve.
Suitable for prototyping or when interleaving isn't needed. Supports context
cancellation.

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

NewJSLeafAdapter and BlockingJSLeaf both accept a context.Context as their first
parameter for cancellation support. Once cancelled, that specific adapter instance
cannot be reused (one-shot semantics). Create a new adapter for retries after
cancellation.

# Error Handling

JS errors (thrown exceptions) are mapped to Failure status with the error
message. Panics in JS callbacks are recovered and reported as Failure.

# API Surface (osm:bt module)

The osm:bt module matches the bt.d.ts API with camelCase naming:

  - Status constants: running, success, failure
  - node(tick, ...children) - Create a bt.Node
  - tick(node) - Tick a node and return status (Status | Promise<Status>)
  - sequence(children) - Tick children in sequence until failure (returns Status)
  - fallback(children) - Tick children until success (returns Status)
  - memorize(tick) - Cache non-running status per execution
  - async(tick) - Wrap tick to run asynchronously
  - not(tick) - Invert tick result
  - fork() - Run all children in parallel
  - createLeafNode(fn) - Create a leaf node from a JS function
  - createBlockingLeafNode(fn) - Create a blocking leaf from a JS function
  - Blackboard - Constructor for a new blackboard

All composite tick implementations (sequence, fallback, parallel, etc.) return
Status synchronously when children return Status directly. They only return
Promise<Status> when a child tick returns a Promise.

# bt.js Compatibility

This package implements Go-side primitives compatible with bt.js-style leaves
(async functions returning string status). The embedded bt.js source (accessible
via GetBTJS()) is provided for reference. Users who need bt.js composites should
use the gist source directly or bundle it for their use case.
*/
package bt
