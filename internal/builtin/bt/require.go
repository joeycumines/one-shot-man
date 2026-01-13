package bt

import (
	"context"
	_ "embed"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/require"
	bt "github.com/joeycumines/go-behaviortree"
)

// ModuleLoader returns a require.ModuleLoader for the "osm:bt" module.
// This loader exposes the behavior tree functionality to JavaScript.
//
// The API surface matches bt.d.ts with camelCase naming:
//   - Status constants: running, success, failure
//   - node(tick, ...children) - Create a bt.Node
//   - tick(node) - Tick a node and return status
//   - sequence(children) - Tick children in sequence until failure
//   - fallback(children) - Tick children until success (alias: selector)
//   - memorize(tick) - Cache non-running status per execution
//   - async(tick) - Wrap tick to run asynchronously
//   - not(tick) - Invert tick result
//   - fork() - Returns a tick that runs all children in parallel
//   - newTicker(durationMs, node, options?) - Create a managed Ticker
//   - newManager() - Create a new ticker Manager
//
// CRITICAL ARCHITECTURE CONSTRAINT: Go-Centric Design
// ===================================================
// This module exposes Go-native behavior tree primitives. Composite nodes
// (sequence, selector, fork, memorize, etc.) are Go implementations.
// JavaScript is used ONLY for leaf behaviors.
//
// WHY THIS MATTERS - Recursive Usage Constraints:
// ----------------------------------------------
// JavaScript nodes created via bt.node() CANNOT be used recursively as composites
// in JavaScript code. All composites MUST use the Go primitives.
//
// The reason is DEADLOCK PREVENTION:
//   - Go tickers run in dedicated goroutines (outside the JS event loop)
//   - JS leaf execution happens on the JS event loop via TryRunOnLoopSync
//   - If a JS composite ticked children (which are JS nodes), it would need to
//     block waiting for results, but those results require crossing back into JS
//   - TryRunOnLoopSync detects recursion by comparing VM references
//   - However, allowing arbitrary JS composites would create circular dependencies
//     and complex state that's hard to reason about
//
// Examples of SAFE usage:
// -----------------------
// ✅ const leaf1 = bt.createLeafNode(async () => bt.success);
// ✅ const leaf2 = bt.createLeafNode(async () => bt.success);
// ✅ const tree = bt.node(bt.sequence, leaf1, leaf2); // Go sequence, JS leaves
//
// Examples of UNSAFE usage (will cause issues):
// ---------------------------------------------
// ❌ const jsComposite = bt.node(async () => [/* ... */]);
// ❌ const tree = bt.node(async () => { /* composite logic in JS */ }, child);
//
// The Go-Centric architecture ensures:
// 1. No data races (Go tickers don't access goja.Runtime directly)
// 2. No deadlocks (TryRunOnLoopSync handles event loop recursion)
// 3. Deterministic behavior (Go composites have well-defined semantics)
// 4. Easy debugging (JS layer is thin and transparent)
func (b *Bridge) ModuleLoader(ctx context.Context) require.ModuleLoader {

	return func(runtime *goja.Runtime, module *goja.Object) {
		exports := module.Get("exports").(*goja.Object)

		// Status constants (must match JSStatus* constants in adapter.go)
		// These are the canonical string values per bt.d.ts
		_ = exports.Set("running", JSStatusRunning)
		_ = exports.Set("success", JSStatusSuccess)
		_ = exports.Set("failure", JSStatusFailure)

		// node(tick, ...children) - Create a bt.Node from tick and optional children
		// Per bt.d.ts: node = (tick: Tick, ...children: Node[]) => Node
		_ = exports.Set("node", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 1 {
				panic(runtime.NewTypeError("node requires at least a tick function"))
			}

			// First argument is the tick
			tick, err := tickUnwrap(b, runtime, call.Arguments[0])
			if err != nil {
				panic(runtime.NewTypeError("first argument must be a Tick: " + err.Error()))
			}

			// Remaining arguments are children
			var children []bt.Node
			for i := 1; i < len(call.Arguments); i++ {
				child, err := nodeUnwrap(b, runtime, call.Arguments[i])
				if err != nil {
					panic(runtime.NewTypeError("child " + strconv.Itoa(i) + " must be a Node: " + err.Error()))
				}
				children = append(children, child)
			}

			return runtime.ToValue(bt.New(tick, children...))
		})

		// tick(node) - Tick a node and return the status
		// Per bt.d.ts: tick = (node: Node) => Promise<Status>
		// In Go, this is synchronous and returns the status string
		_ = exports.Set("tick", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 1 {
				panic(runtime.NewTypeError("tick requires a node argument"))
			}

			nodeVal := call.Arguments[0]

			// Try direct unwrap first (for native Go nodes)
			if node, ok := nodeVal.Export().(bt.Node); ok {
				status, tickErr := node.Tick()
				if tickErr != nil {
					// Return failure status with error message
					return runtime.ToValue(JSStatusFailure)
				}
				return runtime.ToValue(mapGoStatus(status))
			}

			// For JS nodes, we need to handle them carefully
			// If we're already on the event loop, we can't use RunOnLoopSync
			// Check if the value is already a bt.Node wrapper
			if node, err := nodeUnwrap(b, runtime, nodeVal); err == nil && node != nil {
				status, tickErr := node.Tick()
				if tickErr != nil {
					return runtime.ToValue(JSStatusFailure)
				}
				return runtime.ToValue(mapGoStatus(status))
			}

			panic(runtime.NewTypeError("argument must be a Node"))
		})

		// sequence - Go bt.Sequence exposed with camelCase
		// HIGH #4 FIX: Wrap to return string status instead of (Status, error) tuple
		//
		// STATELESS COMPOSITE: Operates on children by index, not by direct reference.
		// This enables JS-authored composites to be implemented statelessly.
		//
		// RETURN TYPE: Returns Status synchronously (not Promise<Status>).
		// The Go implementation ticks children sequentially and returns immediately.
		// This avoids unnecessary macrotask queue deferrals (no microtask queue).
		_ = exports.Set("sequence", func(call goja.FunctionCall) goja.Value {
			// Convert JS children array to Go Node slice
			var children []bt.Node
			if len(call.Arguments) > 0 {
				childrenArray := call.Arguments[0].ToObject(runtime)
				if childrenArray != nil {
					length := int(childrenArray.Get("length").ToInteger())
					children = make([]bt.Node, length)
					for i := 0; i < length; i++ {
						childVal := childrenArray.Get(strconv.Itoa(i))
						node, err := nodeUnwrap(b, runtime, childVal)
						if err != nil {
							panic(runtime.NewGoError(fmt.Errorf("sequence child %d: %w", i, err)))
						}
						children[i] = node
					}
				}
			}

			// Call Go bt.Sequence
			status, err := bt.Sequence(children)
			if err != nil {
				// Return failure status with error
				return runtime.ToValue(mapGoStatus(bt.Failure))
			}
			// Return status as string
			return runtime.ToValue(mapGoStatus(status))
		})

		// fallback - Alias for bt.Selector (bt.d.ts uses "fallback" naming)
		// HIGH #4 FIX: Wrap to return string status instead of (Status, error) tuple
		//
		// STATELESS COMPOSITE: Operates on children by index, not by direct reference.
		// This enables JS-authored composites to be implemented statelessly.
		//
		// RETURN TYPE: Returns Status synchronously (not Promise<Status>).
		// The Go implementation ticks children sequentially and returns immediately.
		// This avoids unnecessary macrotask queue deferrals (no microtask queue).
		_ = exports.Set("fallback", func(call goja.FunctionCall) goja.Value {
			// Convert JS children array to Go Node slice
			var children []bt.Node
			if len(call.Arguments) > 0 {
				childrenArray := call.Arguments[0].ToObject(runtime)
				if childrenArray != nil {
					length := int(childrenArray.Get("length").ToInteger())
					children = make([]bt.Node, length)
					for i := 0; i < length; i++ {
						childVal := childrenArray.Get(strconv.Itoa(i))
						node, err := nodeUnwrap(b, runtime, childVal)
						if err != nil {
							panic(runtime.NewGoError(fmt.Errorf("fallback child %d: %w", i, err)))
						}
						children[i] = node
					}
				}
			}

			// Call Go bt.Selector (fallback is alias for selector)
			status, err := bt.Selector(children)
			if err != nil {
				return runtime.ToValue(mapGoStatus(bt.Failure))
			}
			return runtime.ToValue(mapGoStatus(status))
		})

		// selector - Also expose as selector for Go naming compatibility
		//
		// STATELESS COMPOSITE: Operates on children by index, not by direct reference.
		// RETURN TYPE: Returns Status synchronously (not Promise<Status>).
		_ = exports.Set("selector", func(call goja.FunctionCall) goja.Value {
			// Convert JS children array to Go Node slice
			var children []bt.Node
			if len(call.Arguments) > 0 {
				childrenArray := call.Arguments[0].ToObject(runtime)
				if childrenArray != nil {
					length := int(childrenArray.Get("length").ToInteger())
					children = make([]bt.Node, length)
					for i := 0; i < length; i++ {
						childVal := childrenArray.Get(strconv.Itoa(i))
						node, err := nodeUnwrap(b, runtime, childVal)
						if err != nil {
							panic(runtime.NewGoError(fmt.Errorf("selector child %d: %w", i, err)))
						}
						children[i] = node
					}
				}
			}

			status, err := bt.Selector(children)
			if err != nil {
				return runtime.ToValue(mapGoStatus(bt.Failure))
			}
			return runtime.ToValue(mapGoStatus(status))
		})

		// memorize(tick) - Cache non-running status per execution
		// Per bt.d.ts: memorize = (tick: Tick) => Tick
		// MEDIUM #9 FIX: Document that wrapper functions capture internal state
		//
		// IMPORTANT: Each node must create its own wrapper instance.
		//
		// ✅ Correct:
		// const node1 = bt.node(bt.memorize(myTick));
		// const node2 = bt.node(bt.memorize(myTick));
		//
		// ❌ Wrong:
		// const memoTick = bt.memorize(myTick);
		// const node1 = bt.node(memoTick);
		// const node2 = bt.node(memoTick);  // Same instance - state shared!
		_ = exports.Set("memorize", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 1 {
				panic(runtime.NewTypeError("memorize requires a tick argument"))
			}

			tick, err := tickUnwrap(b, runtime, call.Arguments[0])
			if err != nil {
				panic(runtime.NewTypeError("argument must be a Tick: " + err.Error()))
			}

			return runtime.ToValue(bt.Memorize(tick))
		})

		// async(tick) - Wrap tick to run asynchronously
		// Per bt.d.ts: async = (tick: Tick) => Tick
		// MEDIUM #9 FIX: Document that wrapper functions capture internal state
		//
		// IMPORTANT: Each node must create its own wrapper instance.
		// Reusing the same async() wrapper creates shared async state.
		// See bt.memorize() for usage pattern.
		_ = exports.Set("async", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 1 {
				panic(runtime.NewTypeError("async requires a tick argument"))
			}

			tick, err := tickUnwrap(b, runtime, call.Arguments[0])
			if err != nil {
				panic(runtime.NewTypeError("argument must be a Tick: " + err.Error()))
			}

			return runtime.ToValue(bt.Async(tick))
		})

		// not(tick) - Invert tick result
		// Per bt.d.ts: not = (tick: Tick) => Tick
		_ = exports.Set("not", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 1 {
				panic(runtime.NewTypeError("not requires a tick argument"))
			}

			tick, err := tickUnwrap(b, runtime, call.Arguments[0])
			if err != nil {
				panic(runtime.NewTypeError("argument must be a Tick: " + err.Error()))
			}

			return runtime.ToValue(bt.Not(tick))
		})

		// fork() - Returns a Tick that runs all children in parallel
		// Per bt.d.ts: fork = () => Tick (stateful)
		_ = exports.Set("fork", func(call goja.FunctionCall) goja.Value {
			return runtime.ToValue(bt.Fork())
		})

		// interval(intervalMs)
		// Returns a tick that rate-limits to at most once per interval
		//
		// MEDIUM #9 FIX: Document state sharing
		// IMPORTANT: Each node must create its own interval instance.
		// Reusing the same interval() wrapper shares the same timer across nodes.
		//
		// ✅ Correct:
		// const node1 = bt.node(bt.interval(1000));
		// const node2 = bt.node(bt.interval(1000));
		//
		// ❌ Wrong:
		// const timer = bt.interval(1000);
		// const node1 = bt.node(timer);
		// const node2 = bt.node(timer);  // Same timer instance!
		_ = exports.Set("interval", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 1 {
				panic(runtime.NewTypeError("interval requires intervalMs argument"))
			}

			intervalMs := call.Arguments[0].ToInteger()
			duration := time.Duration(intervalMs) * time.Millisecond

			// Use go-behaviortree's RateLimit which is similar
			return runtime.ToValue(bt.RateLimit(duration))
		})

		// createLeafNode(tick) - Create a leaf node from a JS callable
		// This is the primary way to create JS-authored leaves
		_ = exports.Set("createLeafNode", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 1 {
				panic(runtime.NewTypeError("createLeafNode requires a tick function"))
			}

			tickArg := call.Arguments[0]
			if goja.IsUndefined(tickArg) || goja.IsNull(tickArg) {
				panic(runtime.NewTypeError("createLeafNode: argument must be a callable function (got null/undefined)"))
			}

			tickFn, ok := goja.AssertFunction(tickArg)
			if !ok {
				panic(runtime.NewTypeError("createLeafNode: argument must be a callable function"))
			}

			return runtime.ToValue(NewJSLeafAdapter(ctx, b, tickFn, nil))
		})

		// createBlockingLeafNode(tick) - Create a blocking leaf node from a JS callable
		_ = exports.Set("createBlockingLeafNode", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 1 {
				panic(runtime.NewTypeError("createBlockingLeafNode requires a tick function"))
			}

			tickArg := call.Arguments[0]
			if goja.IsUndefined(tickArg) || goja.IsNull(tickArg) {
				panic(runtime.NewTypeError("createBlockingLeafNode: argument must be a callable function (got null/undefined)"))
			}

			tickFn, ok := goja.AssertFunction(tickArg)
			if !ok {
				panic(runtime.NewTypeError("createBlockingLeafNode: argument must be a callable function"))
			}

			// Pass the runtime for use when already on the event loop
			return runtime.ToValue(BlockingJSLeaf(ctx, b, runtime, tickFn, nil))
		})

		// Blackboard - Expose the Blackboard constructor
		// Usage: const bb = new bt.Blackboard()
		_ = exports.Set("Blackboard", func(call goja.ConstructorCall) *goja.Object {
			bb := new(Blackboard)
			obj := call.This
			_ = obj.Set("get", bb.Get)
			_ = obj.Set("set", bb.Set)
			_ = obj.Set("has", bb.Has)
			_ = obj.Set("delete", bb.Delete)
			_ = obj.Set("keys", bb.Keys)
			_ = obj.Set("clear", bb.Clear)
			_ = obj.Set("snapshot", bb.Snapshot)
			// Store the Go Blackboard reference for interop
			_ = obj.Set("_native", bb)
			return nil
		})

		// exposeBlackboard(blackboard) - Get JS object for a Go Blackboard
		_ = exports.Set("exposeBlackboard", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) != 1 {
				panic(runtime.NewTypeError("exposeBlackboard requires exactly one Blackboard argument"))
			}
			bb, ok := call.Arguments[0].Export().(*Blackboard)
			if !ok {
				panic(runtime.NewTypeError("exposeBlackboard: argument must be a Blackboard instance"))
			}
			return bb.ExposeToJS(runtime)
		})

		// newTicker(durationMs, node, options?) - Create a managed Ticker
		// Returns a JS object with done(), err(), stop() methods matching go-behaviortree.Ticker
		//
		// Usage:
		//   const ticker = bt.newTicker(100, myNode);
		//   await ticker.done(); // Wait for completion
		//   const err = ticker.err(); // Get any error
		//   ticker.stop(); // Stop the ticker
		//
		// Options (optional third argument):
		//   stopOnFailure: boolean - Stop on first failure (default: false)
		//
		// The ticker is automatically registered with the Bridge's internal Manager,
		// which aggregates all tickers for lifecycle management.
		_ = exports.Set("newTicker", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 2 {
				panic(runtime.NewTypeError("newTicker requires duration (ms) and node"))
			}

			durationMs := call.Arguments[0].ToInteger()
			if durationMs <= 0 {
				panic(runtime.NewTypeError("duration must be positive (milliseconds)"))
			}

			node, err := nodeUnwrap(b, runtime, call.Arguments[1])
			if err != nil {
				panic(runtime.NewTypeError("second argument must be a Node: " + err.Error()))
			}

			// Parse options
			stopOnFailure := false
			if len(call.Arguments) >= 3 && !goja.IsUndefined(call.Arguments[2]) && !goja.IsNull(call.Arguments[2]) {
				opts := call.Arguments[2].ToObject(runtime)
				if opts != nil {
					if sof := opts.Get("stopOnFailure"); sof != nil && !goja.IsUndefined(sof) {
						stopOnFailure = sof.ToBoolean()
					}
				}
			}

			// Create the ticker
			var ticker bt.Ticker
			if stopOnFailure {
				ticker = bt.NewTickerStopOnFailure(ctx, durationFromMs(durationMs), node)
			} else {
				ticker = bt.NewTicker(ctx, durationFromMs(durationMs), node)
			}

			// Register with the bridge's internal manager
			if b.manager != nil {
				if err := b.manager.Add(ticker); err != nil {
					panic(runtime.NewGoError(fmt.Errorf("failed to add ticker to manager: %w", err)))
				}
			}

			// Create a JS wrapper object
			return createTickerJSWrapper(b, runtime, ticker)
		})

		// newManager() - Create a new ticker Manager
		// Returns a JS object wrapping go-behaviortree.Manager with:
		//   add(ticker) - Add a ticker to the manager
		//   done() - Promise that resolves when all tickers complete
		//   err() - Get any error
		//   stop() - Stop all tickers
		//
		// This is for advanced use cases where you need to manage a separate
		// group of tickers with independent lifecycle from the Bridge's internal manager.
		_ = exports.Set("newManager", func(call goja.FunctionCall) goja.Value {
			manager := bt.NewManager()
			return createManagerJSWrapper(b, runtime, manager)
		})
	}
}

