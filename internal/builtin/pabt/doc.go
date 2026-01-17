// Package pabt provides integration between go-pabt (Partial Ordered Behavior Trees)
// and bt module, exposing PA-BT planning as a JavaScript module "osm:pabt".
//
// PA-BT is a planning approach where actions are selected to achieve goals
// based on conditions in current state. This package bridges PA-BT planning
// with osm:bt behavior tree execution.
//
// Architecture:
//
//   - PABTState[T] implements pabt.State[T] interface, backed by bt.Blackboard
//   - Actions are registered in ActionRegistry and expose both conditions and effects
//   - Plans are created from goals and return bt.Node for execution
//
// Usage:
//
//	// Create a PA-BT state backed by a blackboard
//	bb := new(bt.Blackboard)
//	state := pabt.NewState(bb)
//
//	// Register an action with preconditions and effects
//	// Actions are created directly via the Action struct, or via JavaScript's pabt.newAction()
//	action := &pabt.Action{
//	    Name: "moveLeft",
//	    Conditions: []pabtpkg.IConditions{
//	        {&pabtpkg.Condition{Key: "canMoveLeft", Match: func(v any) bool { return v.(bool) }}},
//	    },
//	    Effects: []pabtpkg.Effect{{Key: "x", Value: -1}},
//	    Node: moveLeftNode,
//	}
//	state.RegisterAction("moveLeft", action)
//
//	// Create a plan with a goal condition
//	goal := []pabtpkg.IConditions{
//	    {&pabtpkg.Condition{Key: "atTarget", Match: func(v any) bool { return v.(bool) }}},
//	}
//	plan, _ := pabtpkg.INew(state, goal)
//
//	// The plan's Node() returns a bt.Node that can be executed with osm:bt
//	btNode := plan.Node()
//	ticker := bt.NewTicker(ctx, 100*time.Millisecond, btNode)
//
// JavaScript Integration:
//
//	const pabt = require('osm:pabt');
//	const bb = new bt.Blackboard();
//	const state = pabt.newState(bb);
//	const plan = pabt.newPlan(state, [goalConditions]);
//	const ticker = bt.newTicker(100, plan.node());
package pabt
