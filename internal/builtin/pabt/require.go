package pabt

import (
	"context"
	"fmt"
	"log/slog"

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
//
// The bridge parameter is required for thread-safe goja.Runtime access.
// JSCondition.Match is called from the bt.Ticker goroutine and must use
// Bridge.RunOnLoopSync to marshal calls to the event loop goroutine.
func ModuleLoader(ctx context.Context, bridge *btmod.Bridge) require.ModuleLoader {
	return func(runtime *goja.Runtime, module *goja.Object) {
		exports := module.Get("exports").(*goja.Object)

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

			// Create a JavaScript object with proper method exposure
			jsObj := runtime.NewObject()

			// Expose Variable method as 'variable'
			_ = jsObj.Set("variable", func(call goja.FunctionCall) goja.Value {
				if len(call.Arguments) < 1 {
					panic(runtime.NewTypeError("variable requires a key argument"))
				}
				key := call.Arguments[0].Export()
				value, err := state.Variable(key)
				if err != nil {
					panic(runtime.NewGoError(err))
				}
				return runtime.ToValue(value)
			})

			// Expose Blackboard Get method as 'get'
			_ = jsObj.Set("get", func(call goja.FunctionCall) goja.Value {
				if len(call.Arguments) < 1 {
					panic(runtime.NewTypeError("get requires a key argument"))
				}
				key := call.Arguments[0].String()
				return runtime.ToValue(state.Blackboard.Get(key))
			})

			// Expose Blackboard Set method as 'set'
			_ = jsObj.Set("set", func(call goja.FunctionCall) goja.Value {
				if len(call.Arguments) < 2 {
					panic(runtime.NewTypeError("set requires key and value arguments"))
				}
				key := call.Arguments[0].String()
				value := call.Arguments[1].Export()
				state.Blackboard.Set(key, value)
				return goja.Undefined()
			})

			// Expose RegisterAction method
			registerActionFn := func(call goja.FunctionCall) goja.Value {
				if len(call.Arguments) < 2 {
					panic(runtime.NewTypeError("registerAction requires name and action arguments"))
				}
				name := call.Arguments[0].String()
				actionVal := call.Arguments[1].Export()

				action, ok := actionVal.(*Action)
				if !ok {
					panic(runtime.NewTypeError("action must be created via pabt.newAction()"))
				}

				state.RegisterAction(name, action)
				return goja.Undefined()
			}
			_ = jsObj.Set("RegisterAction", registerActionFn)
			_ = jsObj.Set("registerAction", registerActionFn) // lowercase alias for JS convention

			// Expose GetAction to retrieve registered actions by name
			// This allows the action generator to return pre-registered actions
			getActionFn := func(call goja.FunctionCall) goja.Value {
				if len(call.Arguments) < 1 {
					panic(runtime.NewTypeError("GetAction requires a name argument"))
				}
				name := call.Arguments[0].String()
				action := state.actions.Get(name)
				if action == nil {
					return goja.Null()
				}
				return runtime.ToValue(action)
			}
			_ = jsObj.Set("GetAction", getActionFn)
			_ = jsObj.Set("getAction", getActionFn) // lowercase alias for JS convention

			// Expose setActionGenerator for TRUE parametric actions
			// The generator function receives (failedCondition) and returns an array of actions
			setActionGeneratorFn := func(call goja.FunctionCall) goja.Value {
				if len(call.Arguments) < 1 {
					panic(runtime.NewTypeError("setActionGenerator requires a generator function argument"))
				}

				// Allow null/undefined to clear the generator
				if goja.IsNull(call.Arguments[0]) || goja.IsUndefined(call.Arguments[0]) {
					state.SetActionGenerator(nil)
					return goja.Undefined()
				}

				genFn, ok := goja.AssertFunction(call.Arguments[0])
				if !ok {
					panic(runtime.NewTypeError("generator must be a function"))
				}

				// Create a Go ActionGeneratorFunc that calls the JS function
				generator := func(failed pabtpkg.Condition) ([]pabtpkg.IAction, error) {
					var actions []pabtpkg.IAction
					var genErr error

					// CRITICAL: Must use RunOnLoopSync for thread-safe goja access
					err := bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
						// Pass the original JS object back unchanged if available.
						// This preserves ALL properties (including .value) for action templating,
						// equivalent to Go's type assertion for accessing internal state.
						var condObj *goja.Object
						if jsCond, ok := failed.(*JSCondition); ok && jsCond.jsObject != nil {
							// Return the SAME JS object - preserves .value and all properties
							condObj = jsCond.jsObject
						} else if failed != nil {
							// Fallback: create minimal wrapper for non-JS conditions
							condObj = vm.NewObject()
							_ = condObj.Set("key", failed.Key())
							_ = condObj.Set("match", func(fcall goja.FunctionCall) goja.Value {
								if len(fcall.Arguments) < 1 {
									return vm.ToValue(false)
								}
								return vm.ToValue(failed.Match(fcall.Arguments[0].Export()))
							})
						} else {
							condObj = vm.NewObject()
						}

						// Call the JS generator function
						result, callErr := genFn(goja.Undefined(), condObj)
						if callErr != nil {
							genErr = callErr
							return nil
						}

						// Parse the result array into Go actions
						if goja.IsNull(result) || goja.IsUndefined(result) {
							return nil
						}

						resultObj := result.ToObject(vm)
						if resultObj == nil {
							return nil
						}

						length := int(resultObj.Get("length").ToInteger())
						for i := 0; i < length; i++ {
							actionVal := resultObj.Get(fmt.Sprintf("%d", i))
							if goja.IsUndefined(actionVal) || goja.IsNull(actionVal) {
								continue
							}

							// Extract the Go action from the JS wrapper
							actionExport := actionVal.Export()
							if action, ok := actionExport.(*Action); ok {
								actions = append(actions, action)
							}
						}
						return nil
					})

					if err != nil {
						return nil, err
					}
					if genErr != nil {
						return nil, genErr
					}
					return actions, nil
				}

				state.SetActionGenerator(generator)
				return goja.Undefined()
			}
			_ = jsObj.Set("setActionGenerator", setActionGeneratorFn)
			_ = jsObj.Set("SetActionGenerator", setActionGeneratorFn) // uppercase alias

			// Store native reference for interop (e.g., newPlan)
			_ = jsObj.Set("_native", state)

			return jsObj
		})

		// newPlan(state, goals) - Create a PA-BT plan
		_ = exports.Set("newPlan", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 2 {
				panic(runtime.NewTypeError("newPlan requires state and goals arguments"))
			}

			// Extract the state from JS object's _native property
			stateObj := call.Arguments[0].ToObject(runtime)
			if stateObj == nil {
				panic(runtime.NewTypeError("state must be a PABTState created via pabt.newState()"))
			}

			nativeVal := stateObj.Get("_native")
			if nativeVal == nil || goja.IsUndefined(nativeVal) {
				panic(runtime.NewTypeError("state must be a PABTState created via pabt.newState()"))
			}

			state, ok := nativeVal.Export().(*State)
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

				// Extract match function
				matchVal := goalObj.Get("match")
				if matchVal == nil || goja.IsUndefined(matchVal) {
					panic(runtime.NewTypeError(fmt.Sprintf("goal %d must have a 'match' function", i)))
				}

				matchFn, ok := goja.AssertFunction(matchVal)
				if !ok {
					panic(runtime.NewTypeError(fmt.Sprintf("goal %d.match must be a function", i)))
				}

				// Create go-pabt condition, storing original JS object for passthrough
				condition := &JSCondition{
					key:      keyVal.Export(),
					matcher:  matchFn,
					bridge:   bridge,
					jsObject: goalObj, // Store original for action generator access to .value etc
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

					// DEBUG: Log condition creation
					slog.Debug("[PA-BT COND CREATE]", "action", name, "conditionKey", keyVal.Export())

					// Extract match function
					matchVal := condObj.Get("match")
					if matchVal == nil || goja.IsUndefined(matchVal) {
						continue
					}

					matchFn, ok := goja.AssertFunction(matchVal)
					if !ok {
						continue
					}

					// Create go-pabt condition, storing original JS object for passthrough
					condition := &JSCondition{
						key:      keyVal.Export(),
						matcher:  matchFn,
						bridge:   bridge,
						jsObject: condObj, // Store original for action generator access
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

				slog.Debug("[PA-BT EFFECT PARSE] Starting effect parsing", "action", name, "effectCount", length)

				for i := 0; i < length; i++ {
					effectVal := effectsArray.Get(fmt.Sprintf("%d", i))
					if goja.IsUndefined(effectVal) || goja.IsNull(effectVal) {
						slog.Debug("[PA-BT EFFECT PARSE] Effect undefined/null", "action", name, "index", i)
						continue
					}

					effectObj := effectVal.ToObject(runtime)
					if effectObj == nil {
						slog.Debug("[PA-BT EFFECT PARSE] Effect not object", "action", name, "index", i)
						continue
					}

					// Extract key
					keyVal := effectObj.Get("key")
					if keyVal == nil || goja.IsUndefined(keyVal) {
						slog.Debug("[PA-BT EFFECT PARSE] Effect key undefined", "action", name, "index", i)
						continue
					}

					// Extract value
					valueVal := effectObj.Get("Value")
					if valueVal == nil || goja.IsUndefined(valueVal) {
						slog.Debug("[PA-BT EFFECT PARSE] Effect Value undefined", "action", name, "index", i, "key", keyVal.Export())
						continue
					}

					// Create go-pabt effect
					effect := &JSEffect{
						key:   keyVal.Export(),
						value: valueVal.Export(),
					}

					slog.Debug("[PA-BT EFFECT PARSE] Effect created", "action", name, "key", effect.key, "value", effect.value)
					effects = append(effects, pabtpkg.Effect(effect))
				}
				slog.Debug("[PA-BT EFFECT PARSE] Finished effect parsing", "action", name, "totalEffects", len(effects))
			} else {
				// No effects provided - explicitly initialize as empty slice
				effects = []pabtpkg.Effect{}
			}

			// Extract node (must be a bt.Node)
			nodeVal := call.Arguments[3]
			// Try to extract the underlying bt.Node
			if nodeExport, ok := nodeVal.Export().(bt.Node); ok {
				// Create an action using the factory function
				wrappedAction := NewAction(nameStr, conditions, effects, nodeExport)
				return runtime.ToValue(wrappedAction)
			}

			panic(runtime.NewTypeError("node argument must be a bt.Node"))
		})

		// newExprCondition(key, expression) - Create a Go-native ExprCondition (fast path)
		// This creates a condition that uses expr-lang evaluation with ZERO Goja calls.
		_ = exports.Set("newExprCondition", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 2 {
				panic(runtime.NewTypeError("newExprCondition requires key and expression arguments"))
			}

			key := call.Arguments[0].Export()
			expr := call.Arguments[1].String()

			condition := NewExprCondition(key, expr)

			// Return a JS object with key and match for compatibility with newAction
			jsObj := runtime.NewObject()
			_ = jsObj.Set("key", key)
			_ = jsObj.Set("match", func(call goja.FunctionCall) goja.Value {
				if len(call.Arguments) < 1 {
					return runtime.ToValue(false)
				}
				value := call.Arguments[0].Export()
				return runtime.ToValue(condition.Match(value))
			})
			_ = jsObj.Set("_native", condition)

			return jsObj
		})
	}
}