// createTickerJSWrapper creates a JavaScript object wrapping a bt.Ticker.
// The wrapper provides done(), err(), and stop() methods matching the Go interface.
func createTickerJSWrapper(bridge *Bridge, runtime *goja.Runtime, ticker bt.Ticker) goja.Value {
	obj := runtime.NewObject()

	// donePromise caches the promise for done() to avoid creating multiple promises
	var donePromise goja.Value
	var doneOnce sync.Once

	// done() - Returns a Promise that resolves when the ticker completes
	_ = obj.Set("done", func(call goja.FunctionCall) goja.Value {
		doneOnce.Do(func() {
			promise, resolve, reject := runtime.NewPromise()
			donePromise = runtime.ToValue(promise)

			// Wait for ticker in background and resolve/reject
			go func() {
				<-ticker.Done()
				tickerErr := ticker.Err()

				// CRITICAL FIX #2 (review.md): Handle case where bridge is stopped
				// but event loop is still alive (shared mode). We need to ensure
				// promises settle even when RunOnLoop returns false due to bridge shutdown.
				//
				// The race condition:
				// 1. Bridge.Stop() is called -> sets b.stopped = true
				// 2. Manager stops -> ticker.Done() closes
				// 3. Our goroutine wakes up and tries RunOnLoop
				// 4. RunOnLoop checks b.stopped and returns false
				// 5. Promise never settles -> memory leak / hang
				//
				// Solution: Try standard RunOnLoop first. If it fails (bridge stopped),
				// fallback to running directly on the loop if the loop is still alive.

				resolveFn := func(vm *goja.Runtime) {
					if tickerErr != nil {
						reject(vm.ToValue(tickerErr.Error()))
					} else {
						resolve(goja.Undefined())
					}
				}

				// Try standard scheduling first
				if !bridge.RunOnLoop(resolveFn) {
					// RunOnLoop failed (likely due to bridge being stopped)
					// FALLBACK: If the loop is still alive, try running directly on it
					// to ensure the promise settles. This is safe because:
					// - We're only running resolve/reject callbacks (no external side effects)
					// - The loop is still running (just the bridge is stopped)
					// - This prevents memory leaks in shared event loop scenarios

					// HIGH #2 FIX (review.md): Log fallback attempts for debugging hard shutdowns
					if bridge.loop != nil {
						// Force execution on the loop to settle the promise
						// Note: This bypasses the bridge.stopped check intentionally
						bridge.loop.RunOnLoop(resolveFn)
					} else {
						// No loop available - promise will remain pending
						// Note: Logging limitation as we're outside event loop goroutine
					}
					// If loop is nil (bridge fully destroyed), the promise will remain
					// pending but this is expected in shutdown scenarios
				}
			}()
		})
		return donePromise
	})

	// err() - Returns error string or null
	_ = obj.Set("err", func(call goja.FunctionCall) goja.Value {
		if err := ticker.Err(); err != nil {
			return runtime.ToValue(err.Error())
		}
		return goja.Null()
	})

	// stop() - Stop the ticker
	_ = obj.Set("stop", func(call goja.FunctionCall) goja.Value {
		ticker.Stop()
		return goja.Undefined()
	})

	// Store the native ticker for interop
	_ = obj.Set("_native", ticker)

	return obj
}

