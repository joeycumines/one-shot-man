package btbridge

import (
	"context"
	_ "embed"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/require"
	bt "github.com/joeycumines/go-behaviortree"
)

//go:embed bt.js
var btJS string

// Manager manages behavior tree bridges and provides the require module interface.
type Manager struct {
	ctx    context.Context
	bridge *Bridge
}

// NewManager creates a new Manager with its own Bridge.
func NewManager(ctx context.Context) (*Manager, error) {
	bridge, err := NewBridgeWithContext(ctx)
	if err != nil {
		return nil, err
	}

	return &Manager{
		ctx:    ctx,
		bridge: bridge,
	}, nil
}

// GetBridge returns the underlying Bridge for advanced use cases.
func (m *Manager) GetBridge() *Bridge {
	return m.bridge
}

// Close stops the manager and releases resources.
func (m *Manager) Close() error {
	if m.bridge != nil {
		m.bridge.Stop()
	}
	return nil
}

// Require returns a require.ModuleLoader for the "osm:bt" module.
// This loader exposes the behavior tree functionality to JavaScript.
func Require(ctx context.Context, bridge *Bridge) require.ModuleLoader {
	return func(runtime *goja.Runtime, module *goja.Object) {
		exports := module.Get("exports").(*goja.Object)

		// Note: bt.js uses ES module syntax which isn't directly compatible
		// with CommonJS require. We expose the Go-side primitives and bridge
		// functionality instead, with bt.js available for advanced users who
		// want to use a bundler or modify the code.
		// The btJS variable is embedded and available for future use.
		_ = btJS // Silence unused warning; available for future ES module support

		// Status constants
		_ = exports.Set("running", "running")
		_ = exports.Set("success", "success")
		_ = exports.Set("failure", "failure")

		// Go status constants
		_ = exports.Set("Running", int(bt.Running))
		_ = exports.Set("Success", int(bt.Success))
		_ = exports.Set("Failure", int(bt.Failure))

		// Bridge factory
		_ = exports.Set("createBlackboard", func() *Blackboard {
			return NewBlackboard()
		})

		// Expose blackboard to JS helper
		_ = exports.Set("exposeBlackboard", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 1 {
				panic(runtime.NewTypeError("exposeBlackboard requires a Blackboard argument"))
			}
			bb, ok := call.Arguments[0].Export().(*Blackboard)
			if !ok {
				panic(runtime.NewTypeError("exposeBlackboard requires a Blackboard argument"))
			}
			return bb.ExposeToJS(runtime)
		})

		// Helper to create node factory from JS function name
		_ = exports.Set("createLeafNode", func(fnName string) bt.Node {
			return NewJSLeafAdapter(bridge, fnName, nil)
		})

		// Helper to create blocking leaf from JS function name
		_ = exports.Set("createBlockingLeafNode", func(fnName string) bt.Node {
			return BlockingJSLeaf(bridge, fnName, nil)
		})

		// Expose go-behaviortree composites
		_ = exports.Set("Sequence", bt.Sequence)
		_ = exports.Set("Selector", bt.Selector)
		_ = exports.Set("Not", bt.Not)
		_ = exports.Set("Async", bt.Async)
		_ = exports.Set("Fork", bt.Fork)
		_ = exports.Set("Memorize", bt.Memorize)

		// Node constructor
		_ = exports.Set("newNode", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 1 {
				panic(runtime.NewTypeError("newNode requires at least a tick function"))
			}

			// First argument is the tick function
			tickVal := call.Arguments[0].Export()
			tick, ok := tickVal.(bt.Tick)
			if !ok {
				panic(runtime.NewTypeError("first argument must be a Tick function"))
			}

			// Remaining arguments are children
			var children []bt.Node
			for i := 1; i < len(call.Arguments); i++ {
				child, ok := call.Arguments[i].Export().(bt.Node)
				if !ok {
					panic(runtime.NewTypeError("children must be Node functions"))
				}
				children = append(children, child)
			}

			return runtime.ToValue(bt.New(tick, children...))
		})

		// Ticker factory
		_ = exports.Set("newTicker", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 2 {
				panic(runtime.NewTypeError("newTicker requires duration (ms) and node"))
			}

			durationMs := call.Arguments[0].ToInteger()
			node, ok := call.Arguments[1].Export().(bt.Node)
			if !ok {
				panic(runtime.NewTypeError("second argument must be a Node"))
			}

			ticker := bt.NewTicker(ctx, durationFromMs(durationMs), node)
			return runtime.ToValue(ticker)
		})
	}
}

// durationFromMs converts milliseconds to time.Duration
func durationFromMs(ms int64) time.Duration {
	return time.Duration(ms) * time.Millisecond
}
