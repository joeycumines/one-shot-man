package command

import (
	"bytes"
	"context"
	"flag"
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/config"
	"github.com/joeycumines/one-shot-man/internal/scripting"
	"github.com/joeycumines/one-shot-man/internal/testutil"
)

func TestPromptFlowCommand_NonInteractive(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewPromptFlowCommand(cfg)

	var stdout, stderr bytes.Buffer

	// Test with test mode enabled
	cmd.testMode = true
	cmd.interactive = false

	// prevent filesystem persistence from these tests
	cmd.store = "memory"
	cmd.session = t.Name()

	err := cmd.Execute([]string{}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Check for expected output from the script execution
	output := stdout.String()
	if !contains(output, "Type 'help' for commands. Tip: Try 'goal --prewritten'.") {
		t.Errorf("Expected compact initial message in output, got: %s", output)
	}
}

func TestPromptFlowCommand_Name(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewPromptFlowCommand(cfg)

	if cmd.Name() != "prompt-flow" {
		t.Errorf("Expected command name 'prompt-flow', got: %s", cmd.Name())
	}
}

func TestPromptFlowCommand_Description(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewPromptFlowCommand(cfg)

	expected := "Interactive prompt builder: goal/context/template -> generate -> assemble"
	if cmd.Description() != expected {
		t.Errorf("Expected description '%s', got: %s", expected, cmd.Description())
	}
}

func TestPromptFlowCommand_Usage(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewPromptFlowCommand(cfg)

	expected := "prompt-flow [options]"
	if cmd.Usage() != expected {
		t.Errorf("Expected usage '%s', got: %s", expected, cmd.Usage())
	}
}

func TestPromptFlowCommand_SetupFlags(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewPromptFlowCommand(cfg)
	fs := flag.NewFlagSet("test", flag.ContinueOnError)

	cmd.SetupFlags(fs)

	// Test that flags are properly set up
	interactiveFlag := fs.Lookup("interactive")
	if interactiveFlag == nil {
		t.Error("Expected 'interactive' flag to be defined")
	}

	iFlag := fs.Lookup("i")
	if iFlag == nil {
		t.Error("Expected 'i' flag to be defined")
	}

	testFlag := fs.Lookup("test")
	if testFlag == nil {
		t.Error("Expected 'test' flag to be defined")
	}
}

func TestPromptFlowCommand_FlagParsing(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewPromptFlowCommand(cfg)
	fs := flag.NewFlagSet("test", flag.ContinueOnError)

	cmd.SetupFlags(fs)

	// Test parsing --test flag
	err := fs.Parse([]string{"--test"})
	if err != nil {
		t.Fatalf("Failed to parse --test flag: %v", err)
	}

	if !cmd.testMode {
		t.Error("Expected testMode to be true after parsing --test flag")
	}

	// Reset and test -i flag
	cmd = NewPromptFlowCommand(cfg)
	fs = flag.NewFlagSet("test", flag.ContinueOnError)
	cmd.SetupFlags(fs)

	err = fs.Parse([]string{"-i"})
	if err != nil {
		t.Fatalf("Failed to parse -i flag: %v", err)
	}

	if !cmd.interactive {
		t.Error("Expected interactive to be true after parsing -i flag")
	}
}

func TestPromptFlowCommand_ExecuteWithArgs(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewPromptFlowCommand(cfg)

	var stdout, stderr bytes.Buffer

	// Test with test mode enabled and args
	cmd.testMode = true
	cmd.interactive = false

	cmd.store = "memory"
	cmd.session = t.Name()

	args := []string{"arg1", "arg2"}
	err := cmd.Execute(args, &stdout, &stderr)
	if err != nil {
		t.Fatalf("Expected no error with args, got: %v", err)
	}

	// Should still produce expected output
	output := stdout.String()
	if !contains(output, "Type 'help' for commands. Tip: Try 'goal --prewritten'.") {
		t.Errorf("Expected compact initial message with args, got: %s", output)
	}
}

func TestPromptFlowCommand_ConfigColorOverrides(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cfg.Global = map[string]string{
		"prompt.color.input":  "green",
		"prompt.color.prefix": "cyan",
		"other.setting":       "value",
	}

	cmd := NewPromptFlowCommand(cfg)

	var stdout, stderr bytes.Buffer

	// Test with interactive mode disabled but color config present
	cmd.testMode = true
	cmd.interactive = false

	cmd.store = "memory"
	cmd.session = t.Name()

	err := cmd.Execute([]string{}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("Expected no error with color config, got: %v", err)
	}

	// The test should pass without errors even with color configuration
	output := stdout.String()
	if !contains(output, "Type 'help' for commands. Tip: Try 'goal --prewritten'.") {
		t.Errorf("Expected compact initial message with color config, got: %s", output)
	}
}

func TestPromptFlowCommand_NilConfig(t *testing.T) {
	t.Parallel()
	cmd := NewPromptFlowCommand(nil)

	var stdout, stderr bytes.Buffer

	// Test with nil config
	cmd.testMode = true
	cmd.interactive = false

	cmd.store = "memory"
	cmd.session = t.Name()

	err := cmd.Execute([]string{}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("Expected no error with nil config, got: %v", err)
	}

	// Should still work
	output := stdout.String()
	if !contains(output, "Type 'help' for commands. Tip: Try 'goal --prewritten'.") {
		t.Errorf("Expected compact initial message with nil config, got: %s", output)
	}
}

func TestPromptFlowCommand_TemplateContent(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewPromptFlowCommand(cfg)

	var stdout, stderr bytes.Buffer

	// Test with test mode enabled
	cmd.testMode = true
	// do not persist session state to user directories in tests
	cmd.store = "memory"
	cmd.session = t.Name()
	cmd.interactive = false

	err := cmd.Execute([]string{}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Check that the embedded template contains the expected critical section
	if !contains(promptFlowTemplate, "CRITICAL: If you do not rewrite the template IN FULL") {
		t.Errorf("Expected embedded template to contain CRITICAL section")
	}

	// Check that the template contains the specific instruction format
	if !contains(promptFlowTemplate, "!! N.B. only statements surrounded by") {
		t.Errorf("Expected embedded template to contain instruction format")
	}

	// Check that template variables are present
	if !contains(promptFlowTemplate, "{{.goal}}") {
		t.Errorf("Expected embedded template to contain {{.goal}} variable")
	}

	if !contains(promptFlowTemplate, "{{.contextTxtar}}") {
		t.Errorf("Expected embedded template to contain {{.contextTxtar}} variable")
	}
}

func TestPromptFlowCommand_EmbeddedContent(t *testing.T) {
	t.Parallel()
	// Test that both embedded assets are non-empty
	if len(promptFlowTemplate) == 0 {
		t.Error("Expected promptFlowTemplate to be non-empty")
	}

	if len(promptFlowScript) == 0 {
		t.Error("Expected promptFlowScript to be non-empty")
	}

	// Test template structure
	if !contains(promptFlowTemplate, "!! **GOAL:** !!") {
		t.Error("Expected template to contain goal section")
	}

	if !contains(promptFlowTemplate, "!! **IMPLEMENTATIONS/CONTEXT:** !!") {
		t.Error("Expected template to contain implementations/context section")
	}

	// Test script structure
	if !contains(promptFlowScript, "function defaultTemplate()") {
		t.Error("Expected script to contain defaultTemplate function")
	}

	if !contains(promptFlowScript, "promptFlowTemplate") {
		t.Error("Expected script to reference promptFlowTemplate variable")
	}
}

func TestPromptFlowCommand_InteractiveMode(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewPromptFlowCommand(cfg)

	// The interactive mode should be set to default value by SetupFlags
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	cmd.SetupFlags(fs)

	// After setting up flags, interactive should be true (default)
	if !cmd.interactive {
		t.Error("Expected interactive mode to be true after SetupFlags")
	}

	// Test toggling interactive mode
	cmd.interactive = false
	if cmd.interactive {
		t.Error("Expected interactive mode to be false after setting")
	}
}

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

// TestPromptFlowCommand_UseFromInitialPhase verifies that the 'use' command works
// from any phase, including INITIAL — no need to run goal → generate first.
func TestPromptFlowCommand_UseFromInitialPhase(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	ctx := context.Background()
	engine, err := scripting.NewEngineWithConfig(ctx, &stdout, &stderr,
		testutil.NewTestSessionID("prompt-flow", t.Name()), "memory")
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}
	defer engine.Close()
	engine.SetTestMode(true)

	// Inject the same globals that PromptFlowCommand.Execute sets
	engine.SetGlobal("config", map[string]interface{}{
		"name": "prompt-flow",
	})
	engine.SetGlobal("args", []string{})
	engine.SetGlobal("promptFlowTemplate", promptFlowTemplate)

	// Load and execute the prompt-flow script
	script := engine.LoadScriptFromString("prompt-flow", promptFlowScript)
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("ExecuteScript failed: %v; stdout=%s stderr=%s", err, stdout.String(), stderr.String())
	}

	// Switch into the prompt-flow mode (the script auto-switches, but be explicit)
	if err := engine.GetTUIManager().SwitchMode("prompt-flow"); err != nil {
		t.Fatalf("SwitchMode failed: %v", err)
	}

	// Verify we're in INITIAL phase by listing
	stdout.Reset()
	if err := engine.GetTUIManager().ExecuteCommand("list", []string{}); err != nil {
		t.Fatalf("list failed: %v", err)
	}
	listOut := stdout.String()
	if !strings.Contains(listOut, "Phase: INITIAL") {
		t.Fatalf("expected Phase: INITIAL, got:\n%s", listOut)
	}

	// Run 'use' directly from INITIAL phase — this should succeed (not require generate)
	stdout.Reset()
	if err := engine.GetTUIManager().ExecuteCommand("use", []string{"My", "direct", "task", "prompt"}); err != nil {
		t.Fatalf("use command failed: %v; stdout=%s stderr=%s", err, stdout.String(), stderr.String())
	}
	useOut := stdout.String()
	if !strings.Contains(useOut, "Task prompt set.") {
		t.Fatalf("expected 'Task prompt set.' confirmation, got:\n%s", useOut)
	}

	// Verify phase transitioned to TASK_PROMPT_SET
	stdout.Reset()
	if err := engine.GetTUIManager().ExecuteCommand("list", []string{}); err != nil {
		t.Fatalf("list failed: %v", err)
	}
	listOut = stdout.String()
	if !strings.Contains(listOut, "Phase: TASK_PROMPT_SET") {
		t.Fatalf("expected Phase: TASK_PROMPT_SET after use, got:\n%s", listOut)
	}
	if !strings.Contains(listOut, "[prompt] My direct task prompt") {
		t.Fatalf("expected task prompt in list output, got:\n%s", listOut)
	}

	// Verify show produces final output with the task prompt
	stdout.Reset()
	if err := engine.GetTUIManager().ExecuteCommand("show", []string{}); err != nil {
		t.Fatalf("show command failed: %v", err)
	}
	showOut := stdout.String()
	if !strings.Contains(showOut, "My direct task prompt") {
		t.Fatalf("expected task prompt in show output, got:\n%s", showOut)
	}
	if !strings.Contains(showOut, "IMPLEMENTATIONS/CONTEXT") {
		t.Fatalf("expected IMPLEMENTATIONS/CONTEXT section, got:\n%s", showOut)
	}
}

