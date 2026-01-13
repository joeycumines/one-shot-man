package bt

import (
	"errors"
	"fmt"

	"github.com/dop251/goja"
	bt "github.com/joeycumines/go-behaviortree"
)

// nodeUnwrap extracts a bt.Node from a goja.Value.
//
// It handles two cases:
//  1. Wrapped native Go value: val.Export() returns bt.Node directly
//  2. JavaScript function: A function matching the bt.d.ts Node signature
//     Node = () => [Tick, Node[]]
//
// For JS functions, this creates a Go wrapper that executes the JS function
// on the event loop. Note: JS-defined nodes should only be leaves (no children)
// per the Go-Centric architecture constraint.
//
// The vm parameter is required for JS function handling. If the value is already
// a native bt.Node, vm can be nil.
//
// Thread Safety: The returned bt.Node is safe to use from any goroutine.
// JS execution happens on the event loop via Bridge.RunOnLoop.
func nodeUnwrap(bridge *Bridge, vm *goja.Runtime, val goja.Value) (bt.Node, error) {
	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		return nil, errors.New("nodeUnwrap: value is nil or undefined")
	}

	// Case 1: Try to export as native bt.Node
	exported := val.Export()
	if node, ok := exported.(bt.Node); ok {
		return node, nil
	}

	// Case 2: Try as a Go function that matches bt.Node signature
	if fn, ok := exported.(func() (bt.Tick, []bt.Node)); ok {
		return fn, nil
	}

	// Case 3: JavaScript function - wrap it
	jsFn, ok := goja.AssertFunction(val)
	if !ok {
		return nil, fmt.Errorf("nodeUnwrap: value is not a Node (got %T)", exported)
	}

	// Create a Go bt.Node that calls the JS function
	// Per bt.d.ts: Node = () => [Tick, Node[]]

	// CRITICAL: vm is required for JS function unwrapping
	if vm == nil {
		return nil, errors.New("vm is required for JS node function unwrapping")
	}

	return func() (bt.Tick, []bt.Node) {
		var tick bt.Tick
		var children []bt.Node
		var jsErr error

		// Execute the JS function on the event loop synchronously
		// Use TryRunOnLoopSync to avoid deadlock when called from within event loop
		err := bridge.TryRunOnLoopSync(vm, func(loopVm *goja.Runtime) error {
			result, err := jsFn(goja.Undefined())
			if err != nil {
				return fmt.Errorf("JS node function error: %w", err)
			}

			// Result should be an array [tick, children]
			resultObj := result.ToObject(loopVm) // LOW #11 FIX: Use loopVm
			if resultObj == nil {
				return errors.New("JS node function must return [tick, children] array")
			}

			// Get tick (index 0)
			tickVal := resultObj.Get("0")
			if tickVal == nil || goja.IsUndefined(tickVal) {
				return errors.New("JS node function must return tick as first element")
			}

			tick, jsErr = tickUnwrap(bridge, loopVm, tickVal) // LOW #11 FIX: Use loopVm
			if jsErr != nil {
				return fmt.Errorf("failed to unwrap tick: %w", jsErr)
			}

			// Get children (index 1) - may be undefined/null for leaves
			childrenVal := resultObj.Get("1")
			if childrenVal != nil && !goja.IsUndefined(childrenVal) && !goja.IsNull(childrenVal) {
				childrenObj := childrenVal.ToObject(loopVm) // LOW #11 FIX: Use loopVm
				if childrenObj != nil {
					length := childrenObj.Get("length")
					if length != nil && !goja.IsUndefined(length) {
						n := int(length.ToInteger())
						children = make([]bt.Node, 0, n)
						for i := 0; i < n; i++ {
							childVal := childrenObj.Get(fmt.Sprintf("%d", i))
							child, err := nodeUnwrap(bridge, loopVm, childVal) // LOW #11 FIX: Use loopVm
							if err != nil {
								return fmt.Errorf("failed to unwrap child %d: %w", i, err)
							}
							children = append(children, child)
						}
					}
				}
			}

			return nil
		})

		if err != nil {
			// Return a tick that fails with the error
			return func([]bt.Node) (bt.Status, error) {
				return bt.Failure, err
			}, nil
		}

		return tick, children
	}, nil
}

