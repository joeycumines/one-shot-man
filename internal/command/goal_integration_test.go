package command

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/scripting"
)

// buildMockEditor compiles and returns the path to a cross-platform mock editor executable.
// The editor appends the provided content to the target file, simulating user editing.
func buildMockEditor(t *testing.T, tempDir string, content string) string {
	// Create a small Go program that writes content to the file passed as $1
	editorSource := fmt.Sprintf(`package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: %%s <file>\n", os.Args[0])
		os.Exit(1)
	}
	if err := os.WriteFile(os.Args[1], []byte(%q), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write file: %%v\n", err)
		os.Exit(1)
	}
}
`, content)

	// Write source file to temp directory
	srcFile := filepath.Join(tempDir, "editor.go")
	if err := os.WriteFile(srcFile, []byte(editorSource), 0o644); err != nil {
		t.Fatalf("failed to write editor source: %v", err)
	}

	// Compile the editor binary
	var binName string
	if runtime.GOOS == "windows" {
		binName = "editor.exe"
	} else {
		binName = "editor"
	}
	binPath := filepath.Join(tempDir, binName)
	cmd := exec.Command("go", "build", "-o", binPath, srcFile)

	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to compile editor: %v\noutput: %s", err, output)
	}

	return binPath
}

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

func TestGoalScript_MoraleImprover_LoadsAndInitializes(t *testing.T) {
	t.Parallel()

	goalRegistry := newTestGoalRegistryForGoal()
	g, err := goalRegistry.Get("morale-improver")
	if err != nil {
		t.Fatalf("failed to find morale-improver goal: %v", err)
	}

	if g.Name != "morale-improver" {
		t.Errorf("expected goal name 'morale-improver', got %q", g.Name)
	}
	if g.Category != "meta-prompting" {
		t.Errorf("expected category 'meta-prompting', got %q", g.Category)
	}

	// Verify StateVars are initialized
	expectedStateVars := []string{"originalInstructions", "failedPlan", "specificFailures"}
	for _, key := range expectedStateVars {
		if _, exists := g.StateVars[key]; !exists {
			t.Errorf("expected state variable %q to be present", key)
		}
	}

	// Verify NotableVariables is set
	if len(g.NotableVariables) != 3 {
		t.Errorf("expected 3 notable variables, got %d", len(g.NotableVariables))
	}

	var stdout, stderr bytes.Buffer
	ctx := context.Background()
	engine, err := scripting.NewEngineWithConfig(ctx, &stdout, &stderr, "morale-improver-init-test-"+t.Name(), "memory")
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

	// Verify initialization succeeded by checking mode
	currentMode := engine.GetTUIManager().GetCurrentMode()
	if currentMode == nil || currentMode.Name != g.Name {
		t.Errorf("expected mode %q, got %v", g.Name, currentMode)
	}
}

func TestGoalScript_MoraleImprover_ContextManagerCommands(t *testing.T) {
	t.Parallel()

	goalRegistry := newTestGoalRegistryForGoal()
	g, err := goalRegistry.Get("morale-improver")
	if err != nil {
		t.Fatalf("failed to find morale-improver goal: %v", err)
	}

	var stdout, stderr bytes.Buffer
	ctx := context.Background()
	engine, err := scripting.NewEngineWithConfig(ctx, &stdout, &stderr, "morale-improver-ctx-test-"+t.Name(), "memory")
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

	// Test note command (not add - add requires actual files)
	stdout.Reset()
	if err := engine.GetTUIManager().ExecuteCommand("note", []string{"This", "is", "a", "test", "note"}); err != nil {
		t.Fatalf("note command failed: %v; stdout=%s stderr=%s", err, stdout.String(), stderr.String())
	}

	// Test list command
	stdout.Reset()
	if err := engine.GetTUIManager().ExecuteCommand("list", []string{}); err != nil {
		t.Fatalf("list command failed: %v; stdout=%s stderr=%s", err, stdout.String(), stderr.String())
	}
	listOut := stdout.String()
	if !strings.Contains(listOut, "note") {
		t.Errorf("expected list output to contain 'note', got:\n%s", listOut)
	}

	// Test diff command exists
	stdout.Reset()
	if err := engine.GetTUIManager().ExecuteCommand("diff", []string{}); err != nil {
		t.Fatalf("diff command failed: %v; stdout=%s stderr=%s", err, stdout.String(), stderr.String())
	}

	// Test remove command on the note we added
	stdout.Reset()
	if err := engine.GetTUIManager().ExecuteCommand("remove", []string{"1"}); err != nil {
		t.Fatalf("remove command failed: %v; stdout=%s stderr=%s", err, stdout.String(), stderr.String())
	}

	// Verify removal
	stdout.Reset()
	if err := engine.GetTUIManager().ExecuteCommand("list", []string{}); err != nil {
		t.Fatalf("list command after removal failed: %v; stdout=%s stderr=%s", err, stdout.String(), stderr.String())
	}
	listOut2 := stdout.String()
	// After removal, list should be empty or not contain the removed item
	if strings.Contains(listOut2, "This is a test note") {
		t.Errorf("expected note to be removed from list, got:\n%s", listOut2)
	}
}

