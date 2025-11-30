package command

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/scripting"
)

// Integration tests exercising the JS interpreter end-to-end for built-in goals
func TestGoalScript_DocGenerator_PromptContainsTypeInstructions(t *testing.T) {
	t.Parallel()

	goalRegistry := newTestGoalRegistryForGoal()
	g, err := goalRegistry.Get("doc-generator")
	if err != nil {
		t.Fatalf("failed to find doc-generator goal: %v", err)
	}

	var stdout, stderr bytes.Buffer
	ctx := context.Background()
	engine, err := scripting.NewEngineWithConfig(ctx, &stdout, &stderr, "doc-gen-int-test-"+t.Name(), "memory")
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}
	defer engine.Close()
	engine.SetTestMode(true)

	cfgjson, err := json.Marshal(g)
	if err != nil {
		t.Fatalf("failed to marshal goal config: %v", err)
	}

	script := engine.LoadScriptFromString(g.Name, "var GOAL_CONFIG = "+string(cfgjson)+";\n\n"+g.Script)
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("failed to execute goal script: %v; stdout=%s stderr=%s", err, stdout.String(), stderr.String())
	}

	if err := engine.GetTUIManager().SwitchMode(g.Name); err != nil {
		t.Fatalf("failed to switch mode: %v", err)
	}

	// The 'show' command prints the built prompt — invoke it via the Go TUI manager
	if err := engine.GetTUIManager().ExecuteCommand("show", []string{}); err != nil {
		t.Fatalf("show command failed: %v; stdout=%s stderr=%s", err, stdout.String(), stderr.String())
	}

	out := stdout.String()
	if !strings.Contains(out, "Create comprehensive documentation") {
		t.Fatalf("expected prompt to contain the doc type, got:\n%s", out)
	}
	// Also ensure TypeInstructions text is injected
	if !strings.Contains(out, "Generate comprehensive documentation including:") {
		t.Fatalf("expected TypeInstructions content present in prompt, got:\n%s", out)
	}
}

func TestGoalScript_TestGenerator_PromptContainsTypeInstructions(t *testing.T) {
	t.Parallel()

	goalRegistry := newTestGoalRegistryForGoal()
	g, err := goalRegistry.Get("test-generator")
	if err != nil {
		t.Fatalf("failed to find test-generator goal: %v", err)
	}

	var stdout, stderr bytes.Buffer
	ctx := context.Background()
	engine, err := scripting.NewEngineWithConfig(ctx, &stdout, &stderr, "test-gen-int-test-"+t.Name(), "memory")
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}
	defer engine.Close()
	engine.SetTestMode(true)

	cfgjson, err := json.Marshal(g)
	if err != nil {
		t.Fatalf("failed to marshal goal config: %v", err)
	}

	script := engine.LoadScriptFromString(g.Name, "var GOAL_CONFIG = "+string(cfgjson)+";\n\n"+g.Script)
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("failed to execute goal script: %v; stdout=%s stderr=%s", err, stdout.String(), stderr.String())
	}

	if err := engine.GetTUIManager().SwitchMode(g.Name); err != nil {
		t.Fatalf("failed to switch mode: %v", err)
	}

	if err := engine.GetTUIManager().ExecuteCommand("show", []string{}); err != nil {
		t.Fatalf("show command failed: %v; stdout=%s stderr=%s", err, stdout.String(), stderr.String())
	}

	out := stdout.String()
	// Expect default type 'unit' present
	if !strings.Contains(out, "unit tests") && !strings.Contains(out, "Unit tests") {
		t.Fatalf("expected prompt to reference unit tests, got:\n%s", out)
	}
	// Ensure TypeInstructions content is present (unit-specific instructions)
	if !strings.Contains(out, "Generate comprehensive unit tests") && !strings.Contains(out, "Test all public methods") {
		t.Fatalf("expected TypeInstructions content present in test-generator prompt, got:\n%s", out)
	}
}
