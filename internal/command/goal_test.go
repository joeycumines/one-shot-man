package command

import (
	"bytes"
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/config"
)

func newTestGoalRegistryForGoal() GoalRegistry {
	cfg := config.NewConfig()
	// Avoid picking up user/system goals from standard paths during tests
	cfg.SetGlobalOption("goal.disable-standard-paths", "true")
	discovery := NewGoalDiscovery(cfg)
	return NewDynamicGoalRegistry(GetBuiltInGoals(), discovery)
}

func TestGoalCommand_Name(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	goalRegistry := newTestGoalRegistryForGoal()
	cmd := NewGoalCommand(cfg, goalRegistry)
	if cmd.Name() != "goal" {
		t.Errorf("Expected name 'goal', got %s", cmd.Name())
	}
}

func TestGoalCommand_Description(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	goalRegistry := newTestGoalRegistryForGoal()
	cmd := NewGoalCommand(cfg, goalRegistry)
	expected := "Access pre-written goals for common development tasks"
	if cmd.Description() != expected {
		t.Errorf("Expected description %q, got %q", expected, cmd.Description())
	}
}

func TestGoalCommand_Usage(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	goalRegistry := newTestGoalRegistryForGoal()
	cmd := NewGoalCommand(cfg, goalRegistry)
	expected := "goal [options] [goal-name]"
	if cmd.Usage() != expected {
		t.Errorf("Expected usage %q, got %q", expected, cmd.Usage())
	}
}

