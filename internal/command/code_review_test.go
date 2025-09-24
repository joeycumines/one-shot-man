package command

import (
	"bytes"
	"flag"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/config"
)

func TestCodeReviewCommand_NonInteractive(t *testing.T) {
	cfg := config.NewConfig()
	cmd := NewCodeReviewCommand(cfg)

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
	if !contains(output, "Code Review: context -> single prompt for PR review") {
		t.Errorf("Expected banner message in output, got: %s", output)
	}

	if !contains(output, "Commands: add, diff, note, list, edit, remove, show, copy, help, exit") {
		t.Errorf("Expected help message in output, got: %s", output)
	}
}

func TestCodeReviewCommand_Name(t *testing.T) {
	cfg := config.NewConfig()
	cmd := NewCodeReviewCommand(cfg)

	if cmd.Name() != "code-review" {
		t.Errorf("Expected command name 'code-review', got: %s", cmd.Name())
	}
}

func TestCodeReviewCommand_Description(t *testing.T) {
	cfg := config.NewConfig()
	cmd := NewCodeReviewCommand(cfg)

	expected := "Single-prompt code review with context: context -> generate prompt for PR review"
	if cmd.Description() != expected {
		t.Errorf("Expected description '%s', got: %s", expected, cmd.Description())
	}
}

func TestCodeReviewCommand_Usage(t *testing.T) {
	cfg := config.NewConfig()
	cmd := NewCodeReviewCommand(cfg)

	expected := "code-review [options]"
	if cmd.Usage() != expected {
		t.Errorf("Expected usage '%s', got: %s", expected, cmd.Usage())
	}
}

func TestCodeReviewCommand_SetupFlags(t *testing.T) {
	cfg := config.NewConfig()
	cmd := NewCodeReviewCommand(cfg)
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

func TestCodeReviewCommand_FlagParsing(t *testing.T) {
	cfg := config.NewConfig()
	cmd := NewCodeReviewCommand(cfg)
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
	cmd = NewCodeReviewCommand(cfg)
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

func TestCodeReviewCommand_ExecuteWithArgs(t *testing.T) {
	cfg := config.NewConfig()
	cmd := NewCodeReviewCommand(cfg)

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
	if !contains(output, "Code Review: context -> single prompt for PR review") {
		t.Errorf("Expected banner message with args, got: %s", output)
	}
}

func TestCodeReviewCommand_ConfigColorOverrides(t *testing.T) {
	cfg := config.NewConfig()
	cfg.Global = map[string]string{
		"prompt.color.input":  "green",
		"prompt.color.prefix": "cyan",
		"other.setting":       "value",
	}

	cmd := NewCodeReviewCommand(cfg)

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
	if !contains(output, "Code Review: context -> single prompt for PR review") {
		t.Errorf("Expected banner message with color config, got: %s", output)
	}
}

func TestCodeReviewCommand_TemplateOverride(t *testing.T) {
	cfg := config.NewConfig()
	// Set a custom template override
	cfg.SetCommandOption("code-review", "template.content", "Custom review template: {{context_txtar}}")

	cmd := NewCodeReviewCommand(cfg)

	var stdout, stderr bytes.Buffer
	cmd.testMode = true
	cmd.interactive = false

	err := cmd.Execute([]string{}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("Expected no error with template override, got: %v", err)
	}

	// The test should complete successfully with template override
	output := stdout.String()
	if !contains(output, "Code Review: context -> single prompt for PR review") {
		t.Errorf("Expected default banner message, got: %s", output)
	}
}

func TestCodeReviewCommand_ScriptConfig(t *testing.T) {
	cfg := config.NewConfig()
	// Set script-specific configuration
	cfg.SetCommandOption("code-review", "script.ui.title", "Custom Code Review")
	cfg.SetCommandOption("code-review", "script.ui.banner", "Custom review banner")
	cfg.SetGlobalOption("script.ui.enable-history", "false")

	cmd := NewCodeReviewCommand(cfg)

	var stdout, stderr bytes.Buffer
	cmd.testMode = true
	cmd.interactive = false

	err := cmd.Execute([]string{}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("Expected no error with script config, got: %v", err)
	}

	// The test should complete successfully with custom configuration
	output := stdout.String()
	if !contains(output, "Custom review banner") {
		t.Errorf("Expected custom banner message, got: %s", output)
	}
}

func TestCodeReviewCommand_NilConfig(t *testing.T) {
	cmd := NewCodeReviewCommand(nil)

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
	if !contains(output, "Code Review: context -> single prompt for PR review") {
		t.Errorf("Expected banner message with nil config, got: %s", output)
	}
}

func TestCodeReviewCommand_TemplateContent(t *testing.T) {
	cfg := config.NewConfig()
	cmd := NewCodeReviewCommand(cfg)

	var stdout, stderr bytes.Buffer

	// Test with test mode enabled
	cmd.testMode = true
	cmd.interactive = false

	err := cmd.Execute([]string{}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Check that the embedded template contains the expected prompt text
	if !contains(codeReviewTemplate, "GUARANTEE the correctness of my PR") {
		t.Errorf("Expected embedded template to contain GUARANTEE section")
	}

	// Check that the template contains context variable
	if !contains(codeReviewTemplate, "{{context_txtar}}") {
		t.Errorf("Expected embedded template to contain {{context_txtar}} variable")
	}

	// Check for key parts of the prompt
	if !contains(codeReviewTemplate, "sink commensurate effort") {
		t.Errorf("Expected embedded template to contain effort instruction")
	}

	if !contains(codeReviewTemplate, "Provide a succinct summary then more detailed analysis") {
		t.Errorf("Expected embedded template to contain summary instruction")
	}
}

func TestCodeReviewCommand_EmbeddedContent(t *testing.T) {
	// Test that both embedded assets are non-empty
	if len(codeReviewTemplate) == 0 {
		t.Error("Expected codeReviewTemplate to be non-empty")
	}

	if len(codeReviewScript) == 0 {
		t.Error("Expected codeReviewScript to be non-empty")
	}

	// Test template structure
	if !contains(codeReviewTemplate, "## IMPLEMENTATIONS/CONTEXT") {
		t.Error("Expected template to contain IMPLEMENTATIONS/CONTEXT section")
	}

	// Test script structure
	if !contains(codeReviewScript, "function buildPrompt()") {
		t.Error("Expected script to contain buildPrompt function")
	}

	if !contains(codeReviewScript, "codeReviewTemplate") {
		t.Error("Expected script to reference codeReviewTemplate variable")
	}
}

func TestCodeReviewCommand_InteractiveMode(t *testing.T) {
	cfg := config.NewConfig()
	cmd := NewCodeReviewCommand(cfg)

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
