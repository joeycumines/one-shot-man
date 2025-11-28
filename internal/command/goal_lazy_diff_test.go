package command

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/scripting"
)

// Reproduction test: ensure 'diff HEAD' works in goal.js driven modes (e.g. test-generator)
func TestGoalScript_DiffHeadDoesNotThrowSyntaxError(t *testing.T) {
	t.Parallel()

	goalRegistry := newTestGoalRegistryForGoal()

	// Find the built-in test-generator goal
	g, err := goalRegistry.Get("test-generator")
	if err != nil {
		t.Fatalf("failed to find test-generator goal: %v", err)
	}

	var stdout, stderr bytes.Buffer
	ctx := context.Background()
	// Use a per-test session id to avoid collisions when tests run in parallel
	engine, err := scripting.NewEngineWithConfig(ctx, &stdout, &stderr, "goal-lazy-diff-test-"+t.Name(), "memory")
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}
	defer engine.Close()
	engine.SetTestMode(true)

	// Inject GOAL_CONFIG and the generic interpreter script
	cfgjson, err := json.Marshal(g)
	if err != nil {
		t.Fatalf("failed to marshal goal config: %v", err)
	}
	script := engine.LoadScriptFromString(g.Name, "var GOAL_CONFIG = "+string(cfgjson)+";\n\n"+g.Script)
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("Failed to execute goal script: %v; stdout=%s stderr=%s", err, stdout.String(), stderr.String())
	}

	// Switch into the goal mode so it's initialized
	if err := engine.GetTUIManager().SwitchMode(g.Name); err != nil {
		t.Fatalf("Failed to switch mode: %v", err)
	}

	// Invoke the diff command on the mode using the Go-side TUI manager
	// Run the diff command using the Go-side TUI manager (runs JS handler under the hood)
	if err := engine.GetTUIManager().ExecuteCommand("diff", []string{"HEAD"}); err != nil {
		t.Fatalf("diff HEAD raised error: %v; stdout=%s stderr=%s", err, stdout.String(), stderr.String())
	}

	out := stdout.String()
	if !strings.Contains(out, "Added diff") {
		t.Fatalf("expected 'Added diff' message in output, got: %s", out)
	}
}
