package pabt

import (
	"context"
	"fmt"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/require"
	bt "github.com/joeycumines/go-behaviortree"
	pabtpkg "github.com/joeycumines/go-pabt"
	btmod "github.com/joeycumines/one-shot-man/internal/builtin/bt"
)

// ModuleLoader returns a require.ModuleLoader for "osm:pabt" module.
// This loader exposes PA-BT planning functionality to JavaScript.
//
// The API surface includes:
//   - newState(blackboard) - Create a PA-BT state backed by a bt.Blackboard
//   - newSymbol(name) - Create a type-safe key for State.Variable()
//   - newPlan(state, goals) - Create a PA-BT plan with goal conditions
//   - newAction(name, conditions, effects, node) - Create an action
//   - Running/Success/Failure status constants
func ModuleLoader(ctx context.Context) require.ModuleLoader {
	return func(runtime *goja.Runtime, module *goja.Object) {
		exports := module.Get("exports").(*goja.Object)

		// Status constants (match osm:bt)
		_ = exports.Set("Running", "running")
		_ = exports.Set("Success", "success")
		_ = exports.Set("Failure", "failure")

		// newState(blackboard) - Create a PA-BT state
		_ = exports.Set("newState", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 1 {
				panic(runtime.NewTypeError("newState requires a blackboard argument"))
			}

			// Extract bt.Blackboard from argument
			bbObj := call.Arguments[0].ToObject(runtime)
			if bbObj == nil {
				panic(runtime.NewTypeError("blackboard argument is not an object"))
			}

			nativeVal := bbObj.Get("_native")
			if nativeVal == nil || goja.IsUndefined(nativeVal) {
				panic(runtime.NewTypeError("blackboard must be created via new bt.Blackboard()"))
			}

			bb, ok := nativeVal.Export().(*btmod.Blackboard)
			if !ok {
				panic(runtime.NewTypeError("blackboard argument must be a bt.Blackboard instance"))
			}

			// Create PABTState wrapping the blackboard
			state := NewState(bb)
			return runtime.ToValue(state)
		})

		// newSymbol(name)NOT USED in go-pabt v0.2.0, we use raw values
		_ = exports.Set("newSymbol", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 1 {
				panic(runtime.NewTypeError("newSymbol requires a name argument"))
			}
			// In go-pabt v0.2.0, you can use raw values directly as keys.
			// This is just for compatibility.
			return call.Arguments[0]
		})

		// newPlan(state, goals) - Create a PA-BT plan
		_ = exports.Set("newPlan", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 2 {
				panic(runtime.NewTypeError("newPlan requires state and goals arguments"))
			}

			// Extract the state
			stateVal := call.Arguments[0]
			state, ok := stateVal.Export().(*State)
			if !ok {
				panic(runtime.NewTypeError("state must be a PABTState created via pabt.newState()"))
			}

			// Extract the goals (should be an array)
			goalsArray := call.Arguments[1].ToObject(runtime)
			if goalsArray == nil {
				panic(runtime.NewTypeError("goals must be an array of condition objects"))
			}

			length := int(goalsArray.Get("length").ToInteger())
			goals := make([]pabtpkg.IConditions, 0, length)

			for i := 0; i < length; i++ {
				goalVal := goalsArray.Get(fmt.Sprintf("%d", i))
				if goja.IsUndefined(goalVal) || goja.IsNull(goalVal) {
					continue
				}

				goalObj := goalVal.ToObject(runtime)
				if goalObj == nil {
					panic(runtime.NewTypeError(fmt.Sprintf("goal %d is not an object", i)))
				}

				// Extract key
				keyVal := goalObj.Get("key")
				if keyVal == nil || goja.IsUndefined(keyVal) {
					panic(runtime.NewTypeError(fmt.Sprintf("goal %d must have a 'key' property", i)))
				}

				// Extract Match function
				matchVal := goalObj.Get("Match")
				if matchVal == nil || goja.IsUndefined(matchVal) {
					panic(runtime.NewTypeError(fmt.Sprintf("goal %d must have a 'Match' function", i)))
				}

				matchFn, ok := goja.AssertFunction(matchVal)
				if !ok {
					panic(runtime.NewTypeError(fmt.Sprintf("goal %d.Match must be a function", i)))
				}

				// Create go-pabt condition
				condition := &JSCondition{
					key:     keyVal.Export(),
					matcher: matchFn,
					runtime: runtime,
				}

				// Each goal is wrapped as IConditions (a single group)
				// This means all conditions in the group must pass
				goals = append(goals, pabtpkg.IConditions{condition})
			}

			// Create the plan using pabtpkg.INew which is the non-generic version
			plan, err := pabtpkg.INew(state, goals)
			if err != nil {
				panic(runtime.NewGoError(fmt.Errorf("failed to create plan: %w", err)))
			}

			return runtime.ToValue(plan)
		})

		// newAction(name, conditions, effects, node) - Create an action (NOT registered yet)
		// Note: This does NOT register the action. You must call state.RegisterAction() explicitly.
		_ = exports.Set("newAction", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 4 {
				panic(runtime.NewTypeError("newAction requires name, conditions, effects, and node arguments"))
			}

			// Extract name
			name := call.Arguments[0].Export()
			nameStr, ok := name.(string)
			if !ok {
				panic(runtime.NewTypeError("action name must be a string"))
			}

			// Extract conditions (array)
			conditionsArray := call.Arguments[1].ToObject(runtime)
			var conditions []pabtpkg.IConditions
			if conditionsArray != nil && !goja.IsUndefined(call.Arguments[1]) {
				length := int(conditionsArray.Get("length").ToInteger())
				var conditionSlice []pabtpkg.Condition

				for i := 0; i < length; i++ {
					condVal := conditionsArray.Get(fmt.Sprintf("%d", i))
					if goja.IsUndefined(condVal) || goja.IsNull(condVal) {
						continue
					}

					condObj := condVal.ToObject(runtime)
					if condObj == nil {
						continue
					}

					// Extract key
					keyVal := condObj.Get("key")
					if keyVal == nil || goja.IsUndefined(keyVal) {
						continue
					}

					// Extract Match function
					matchVal := condObj.Get("Match")
					if matchVal == nil || goja.IsUndefined(matchVal) {
						continue
					}

					matchFn, ok := goja.AssertFunction(matchVal)
					if !ok {
						continue
					}

					// Create go-pabt condition
					condition := &JSCondition{
						key:     keyVal.Export(),
						matcher: matchFn,
						runtime: runtime,
					}

					conditionSlice = append(conditionSlice, pabtpkg.Condition(condition))
				}

				// Wrap as IConditions - all conditions in this slice must pass
				// Always initialize as empty slice (not nil) to avoid nil pointer issues
				if len(conditionSlice) > 0 {
					conditions = []pabtpkg.IConditions{conditionSlice}
				} else {
					conditions = []pabtpkg.IConditions{}
				}
			} else {
				// No conditions provided - explicitly initialize as empty slice
				conditions = []pabtpkg.IConditions{}
			}

			// Extract effects (array)
			effectsArray := call.Arguments[2].ToObject(runtime)
			var effects []pabtpkg.Effect
			if effectsArray != nil && !goja.IsUndefined(call.Arguments[2]) {
				length := int(effectsArray.Get("length").ToInteger())
				effects = make([]pabtpkg.Effect, 0, length)

				for i := 0; i < length; i++ {
					effectVal := effectsArray.Get(fmt.Sprintf("%d", i))
					if goja.IsUndefined(effectVal) || goja.IsNull(effectVal) {
						continue
					}

					effectObj := effectVal.ToObject(runtime)
					if effectObj == nil {
						continue
					}

					// Extract key
					keyVal := effectObj.Get("key")
					if keyVal == nil || goja.IsUndefined(keyVal) {
						continue
					}

					// Extract value
					valueVal := effectObj.Get("Value")
					if valueVal == nil || goja.IsUndefined(valueVal) {
						continue
					}

					// Create go-pabt effect
					effect := &JSEffect{
						key:   keyVal.Export(),
						value: valueVal.Export(),
					}

					effects = append(effects, pabtpkg.Effect(effect))
				}
			} else {
				// No effects provided - explicitly initialize as empty slice
				effects = []pabtpkg.Effect{}
			}

			// Extract node (must be a bt.Node)
			nodeVal := call.Arguments[3]
			// Try to extract the underlying bt.Node
			if nodeExport, ok := nodeVal.Export().(bt.Node); ok {
				// Create an action that wraps the node
				wrappedAction := &Action{
					Name:       nameStr,
					conditions: conditions,
					effects:    effects,
					node:       nodeExport,
				}
				return runtime.ToValue(wrappedAction)
			}

			panic(runtime.NewTypeError("node argument must be a bt.Node"))
		})
	}
}

// JSCondition implements pabtpkg.Condition interface using JavaScript Match function.
type JSCondition struct {
	key     any
	matcher goja.Callable
	runtime *goja.Runtime
}

// Key implements pabtpkg.Variable.Key().
func (c *JSCondition) Key() any {
	return c.key
}

// Match implements pabttpkg.Condition.Match(value any) bool.
// It calls the JavaScript matcher function dynamically.
func (c *JSCondition) Match(value any) bool {
	// Call the JavaScript matcher function
	result, err := c.matcher(goja.Undefined(), c.runtime.ToValue(value))

	if err != nil {
		// On error, return false (conservative)
		return false
	}

	return result.ToBoolean()
}

// JSEffect implements pabtpkg.Effect interface.
type JSEffect struct {
	key   any
	value any
}

// Key implements pabtpkg.Variable.Key().
func (e *JSEffect) Key() any {
	return e.key
}

// Value implements pabttpkg.Effect.Value().
func (e *JSEffect) Value() any {
	return e.value
}