func TestGoalScript_MoraleImprover_StateVariableCommands(t *testing.T) {
	t.Parallel()

	goalRegistry := newTestGoalRegistryForGoal()
	g, err := goalRegistry.Get("morale-improver")
	if err != nil {
		t.Fatalf("failed to find morale-improver goal: %v", err)
	}

	var stdout, stderr bytes.Buffer
	ctx := context.Background()
	engine, err := scripting.NewEngineWithConfig(ctx, &stdout, &stderr, "morale-improver-state-test-"+t.Name(), "memory")
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

	// Test set-original command
	stdout.Reset()
	if err := engine.GetTUIManager().ExecuteCommand("set-original", []string{"Complete", "all", "integration", "tests"}); err != nil {
		t.Fatalf("set-original command failed: %v; stdout=%s stderr=%s", err, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "successfully") {
		t.Errorf("expected success message for set-original, got: %s", stdout.String())
	}

	// Test set-plan command
	stdout.Reset()
	if err := engine.GetTUIManager().ExecuteCommand("set-plan", []string{"1.", "Write", "tests", "2.", "Run", "tests"}); err != nil {
		t.Fatalf("set-plan command failed: %v; stdout=%s stderr=%s", err, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "successfully") {
		t.Errorf("expected success message for set-plan, got: %s", stdout.String())
	}

	// Test set-failures command
	stdout.Reset()
	if err := engine.GetTUIManager().ExecuteCommand("set-failures", []string{"Missing", "diff", "command,", "no", "integration", "tests"}); err != nil {
		t.Fatalf("set-failures command failed: %v; stdout=%s stderr=%s", err, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "successfully") {
		t.Errorf("expected success message for set-failures, got: %s", stdout.String())
	}

	// Verify state variables are accessible in prompt via show
	stdout.Reset()
	if err := engine.GetTUIManager().ExecuteCommand("show", []string{}); err != nil {
		t.Fatalf("show command failed: %v; stdout=%s stderr=%s", err, stdout.String(), stderr.String())
	}
	showOut := stdout.String()
	if !strings.Contains(showOut, "Complete all integration tests") {
		t.Errorf("expected show output to contain original instructions, got:\n%s", showOut)
	}
	if !strings.Contains(showOut, "1. Write tests 2. Run tests") {
		t.Errorf("expected show output to contain failed plan, got:\n%s", showOut)
	}
	if !strings.Contains(showOut, "Missing diff command, no integration tests") {
		t.Errorf("expected show output to contain specific failures, got:\n%s", showOut)
	}
}

func TestGoalScript_MoraleImprover_PromptTemplateRendering(t *testing.T) {
	t.Parallel()

	goalRegistry := newTestGoalRegistryForGoal()
	g, err := goalRegistry.Get("morale-improver")
	if err != nil {
		t.Fatalf("failed to find morale-improver goal: %v", err)
	}

	var stdout, stderr bytes.Buffer
	ctx := context.Background()
	engine, err := scripting.NewEngineWithConfig(ctx, &stdout, &stderr, "morale-improver-template-test-"+t.Name(), "memory")
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

	// Add some context
	if err := engine.GetTUIManager().ExecuteCommand("note", []string{"Failed", "to", "implement", "feature"}); err != nil {
		t.Fatalf("note command failed: %v", err)
	}

	// Set state variables
	if err := engine.GetTUIManager().ExecuteCommand("set-original", []string{"Implement", "complete", "feature"}); err != nil {
		t.Fatalf("set-original command failed: %v", err)
	}

	stdout.Reset()
	if err := engine.GetTUIManager().ExecuteCommand("show", []string{}); err != nil {
		t.Fatalf("show command failed: %v; stdout=%s stderr=%s", err, stdout.String(), stderr.String())
	}

	out := stdout.String()

	// Verify prompt contains key elements
	if !strings.Contains(out, "GENERATE A DERISIVE PROMPT TO FORCE TASK COMPLETION") {
		t.Errorf("expected prompt to contain uppercased description, got:\n%s", out)
	}

	// Verify prompt instructions are present
	if !strings.Contains(out, "motivate") && !strings.Contains(out, "moron") {
		t.Errorf("expected prompt instructions present, got:\n%s", out)
	}

	// Verify context is present
	if !strings.Contains(out, "Failed to implement feature") {
		t.Errorf("expected context note in prompt, got:\n%s", out)
	}

	// Verify state variable section appears
	if !strings.Contains(out, "ORIGINAL INSTRUCTIONS THEY IGNORED") {
		t.Errorf("expected original instructions section, got:\n%s", out)
	}
	if !strings.Contains(out, "Implement complete feature") {
		t.Errorf("expected original instructions content, got:\n%s", out)
	}

	// Verify enhanced footer is present
	if !strings.Contains(out, "100% complete") || !strings.Contains(out, "ALL OF IT") {
		t.Errorf("expected enhanced footer with strong language, got:\n%s", out)
	}
}

func TestGoalScript_MoraleImprover_ErrorHandling(t *testing.T) {
	goalRegistry := newTestGoalRegistryForGoal()
	g, err := goalRegistry.Get("morale-improver")
	if err != nil {
		t.Fatalf("failed to find morale-improver goal: %v", err)
	}

	var stdout, stderr bytes.Buffer
	ctx := context.Background()
	engine, err := scripting.NewEngineWithConfig(ctx, &stdout, &stderr, "morale-improver-error-test-"+t.Name(), "memory")
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

	// Test set-original with no arguments opens editor and sets value
	stdout.Reset()
	// Build a cross-platform mock editor that provides input
	editorPath := buildMockEditor(t, t.TempDir(), "Edited original instructions")
	t.Setenv("EDITOR", editorPath)
	if err := engine.GetTUIManager().ExecuteCommand("set-original", []string{}); err != nil {
		t.Fatalf("set-original with no args failed: %v; stdout=%s stderr=%s", err, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "successfully") {
		t.Errorf("expected success message when editor provided content, got: %s", stdout.String())
	}

	// Test set-plan with no arguments opens editor and sets value
	stdout.Reset()
	editorPath = buildMockEditor(t, t.TempDir(), "1. Edited plan")
	t.Setenv("EDITOR", editorPath)
	if err := engine.GetTUIManager().ExecuteCommand("set-plan", []string{}); err != nil {
		t.Fatalf("set-plan with no args failed: %v; stdout=%s stderr=%s", err, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "successfully") {
		t.Errorf("expected success message when editor provided content, got: %s", stdout.String())
	}

	// Test set-failures with no arguments opens editor and sets value
	stdout.Reset()
	editorPath = buildMockEditor(t, t.TempDir(), "Edited specific failures")
	t.Setenv("EDITOR", editorPath)
	if err := engine.GetTUIManager().ExecuteCommand("set-failures", []string{}); err != nil {
		t.Fatalf("set-failures with no args failed: %v; stdout=%s stderr=%s", err, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "successfully") {
		t.Errorf("expected success message when editor provided content, got: %s", stdout.String())
	}
}

func TestGoalScript_MoraleImprover_TUIElements(t *testing.T) {
	t.Parallel()

	goalRegistry := newTestGoalRegistryForGoal()
	g, err := goalRegistry.Get("morale-improver")
	if err != nil {
		t.Fatalf("failed to find morale-improver goal: %v", err)
	}

	// Verify TUI configuration
	if g.TUITitle != "The Beatings Will Continue Until Morale Improves" {
		t.Errorf("expected specific TUI title, got %q", g.TUITitle)
	}
	if g.TUIPrompt != "(morale-improver) > " {
		t.Errorf("expected specific TUI prompt, got %q", g.TUIPrompt)
	}
	if g.HistoryFile != ".morale-improver_history" {
		t.Errorf("expected specific history file, got %q", g.HistoryFile)
	}
	if !g.EnableHistory {
		t.Error("expected history to be enabled")
	}

	var stdout, stderr bytes.Buffer
	ctx := context.Background()
	engine, err := scripting.NewEngineWithConfig(ctx, &stdout, &stderr, "morale-improver-tui-test-"+t.Name(), "memory")
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
}
