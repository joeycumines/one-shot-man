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
func (b *Bridge) ModuleLoader(ctx context.Context) require.ModuleLoader {

	return func(runtime *goja.Runtime, module *goja.Object) {
		exports := module.Get("exports").(*goja.Object)

		// Status constants (must match JSStatus* constants in adapter.go)
		_ = exports.Set("running", JSStatusRunning)
		_ = exports.Set("success", JSStatusSuccess)
		_ = exports.Set("failure", JSStatusFailure)

		// node(tick, ...children) - Create a bt.Node from tick and optional children
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
		_ = exports.Set("tick", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 1 {
				panic(runtime.NewTypeError("tick requires a node argument"))
			}

			nodeVal := call.Arguments[0]

			// Try direct unwrap first (for native Go nodes)
			if node, ok := nodeVal.Export().(bt.Node); ok {
				status, tickErr := node.Tick()
				if tickErr != nil {
					return runtime.ToValue(JSStatusFailure)
				}
				return runtime.ToValue(mapGoStatus(status))
			}

			// For JS nodes
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
		_ = exports.Set("sequence", func(call goja.FunctionCall) goja.Value {
			var children []bt.Node
			if len(call.Arguments) > 0 {
				childrenArray := call.Arguments[0].ToObject(runtime)
				if childrenArray != nil {
					length := int(childrenArray.Get("length").ToInteger())
					children = make([]bt.Node, length)
					for i := range length {
						childVal := childrenArray.Get(strconv.Itoa(i))
						node, err := nodeUnwrap(b, runtime, childVal)
						if err != nil {
							panic(runtime.NewGoError(fmt.Errorf("sequence child %d: %w", i, err)))
						}
						children[i] = node
					}
				}
			}

			status, err := bt.Sequence(children)
			if err != nil {
				return runtime.ToValue(mapGoStatus(bt.Failure))
			}
			return runtime.ToValue(mapGoStatus(status))
		})

		// fallback - Alias for bt.Selector
		_ = exports.Set("fallback", func(call goja.FunctionCall) goja.Value {
			var children []bt.Node
			if len(call.Arguments) > 0 {
				childrenArray := call.Arguments[0].ToObject(runtime)
				if childrenArray != nil {
					length := int(childrenArray.Get("length").ToInteger())
					children = make([]bt.Node, length)
					for i := range length {
						childVal := childrenArray.Get(strconv.Itoa(i))
						node, err := nodeUnwrap(b, runtime, childVal)
						if err != nil {
							panic(runtime.NewGoError(fmt.Errorf("fallback child %d: %w", i, err)))
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

		// selector - Also expose as selector
		_ = exports.Set("selector", func(call goja.FunctionCall) goja.Value {
			var children []bt.Node
			if len(call.Arguments) > 0 {
				childrenArray := call.Arguments[0].ToObject(runtime)
				if childrenArray != nil {
					length := int(childrenArray.Get("length").ToInteger())
					children = make([]bt.Node, length)
					for i := range length {
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

		// memorize(tick)
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

		// async(tick)
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

		// not(tick)
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

		// fork()
		_ = exports.Set("fork", func(call goja.FunctionCall) goja.Value {
			return runtime.ToValue(bt.Fork())
		})

		// interval(intervalMs)
		_ = exports.Set("interval", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 1 {
				panic(runtime.NewTypeError("interval requires intervalMs argument"))
			}

			intervalMs := call.Arguments[0].ToInteger()
			duration := time.Duration(intervalMs) * time.Millisecond

			return runtime.ToValue(bt.RateLimit(duration))
		})

		// createLeafNode(tick)
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

		// createBlockingLeafNode(tick)
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

			return runtime.ToValue(BlockingJSLeaf(ctx, b, runtime, tickFn, nil))
		})

		// Blackboard
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
			_ = obj.Set("_native", bb)
			return nil
		})

		// exposeBlackboard(blackboard)
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

		// newTicker(durationMs, node, options?)
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

			// Keep the event loop alive as long as the ticker is running.
			// This covers the background ticking goroutine inside go-behaviortree.
			if b.promisify != nil {
				b.promisify(ctx, func(ctx context.Context) (any, error) {
					<-ticker.Done()
					return nil, ticker.Err()
				})
			}

			// Create a JS wrapper object
			return createTickerJSWrapper(b, runtime, ticker)
		})

		// newManager()
		_ = exports.Set("newManager", func(call goja.FunctionCall) goja.Value {
			manager := bt.NewManager()
			return createManagerJSWrapper(b, runtime, manager)
		})
	}
}

// createTickerJSWrapper creates a JavaScript object wrapping a bt.Ticker.
func createTickerJSWrapper(bridge *Bridge, runtime *goja.Runtime, ticker bt.Ticker) goja.Value {
	obj := runtime.NewObject()

	var donePromise goja.Value
	var doneOnce sync.Once

	// done() - Returns a Promise that resolves when the ticker completes
	_ = obj.Set("done", func(call goja.FunctionCall) goja.Value {
		doneOnce.Do(func() {
			promise, resolve, reject := runtime.NewPromise()
			donePromise = runtime.ToValue(promise)

			waitAndResolve := func(ctx context.Context) (any, error) {
				<-ticker.Done()
				tickerErr := ticker.Err()

				resolveFn := func(vm *goja.Runtime) {
					if tickerErr != nil {
						reject(vm.ToValue(tickerErr.Error()))
					} else {
						resolve(goja.Undefined())
					}
				}

				if !bridge.RunOnLoop(resolveFn) {
					if bridge.loop != nil {
						_ = bridge.loop.Submit(func() { resolveFn(bridge.vm) })
					}
				}
				return nil, tickerErr
			}

			if bridge.promisify != nil {
				bridge.promisify(context.Background(), waitAndResolve)
			} else {
				go func() { _, _ = waitAndResolve(context.Background()) }()
			}
		})
		return donePromise
	})

	_ = obj.Set("err", func(call goja.FunctionCall) goja.Value {
		if err := ticker.Err(); err != nil {
			return runtime.ToValue(err.Error())
		}
		return goja.Null()
	})

	_ = obj.Set("stop", func(call goja.FunctionCall) goja.Value {
		ticker.Stop()
		return goja.Undefined()
	})

	_ = obj.Set("_native", ticker)

	return obj
}

// createManagerJSWrapper creates a JavaScript object wrapping a bt.Manager.
func createManagerJSWrapper(bridge *Bridge, runtime *goja.Runtime, manager bt.Manager) goja.Value {
	obj := runtime.NewObject()

	var donePromise goja.Value
	var doneOnce sync.Once

	_ = obj.Set("add", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			panic(runtime.NewTypeError("add requires a ticker argument"))
		}

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

	_ = obj.Set("done", func(call goja.FunctionCall) goja.Value {
		doneOnce.Do(func() {
			promise, resolve, reject := runtime.NewPromise()
			donePromise = runtime.ToValue(promise)

			waitAndResolve := func(ctx context.Context) (any, error) {
				<-manager.Done()
				managerErr := manager.Err()

				resolveFn := func(vm *goja.Runtime) {
					if managerErr != nil {
						reject(vm.ToValue(managerErr.Error()))
					} else {
						resolve(goja.Undefined())
					}
				}

				if !bridge.RunOnLoop(resolveFn) {
					if bridge.loop != nil {
						_ = bridge.loop.Submit(func() { resolveFn(bridge.vm) })
					}
				}
				return nil, managerErr
			}

			if bridge.promisify != nil {
				bridge.promisify(context.Background(), waitAndResolve)
			} else {
				go func() { _, _ = waitAndResolve(context.Background()) }()
			}
		})
		return donePromise
	})

	_ = obj.Set("err", func(call goja.FunctionCall) goja.Value {
		if err := manager.Err(); err != nil {
			return runtime.ToValue(err.Error())
		}
		return goja.Null()
	})

	_ = obj.Set("stop", func(call goja.FunctionCall) goja.Value {
		manager.Stop()
		return goja.Undefined()
	})

	_ = obj.Set("_native", manager)

	return obj
}

func durationFromMs(ms int64) time.Duration {
	return time.Duration(ms) * time.Millisecond
}

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
