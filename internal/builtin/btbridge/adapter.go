package btbridge

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/dop251/goja"
	bt "github.com/joeycumines/go-behaviortree"
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
// The adapter is safe for concurrent access. The state is protected by a mutex.
// JavaScript execution happens on the event loop goroutine via Bridge.RunOnLoop.
type JSLeafAdapter struct {
	bridge *Bridge
	fnName string
	getCtx func() any

	mu         sync.Mutex
	state      AsyncState
	lastStatus bt.Status
	lastError  error

	// Cancellation support
	ctx    context.Context
	cancel context.CancelFunc
}

// NewJSLeafAdapter creates a new go-behaviortree Node that executes a JavaScript function.
//
// Parameters:
//   - bridge: The Bridge managing the JavaScript runtime
//   - fnName: Name of the JavaScript function to call (must be globally accessible)
//   - getCtx: Function that returns the context/blackboard to pass to the JS function
//
// The JavaScript function should have the signature:
//
//	async function leafName(ctx, args) {
//	    // ... perform work ...
//	    return bt.success; // or bt.failure or bt.running
//	}
func NewJSLeafAdapter(bridge *Bridge, fnName string, getCtx func() any) bt.Node {
	return NewJSLeafAdapterWithContext(context.Background(), bridge, fnName, getCtx)
}

// NewJSLeafAdapterWithContext creates a JSLeafAdapter with cancellation support.
// When the context is canceled, the adapter will return Failure on the next tick.
func NewJSLeafAdapterWithContext(ctx context.Context, bridge *Bridge, fnName string, getCtx func() any) bt.Node {
	childCtx, cancel := context.WithCancel(ctx)
	adapter := &JSLeafAdapter{
		bridge: bridge,
		fnName: fnName,
		getCtx: getCtx,
		state:  StateIdle,
		ctx:    childCtx,
		cancel: cancel,
	}

	return func() (bt.Tick, []bt.Node) {
		return adapter.Tick, nil
	}
}

// Cancel cancels the adapter's context, causing future ticks to fail.
func (a *JSLeafAdapter) Cancel() {
	a.cancel()
}

// Tick implements the go-behaviortree Tick interface.
func (a *JSLeafAdapter) Tick(children []bt.Node) (bt.Status, error) {
	a.mu.Lock()
	currentState := a.state
	a.mu.Unlock()

	switch currentState {
	case StateIdle:
		// Check for cancellation before starting
		select {
		case <-a.ctx.Done():
			return bt.Failure, errors.New("execution cancelled")
		default:
		}
		// Start the JS execution
		a.dispatchJS()
		return bt.Running, nil

	case StateRunning:
		// Check for cancellation while running
		select {
		case <-a.ctx.Done():
			a.mu.Lock()
			a.state = StateIdle
			a.mu.Unlock()
			return bt.Failure, errors.New("execution cancelled")
		default:
		}
		// Still waiting for JS to complete
		return bt.Running, nil

	case StateCompleted:
		// Collect the result and reset to idle
		a.mu.Lock()
		defer a.mu.Unlock()
		status, err := a.lastStatus, a.lastError
		a.state = StateIdle
		a.lastStatus = 0
		a.lastError = nil
		return status, err
	}

	return bt.Failure, errors.New("invalid async state")
}

// dispatchJS sends the execution request to the JavaScript event loop.
func (a *JSLeafAdapter) dispatchJS() {
	a.mu.Lock()
	a.state = StateRunning
	a.mu.Unlock()

	ok := a.bridge.RunOnLoop(func(vm *goja.Runtime) {
		defer func() {
			if r := recover(); r != nil {
				a.finalize(bt.Failure, fmt.Errorf("panic in JS leaf %s: %v", a.fnName, r))
			}
		}()

		// Get the JS function
		fnVal := vm.Get(a.fnName)
		if _, ok := goja.AssertFunction(fnVal); !ok {
			a.finalize(bt.Failure, fmt.Errorf("JS function '%s' not callable", a.fnName))
			return
		}

		// Get the runLeaf helper
		runLeafVal := vm.Get("runLeaf")
		runLeafFn, ok := goja.AssertFunction(runLeafVal)
		if !ok {
			a.finalize(bt.Failure, errors.New("runLeaf helper not found"))
			return
		}

		// Create the callback that will be called when the Promise resolves
		callback := func(call goja.FunctionCall) goja.Value {
			statusStr := call.Argument(0).String()
			var err error
			if !goja.IsNull(call.Argument(1)) && !goja.IsUndefined(call.Argument(1)) {
				err = fmt.Errorf("%s", call.Argument(1).String())
			}
			a.finalize(mapJSStatus(statusStr), err)
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
			fnVal,
			ctxVal,
			argsVal,
			vm.ToValue(callback),
		)
		if err != nil {
			a.finalize(bt.Failure, fmt.Errorf("runLeaf failed for function '%s': %w", a.fnName, err))
		}
	})

	if !ok {
		a.finalize(bt.Failure, errors.New("event loop terminated"))
	}
}

// finalize sets the result and transitions to StateCompleted.
func (a *JSLeafAdapter) finalize(status bt.Status, err error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.lastStatus = status
	a.lastError = err
	a.state = StateCompleted
}

// mapJSStatus converts a JavaScript status string to a go-behaviortree Status.
func mapJSStatus(s string) bt.Status {
	switch s {
	case "running":
		return bt.Running
	case "success":
		return bt.Success
	case "failure":
		return bt.Failure
	default:
		return bt.Failure
	}
}

// BlockingJSLeaf creates a simpler blocking version of a JS leaf.
// This is useful for simple scripts or prototyping where you don't need
// interleaving of multiple nodes during a single tick.
//
// Unlike JSLeafAdapter, this blocks the calling goroutine until the JS
// Promise resolves. It's simpler but doesn't support true interleaving.
func BlockingJSLeaf(bridge *Bridge, fnName string, getCtx func() any) bt.Node {
	return func() (bt.Tick, []bt.Node) {
		return func(children []bt.Node) (bt.Status, error) {
			type result struct {
				status bt.Status
				err    error
			}
			ch := make(chan result, 1)

			ok := bridge.RunOnLoop(func(vm *goja.Runtime) {
				defer func() {
					if r := recover(); r != nil {
						ch <- result{bt.Failure, fmt.Errorf("panic: %v", r)}
					}
				}()

				// Get the JS function
				fnVal := vm.Get(fnName)
				if _, ok := goja.AssertFunction(fnVal); !ok {
					ch <- result{bt.Failure, fmt.Errorf("function '%s' not callable", fnName)}
					return
				}

				// Get the runLeaf helper
				runLeafVal := vm.Get("runLeaf")
				runLeafFn, ok := goja.AssertFunction(runLeafVal)
				if !ok {
					ch <- result{bt.Failure, errors.New("runLeaf helper not found")}
					return
				}

				callback := func(call goja.FunctionCall) goja.Value {
					statusStr := call.Argument(0).String()
					var err error
					if arg1 := call.Argument(1); !goja.IsNull(arg1) && !goja.IsUndefined(arg1) {
						err = fmt.Errorf("%s", arg1.String())
					}
					ch <- result{mapJSStatus(statusStr), err}
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
					fnVal,
					ctxVal,
					goja.Undefined(),
					vm.ToValue(callback),
				)
				if err != nil {
					ch <- result{bt.Failure, err}
				}
			})

			if !ok {
				return bt.Failure, errors.New("event loop terminated")
			}

			r := <-ch
			return r.status, r.err
		}, nil
	}
}
