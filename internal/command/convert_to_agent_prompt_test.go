package command

import (
	"bytes"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/config"
)

func TestConvertToAgentPromptCommand_Name(t *testing.T) {
	cfg := config.NewConfig()
	cmd := NewConvertToAgentPromptCommand(cfg)
	
	if cmd.Name() != "convert-to-agent-prompt" {
		t.Errorf("Expected command name 'convert-to-agent-prompt', got '%s'", cmd.Name())
	}
}

func TestConvertToAgentPromptCommand_Description(t *testing.T) {
	cfg := config.NewConfig()
	cmd := NewConvertToAgentPromptCommand(cfg)
	
	expected := "Convert goal/context to structured agentic AI prompt"
	if cmd.Description() != expected {
		t.Errorf("Expected description '%s', got '%s'", expected, cmd.Description())
	}
}

func TestConvertToAgentPromptCommand_Usage(t *testing.T) {
	cfg := config.NewConfig()
	cmd := NewConvertToAgentPromptCommand(cfg)
	
	expected := "convert-to-agent-prompt [options]"
	if cmd.Usage() != expected {
		t.Errorf("Expected usage '%s', got '%s'", expected, cmd.Usage())
	}
}

func TestConvertToAgentPromptCommand_Execute(t *testing.T) {
	cfg := config.NewConfig()
	cmd := NewConvertToAgentPromptCommand(cfg)

	var stdout, stderr bytes.Buffer

	// Test with test mode enabled
	cmd.testMode = true
	cmd.interactive = false

	err := cmd.Execute([]string{}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Should produce expected output
	output := stdout.String()
	if !contains(output, "Convert to Agent Prompt: goal/context -> structured agent prompt") {
		t.Errorf("Expected banner message, got: %s", output)
	}
}

func TestConvertToAgentPromptCommand_EmbeddedContent(t *testing.T) {
	// Test that both embedded assets are non-empty
	if len(convertToAgentPromptTemplate) == 0 {
		t.Error("Expected convertToAgentPromptTemplate to be non-empty")
	}

	if len(convertToAgentPromptScript) == 0 {
		t.Error("Expected convertToAgentPromptScript to be non-empty")
	}

	// Test template structure
	if !contains(convertToAgentPromptTemplate, "!! **USER GOAL/DESCRIPTION:** !!") {
		t.Error("Expected template to contain user goal section")
	}

	if !contains(convertToAgentPromptTemplate, "!! **CONTEXT:** !!") {
		t.Error("Expected template to contain context section")
	}

	if !contains(convertToAgentPromptTemplate, "!! **TEMPLATE:** !!") {
		t.Error("Expected template to contain template section")
	}

	// Test script structure
	if !contains(convertToAgentPromptScript, "function defaultTemplate()") {
		t.Error("Expected script to contain defaultTemplate function")
	}

	if !contains(convertToAgentPromptScript, "convertToAgentPromptTemplate") {
		t.Error("Expected script to reference convertToAgentPromptTemplate variable")
	}

	if !contains(convertToAgentPromptScript, "convert-to-agent-prompt") {
		t.Error("Expected script to contain mode name")
	}
}

func TestConvertToAgentPromptCommand_TemplateContent(t *testing.T) {
	cfg := config.NewConfig()
	cmd := NewConvertToAgentPromptCommand(cfg)

	var stdout, stderr bytes.Buffer

	// Test with test mode enabled
	cmd.testMode = true
	cmd.interactive = false

	err := cmd.Execute([]string{}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Check that the embedded template contains the expected sections
	if !contains(convertToAgentPromptTemplate, "## Objective") {
		t.Errorf("Expected embedded template to contain Objective section")
	}

	if !contains(convertToAgentPromptTemplate, "## Step-by-Step Instructions") {
		t.Errorf("Expected embedded template to contain Step-by-Step Instructions section")
	}

	if !contains(convertToAgentPromptTemplate, "## Tool Usage Guidelines") {
		t.Errorf("Expected embedded template to contain Tool Usage Guidelines section")
	}

	if !contains(convertToAgentPromptTemplate, "## Best Practices") {
		t.Errorf("Expected embedded template to contain Best Practices section")
	}

	if !contains(convertToAgentPromptTemplate, "## Success Criteria") {
		t.Errorf("Expected embedded template to contain Success Criteria section")
	}

	// Check that template variables are present
	if !contains(convertToAgentPromptTemplate, "{{goal}}") {
		t.Errorf("Expected embedded template to contain {{goal}} variable")
	}

	if !contains(convertToAgentPromptTemplate, "{{context_txtar}}") {
		t.Errorf("Expected embedded template to contain {{context_txtar}} variable")
	}
}

func TestConvertToAgentPromptCommand_ExecuteWithArgs(t *testing.T) {
	cfg := config.NewConfig()
	cmd := NewConvertToAgentPromptCommand(cfg)

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
	if !contains(output, "Convert to Agent Prompt: goal/context -> structured agent prompt") {
		t.Errorf("Expected banner message with args, got: %s", output)
	}
}

func TestConvertToAgentPromptCommand_ScriptFunctionality(t *testing.T) {
	// Test that the script contains key functionality
	if !contains(convertToAgentPromptScript, "function buildCommands()") {
		t.Error("Expected script to contain buildCommands function")
	}

	if !contains(convertToAgentPromptScript, "function buildMetaPrompt()") {
		t.Error("Expected script to contain buildMetaPrompt function")
	}

	if !contains(convertToAgentPromptScript, "function assembleFinal()") {
		t.Error("Expected script to contain assembleFinal function")
	}

	// Check for key commands
	expectedCommands := []string{"goal", "add", "diff", "note", "list", "edit", "remove", "template", "generate", "use", "show", "copy", "help"}
	for _, cmdName := range expectedCommands {
		if !contains(convertToAgentPromptScript, cmdName+":") {
			t.Errorf("Expected script to contain command '%s'", cmdName)
		}
	}
}