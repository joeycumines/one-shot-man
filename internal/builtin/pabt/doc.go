// Package pabt implements the Planning and Acting using Behavior Trees (PA-BT)
// algorithm, enabling autonomous agents to generate and refine reactive plans
// online.
//
// This package provides a generic, type-safe implementation of the synthesis
// and execution strategies described in "Behavior Trees in Robotics and AI:
// An Introduction" (Colledanchise and Ögren, 2018).
//
// # Overview
//
// PA-BT blends the reactivity of Behavior Trees (BTs) with the goal-directed
// nature of automated planning. Unlike classical planners that generate a
// fixed sequence of actions offline, PA-BT iteratively expands a Behavior Tree
// at runtime. It maintains a running plan and "grafts" new subtrees only when
// the current plan fails to meet a precondition.
//
// This approach ensures the agent remains responsive to environmental changes
// (reactivity) while ensuring long-term goal achievement (deliberation).
//
// # The PPA Pattern (Postcondition-Precondition-Action)
//
// The core building block of a PA-BT plan is the PPA subtree. When a
// condition C fails, the planner replaces it with a Fallback (Selector) node
// structured as:
//
//	Fallback
//	├── Check(C)                 (The Goal/Postcondition)
//	└── Sequence
//	    ├── Plan(Preconditions)  (Recursive expansion)
//	    └── Action(A)            (The Action that achieves C)
//
// This structure guarantees "Lazy Planning": the action A is only executed
// if C is not already met. If C becomes true due to external factors
// (serendipity), the Fallback succeeds immediately, skipping the action.
//
// # Integration with go-behaviortree
//
// This package is built on top of github.com/joeycumines/go-behaviortree.
// The resulting plans are standard BT nodes that can be ticked by a
// bt.Ticker. The integration involves:
//
//  1. Action.Node(): Returns a bt.Node representing the execution logic.
//  2. Plan.Node(): Returns the root bt.Node of the generated tree.
//  3. Plan.Running(): A hint for schedulers/tickers to optimize tick cycles.
//
// # Implementing the Domain
//
// To use pabt, you must implement the State[T] interface and Action types
// specific to your domain. This package uses Go generics, where T is your
// specific Condition type.
//
//  1. Define your Condition type:
//     Your conditions (e.g., predicates) must be comparable or identifiable
//     so the planner can match failed conditions to action effects.
//
//  2. Implement the State[T] interface:
//     The State serves as the "Action Library". When the planner encounters
//     a failure, it queries State.Actions(failedCondition).
//
//  3. Define Action Templates:
//     An Action[T] defines:
//     - Preconditions: What must hold true before execution.
//     - Effects: What conditions this action satisfies (Postconditions).
//     - Node: The executable behavior (bt.Node).
//
// # Algorithm Details
//
// The planner implements the standard PA-BT algorithms:
//
//   - Algorithm 5 (Execution Loop): Ticks the tree and monitors for failure.
//   - Algorithm 6 (Expansion): Backchains from failed conditions to find
//     actions in the State library. It handles multiple potential actions
//     using a Memorize(Selector) structure to maintain commitment to a
//     chosen strategy until it fails.
//   - Algorithm 7 (Conflict Resolution): Ensures that newly grafted subtrees
//     do not invalidate the preconditions of higher-priority goals. If a
//     conflict is detected, the conflicting subtree is moved leftward
//     (higher priority) to preserve logical consistency.
//
// # Usage Example
//
//	// 1. Define a concrete Condition type (e.g., a string or struct)
//	type MyCond string
//
//	// 2. Create the State (the library of actions)
//	state := NewMyState() // Implements pabt.State[MyCond]
//
//	// 3. Define the high-level Goal
//	goal := []MyCond{"DoorOpen", "RobotAtHome"}
//
//	// 4. Initialize the Planner
//	plan, err := pabt.New(state, goal)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// 5. Run the Plan using the standard behavior tree ticker
//	ticker := bt.NewTicker(context.Background(), 100*time.Millisecond, plan.Node())
//	<-ticker.Done()
//
// # Dynamic vs Static Actions
//
// Unlike systems that require strict separation between static and parametric
// actions, PA-BT relies on the State interface to resolve this. If a condition
// requires a parametric response (e.g., "AtLocation(42)"), your State
// implementation should dynamically generate or retrieve an Action instance
// configured for that specific parameter (e.g., "MoveTo(42)").
package pabt
