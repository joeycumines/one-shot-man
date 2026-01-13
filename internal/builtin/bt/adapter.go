package bt

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/dop251/goja"
	bt "github.com/joeycumines/go-behaviortree"
	"github.com/joeycumines/one-shot-man/internal/goroutineid"
)

// JS status string constants - single source of truth for status string values.
const (
	JSStatusRunning = "running"
	JSStatusSuccess = "success"
	JSStatusFailure = "failure"
)

// AsyncState represents the state of an asynchronous JS leaf execution.
type AsyncState int

const (
	// StateIdle indicates the leaf is ready to start execution.
	StateIdle AsyncState = iota
	// StateRunning indicates the leaf is currently executing.
	StateRunning
	// StateCompleted indicates the leaf has finished execution.
	StateCompleted
)

// JSLeafAdapter bridges a JavaScript leaf function to a go-behaviortree Node.
// It implements a state machine to handle the async nature of JavaScript Promises
// while conforming to the synchronous go-behaviortree tick interface.
//
// The adapter has three states:
//   - Idle: Ready to start. On tick, dispatches to JS and returns Running.
//   - Running: Waiting for JS. On tick, checks completion and returns Running.
//   - Completed: JS finished. On tick, returns the final status and resets to Idle.
//
// Thread Safety:
// The adapter is safe for concurrent Tick() calls. All state transitions are atomic
// (under mutex), and generation counting prevents stale callbacks from corrupting state.
// JavaScript execution happens on the event loop goroutine via Bridge.RunOnLoop.
//
// One-Shot Context Semantics:
// JSLeafAdapter uses the parent context directly without creating child contexts.
// This prevents memory leaks from unbounded context derivation in high-churn
// environments. The adapter can be stopped by cancelling the parent context or
// creating a new adapter instance.
//
// MEDIUM #10 FIX: Stop() method is only accessible from Go side
// The Stop() method exists on *JSLeafAdapter for Go consumers, but the
// NewJSLeafAdapter returns a closure for bt.Node interface. There is no
// way to call Stop() from JavaScript code. This is intentional - JS nodes
// are designed as one-shot resources. If you need explicit cancellation from JS,
// use context cancellation patterns or create new nodes instead.
type JSLeafAdapter struct {
	bridge *Bridge
	tick   goja.Callable // The JS tick function to call
	getCtx func() any

	mu    sync.Mutex
	state AsyncState
	// Generation counter for rejecting stale callbacks.
	// Overflow would take ~584 years at 1B ticks/sec, making wraparound practically impossible.
	generation uint64 // Monotonic dispatch identifier to prevent stale callbacks
	lastStatus bt.Status
	lastError  error

	// Cancellation support
	// FIXED: Use parent context directly to prevent memory leak (CRITICAL #2)
	ctx context.Context
}

// NewJSLeafAdapter creates a new go-behaviortree Node that executes a JavaScript function.
//
// Parameters:
//   - ctx: Context for cancellation support. This context is used directly (no child derivation).
//   - bridge: The Bridge managing the JavaScript runtime
//   - tick: The JavaScript callable (goja.Callable) to execute as the leaf tick
//   - getCtx: Function that returns the context/blackboard to pass to the JS function
//
// The JavaScript function should have the signature:
//
//	async function leafName(ctx, args) {
//	    // ... perform work ...
//	    return bt.success; // or bt.failure or bt.running
//	}
//
// CRITICAL #2 FIX: No child context derivation to prevent memory leaks.
// The parent context is used directly. This prevents unbounded growth of the
// parent context's children map in high-churn environments.
//
// Context Cancellation Example:
//
//	ctx, cancel := context.WithCancel(context.Background())
//	defer cancel() // Ensure cleanup when done
//
//	adapter := NewJSLeafAdapter(ctx, bridge, tick, getCtx)
//
//	tree := bt.Sequence(adapter, otherNode1, otherNode2)
//
//	// Later, if user cancels (e.g., Ctrl+C), call:
//	cancel() // Adapter will return Failure on next Tick
//
// Thread Safety Requirements for getCtx:
// The getCtx function is called from the event loop goroutine WITHOUT any
// synchronization. If getCtx is a closure that captures shared mutable state,
// it MUST ensure thread safety:
//   - Use thread-safe data structures (e.g., Blackboard which uses RWMutex)
//   - Lock/copy appropriately before returning
//   - Be aware that getCtx may be called concurrently with Tick()
//
// Example of correct usage with Blackboard:
//
//	bb := new(Blackboard)
//	adapter := NewJSLeafAdapter(ctx, bridge, tick, func() any {
//	    // Blackboard is thread-safe, so returning it directly is safe
//	    return bb.ExposeToJS(..., vm) // or just bb if using safe accessor
//	})
//
// Example of incorrect usage (DATA RACE):
//
//	sharedMap := make(map[string]any)
//	adapter := NewJSLeafAdapter(ctx, bridge, tick, func() any {
//	    // DANGER: sharedMap is NOT thread-safe, this will cause races
//	    return sharedMap
//	})
//
// Correct way for the above example:
//
//	mu := new(sync.RWMutex)
//	sharedMap := make(map[string]any)
//	adapter := NewJSLeafAdapter(ctx, bridge, tick, func() any {
//	    mu.RLock()
//	    defer mu.RUnlock()
//	    // Return a defensive copy OR ensure caller only reads
//	    result := make(map[string]any, len(sharedMap))
//	    for k, v := range sharedMap {
//	        result[k] = v
//	    }
//	    return result
//	})