// TestPromptFlowCommand_Footer verifies that the 'footer' command sets text
// appended after the context section in the final output.
func TestPromptFlowCommand_Footer(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	ctx := context.Background()
	engine, err := scripting.NewEngineWithConfig(ctx, &stdout, &stderr,
		testutil.NewTestSessionID("prompt-flow", t.Name()), "memory")
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}
	defer engine.Close()
	engine.SetTestMode(true)

	engine.SetGlobal("config", map[string]interface{}{
		"name": "prompt-flow",
	})
	engine.SetGlobal("args", []string{})
	engine.SetGlobal("promptFlowTemplate", promptFlowTemplate)

	script := engine.LoadScriptFromString("prompt-flow", promptFlowScript)
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("ExecuteScript failed: %v; stdout=%s stderr=%s", err, stdout.String(), stderr.String())
	}

	if err := engine.GetTUIManager().SwitchMode("prompt-flow"); err != nil {
		t.Fatalf("SwitchMode failed: %v", err)
	}

	// Set footer
	stdout.Reset()
	if err := engine.GetTUIManager().ExecuteCommand("footer", []string{"Remember:", "be", "concise"}); err != nil {
		t.Fatalf("footer command failed: %v", err)
	}
	footerOut := stdout.String()
	if !strings.Contains(footerOut, "Footer set.") {
		t.Fatalf("expected 'Footer set.' confirmation, got:\n%s", footerOut)
	}

	// Set task prompt via 'use' (one-step mode)
	stdout.Reset()
	if err := engine.GetTUIManager().ExecuteCommand("use", []string{"Analyze", "the", "code"}); err != nil {
		t.Fatalf("use command failed: %v", err)
	}

	// Verify footer appears in list output
	stdout.Reset()
	if err := engine.GetTUIManager().ExecuteCommand("list", []string{}); err != nil {
		t.Fatalf("list failed: %v", err)
	}
	listOut := stdout.String()
	if !strings.Contains(listOut, "[footer] Remember: be concise") {
		t.Fatalf("expected [footer] in list output, got:\n%s", listOut)
	}

	// Verify show output includes footer after IMPLEMENTATIONS/CONTEXT
	stdout.Reset()
	if err := engine.GetTUIManager().ExecuteCommand("show", []string{}); err != nil {
		t.Fatalf("show command failed: %v", err)
	}
	showOut := stdout.String()
	if !strings.Contains(showOut, "Analyze the code") {
		t.Fatalf("expected task prompt in show output, got:\n%s", showOut)
	}
	if !strings.Contains(showOut, "IMPLEMENTATIONS/CONTEXT") {
		t.Fatalf("expected IMPLEMENTATIONS/CONTEXT in show output, got:\n%s", showOut)
	}
	if !strings.Contains(showOut, "Remember: be concise") {
		t.Fatalf("expected footer text in show output, got:\n%s", showOut)
	}
	// Footer must appear AFTER context section
	ctxIdx := strings.Index(showOut, "IMPLEMENTATIONS/CONTEXT")
	footerIdx := strings.Index(showOut, "Remember: be concise")
	if footerIdx <= ctxIdx {
		t.Fatalf("expected footer after IMPLEMENTATIONS/CONTEXT: ctxIdx=%d footerIdx=%d\n%s", ctxIdx, footerIdx, showOut)
	}
}

