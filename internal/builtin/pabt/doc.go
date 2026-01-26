// Package pabt provides integration between go-pabt (Planning-Augmented Behavior Trees)
// and the bt module, exposing PA-BT planning as a JavaScript module "osm:pabt".
//
// # What is PA-BT?
//
// PA-BT (Planning-Augmented Behavior Trees) combines the reactive execution model of
// Behavior Trees with the goal-directed planning of GOAP (Goal-Oriented Action Planning).
// Unlike traditional BTs that require explicit tree structure, PA-BT uses declarative
// action templates to automatically construct execution plans at runtime.
//
// # Action Templates: The Core Concept
//
// An action template is a declarative specification of what an action DOES, independent
// of when or how it will be used. Each template consists of three components:
//
//  1. Preconditions: Constraints that must be satisfied BEFORE the action can execute
//  2. Effects: Changes to state variables that the action ACHIEVES when successful
//  3. Node: The actual behavior tree node that implements the action's logic
//
// The planner uses these templates to construct action sequences that achieve goals.
// When a goal condition fails, the planner searches for action templates whose effects
// would satisfy that condition, then recursively plans for those actions' preconditions.
//
// Example:
//
//	Action Template: "Pick"
//	  Preconditions: [atCube == true, handsEmpty == true]
//	  Effects:       [heldItem == cube.id]
//	  Node:          bt.createLeafNode(() => /* pick up cube */)
//
//	Goal: heldItem == 1
//	  → Planner finds "Pick" (effect satisfies goal)
//	  → Plans for preconditions: atCube, handsEmpty
//	  → Recursively plans for those...
//
// This bridges BTs (reactive execution) with GOAP (declarative planning).
//
// # Static vs Parametric Action Templates
//
// Action templates come in two forms:
//
// Static Templates: Registered once at initialization. Suitable for:
//   - Actions with fixed parameters (Pick, Place, etc.)
//   - Small finite sets (4 blockade cubes → 4 Pick_Blockade_X actions)
//
// Parametric Templates: Generated dynamically via ActionGenerator. Required for:
//   - Infinite or large action spaces (MoveTo for any of 1000s of entities)
//   - Actions whose parameters depend on runtime world state
//   - TRUE parametric actions like MoveTo(entityId) where entityId is a planning parameter
//
// Example of when to use which:
//
//	Static:     Pick_Target, Deliver_Target (exactly 1 target)
//	Static:     Pick_Blockade_1..4 (4 blockades, small finite set)
//	Parametric: MoveTo(entityId) (unlimited entities, parameter chosen at planning time)
//
// # ActionGenerator: TRUE Parametric Actions
//
// The ActionGenerator callback enables TRUE parametric actions by generating action
// templates on-demand based on the failed condition. When the planner needs actions
// to satisfy a condition like "atEntity_42", it calls the generator with that
// condition, and the generator returns action templates for relevant entities.
//
//	state.SetActionGenerator(func(failed pabtpkg.Condition) ([]pabtpkg.IAction, error) {
//	    key := failed.Key()
//	    if key == "atEntity_42" {
//	        // Generate MoveTo actions for entity 42
//	        return []pabtpkg.IAction{createMoveToAction(42)}, nil
//	    }
//	    return nil, nil
//	})
//
// The generator is called from the bt.Ticker goroutine. If it accesses JavaScript
// state, it MUST use Bridge.RunOnLoopSync for thread-safe access.
//
// # Architecture: Go vs JavaScript Layers
//
// This package deliberately keeps the Go layer minimal - it provides only the PA-BT
// primitives (State, Action, Condition, Effect). Application-specific types like
// simulation state, sprites, shapes, etc. belong in the JavaScript layer where
// they can be customized per-application.
//
//	┌─────────────────────────────────────────────────────────┐
//	│              JavaScript Layer                            │
//	│  • Application types (Actor, Cube, etc)                  │
//	│  • Action definitions (pick, place, moveTo)              │
//	│  • Blackboard sync (syncToBlackboard())                  │
//	│  • Node execution (closures mutate JS state)             │
//	└─────────────────────────────────────────────────────────┘
//	                          ▼
//	┌─────────────────────────────────────────────────────────┐
//	│                Go Layer                                  │
//	│  • pabt.State (wraps bt.Blackboard)                      │
//	│  • pabt.Action (wraps conditions, effects, bt.Node)      │
//	│  • pabt.ActionRegistry (thread-safe storage)             │
//	│  • go-pabt planning algorithm                            │
//	└─────────────────────────────────────────────────────────┘
//
// Key architectural principles:
//   - One-way sync: JavaScript → Blackboard (no reverse sync)
//   - Blackboard stores primitives only (numbers, strings, booleans)
//   - BT nodes execute in JavaScript (closures over JS state)
//   - Go layer reads primitives from blackboard for planning
//
// # Condition Evaluation Modes
//
// Conditions can be evaluated in different modes with different performance characteristics:
//
//   - JSCondition: JavaScript functions via Goja runtime. Supports complex logic and
//     closure state. Requires Bridge.RunOnLoopSync for thread safety. ~5μs per evaluation.
//
//   - ExprCondition: Go-native expr-lang evaluation with ZERO Goja calls. Compiled
//     expressions are cached. 10-100x faster than JSCondition. ~100ns per evaluation.
//     Use for simple comparisons, arithmetic, boolean logic.
//
//   - FuncCondition: Direct Go function calls. Fastest option for Go-side conditions.
//     ~50ns per evaluation.
//
// # Thread Safety
//
// The bt.Ticker runs in a separate goroutine from the event loop. All Goja runtime
// operations MUST be marshalled to the event loop goroutine via Bridge.RunOnLoopSync.
//
// This affects:
//   - JSCondition.match (called from ticker, uses RunOnLoopSync)
//   - ActionGenerator callback (if accessing JS state, must use RunOnLoopSync)
//   - Action Node execution (already runs on event loop, no special handling needed)
//
// For performance-critical conditions, prefer ExprCondition or FuncCondition which
// execute natively in Go without any Goja overhead or thread synchronization.
//
// # Usage Example
//
//	// Go layer setup
//	bb := new(bt.Blackboard)
//	state := pabt.NewState(bb)
//
//	// Register a static action template
//	pick := pabt.NewAction(
//	    "Pick",
//	    []pabtpkg.IConditions{{
//	        pabt.NewSimpleCond("atCube", func(v any) bool { return v == true }),
//	        pabt.NewSimpleCond("handsEmpty", func(v any) bool { return v == true }),
//	    }},
//	    pabtpkg.Effects{pabt.NewSimpleEffect("heldItem", 1)},
//	    pickNode,
//	)
//	state.RegisterAction("Pick", pick)
//
//	// Create a plan with a goal
//	goal := []pabtpkg.IConditions{{
//	    pabt.NewSimpleCond("heldItem", func(v any) bool { return v == 1 }),
//	}}
//	plan, _ := pabtpkg.INew(state, goal)
//
//	// Execute via behavior tree ticker
//	ticker := bt.NewTicker(context.Background(), time.Millisecond*100, plan.Node())
//
// # JavaScript Integration Example
//
//	const pabt = require('osm:pabt');
//	const bt = require('osm:bt');
//
//	// Setup
//	const bb = new bt.Blackboard();
//	const state = pabt.newState(bb);
//
//	// Static action template
//	state.registerAction('Pick', pabt.newAction('Pick',
//	    [{key: 'atCube', match: v => v === true}],  // preconditions
//	    [{key: 'heldItem', Value: 1}],              // effects
//	    bt.createLeafNode(() => { /* ... */ })      // execution node
//	));
//
//	// Parametric action template (via generator)
//	state.setActionGenerator(function(failedCondition) {
//	    const key = failedCondition.key;
//	    if (key.startsWith('atEntity_')) {
//	        const entityId = parseInt(key.replace('atEntity_', ''));
//	        return [createMoveToAction(entityId)];
//	    }
//	    return [];
//	});
//
//	// Create plan with goal
//	const plan = pabt.newPlan(state, [
//	    {key: 'targetDelivered', match: v => v === true}
//	]);
//
//	// Execute
//	const ticker = bt.newTicker(100, plan.Node());
package pabt