func NewJSLeafAdapter(ctx context.Context, bridge *Bridge, tick goja.Callable, getCtx func() any) bt.Node {
	if ctx == nil {
		ctx = context.Background()
	}
	adapter := &JSLeafAdapter{
		bridge: bridge,
		tick:   tick,
		getCtx: getCtx,
		state:  StateIdle,
		ctx:    ctx, // Use parent context directly - no child derivation
	}

	return func() (bt.Tick, []bt.Node) {
		return adapter.Tick, nil
	}
}

// Tick implements the go-behaviortree Tick interface.
// Thread-safe: state transitions are atomic (under mutex) and generation counting
// prevents stale callbacks from corrupting state.
func (a *JSLeafAdapter) Tick(children []bt.Node) (bt.Status, error) {
	a.mu.Lock()

	switch a.state {
	case StateIdle:
		// Check for cancellation before starting
		select {
		case <-a.ctx.Done():
			a.mu.Unlock()
			return bt.Failure, errors.New("execution cancelled")
		default:
		}
		// ATOMIC: Transition to running and bump generation before unlock
		// This prevents double-dispatch under concurrent Tick() calls
		a.generation++
		gen := a.generation
		a.state = StateRunning
		a.mu.Unlock()

		// CRITICAL FIX #3 (review.md): Double-check context cancellation
		// BEFORE dispatching to JS loop. There's a race window between
		// unlocking the mutex and calling dispatchJSWithGen where the
		// context could be cancelled. If we dispatch after cancellation,
		// we risk executing stale logic or corrupting state.
		// CRIT-1 FIX: If cancelled, must reset state to prevent zombie
		select {
		case <-a.ctx.Done():
			// Context was cancelled right after we unlocked
			// Don't dispatch - reset state and return failure immediately
			a.mu.Lock()
			a.generation++ // Invalidate any racing finalize callback
			a.state = StateIdle
			a.mu.Unlock()
			return bt.Failure, errors.New("execution cancelled")
		default:
		}

		// Dispatch with the captured generation
		a.dispatchJSWithGen(gen)
		return bt.Running, nil

	case StateRunning:
		// Check for cancellation while running
		select {
		case <-a.ctx.Done():
			// CRITICAL FIX: Bump generation BEFORE changing state
			// This prevents finalize() from applying stale result to new request
			a.generation++
			// Now safe to move state
			a.state = StateIdle
			a.mu.Unlock()
			return bt.Failure, errors.New("execution cancelled")
		default:
		}
		a.mu.Unlock()
		// Still waiting for JS to complete
		return bt.Running, nil

	case StateCompleted:
		// Collect the result and reset to idle
		status, err := a.lastStatus, a.lastError
		a.state = StateIdle
		a.lastStatus = 0
		a.lastError = nil
		a.mu.Unlock()
		return status, err

	default:
		a.mu.Unlock()
		return bt.Failure, errors.New("invalid async state")
	}
}

