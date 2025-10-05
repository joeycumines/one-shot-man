package command

import (
	"bytes"
	"flag"
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/config"
)

func TestPromptFlowCommand_NonInteractive(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewPromptFlowCommand(cfg)

	var stdout, stderr bytes.Buffer

	// Test with test mode enabled
	cmd.testMode = true
	cmd.interactive = false

	err := cmd.Execute([]string{}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Check for expected output from the script execution
	output := stdout.String()
	if !contains(output, "Prompt Flow: goal/context/template -> generate -> use -> assemble") {
		t.Errorf("Expected banner message in output, got: %s", output)
	}

	if !contains(output, "Commands: goal, add, diff, note, list, edit, remove, template, generate, use, show [meta|prompt], copy [meta|prompt], help, exit") {
		t.Errorf("Expected help message in output, got: %s", output)
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

	args := []string{"arg1", "arg2"}
	err := cmd.Execute(args, &stdout, &stderr)
	if err != nil {
		t.Fatalf("Expected no error with args, got: %v", err)
	}

	// Should still produce expected output
	output := stdout.String()
	if !contains(output, "Prompt Flow: goal/context/template -> generate -> use -> assemble") {
		t.Errorf("Expected banner message with args, got: %s", output)
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

	err := cmd.Execute([]string{}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("Expected no error with color config, got: %v", err)
	}

	// The test should pass without errors even with color configuration
	output := stdout.String()
	if !contains(output, "Prompt Flow: goal/context/template -> generate -> use -> assemble") {
		t.Errorf("Expected banner message with color config, got: %s", output)
	}
}

func TestPromptFlowCommand_NilConfig(t *testing.T) {
	t.Parallel()
	cmd := NewPromptFlowCommand(nil)

	var stdout, stderr bytes.Buffer

	// Test with nil config
	cmd.testMode = true
	cmd.interactive = false

	err := cmd.Execute([]string{}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("Expected no error with nil config, got: %v", err)
	}

	// Should still work
	output := stdout.String()
	if !contains(output, "Prompt Flow: goal/context/template -> generate -> use -> assemble") {
		t.Errorf("Expected banner message with nil config, got: %s", output)
	}
}

func TestPromptFlowCommand_TemplateContent(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewPromptFlowCommand(cfg)

	var stdout, stderr bytes.Buffer

	// Test with test mode enabled
	cmd.testMode = true
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
	if !contains(promptFlowTemplate, "{{goal}}") {
		t.Errorf("Expected embedded template to contain {{goal}} variable")
	}

	if !contains(promptFlowTemplate, "{{context_txtar}}") {
		t.Errorf("Expected embedded template to contain {{context_txtar}} variable")
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