func TestGoalCommand_ListGoals(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	goalRegistry := newTestGoalRegistryForGoal()
	cmd := NewGoalCommand(cfg, goalRegistry)
	var stdout, stderr bytes.Buffer
	cmd.list = true

	// keep test runs isolated from real session storage
	cmd.storageBackend = "memory"
	cmd.session = t.Name()

	err := cmd.Execute([]string{}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	output := stdout.String()

	// Check for expected goals
	expectedGoals := []string{
		"comment-stripper",
		"doc-generator",
		"test-generator",
		"commit-message",
	}

	for _, goal := range expectedGoals {
		if !strings.Contains(output, goal) {
			t.Errorf("Expected output to contain %q, got: %s", goal, output)
		}
	}

	// Check for category sections
	expectedCategories := []string{
		"Code Refactoring:",
		"Documentation:",
		"Testing:",
		"Git Workflow:",
	}

	for _, category := range expectedCategories {
		if !strings.Contains(output, category) {
			t.Errorf("Expected output to contain category %q, got: %s", category, output)
		}
	}
}

func TestGoalCommand_ListGoalsByCategory(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	goalRegistry := newTestGoalRegistryForGoal()
	cmd := NewGoalCommand(cfg, goalRegistry)

	var stdout, stderr bytes.Buffer
	cmd.list = true
	cmd.category = "testing"

	// keep test runs isolated from real session storage
	cmd.storageBackend = "memory"
	cmd.session = t.Name()

	err := cmd.Execute([]string{}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	output := stdout.String()

	// Should contain test-generator but not others
	if !strings.Contains(output, "test-generator") {
		t.Errorf("Expected output to contain test-generator when filtering by testing category")
	}

	if strings.Contains(output, "comment-stripper") {
		t.Errorf("Expected output to NOT contain comment-stripper when filtering by testing category")
	}

	if strings.Contains(output, "doc-generator") {
		t.Errorf("Expected output to NOT contain doc-generator when filtering by testing category")
	}
}

func TestGoalCommand_GetAvailableGoals(t *testing.T) {
	t.Parallel()
	goalRegistry := newTestGoalRegistryForGoal()

	goals := goalRegistry.GetAllGoals()

	if len(goals) == 0 {
		t.Error("Expected at least one goal to be available")
	}

	// Verify all goals have required fields
	for _, goal := range goals {
		if goal.Name == "" {
			t.Error("Goal should have a name")
		}
		if goal.Description == "" {
			t.Error("Goal should have a description")
		}
		if goal.Category == "" {
			t.Error("Goal should have a category")
		}
		if goal.Script == "" {
			t.Error("Goal should have a script")
		}
		if goal.FileName == "" {
			t.Error("Goal should have a filename")
		}
	}

	// Check for expected goals
	goalNames := make(map[string]bool)
	for _, goal := range goals {
		goalNames[goal.Name] = true
	}

	expectedGoals := []string{"comment-stripper", "doc-generator", "test-generator", "commit-message"}
	for _, expected := range expectedGoals {
		if !goalNames[expected] {
			t.Errorf("Expected goal %q to be available", expected)
		}
	}
}

func TestGoalCommand_EmbeddedScripts(t *testing.T) {
	t.Parallel()
	// Test that embedded script is non-empty
	if len(goalScript) == 0 {
		t.Error("Expected goalScript to be non-empty")
	}

	// Test script structure - should be a generic interpreter
	// that reads GOAL_CONFIG from Go

	// Should check for GOAL_CONFIG injection
	if !strings.Contains(goalScript, "GOAL_CONFIG") {
		t.Error("Expected goalScript to reference GOAL_CONFIG")
	}

	// Should contain buildPrompt function
	if !strings.Contains(goalScript, "function buildPrompt") {
		t.Error("Expected goalScript to contain buildPrompt function")
	}

	// Should contain tui.registerMode
	if !strings.Contains(goalScript, "tui.registerMode") {
		t.Error("Expected goalScript to contain mode registration")
	}

	// Should contain contextManager usage
	if !strings.Contains(goalScript, "contextManager") {
		t.Error("Expected goalScript to use contextManager")
	}

	// Should be generic and NOT contain goal-specific hardcoded strings
	// (This validates that we've moved logic to Go)
	goalSpecificStrings := []string{
		"Remove useless comments",
		"Generate comprehensive documentation",
		"Generate comprehensive test suites",
		"Kubernetes-style commit",
		"CODE TO DOCUMENT",
		"CODE TO ANALYZE",
		"CODE TO TEST",
		"DIFF CONTEXT",
		"commit-message",
		"doc-generator",
		"test-generator",
		"comment-stripper",
	}
	for _, str := range goalSpecificStrings {
		if strings.Contains(goalScript, str) {
			t.Errorf("Expected goalScript to NOT contain goal-specific string %q (should be in Go)", str)
		}
	}

	// Should NOT contain eval() - security risk
	if strings.Contains(goalScript, "eval(") {
		t.Error("Expected goalScript to NOT use eval() for security reasons")
	}

	// Should NOT contain hardcoded if/else for goal names
	if strings.Contains(goalScript, `config.Name === "commit-message"`) ||
		strings.Contains(goalScript, `config.Name === "doc-generator"`) {
		t.Error("Expected goalScript to NOT contain hardcoded goal name conditionals")
	}
}

func TestGoalCommand_UnknownGoal(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	goalRegistry := newTestGoalRegistryForGoal()
	cmd := NewGoalCommand(cfg, goalRegistry)

	var stdout, stderr bytes.Buffer

	// keep test runs isolated from real session storage
	cmd.storageBackend = "memory"
	cmd.session = t.Name()

	err := cmd.Execute([]string{"nonexistent-goal"}, &stdout, &stderr)
	if err == nil {
		t.Error("Expected error when running nonexistent goal")
	}

	if !strings.Contains(stderr.String(), "Goal 'nonexistent-goal' not found") {
		t.Errorf("Expected error message about goal not found, got: %s", stderr.String())
	}
}

func TestGoalCommand_RunGoal_Success_NonInteractive(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	goalRegistry := newTestGoalRegistryForGoal()
	cmd := NewGoalCommand(cfg, goalRegistry)
	// use -r to imply non-interactive
	cmd.run = "comment-stripper"
	cmd.testMode = true // avoid launching TUI; still executes script

	var stdout, stderr bytes.Buffer
	// avoid polluting real session storage
	cmd.storageBackend = "memory"
	cmd.session = t.Name()
	err := cmd.Execute(nil, &stdout, &stderr)
	if err != nil {
		t.Fatalf("expected success running goal non-interactively, got: %v, stderr=%s", err, stderr.String())
	}
	// In testMode (non-interactive), scripts execute and register modes via ctx.run,
	// which logs sub-test pass messages. We should NOT auto-switch modes.
	got := stdout.String()
	if !strings.Contains(got, "Sub-test register-mode passed") {
		t.Errorf("expected output to include sub-test registration pass, got: %s", got)
	}
	if strings.Contains(got, "Switched to mode:") {
		t.Errorf("did not expect to switch modes in non-interactive run, got: %s", got)
	}
}

func TestGoalCommand_RunGoal_Success_Interactive_Positional(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	goalRegistry := newTestGoalRegistryForGoal()
	cmd := NewGoalCommand(cfg, goalRegistry)
	// positional argument should default to interactive per README
	// but we set testMode to avoid actually running the TUI
	cmd.testMode = true

	var stdout, stderr bytes.Buffer
	// avoid polluting real session storage
	cmd.storageBackend = "memory"
	cmd.session = t.Name()
	err := cmd.Execute([]string{"doc-generator"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("expected success running positional goal, got: %v, stderr=%s", err, stderr.String())
	}
	got := stdout.String()
	// We expect evidence of entering the mode: either explicit switch message or banner/help.
	if !strings.Contains(got, "Type 'help' for commands.") {
		t.Errorf("expected to enter doc-generator mode, got: %s", got)
	}
}

func TestGoalCommand_GoToJSPipeline_PromptTemplate(t *testing.T) {
	t.Parallel()
	// Verify that PromptTemplate from Go is correctly used by JS
	goalRegistry := newTestGoalRegistryForGoal()

	goals := goalRegistry.GetAllGoals()

	// Check that all goals have non-empty PromptTemplate
	for _, goal := range goals {
		if goal.PromptTemplate == "" {
			t.Errorf("Goal %q has empty PromptTemplate", goal.Name)
		}

		// Verify template contains expected placeholders
		if !strings.Contains(goal.PromptTemplate, "{{.description") {
			t.Errorf("Goal %q PromptTemplate missing {{.description}} placeholder", goal.Name)
		}
		if !strings.Contains(goal.PromptTemplate, "{{.promptInstructions}}") {
			t.Errorf("Goal %q PromptTemplate missing {{.promptInstructions}} placeholder", goal.Name)
		}
		if !strings.Contains(goal.PromptTemplate, "{{.contextHeader}}") {
			t.Errorf("Goal %q PromptTemplate missing {{.contextHeader}} placeholder", goal.Name)
		}
		if !strings.Contains(goal.PromptTemplate, "{{.contextTxtar}}") {
			t.Errorf("Goal %q PromptTemplate missing {{.contextTxtar}} placeholder", goal.Name)
		}
	}
}

func TestGoalCommand_GoToJSPipeline_ContextHeader(t *testing.T) {
	t.Parallel()
	// Verify that ContextHeader from Go is correctly set
	goalRegistry := newTestGoalRegistryForGoal()

	goals := goalRegistry.GetAllGoals()

	expectedHeaders := map[string]string{
		"commit-message":   "DIFF CONTEXT / CHANGES",
		"doc-generator":    "CODE TO DOCUMENT",
		"test-generator":   "CODE TO TEST",
		"comment-stripper": "CODE TO ANALYZE",
	}

	for _, goal := range goals {
		expected, ok := expectedHeaders[goal.Name]
		if !ok {
			t.Errorf("Test missing expected ContextHeader for goal %q", goal.Name)
			continue
		}
		if goal.ContextHeader != expected {
			t.Errorf("Goal %q has ContextHeader %q, expected %q", goal.Name, goal.ContextHeader, expected)
		}
	}
}

func TestGoalCommand_GoToJSPipeline_CommandsArray(t *testing.T) {
	t.Parallel()
	// Verify that Commands array is properly defined for all goals
	goalRegistry := newTestGoalRegistryForGoal()

	goals := goalRegistry.GetAllGoals()

	for _, goal := range goals {
		if len(goal.Commands) == 0 {
			t.Errorf("Goal %q has no Commands defined", goal.Name)
		}

		// Verify each command has required fields
		for _, cmdConfig := range goal.Commands {
			if cmdConfig.Name == "" {
				t.Errorf("Goal %q has command with empty Name", goal.Name)
			}
			if cmdConfig.Type == "" {
				t.Errorf("Goal %q has command %q with empty Type", goal.Name, cmdConfig.Name)
			}

			// Custom commands must have a Handler
			if cmdConfig.Type == "custom" && cmdConfig.Handler == "" {
				t.Errorf("Goal %q has custom command %q with empty Handler", goal.Name, cmdConfig.Name)
			}
		}
	}
}

func TestGoalCommand_BuiltInGoal_NotableVariablesAndTypeState(t *testing.T) {
	t.Parallel()

	goals := GetBuiltInGoals()

	var tg *Goal
	for i := range goals {
		if goals[i].Name == "test-generator" {
			tg = &goals[i]
			break
		}
	}

	if tg == nil {
		t.Fatalf("test-generator goal not found in built-ins")
	}

	// NotableVariables must exist and include the expected entries
	foundType := false
	foundFramework := false
	for _, v := range tg.NotableVariables {
		if v == "type" {
			foundType = true
		}
		if v == "framework" {
			foundFramework = true
		}
	}
	if !foundType || !foundFramework {
		t.Fatalf("test-generator NotableVariables expected to contain 'type' and 'framework', got: %v", tg.NotableVariables)
	}

	// Ensure the stateKeys map uses "type" (not the old "testType") and has default "unit"
	if _, ok := tg.StateVars["testType"]; ok {
		t.Fatalf("unexpected legacy key 'testType' found in stateVars")
	}

	v, ok := tg.StateVars["type"]
	if !ok {
		t.Fatalf("expected stateVars to contain 'type', got keys: %v", tg.StateVars)
	}
	if sv, ok := v.(string); !ok || sv != "unit" {
		t.Fatalf("expected default stateVars['type'] == 'unit', got: %#v", v)
	}
}

func TestGoalCommand_BannerIncludesNotableVariablesOnEnter(t *testing.T) {
	t.Parallel()

	cfg := config.NewConfig()
	goalRegistry := newTestGoalRegistryForGoal()
	cmd := NewGoalCommand(cfg, goalRegistry)

	var stdout, stderr bytes.Buffer
	// positional arg implies interactive; use testMode to avoid starting an actual TUI
	cmd.testMode = true

	// keep test isolated
	cmd.storageBackend = "memory"
	cmd.session = t.Name()

	err := cmd.Execute([]string{"test-generator"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("expected success running positional test-generator, got: %v, stderr=%s", err, stderr.String())
	}

	got := stdout.String()
	// Banner should include the TUITitle and the notable variables
	if !strings.Contains(got, "Test Generator") {
		t.Errorf("expected banner to include 'Test Generator' title, got: %s", got)
	}
	if !strings.Contains(got, "type=unit") {
		t.Errorf("expected banner to include 'type=unit', got: %s", got)
	}
	if !strings.Contains(got, "framework=auto") {
		t.Errorf("expected banner to include 'framework=auto', got: %s", got)
	}
}