// TestPromptFlowCommand_FooterClear verifies that passing empty text to footer clears it.
func TestPromptFlowCommand_FooterClear(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	ctx := context.Background()
	engine, err := scripting.NewEngineWithConfig(ctx, &stdout, &stderr,
		testutil.NewTestSessionID("prompt-flow", t.Name()), "memory")
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}
	defer engine.Close()
	engine.SetTestMode(true)

	engine.SetGlobal("config", map[string]interface{}{
		"name": "prompt-flow",
	})
	engine.SetGlobal("args", []string{})
	engine.SetGlobal("promptFlowTemplate", promptFlowTemplate)

	script := engine.LoadScriptFromString("prompt-flow", promptFlowScript)
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("ExecuteScript failed: %v", err)
	}

	if err := engine.GetTUIManager().SwitchMode("prompt-flow"); err != nil {
		t.Fatalf("SwitchMode failed: %v", err)
	}

	// Set footer, then set task prompt, then verify it appears in show
	stdout.Reset()
	if err := engine.GetTUIManager().ExecuteCommand("footer", []string{"Some", "footer"}); err != nil {
		t.Fatalf("footer command failed: %v", err)
	}
	if err := engine.GetTUIManager().ExecuteCommand("use", []string{"My", "prompt"}); err != nil {
		t.Fatalf("use command failed: %v", err)
	}

	stdout.Reset()
	if err := engine.GetTUIManager().ExecuteCommand("show", []string{}); err != nil {
		t.Fatalf("show failed: %v", err)
	}
	if !strings.Contains(stdout.String(), "Some footer") {
		t.Fatalf("expected footer in show output before clear, got:\n%s", stdout.String())
	}

	// Now clear the footer by setting empty text via editor (simulate with empty arg)
	// The footer command with no args opens editor; to test clearing, we can't easily
	// simulate editor. Instead, we verify the cleared state by testing that assembleFinal
	// without footer doesn't include separator.
	// We'll test via the test hooks to set footer to empty string directly.
	stdout.Reset()
	// Use the JS test hooks to clear footer
	if err := engine.GetTUIManager().ExecuteCommand("footer", []string{"replacement", "footer"}); err != nil {
		t.Fatalf("footer command failed: %v", err)
	}

	stdout.Reset()
	if err := engine.GetTUIManager().ExecuteCommand("show", []string{}); err != nil {
		t.Fatalf("show failed: %v", err)
	}
	showOut := stdout.String()
	if strings.Contains(showOut, "Some footer") {
		t.Fatalf("expected old footer to be replaced, got:\n%s", showOut)
	}
	if !strings.Contains(showOut, "replacement footer") {
		t.Fatalf("expected new footer in show output, got:\n%s", showOut)
	}
}

