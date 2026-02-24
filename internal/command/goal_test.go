package command

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/config"
)

func newTestGoalRegistryForGoal() GoalRegistry {
	cfg := config.NewConfig()
	// Avoid picking up user/system goals from standard paths during tests
	cfg.SetGlobalOption("goal.disable-standard-paths", "true")
	// Disable autodiscovery to prevent filesystem goals (e.g. goals/examples/*.json)
	// from leaking into tests when run from the project root
	cfg.SetGlobalOption("goal.autodiscovery", "false")
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
	cmd.store = "memory"
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

func TestGoalCommand_ListGoals_CustomCommandSummary(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	goalRegistry := newTestGoalRegistryForGoal()
	cmd := NewGoalCommand(cfg, goalRegistry)
	var stdout, stderr bytes.Buffer
	cmd.list = true
	cmd.store = "memory"
	cmd.session = t.Name()

	err := cmd.Execute([]string{}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	output := stdout.String()

	// Goals with custom commands should show [cmds: ...] suffix
	expectedSuffixes := map[string]string{
		"doc-generator":       "[cmds: type]",
		"test-generator":      "[cmds: type, framework]",
		"morale-improver":     "[cmds: set-original, set-plan, set-failures]",
		"implementation-plan": "[cmds: goal]",
	}
	for goalName, suffix := range expectedSuffixes {
		if !strings.Contains(output, suffix) {
			t.Errorf("Expected output to contain %q for goal %q, got:\n%s", suffix, goalName, output)
		}
	}

	// Goals without custom commands should NOT have [cmds: ...]
	goalsWithoutCustom := []string{"comment-stripper", "commit-message"}
	lines := strings.Split(output, "\n")
	for _, goalName := range goalsWithoutCustom {
		for _, line := range lines {
			if strings.Contains(line, goalName) && strings.Contains(line, "[cmds:") {
				t.Errorf("Goal %q should NOT have [cmds: ...] suffix, but found: %s", goalName, line)
			}
		}
	}
}

func TestGoalCommand_ListGoals_ParameterSummary(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	goalRegistry := newTestGoalRegistryForGoal()
	cmd := NewGoalCommand(cfg, goalRegistry)
	var stdout, stderr bytes.Buffer
	cmd.list = true
	cmd.store = "memory"
	cmd.session = t.Name()

	err := cmd.Execute([]string{}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	output := stdout.String()

	// Goals with non-nil state vars should show [vars: ...] with defaults
	expectedVars := map[string][]string{
		"doc-generator":  {"type=comprehensive"},
		"test-generator": {"framework=auto", "type=unit"},
		"code-explainer": {"depth=detailed"},
	}
	for goalName, vars := range expectedVars {
		for _, v := range vars {
			if !strings.Contains(output, v) {
				t.Errorf("Expected output to contain %q for goal %q, got:\n%s", v, goalName, output)
			}
		}
	}

	// Goals with empty or all-nil StateVars should NOT show [vars: ...]
	goalsWithoutVars := []string{"comment-stripper", "commit-message", "bug-buster", "code-optimizer", "meeting-notes"}
	lines := strings.Split(output, "\n")
	for _, goalName := range goalsWithoutVars {
		for _, line := range lines {
			if strings.Contains(line, goalName) && strings.Contains(line, "[vars:") {
				t.Errorf("Goal %q should NOT have [vars: ...] suffix, but found: %s", goalName, line)
			}
		}
	}

	// morale-improver has StateVars but all are nil — should NOT show [vars: ...]
	for _, line := range lines {
		if strings.Contains(line, "morale-improver") && strings.Contains(line, "[vars:") {
			t.Errorf("morale-improver should NOT have [vars: ...] (all nil), but found: %s", line)
		}
	}
}

func TestFormatGoalLine(t *testing.T) {
	t.Parallel()

	t.Run("NoStateVarsNoCustomCmds", func(t *testing.T) {
		goal := Goal{
			Name:        "simple",
			Description: "A simple goal",
		}
		line := formatGoalLine(goal)
		if strings.Contains(line, "[vars:") {
			t.Errorf("Expected no [vars:] suffix, got: %s", line)
		}
		if strings.Contains(line, "[cmds:") {
			t.Errorf("Expected no [cmds:] suffix, got: %s", line)
		}
		if !strings.Contains(line, "simple") || !strings.Contains(line, "A simple goal") {
			t.Errorf("Expected name and description, got: %s", line)
		}
	})

	t.Run("WithStateVars", func(t *testing.T) {
		goal := Goal{
			Name:        "test",
			Description: "Test goal",
			StateVars: map[string]interface{}{
				"mode":  "fast",
				"level": 3,
			},
		}
		line := formatGoalLine(goal)
		if !strings.Contains(line, "[vars: level=3, mode=fast]") {
			t.Errorf("Expected sorted vars summary, got: %s", line)
		}
	})

	t.Run("NilVarsSkipped", func(t *testing.T) {
		goal := Goal{
			Name:        "test",
			Description: "Test goal",
			StateVars: map[string]interface{}{
				"a": nil,
				"b": "value",
				"c": nil,
			},
		}
		line := formatGoalLine(goal)
		if !strings.Contains(line, "[vars: b=value]") {
			t.Errorf("Expected only non-nil var, got: %s", line)
		}
		if strings.Contains(line, "a=") || strings.Contains(line, "c=") {
			t.Errorf("Expected nil vars to be skipped, got: %s", line)
		}
	})

	t.Run("EmptyStringVarsSkipped", func(t *testing.T) {
		goal := Goal{
			Name:        "test",
			Description: "Test goal",
			StateVars: map[string]interface{}{
				"blank": "",
				"valid": "ok",
			},
		}
		line := formatGoalLine(goal)
		if !strings.Contains(line, "[vars: valid=ok]") {
			t.Errorf("Expected only non-empty var, got: %s", line)
		}
		if strings.Contains(line, "blank=") {
			t.Errorf("Expected empty string var to be skipped, got: %s", line)
		}
	})

	t.Run("LongValueTruncated", func(t *testing.T) {
		goal := Goal{
			Name:        "test",
			Description: "Test goal",
			StateVars: map[string]interface{}{
				"text": "This is a very long default value that should be truncated for display",
			},
		}
		line := formatGoalLine(goal)
		if !strings.Contains(line, "...") {
			t.Errorf("Expected truncated value with '...', got: %s", line)
		}
		// Truncated value should be at most 30 chars
		// Format: text=<value> where value is at most 30 chars
		start := strings.Index(line, "text=")
		if start < 0 {
			t.Fatalf("Expected 'text=' in line, got: %s", line)
		}
		valStart := start + len("text=")
		end := strings.Index(line[valStart:], "]")
		if end < 0 {
			t.Fatalf("Expected ']' after value, got: %s", line)
		}
		val := line[valStart : valStart+end]
		if len(val) > 30 {
			t.Errorf("Expected value to be at most 30 chars, got %d: %q", len(val), val)
		}
	})

	t.Run("BothVarsAndCmds", func(t *testing.T) {
		goal := Goal{
			Name:        "full",
			Description: "Full goal",
			StateVars: map[string]interface{}{
				"mode": "default",
			},
			Commands: []CommandConfig{
				{Name: "set-mode", Type: "custom"},
				{Name: "add", Type: "contextManager"},
			},
		}
		line := formatGoalLine(goal)
		if !strings.Contains(line, "[vars: mode=default]") {
			t.Errorf("Expected vars summary, got: %s", line)
		}
		if !strings.Contains(line, "[cmds: set-mode]") {
			t.Errorf("Expected cmds summary, got: %s", line)
		}
		// [vars:] should appear before [cmds:]
		varsIdx := strings.Index(line, "[vars:")
		cmdsIdx := strings.Index(line, "[cmds:")
		if varsIdx >= cmdsIdx {
			t.Errorf("Expected [vars:] before [cmds:], got: %s", line)
		}
	})

	t.Run("AllNilVarsProducesNoSuffix", func(t *testing.T) {
		goal := Goal{
			Name:        "test",
			Description: "Test goal",
			StateVars: map[string]interface{}{
				"a": nil,
				"b": nil,
			},
		}
		line := formatGoalLine(goal)
		if strings.Contains(line, "[vars:") {
			t.Errorf("Expected no [vars:] for all-nil state vars, got: %s", line)
		}
	})

	t.Run("NumericAndBoolValues", func(t *testing.T) {
		goal := Goal{
			Name:        "test",
			Description: "Test goal",
			StateVars: map[string]interface{}{
				"count":   42,
				"enabled": true,
			},
		}
		line := formatGoalLine(goal)
		if !strings.Contains(line, "count=42") {
			t.Errorf("Expected numeric var, got: %s", line)
		}
		if !strings.Contains(line, "enabled=true") {
			t.Errorf("Expected bool var, got: %s", line)
		}
	})
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
	cmd.store = "memory"
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
	cmd.store = "memory"
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
	cmd.store = "memory"
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
	cmd.store = "memory"
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
		if goal.Name != "implementation-plan" && !strings.Contains(goal.PromptTemplate, "{{.description") {
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
		"commit-message":      "DIFF CONTEXT / CHANGES",
		"doc-generator":       "CODE TO DOCUMENT",
		"test-generator":      "CODE TO TEST",
		"comment-stripper":    "CODE TO ANALYZE",
		"morale-improver":     "CONTEXT",
		"implementation-plan": "CONTEXT & REQUIREMENTS",
		"bug-buster":          "CODE TO ANALYZE",
		"code-optimizer":      "CODE TO OPTIMIZE",
		"code-explainer":      "CODE TO EXPLAIN",
		"meeting-notes":       "MEETING NOTES / TRANSCRIPT",
		"pii-scrubber":        "CONTENT TO SCRUB",
		"prose-polisher":      "PROSE TO POLISH",
		"data-to-json":        "RAW DATA / UNSTRUCTURED INPUT",
		"cite-sources":        "SOURCE MATERIAL",
		"which-one-is-better": "OPTIONS & CONTEXT",
		"sql-generator":       "SCHEMA & QUERY REQUEST",
		"report-analyzer":     "REPORT / DOCUMENT",
		"review-classifier":   "FEEDBACK / REVIEWS",
		"adaptive-editor":     "TEXT TO REWRITE",
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
	cmd.store = "memory"
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

func TestGoalHotSnippet_JSONSerialization(t *testing.T) {
	t.Parallel()

	goal := Goal{
		Name: "test",
		HotSnippets: []GoalHotSnippet{
			{Name: "ask", Text: "Ask this question.", Description: "Ask a follow-up"},
			{Name: "verify", Text: "Verify the output."},
		},
	}

	data, err := json.Marshal(goal)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	s := string(data)
	if !strings.Contains(s, `"hotSnippets"`) {
		t.Errorf("expected JSON to contain hotSnippets key, got: %s", s)
	}
	if !strings.Contains(s, `"ask"`) || !strings.Contains(s, `"verify"`) {
		t.Errorf("expected snippet names in JSON, got: %s", s)
	}

	// Roundtrip
	var decoded Goal
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if len(decoded.HotSnippets) != 2 {
		t.Fatalf("expected 2 snippets after roundtrip, got %d", len(decoded.HotSnippets))
	}
	if decoded.HotSnippets[0].Name != "ask" || decoded.HotSnippets[0].Description != "Ask a follow-up" {
		t.Errorf("unexpected first snippet: %+v", decoded.HotSnippets[0])
	}
	if decoded.HotSnippets[1].Name != "verify" || decoded.HotSnippets[1].Description != "" {
		t.Errorf("unexpected second snippet (expected empty description with omitempty): %+v", decoded.HotSnippets[1])
	}
}

func TestGoalHotSnippet_OmittedWhenEmpty(t *testing.T) {
	t.Parallel()

	goal := Goal{Name: "no-snippets"}
	data, err := json.Marshal(goal)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	if strings.Contains(string(data), "hotSnippets") {
		t.Errorf("expected hotSnippets to be omitted when empty, got: %s", string(data))
	}
}

// TestGoal_PromptFooterJSONSerialization verifies that PromptFooter
// round-trips through JSON correctly.
func TestGoal_PromptFooterJSONSerialization(t *testing.T) {
	t.Parallel()

	goal := Goal{
		Name:         "test-footer",
		PromptFooter: "Remember: output must be valid JSON.",
	}
	data, err := json.Marshal(goal)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	if !strings.Contains(string(data), `"promptFooter":"Remember: output must be valid JSON."`) {
		t.Errorf("expected promptFooter in JSON, got: %s", string(data))
	}

	var decoded Goal
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if decoded.PromptFooter != "Remember: output must be valid JSON." {
		t.Errorf("expected PromptFooter to round-trip, got: %q", decoded.PromptFooter)
	}
}

// TestGoal_PromptFooterInBuildPrompt verifies that PromptFooter is
// template-interpolated and available as {{.promptFooter}} in promptTemplate.
func TestGoal_PromptFooterInBuildPrompt(t *testing.T) {
	t.Parallel()

	// Create a minimal goal with a footer referencing state vars.
	goal := Goal{
		Name:         "footer-test",
		Description:  "Test footer interpolation",
		Category:     "testing",
		Script:       goalScript,
		FileName:     "footer-test.js",
		PromptFooter: "Format: {{index .stateKeys \"outputFormat\"}}",
		StateVars: map[string]interface{}{
			"outputFormat": "markdown",
		},
		PromptTemplate:     "Instructions\n{{.contextHeader}}\n{{.contextTxtar}}\n{{.promptInstructions}}\n---\n{{.promptFooter}}",
		PromptInstructions: "Do the thing.",
		ContextHeader:      "CONTEXT",
		Commands: []CommandConfig{
			{Name: "add", Type: "contextManager"},
			{Name: "show", Type: "contextManager"},
			{Name: "copy", Type: "contextManager"},
			{Name: "list", Type: "contextManager"},
		},
	}

	// Build a registry with just this goal
	cfg := config.NewConfig()
	cfg.SetGlobalOption("goal.disable-standard-paths", "true")
	cfg.SetGlobalOption("goal.autodiscovery", "false")
	discovery := NewGoalDiscovery(cfg)
	registry := NewDynamicGoalRegistry([]Goal{goal}, discovery)

	cmd := NewGoalCommand(cfg, registry)
	var stdout, stderr bytes.Buffer
	cmd.testMode = true
	cmd.store = "memory"
	cmd.session = t.Name()

	// Run the goal (positional arg = interactive in test mode)
	if err := cmd.Execute([]string{"footer-test"}, &stdout, &stderr); err != nil {
		t.Fatalf("execute failed: %v; stderr=%s", err, stderr.String())
	}

	// The banner should appear - basic sanity
	got := stdout.String()
	if !strings.Contains(got, "footer-test") && !strings.Contains(got, "Footer Test") {
		t.Fatalf("expected goal banner, got: %s", got)
	}
}

func TestGoalBuiltin_WhichOneIsBetter_Metadata(t *testing.T) {
	t.Parallel()

	goals := GetBuiltInGoals()

	var wib *Goal
	for i := range goals {
		if goals[i].Name == "which-one-is-better" {
			wib = &goals[i]
			break
		}
	}

	if wib == nil {
		t.Fatalf("which-one-is-better goal not found in built-ins")
	}

	// Basic metadata
	if wib.Description == "" {
		t.Error("expected non-empty description")
	}
	if wib.Category != "decision-making" {
		t.Errorf("expected category 'decision-making', got %q", wib.Category)
	}
	if wib.TUITitle == "" {
		t.Error("expected non-empty TUITitle")
	}
	if wib.TUIPrompt == "" {
		t.Error("expected non-empty TUIPrompt")
	}
	if wib.Script == "" {
		t.Error("expected non-empty Script")
	}
	if wib.FileName == "" {
		t.Error("expected non-empty FileName")
	}
	if wib.ContextHeader != "OPTIONS & CONTEXT" {
		t.Errorf("expected ContextHeader 'OPTIONS & CONTEXT', got %q", wib.ContextHeader)
	}
}

func TestGoalBuiltin_WhichOneIsBetter_StateVars(t *testing.T) {
	t.Parallel()

	goals := GetBuiltInGoals()

	var wib *Goal
	for i := range goals {
		if goals[i].Name == "which-one-is-better" {
			wib = &goals[i]
			break
		}
	}
	if wib == nil {
		t.Fatalf("which-one-is-better goal not found")
	}

	// Must have comparisonType state var with default "general"
	v, ok := wib.StateVars["comparisonType"]
	if !ok {
		t.Fatalf("expected stateVars to contain 'comparisonType'")
	}
	if sv, ok := v.(string); !ok || sv != "general" {
		t.Fatalf("expected default comparisonType='general', got %#v", v)
	}

	// NotableVariables must include comparisonType
	found := false
	for _, nv := range wib.NotableVariables {
		if nv == "comparisonType" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected NotableVariables to contain 'comparisonType', got %v", wib.NotableVariables)
	}
}

func TestGoalBuiltin_WhichOneIsBetter_PromptOptions(t *testing.T) {
	t.Parallel()

	goals := GetBuiltInGoals()

	var wib *Goal
	for i := range goals {
		if goals[i].Name == "which-one-is-better" {
			wib = &goals[i]
			break
		}
	}
	if wib == nil {
		t.Fatalf("which-one-is-better goal not found")
	}

	// PromptOptions must have comparisonTypeInstructions map
	raw, ok := wib.PromptOptions["comparisonTypeInstructions"]
	if !ok {
		t.Fatalf("expected PromptOptions to contain 'comparisonTypeInstructions'")
	}

	instructions, ok := raw.(map[string]string)
	if !ok {
		t.Fatalf("expected comparisonTypeInstructions to be map[string]string, got %T", raw)
	}

	// Must cover all 5 variants
	expectedVariants := []string{"general", "technology", "architecture", "strategy", "design"}
	for _, variant := range expectedVariants {
		if _, exists := instructions[variant]; !exists {
			t.Errorf("expected comparisonTypeInstructions to contain %q", variant)
		}
	}

	// Each variant should have non-trivial content
	for variant, text := range instructions {
		if len(text) < 50 {
			t.Errorf("variant %q has suspiciously short instructions (%d chars)", variant, len(text))
		}
	}
}

func TestGoalBuiltin_WhichOneIsBetter_Commands(t *testing.T) {
	t.Parallel()

	goals := GetBuiltInGoals()

	var wib *Goal
	for i := range goals {
		if goals[i].Name == "which-one-is-better" {
			wib = &goals[i]
			break
		}
	}
	if wib == nil {
		t.Fatalf("which-one-is-better goal not found")
	}

	// Must have set-type custom command
	var setTypeCmd *CommandConfig
	for i := range wib.Commands {
		if wib.Commands[i].Name == "set-type" {
			setTypeCmd = &wib.Commands[i]
			break
		}
	}
	if setTypeCmd == nil {
		t.Fatalf("expected 'set-type' custom command")
	}
	if setTypeCmd.Type != "custom" {
		t.Errorf("expected set-type type 'custom', got %q", setTypeCmd.Type)
	}
	if setTypeCmd.Handler == "" {
		t.Error("expected set-type to have a non-empty Handler")
	}

	// Must have standard contextManager commands
	expectedCM := []string{"add", "diff", "note", "list", "edit", "remove", "show", "copy"}
	cmdNames := make(map[string]bool)
	for _, cmd := range wib.Commands {
		cmdNames[cmd.Name] = true
	}
	for _, name := range expectedCM {
		if !cmdNames[name] {
			t.Errorf("expected contextManager command %q", name)
		}
	}

	// Must have help command
	if !cmdNames["help"] {
		t.Error("expected 'help' command")
	}
}

func TestGoalBuiltin_WhichOneIsBetter_HotSnippets(t *testing.T) {
	t.Parallel()

	goals := GetBuiltInGoals()

	var wib *Goal
	for i := range goals {
		if goals[i].Name == "which-one-is-better" {
			wib = &goals[i]
			break
		}
	}
	if wib == nil {
		t.Fatalf("which-one-is-better goal not found")
	}

	if len(wib.HotSnippets) < 2 {
		t.Fatalf("expected at least 2 hot-snippets, got %d", len(wib.HotSnippets))
	}

	snippetNames := make(map[string]bool)
	for _, hs := range wib.HotSnippets {
		snippetNames[hs.Name] = true
		if hs.Text == "" {
			t.Errorf("hot-snippet %q has empty text", hs.Name)
		}
		if hs.Description == "" {
			t.Errorf("hot-snippet %q has empty description", hs.Name)
		}
	}

	if !snippetNames["deeper-analysis"] {
		t.Error("expected hot-snippet 'deeper-analysis'")
	}
	if !snippetNames["devils-advocate"] {
		t.Error("expected hot-snippet 'devils-advocate'")
	}
}

func TestGoalBuiltin_WhichOneIsBetter_PostCopyHint(t *testing.T) {
	t.Parallel()

	goals := GetBuiltInGoals()

	var wib *Goal
	for i := range goals {
		if goals[i].Name == "which-one-is-better" {
			wib = &goals[i]
			break
		}
	}
	if wib == nil {
		t.Fatalf("which-one-is-better goal not found")
	}

	if wib.PostCopyHint == "" {
		t.Error("expected non-empty PostCopyHint")
	}
	if !strings.Contains(wib.PostCopyHint, "deeper-analysis") {
		t.Errorf("expected PostCopyHint to mention deeper-analysis, got %q", wib.PostCopyHint)
	}
	if !strings.Contains(wib.PostCopyHint, "devils-advocate") {
		t.Errorf("expected PostCopyHint to mention devils-advocate, got %q", wib.PostCopyHint)
	}
}

func TestGoalBuiltin_WhichOneIsBetter_UniqueName(t *testing.T) {
	t.Parallel()

	goals := GetBuiltInGoals()
	names := make(map[string]int)
	for _, g := range goals {
		names[g.Name]++
	}
	if count := names["which-one-is-better"]; count != 1 {
		t.Errorf("expected exactly 1 'which-one-is-better' goal, found %d", count)
	}
	for name, count := range names {
		if count > 1 {
			t.Errorf("duplicate goal name %q (count: %d)", name, count)
		}
	}
}

func TestGoalBuiltin_WhichOneIsBetter_JSONRoundTrip(t *testing.T) {
	t.Parallel()

	goals := GetBuiltInGoals()

	var wib *Goal
	for i := range goals {
		if goals[i].Name == "which-one-is-better" {
			wib = &goals[i]
			break
		}
	}
	if wib == nil {
		t.Fatalf("which-one-is-better goal not found")
	}

	data, err := json.Marshal(wib)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var decoded Goal
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if decoded.Name != "which-one-is-better" {
		t.Errorf("expected name 'which-one-is-better' after roundtrip, got %q", decoded.Name)
	}
	if decoded.Category != "decision-making" {
		t.Errorf("expected category 'decision-making' after roundtrip, got %q", decoded.Category)
	}
	if len(decoded.HotSnippets) != 2 {
		t.Errorf("expected 2 HotSnippets after roundtrip, got %d", len(decoded.HotSnippets))
	}
	if decoded.PostCopyHint == "" {
		t.Error("expected PostCopyHint to survive roundtrip")
	}
}

func TestGoalBuiltin_WhichOneIsBetter_ListOutput(t *testing.T) {
	t.Parallel()

	cfg := config.NewConfig()
	cfg.SetGlobalOption("goal.disable-standard-paths", "true")
	cfg.SetGlobalOption("goal.autodiscovery", "false")
	discovery := NewGoalDiscovery(cfg)
	goalRegistry := NewDynamicGoalRegistry(GetBuiltInGoals(), discovery)

	cmd := NewGoalCommand(cfg, goalRegistry)
	var stdout, stderr bytes.Buffer
	cmd.list = true
	cmd.store = "memory"
	cmd.session = t.Name()

	err := cmd.Execute([]string{}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	output := stdout.String()

	// Goal should appear in list
	if !strings.Contains(output, "which-one-is-better") {
		t.Errorf("expected list to contain 'which-one-is-better', got:\n%s", output)
	}

	// Category should appear
	if !strings.Contains(output, "Decision Making") {
		t.Errorf("expected list to contain 'Decision Making' category, got:\n%s", output)
	}

	// Should show vars
	if !strings.Contains(output, "comparisonType=general") {
		t.Errorf("expected list to show comparisonType=general, got:\n%s", output)
	}

	// Should show custom commands
	if !strings.Contains(output, "[cmds: set-type]") {
		t.Errorf("expected list to show [cmds: set-type], got:\n%s", output)
	}
}

// TestGoal_PromptFooterEmpty verifies that an empty PromptFooter
// produces an empty string in template data (no error).
func TestGoal_PromptFooterEmpty(t *testing.T) {
	t.Parallel()

	goal := Goal{Name: "no-footer"}
	data, err := json.Marshal(goal)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	// Should have the field but empty
	if !strings.Contains(string(data), `"promptFooter":""`) {
		t.Errorf("expected empty promptFooter in JSON, got: %s", string(data))
	}
}
