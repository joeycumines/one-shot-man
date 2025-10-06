package scripting

import "github.com/dop251/goja"

// StateAccessor is the Go representation of the object injected into JS
// command handlers to manage state.
type StateAccessor struct {
	tui *TUIManager
}

// jsStateAccessor is the interface exposed to JavaScript.
type jsStateAccessor struct {
	accessor *StateAccessor
	runtime  *goja.Runtime
}

// NewStateAccessor creates a new StateAccessor for the current mode.
func NewStateAccessor(tui *TUIManager) *StateAccessor {
	return &StateAccessor{tui: tui}
}

// ToJS exports the StateAccessor as a JavaScript object with get/set methods.
func (a *StateAccessor) ToJS(runtime *goja.Runtime) goja.Value {
	js := &jsStateAccessor{accessor: a, runtime: runtime}

	// Create an object to hold the accessor functions
	obj := runtime.NewObject()
	// Bind 'get' to the jsGet method
	_ = obj.Set("get", js.jsGet)
	// Bind 'set' to the jsSet method
	_ = obj.Set("set", js.jsSet)

	return obj
}

// jsGet retrieves a value from the current mode's state map using a Symbol key.
// JS signature: get(symbolKey: Symbol): any
func (js *jsStateAccessor) jsGet(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) == 0 {
		// Return undefined for an empty call, matching tui.getState(undefined)
		return goja.Undefined()
	}
	symbolKey := call.Arguments[0]
	return js.accessor.tui.getStateBySymbol(symbolKey)
}

// jsSet sets a value in the current mode's state map using a Symbol key.
// JS signature: set(symbolKey: Symbol, value: any): void
func (js *jsStateAccessor) jsSet(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 2 {
		return goja.Undefined()
	}
	symbolKey := call.Arguments[0]
	value := call.Arguments[1].Export() // Export the new value to a Go type

	js.accessor.tui.setStateBySymbol(symbolKey, value)
	return goja.Undefined()
}