// dispatchJSWithGen sends the execution request to the JavaScript event loop.
// The gen parameter is passed to finalize to ensure stale callbacks are ignored.
func (a *JSLeafAdapter) dispatchJSWithGen(gen uint64) {
	ok := a.bridge.RunOnLoop(func(vm *goja.Runtime) {
		defer func() {
			if r := recover(); r != nil {
				a.finalize(gen, bt.Failure, fmt.Errorf("panic in JS leaf: %v", r))
			}
		}()

		// Get the runLeaf helper
		runLeafVal := vm.Get("runLeaf")
		if goja.IsUndefined(runLeafVal) || goja.IsNull(runLeafVal) {
			a.finalize(gen, bt.Failure, errors.New("runLeaf helper not defined"))
			return
		}
		runLeafFn, ok := goja.AssertFunction(runLeafVal)
		if !ok {
			a.finalize(gen, bt.Failure, errors.New("runLeaf is not a callable function"))
			return
		}

		// Create wrapper function that calls our tick callable
		wrapperFn := vm.ToValue(func(call goja.FunctionCall) goja.Value {
			ctx := call.Argument(0)
			args := call.Argument(1)
			result, err := a.tick(goja.Undefined(), ctx, args)
			if err != nil {
				panic(vm.NewGoError(err))
			}
			return result
		})

		// Create the callback that will be called when the Promise resolves
		callback := func(call goja.FunctionCall) goja.Value {
			statusStr := call.Argument(0).String()
			var err error
			if !goja.IsNull(call.Argument(1)) && !goja.IsUndefined(call.Argument(1)) {
				err = fmt.Errorf("%s", call.Argument(1).String())
			}
			a.finalize(gen, mapJSStatus(statusStr), err)
			return goja.Undefined()
		}

		// Prepare context and arguments
		var ctxVal goja.Value
		if a.getCtx != nil {
			ctxVal = vm.ToValue(a.getCtx())
		} else {
			ctxVal = goja.Undefined()
		}
		argsVal := goja.Undefined() // Reserved for future use

		// Call runLeaf(fn, ctx, args, callback)
		_, err := runLeafFn(
			goja.Undefined(), // this
			wrapperFn,
			ctxVal,
			argsVal,
			vm.ToValue(callback),
		)
		if err != nil {
			a.finalize(gen, bt.Failure, fmt.Errorf("runLeaf failed: %w", err))
		}
	})

	if !ok {
		a.finalize(gen, bt.Failure, errors.New("event loop terminated"))
	}
}

// finalize sets the result and transitions to StateCompleted.
// The gen parameter is verified against the current generation to discard stale callbacks.
func (a *JSLeafAdapter) finalize(gen uint64, status bt.Status, err error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Discard stale callbacks from cancelled or superseded runs
	if gen != a.generation {
		return
	}

	a.lastStatus = status
	a.lastError = err
	a.state = StateCompleted
}

// mapJSStatus converts a JavaScript status string to a go-behaviortree Status.
func mapJSStatus(s string) bt.Status {
	switch s {
	case JSStatusRunning:
		return bt.Running
	case JSStatusSuccess:
		return bt.Success
	case JSStatusFailure:
		return bt.Failure
	default:
		return bt.Failure
	}
}