// tickUnwrap extracts a bt.Tick from a goja.Value.
//
// It handles two cases:
//  1. Wrapped native Go value: val.Export() returns bt.Tick directly
//  2. JavaScript function: A function matching the bt.d.ts Tick signature
//     Tick = (children: Node[]) => Status | Promise<Status>
//
// RETURN TYPE SEMANTICS:
// Tick functions may return either Status directly (synchronous) or
// Promise<Status> (asynchronous). This implementation:
//   - Detects synchronous Status returns and propagates them immediately
//   - Rejects async functions (which always return Promise) to prevent
//     infinite Promise loops - use bt.createLeafNode() for async behavior
//
// SYNCHRONOUS OPTIMIZATION:
// This runtime does NOT provide a microtask queue. Returning a Promise
// defers resolution to the macrotask queue. Tick implementations SHOULD
// return Status directly when the result is immediately available.
//
// For JS functions, this creates a Go wrapper that executes the JS function
// on the event loop and blocks until the Promise resolves.
//
// Thread Safety: The returned bt.Tick is safe to use from any goroutine.
// JS execution happens on the event loop via Bridge.RunOnLoop.
func tickUnwrap(bridge *Bridge, vm *goja.Runtime, val goja.Value) (bt.Tick, error) {
	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		return nil, errors.New("tickUnwrap: value is nil or undefined")
	}

	// Case 1: Try to export as native bt.Tick
	exported := val.Export()
	if tick, ok := exported.(bt.Tick); ok {
		return tick, nil
	}

	// Case 2: Try as a Go function that matches bt.Tick signature
	if fn, ok := exported.(func([]bt.Node) (bt.Status, error)); ok {
		return fn, nil
	}

	// Case 3: JavaScript function - wrap it
	jsFn, ok := goja.AssertFunction(val)
	if !ok {
		return nil, fmt.Errorf("tickUnwrap: value is not a Tick (got %T)", exported)
	}

	// Create a Go bt.Tick that calls the JS function
	// Per bt.d.ts: Tick = (children: Node[]) => Promise<Status>
	// CRITICAL: This Tick may be called from Go Ticker goroutine, which is concurrent
	// with the JS event loop. To avoid data races, all JS access MUST go through
	// bridge.RunOnLoopSync to ensure it executes on the event loop thread.
	return func(children []bt.Node) (bt.Status, error) {
		// Synchronous path: try calling the JS function directly on the event loop
		// MUST use RunOnLoopSync to avoid data race - accessing vm* from concurrent goroutine
		var syncResult bt.Status = bt.Running // Default to Running for async safety
		var syncErr error

		// CRITICAL: vm is required for JS function unwrapping
		if vm == nil {
			return bt.Failure, errors.New("vm is required for JS function unwrapping")
		}

		// All JS execution must happen on the event loop via RunOnLoopSync
		// Use TryRunOnLoopSync to avoid deadlock when called from within event loop
		err := bridge.TryRunOnLoopSync(vm, func(loopVm *goja.Runtime) error {
			defer func() {
				if r := recover(); r != nil {
					syncErr = fmt.Errorf("panic in JS tick: %v", r)
				}
			}()

			// Convert Go children to JS array
			jsChildren := loopVm.NewArray()
			for i, child := range children {
				if err := jsChildren.Set(fmt.Sprintf("%d", i), loopVm.ToValue(child)); err != nil {
					syncErr = fmt.Errorf("failed to set child %d: %w", i, err)
					return syncErr
				}
			}

			// Call the JS tick function
			retVal, err := jsFn(goja.Undefined(), jsChildren)
			if err != nil {
				syncErr = fmt.Errorf("JS tick error: %w", err)
				return syncErr
			}

			// Check if it's a Promise vs direct status string
			// Use Export to check - Promises export as objects
			exported := retVal.Export()

			// Check if it's a Promise-like object
			// A Promise is an object, string is not
			if _, ok := exported.(map[string]any); ok {
				// It's an object - check if it has a callable 'then' property (Promise signature)
				obj := retVal.ToObject(loopVm)
				if thenProp := obj.Get("then"); thenProp != nil && !goja.IsUndefined(thenProp) {
					// HIGH #1 FIX: Verify 'then' is actually callable (a function)
					// This prevents false positives from objects like {then: "value"}
					if _, callable := goja.AssertFunction(thenProp); callable {
						// CRITICAL FIX: Reject async functions to prevent infinite Promise loop
						// Async functions return a Promise that would be discarded, creating
						// a memory leak and infinite Running state. User must use bt.createLeafNode()
						// for async behavior, not raw bt.node() with async functions.
						syncErr = fmt.Errorf("async functions cannot be used as raw Ticks (infinite loop risk). " +
							"Use bt.createLeafNode() for async behavior, not bt.node()")
						return syncErr
					}
				}
				// Not a Promise, but some other object - treat as error
				syncErr = fmt.Errorf("tick function returned non-Promise object - use bt.createLeafNode() for async or return string status directly")
				return syncErr
			} else if exported == nil {
				// null or undefined from async function - treat as async error
				syncErr = fmt.Errorf("tick function returned null/undefined - must return 'success', 'failure', or 'running' string, or use bt.createLeafNode() for async")
				return syncErr
			} else {
				// Direct value (string, number, etc.)
				statusStr := retVal.String()
				syncResult = mapJSStatus(statusStr)
			}
			return nil
		})

		if err != nil {
			// TryRunOnLoopSync failed (bridge stopped, etc.)
			return bt.Failure, err
		}

		if syncErr != nil {
			// Sync execution failed (e.g., JS threw an error or invalid return type)
			return bt.Failure, syncErr
		}

		// CRITICAL FIX: No async path - we reject async functions explicitly above
		// This prevents the infinite Promise loop bug where Promises were discarded

		// Return the sync result (must be Success, Failure, or Running)
		return syncResult, nil
	}, nil
}
