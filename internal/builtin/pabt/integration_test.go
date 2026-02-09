package pabt

import (
	"context"
	"testing"
	"time"

	bt "github.com/joeycumines/go-behaviortree"
	pabtpkg "github.com/joeycumines/go-pabt"
	btmod "github.com/joeycumines/one-shot-man/internal/builtin/bt"
)

func TestPlanCreation(t *testing.T) {
	// Create a simple state
	bb := new(btmod.Blackboard)
	bb.Set("x", 0)
	bb.Set("atTarget", false)
	state := NewState(bb)

	// Create a simple action
	action := &Action{
		Name: "moveRight",
		conditions: []pabtpkg.IConditions{
			{
				&AlwaysTrueCondition{},
			},
		},
		effects: []pabtpkg.Effect{
			&SimpleEffect{key: "x", value: 1}, // Generic effect
		},
		node: bt.New(func(children []bt.Node) (bt.Status, error) {
			bb.Set("x", bb.Get("x").(int)+1)
			bb.Set("atTarget", bb.Get("x").(int) >= 5)
			return bt.Success, nil
		}),
	}
	state.RegisterAction("moveRight", action)

	// Create a goal: reach target
	goal := []pabtpkg.IConditions{
		{
			&CustomCondition{
				key:     "atTarget",
				matchFn: func(v any) bool { return v == true },
			},
		},
	}

	// Create the plan using non-generic INew
	plan, err := pabtpkg.INew(state, goal)
	if err != nil {
		t.Fatalf("Failed to create plan: %v", err)
	}

	if plan == nil {
		t.Fatal("Plan is nil")
	}
}

func TestPlanNode(t *testing.T) {
	// Test that Plan.Node() returns a bt.Node that can be used
	bb := new(btmod.Blackboard)
	bb.Set("x", 0)
	state := NewState(bb)

	// Create a simple action
	moveNode := bt.New(func([]bt.Node) (bt.Status, error) {
		bb.Set("x", bb.Get("x").(int)+1)
		return bt.Success, nil
	})

	action := &Action{
		Name: "move",
		node: moveNode,
	}

	state.RegisterAction("move", action)

	// Create a trivial plan (no goals needed for this test)
	plan, err := pabtpkg.INew(state, nil)
	if err != nil {
		t.Fatalf("Failed to create plan: %v", err)
	}

	// Get bt.Node from plan
	btNode := plan.Node()

	if btNode == nil {
		t.Fatal("Plan.Node() returned nil")
	}

	// Try to tick the node
	status, err := btNode.Tick()
	if err != nil {
		t.Errorf("Node.Tick() returned error: %v", err)
	}

	if status != bt.Failure {
		// No goals, so no action runs
		t.Logf("Node tick returned: %v", status)
	}
}

func TestPlanNodeComposition(t *testing.T) {
	// Test that Plan.Node() can be composed with other bt nodes
	bb := new(btmod.Blackboard)
	bb.Set("x", 0)
	state := NewState(bb)

	// Create actions
	move1Node := bt.New(func([]bt.Node) (bt.Status, error) {
		bb.Set("x", 1)
		return bt.Success, nil
	})

	move2Node := bt.New(func([]bt.Node) (bt.Status, error) {
		bb.Set("x", 2)
		return bt.Success, nil
	})

	action1 := &Action{Name: "move1", node: move1Node}
	action2 := &Action{Name: "move2", node: move2Node}

	state.RegisterAction("move1", action1)
	state.RegisterAction("move2", action2)

	// Create another plan
	plan, err := pabtpkg.INew(state, nil)
	if err != nil {
		t.Fatalf("Failed to create plan: %v", err)
	}

	// Compose with bt.sequence
	btNode := plan.Node()
	composed := bt.New(func([]bt.Node) (bt.Status, error) {
		// Sequence: run plan
		return btNode.Tick()
	})

	status, err := composed.Tick()
	if err != nil {
		t.Errorf("Composed node.Tick() returned error: %v", err)
	}

	t.Logf("Composed node tick returned: %v", status)
}

// AlwaysTrueCondition is a test helper - always passes.
type AlwaysTrueCondition struct{}

func (c *AlwaysTrueCondition) Key() any {
	return "always_true"
}

func (c *AlwaysTrueCondition) Match(value any) bool {
	return true
}

// CustomCondition for testing with custom match function
type CustomCondition struct {
	key     any
	matchFn func(v any) bool
}

func (c *CustomCondition) Key() any {
	return c.key
}

func (c *CustomCondition) Match(value any) bool {
	return c.matchFn(value)
}

// TestStateActions_IsCalled verifies that State.Actions is actually called during planning.
// This is a critical integration test to ensure the PA-BT planner is properly integrated.
func TestStateActions_IsCalled(t *testing.T) {
	bb := new(btmod.Blackboard)
	bb.Set("done", false)
	state := NewState(bb)

	// Track whether actions were called
	actionsCallCount := 0
	var originalActions = state.actions
	_ = originalActions // suppress unused warning - we're using the registry

	// Create an action that sets done=true
	moveNode := bt.New(func([]bt.Node) (bt.Status, error) {
		bb.Set("done", true)
		return bt.Success, nil
	})

	action := NewAction("markDone",
		[]pabtpkg.IConditions{}, // no preconditions
		[]pabtpkg.Effect{NewJSEffect("done", true)},
		moveNode,
	)
	state.RegisterAction("markDone", action)

	// Goal: done == true
	goal := []pabtpkg.IConditions{
		{NewFuncCondition("done", func(v any) bool {
			return v == true
		})},
	}

	// Create the plan
	plan, err := pabtpkg.INew(state, goal)
	if err != nil {
		t.Fatalf("Failed to create plan: %v", err)
	}

	btNode := plan.Node()
	if btNode == nil {
		t.Fatal("Plan.Node() returned nil")
	}

	// Use a ticker to execute the plan
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ticker := bt.NewTicker(ctx, 50*time.Millisecond, btNode)

	// Wait for completion
	select {
	case <-ticker.Done():
		if err := ticker.Err(); err != nil && err != context.Canceled && err != context.DeadlineExceeded {
			t.Fatalf("Ticker error: %v", err)
		}
	case <-ctx.Done():
		t.Log("Context deadline reached")
	}

	// Verify the action was executed
	doneVal := bb.Get("done")
	if doneVal != true {
		t.Errorf("Expected done=true, got %v", doneVal)
	}

	t.Logf("Plan executed successfully, actionsCallCount=%d, done=%v", actionsCallCount, doneVal)
}