// BlockingJSLeaf creates a blocking version of a JS leaf with an optional VM reference.
// When vm is provided and the tick is called from the event loop goroutine, the VM is used
// directly to avoid deadlock. This is CRITICAL for composite trees where leaves are ticked
// from within event loop callbacks.
//
// Parameters:
//   - ctx: Context for cancellation support (pass nil for context.Background())
//   - bridge: The Bridge managing the JavaScript runtime
//   - vm: Optional VM reference for direct execution when on event loop (can be nil)
//   - tick: The JavaScript callable (goja.Callable) to execute
//   - getCtx: Function that returns the context/blackboard to pass to the JS function
func BlockingJSLeaf(ctx context.Context, bridge *Bridge, vm *goja.Runtime, tick goja.Callable, getCtx func() any) bt.Node {
	if ctx == nil {
		ctx = context.Background()
	}
	return func() (bt.Tick, []bt.Node) {
		return func(children []bt.Node) (bt.Status, error) {
			type result struct {
				status bt.Status
				err    error
			}

			// Check if we're already on the event loop goroutine
			// If so and we have a VM reference, execute directly to avoid deadlock
			eventLoopID := bridge.eventLoopGoroutineID.Load()
			onEventLoop := eventLoopID > 0 && goroutineid.Get() == eventLoopID && vm != nil

			if onEventLoop {
				// We're on the event loop - execute directly (SYNC ONLY)
				// For async functions, this will fail gracefully
				runLeafVal := vm.Get("runLeaf")
				runLeafFn, ok := goja.AssertFunction(runLeafVal)
				if !ok {
					return bt.Failure, errors.New("runLeaf helper not found")
				}

				// Create wrapper function that calls our tick callable
				wrapperFn := vm.ToValue(func(call goja.FunctionCall) goja.Value {
					ctxArg := call.Argument(0)
					argsArg := call.Argument(1)
					retVal, err := tick(goja.Undefined(), ctxArg, argsArg)
					if err != nil {
						panic(vm.NewGoError(err))
					}
					return retVal
				})

				// For synchronous execution, capture result directly
				var res result
				var called bool
				callback := func(call goja.FunctionCall) goja.Value {
					statusStr := call.Argument(0).String()
					var err error
					if arg1 := call.Argument(1); !goja.IsNull(arg1) && !goja.IsUndefined(arg1) {
						err = fmt.Errorf("%s", arg1.String())
					}
					res = result{mapJSStatus(statusStr), err}
					called = true
					return goja.Undefined()
				}

				var ctxVal goja.Value
				if getCtx != nil {
					ctxVal = vm.ToValue(getCtx())
				} else {
					ctxVal = goja.Undefined()
				}

				_, err := runLeafFn(
					goja.Undefined(),
					wrapperFn,
					ctxVal,
					goja.Undefined(),
					vm.ToValue(callback),
				)
				if err != nil {
					return bt.Failure, err
				}

				if !called {
					// Async function - can't execute synchronously on event loop
					// This shouldn't happen with properly constructed trees (sync leaves in composites)
					return bt.Failure, errors.New("async JS function cannot be executed synchronously on event loop - use sync functions for composite tree leaves")
				}

				return res.status, res.err
			}

			// Not on event loop (or no VM reference) - use channel-based async approach
			ch := make(chan result, 1)

			// Use sync.Once to guarantee single send to channel
			var once sync.Once
			send := func(r result) {
				once.Do(func() { ch <- r })
			}

			ok := bridge.RunOnLoop(func(loopVM *goja.Runtime) {
				defer func() {
					if r := recover(); r != nil {
						send(result{bt.Failure, fmt.Errorf("panic: %v", r)})
					}
				}()

				runLeafVal := loopVM.Get("runLeaf")
				runLeafFn, ok := goja.AssertFunction(runLeafVal)
				if !ok {
					send(result{bt.Failure, errors.New("runLeaf helper not found")})
					return
				}

				// Create wrapper function that calls our tick callable
				wrapperFn := loopVM.ToValue(func(call goja.FunctionCall) goja.Value {
					ctxArg := call.Argument(0)
					argsArg := call.Argument(1)
					retVal, err := tick(goja.Undefined(), ctxArg, argsArg)
					if err != nil {
						panic(loopVM.NewGoError(err))
					}
					return retVal
				})

				// Callback sends to channel - works for both sync and async
				callback := func(call goja.FunctionCall) goja.Value {
					statusStr := call.Argument(0).String()
					var err error
					if arg1 := call.Argument(1); !goja.IsNull(arg1) && !goja.IsUndefined(arg1) {
						err = fmt.Errorf("%s", arg1.String())
					}
					send(result{mapJSStatus(statusStr), err})
					return goja.Undefined()
				}

				var ctxVal goja.Value
				if getCtx != nil {
					ctxVal = loopVM.ToValue(getCtx())
				} else {
					ctxVal = goja.Undefined()
				}

				_, err := runLeafFn(
					goja.Undefined(),
					wrapperFn,
					ctxVal,
					goja.Undefined(),
					loopVM.ToValue(callback),
				)
				if err != nil {
					send(result{bt.Failure, err})
				}
				// For async functions, callback will be called later via Promise.then
				// For sync functions, callback was already called
			})

			if !ok {
				return bt.Failure, errors.New("event loop terminated")
			}

			// MAJ-4 FIX: Defer channel cleanup to prevent leak on early return
			// If ctx.Done() or bridge.Done() wins in select below, the ch channel
			// is never received from. The goroutine scheduled via RunOnLoop may still
			// complete and send to ch. sync.Once prevents double-sends but channel leaks.
			// Drain in deferred cleanup to reclaim the buffered channel.
			defer func() {
				select {
				case <-ch:
					// Drain if available
				default:
					// Not sent yet, safe to ignore
				}
			}()

			// Wait with cancellation support to avoid deadlock if bridge stops
			select {
			case r := <-ch:
				return r.status, r.err
			case <-ctx.Done():
				return bt.Failure, ctx.Err()
			case <-bridge.Done():
				return bt.Failure, errors.New("bridge stopped")
			}
		}, nil
	}
}
