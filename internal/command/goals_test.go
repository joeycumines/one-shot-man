package command

import (
	"bytes"
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/config"
)

func TestGoalsCommand_Name(t *testing.T) {
	cfg := config.NewConfig()
	cmd := NewGoalsCommand(cfg)
	if cmd.Name() != "goals" {
		t.Errorf("Expected name 'goals', got %s", cmd.Name())
	}
}

func TestGoalsCommand_Description(t *testing.T) {
	cfg := config.NewConfig()
	cmd := NewGoalsCommand(cfg)
	expected := "Access pre-written goals for common development tasks"
	if cmd.Description() != expected {
		t.Errorf("Expected description %q, got %q", expected, cmd.Description())
	}
}

func TestGoalsCommand_Usage(t *testing.T) {
	cfg := config.NewConfig()
	cmd := NewGoalsCommand(cfg)
	expected := "goals [options] [goal-name]"
	if cmd.Usage() != expected {
		t.Errorf("Expected usage %q, got %q", expected, cmd.Usage())
	}
}

func TestGoalsCommand_ListGoals(t *testing.T) {
	cfg := config.NewConfig()
	cmd := NewGoalsCommand(cfg)

	var stdout, stderr bytes.Buffer
	cmd.list = true

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

func TestGoalsCommand_ListGoalsByCategory(t *testing.T) {
	cfg := config.NewConfig()
	cmd := NewGoalsCommand(cfg)

	var stdout, stderr bytes.Buffer
	cmd.list = true
	cmd.category = "testing"

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

func TestGoalsCommand_GetAvailableGoals(t *testing.T) {
	cfg := config.NewConfig()
	cmd := NewGoalsCommand(cfg)

	goals := cmd.getAvailableGoals()

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

func TestGoalsCommand_EmbeddedScripts(t *testing.T) {
	// Test that embedded scripts are non-empty
	if len(commentStripperGoal) == 0 {
		t.Error("Expected commentStripperGoal to be non-empty")
	}

	if len(docGeneratorGoal) == 0 {
		t.Error("Expected docGeneratorGoal to be non-empty")
	}

	if len(testGeneratorGoal) == 0 {
		t.Error("Expected testGeneratorGoal to be non-empty")
	}

	if len(commitMessageGoal) == 0 {
		t.Error("Expected commitMessageGoal to be non-empty")
	}

	// Test script structure - should contain expected patterns
	scripts := map[string]string{
		"comment-stripper": commentStripperGoal,
		"doc-generator":    docGeneratorGoal,
		"test-generator":   testGeneratorGoal,
		"commit-message":   commitMessageGoal,
	}

	for name, script := range scripts {
		// Should contain mode registration
		if !strings.Contains(script, "tui.registerMode") {
			t.Errorf("Expected %s script to contain mode registration", name)
		}

		// Should contain GOAL_META export
		if !strings.Contains(script, "GOAL_META") {
			t.Errorf("Expected %s script to contain GOAL_META", name)
		}

		// Should contain buildPrompt function
		if !strings.Contains(script, "function buildPrompt") {
			t.Errorf("Expected %s script to contain buildPrompt function", name)
		}
	}
}

func TestGoalsCommand_InvalidGoal(t *testing.T) {
	cfg := config.NewConfig()
	cmd := NewGoalsCommand(cfg)

	var stdout, stderr bytes.Buffer

	err := cmd.Execute([]string{"nonexistent-goal"}, &stdout, &stderr)
	if err == nil {
		t.Error("Expected error when running nonexistent goal")
	}

	if !strings.Contains(stderr.String(), "Goal 'nonexistent-goal' not found") {
		t.Errorf("Expected error message about goal not found, got: %s", stderr.String())
	}
}

func TestGoalsCommand_RunGoal_Success_NonInteractive(t *testing.T) {
	cfg := config.NewConfig()
	cmd := NewGoalsCommand(cfg)
	// use -r to imply non-interactive
	cmd.run = "comment-stripper"
	cmd.testMode = true // avoid launching TUI; still executes script

	var stdout, stderr bytes.Buffer
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

func TestGoalsCommand_RunGoal_Success_Interactive_Positional(t *testing.T) {
	cfg := config.NewConfig()
	cmd := NewGoalsCommand(cfg)
	// positional argument should default to interactive per README
	// but we set testMode to avoid actually running the TUI
	cmd.testMode = true

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"doc-generator"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("expected success running positional goal, got: %v, stderr=%s", err, stderr.String())
	}
	got := stdout.String()
	// We expect evidence of entering the mode: either explicit switch message or banner/help.
	if !(strings.Contains(got, "Switched to mode: doc-generator") || strings.Contains(got, "Code Documentation Generator")) {
		t.Errorf("expected to enter doc-generator mode, got: %s", got)
	}
}
