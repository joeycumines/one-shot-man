// Package pabt provides integration between go-pabt (Partial Ordered Behavior Trees)
// and bt module, exposing PA-BT planning as a JavaScript module "osm:pabt".
//
// PA-BT is a planning approach where actions are selected to achieve goals
// based on conditions in current state. This package bridges PA-BT planning
// with osm:bt behavior tree execution.
//
// # Architecture
//
// The pabt module follows a layered architecture:
//
//   - Go Layer: Provides the core primitives (State, Action, ActionRegistry)
//   - JavaScript Layer: Defines application types (Shapes, Sprites, Simulations)
//
// # Core Types
//
//   - State: Implements pabt.IState, backed by bt.Blackboard. Provides Variable()
//     for state queries and Actions(failed) for effect-based action filtering.
//   - Action: Implements pabt.IAction with conditions (preconditions),
//     effects (what the action achieves), and a bt.Node for execution.
//   - ActionRegistry: Thread-safe registry for actions using sync.RWMutex.
//
// # Condition Evaluation Modes
//
// The module supports two evaluation modes for conditions:
//
//   - EvalModeJavaScript: Uses Goja runtime via Bridge.RunOnLoopSync.
//     Required for complex JavaScript logic or closure state.
//   - EvalModeExpr: Uses expr-lang (github.com/expr-lang/expr) for Go-native
//     evaluation with ZERO Goja calls. 10-100x faster for simple conditions.
//
// # Usage
//
//	// Create a PA-BT state backed by a blackboard
//	bb := new(bt.Blackboard)
//	state := pabt.NewState(bb)
//
//	// Create an action with preconditions and effects
//	action := pabt.NewAction(
//	    "moveLeft",
//	    []pabtpkg.IConditions{{pabt.NewSimpleCond("canMove", func(v any) bool { return v == true })}},
//	    pabtpkg.Effects{pabt.NewSimpleEffect("x", -1)},
//	    moveLeftNode,
//	)
//	state.RegisterAction("moveLeft", action)
//
//	// Create a plan with a goal condition
//	goal := []pabtpkg.IConditions{{pabt.NewSimpleCond("atTarget", func(v any) bool { return v == true })}}
//	plan, _ := pabtpkg.INew(state, goal)
//
//	// The plan's Node() returns a bt.Node for execution
//	btNode := plan.Node()
//
// # JavaScript Integration
//
//	const pabt = require('osm:pabt');
//	const bt = require('osm:bt');
//
//	const bb = new bt.Blackboard();
//	const state = pabt.newState(bb);
//
//	// Define a condition
//	const atTarget = { key: 'x', Match: v => v === 10 };
//
//	// Define an action
//	const moveRight = pabt.newAction(
//	    'moveRight',
//	    [], // no preconditions
//	    [{ key: 'x', Value: () => 10 }], // effects
//	    bt.action(() => { state.set('x', state.get('x') + 1); return 'success'; })
//	);
//	state.RegisterAction('moveRight', moveRight);
//
//	// Create and execute plan
//	const plan = pabt.newPlan(state, [atTarget]);
//	const ticker = bt.newTicker(100, plan.node());
//
// # Thread Safety
//
// The bt.Ticker runs in a separate goroutine. JSCondition.Match is called from
// this ticker goroutine but Goja runtime is NOT thread-safe. The Bridge provides
// RunOnLoopSync to marshal calls to the event loop goroutine.
//
// For performance-critical conditions, use ExprCondition which evaluates
// entirely in Go with no Goja calls.
package pabt