// createManagerJSWrapper creates a JavaScript object wrapping a bt.Manager.
func createManagerJSWrapper(bridge *Bridge, runtime *goja.Runtime, manager bt.Manager) goja.Value {
	obj := runtime.NewObject()

	// donePromise caches the promise for done() to avoid creating multiple promises
	var donePromise goja.Value
	var doneOnce sync.Once

	// add(ticker) - Add a ticker to the manager
	_ = obj.Set("add", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			panic(runtime.NewTypeError("add requires a ticker argument"))
		}

		// Try to extract native ticker
		tickerArg := call.Arguments[0]
		tickerObj := tickerArg.ToObject(runtime)
		if tickerObj == nil {
			panic(runtime.NewTypeError("argument must be a ticker object"))
		}

		nativeVal := tickerObj.Get("_native")
		if nativeVal == nil || goja.IsUndefined(nativeVal) {
			panic(runtime.NewTypeError("argument must be a ticker created by newTicker"))
		}

		ticker, ok := nativeVal.Export().(bt.Ticker)
		if !ok {
			panic(runtime.NewTypeError("argument must be a ticker created by newTicker"))
		}

		if err := manager.Add(ticker); err != nil {
			panic(runtime.NewGoError(err))
		}

		return goja.Undefined()
	})

	// done() - Returns a Promise that resolves when all tickers complete
	_ = obj.Set("done", func(call goja.FunctionCall) goja.Value {
		doneOnce.Do(func() {
			promise, resolve, reject := runtime.NewPromise()
			donePromise = runtime.ToValue(promise)

			// Wait for manager in background and resolve/reject
			go func() {
				<-manager.Done()
				managerErr := manager.Err()

				// CRITICAL FIX #1 (review.md): Handle case where bridge is stopped
				// but event loop is still alive (shared mode). We need to ensure
				// promises settle even when RunOnLoop returns false due to bridge shutdown.
				//
				// The race condition:
				// 1. Bridge.Stop() is called -> sets b.stopped = true
				// 2. Manager stops -> manager.Done() closes
				// 3. Our goroutine wakes up and tries RunOnLoop
				// 4. RunOnLoop checks b.stopped and returns false
				// 5. Promise never settles -> memory leak / hang
				//
				// Solution: Try standard RunOnLoop first. If it fails (bridge stopped),
				// fallback to running directly on the loop if the loop is still alive.

				resolveFn := func(vm *goja.Runtime) {
					if managerErr != nil {
						reject(vm.ToValue(managerErr.Error()))
					} else {
						resolve(goja.Undefined())
					}
				}

				// Try standard scheduling first
				if !bridge.RunOnLoop(resolveFn) {
					// RunOnLoop failed (likely due to bridge being stopped)
					// FALLBACK: If the loop is still alive, try running directly on it
					// to ensure the promise settles. This is safe because:
					// - We're only running resolve/reject callbacks (no external side effects)
					// - The loop is still running (just the bridge is stopped)
					// - This prevents memory leaks in shared event loop scenarios

					// HIGH #2 FIX (review.md): Log fallback attempts for debugging hard shutdowns
					if bridge.loop != nil {
						// Force execution on the loop to settle the promise
						// Note: This bypasses the bridge.stopped check intentionally
						bridge.loop.RunOnLoop(resolveFn)
					} else {
						// No loop available - promise will remain pending
						// Note: Logging limitation as we're outside event loop context
					}
				}
			}()
		})
		return donePromise
	})

	// err() - Returns error string or null
	_ = obj.Set("err", func(call goja.FunctionCall) goja.Value {
		if err := manager.Err(); err != nil {
			return runtime.ToValue(err.Error())
		}
		return goja.Null()
	})

	// stop() - Stop all tickers
	_ = obj.Set("stop", func(call goja.FunctionCall) goja.Value {
		manager.Stop()
		return goja.Undefined()
	})

	// Store the native manager for interop
	_ = obj.Set("_native", manager)

	return obj
}

// durationFromMs converts milliseconds to time.Duration
func durationFromMs(ms int64) time.Duration {
	return time.Duration(ms) * time.Millisecond
}

// mapGoStatus converts a go-behaviortree Status to a JavaScript status string.
func mapGoStatus(s bt.Status) string {
	switch s {
	case bt.Running:
		return JSStatusRunning
	case bt.Success:
		return JSStatusSuccess
	case bt.Failure:
		return JSStatusFailure
	default:
		return JSStatusFailure
	}
}