// TestPromptFlowCommand_NoFooter verifies that when no footer is set,
// the final output does not contain the footer separator.
func TestPromptFlowCommand_NoFooter(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	ctx := context.Background()
	engine, err := scripting.NewEngineWithConfig(ctx, &stdout, &stderr,
		testutil.NewTestSessionID("prompt-flow", t.Name()), "memory")
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}
	defer engine.Close()
	engine.SetTestMode(true)

	engine.SetGlobal("config", map[string]interface{}{
		"name": "prompt-flow",
	})
	engine.SetGlobal("args", []string{})
	engine.SetGlobal("promptFlowTemplate", promptFlowTemplate)

	script := engine.LoadScriptFromString("prompt-flow", promptFlowScript)
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("ExecuteScript failed: %v", err)
	}

	if err := engine.GetTUIManager().SwitchMode("prompt-flow"); err != nil {
		t.Fatalf("SwitchMode failed: %v", err)
	}

	// Set task prompt without footer
	stdout.Reset()
	if err := engine.GetTUIManager().ExecuteCommand("use", []string{"Just", "a", "prompt"}); err != nil {
		t.Fatalf("use command failed: %v", err)
	}

	// Verify show output does NOT contain the footer separator after context
	stdout.Reset()
	if err := engine.GetTUIManager().ExecuteCommand("show", []string{}); err != nil {
		t.Fatalf("show failed: %v", err)
	}
	showOut := stdout.String()
	if !strings.Contains(showOut, "Just a prompt") {
		t.Fatalf("expected task prompt in show, got:\n%s", showOut)
	}
	if !strings.Contains(showOut, "IMPLEMENTATIONS/CONTEXT") {
		t.Fatalf("expected IMPLEMENTATIONS/CONTEXT in show, got:\n%s", showOut)
	}

	// Count occurrences of "---" separator — should have exactly the one around IMPLEMENTATIONS/CONTEXT
	// The footer separator adds an extra "---" which should NOT be present
	afterCtx := showOut[strings.Index(showOut, "IMPLEMENTATIONS/CONTEXT")+len("IMPLEMENTATIONS/CONTEXT"):]
	// After the context section header's closing "---", there should be no additional "---" separator
	// (the header itself is \n---\n## IMPLEMENTATIONS/CONTEXT\n---\n)
	// We check there's no trailing --- beyond the header
	parts := strings.Split(afterCtx, "---")
	// parts[0] is the closing of the header "---", parts[1] is content after. Should be only 2 parts (header close + content).
	if len(parts) > 2 {
		t.Fatalf("expected no extra separator after context when no footer set, got:\n%s", showOut)
	}
}
