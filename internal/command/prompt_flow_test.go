package command

import (
	"bytes"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/config"
)

func TestPromptFlowCommand_NonInteractive(t *testing.T) {
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
	if !contains(output, "Prompt Flow: goal/context/template -> generate -> assemble") {
		t.Errorf("Expected banner message in output, got: %s", output)
	}
	
	if !contains(output, "Commands: goal, add, diff, note, list, edit, remove, template, generate, show [meta], copy [meta], help, exit") {
		t.Errorf("Expected help message in output, got: %s", output)
	}
}

func TestPromptFlowCommand_Name(t *testing.T) {
	cfg := config.NewConfig()
	cmd := NewPromptFlowCommand(cfg)
	
	if cmd.Name() != "prompt-flow" {
		t.Errorf("Expected command name 'prompt-flow', got: %s", cmd.Name())
	}
}

func TestPromptFlowCommand_Description(t *testing.T) {
	cfg := config.NewConfig()
	cmd := NewPromptFlowCommand(cfg)
	
	expected := "Interactive prompt builder: goal/context/template -> generate -> assemble"
	if cmd.Description() != expected {
		t.Errorf("Expected description '%s', got: %s", expected, cmd.Description())
	}
}

func TestPromptFlowCommand_TemplateContent(t *testing.T) {
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
}

func contains(s, substr string) bool {
	return bytes.Contains([]byte(s), []byte(substr))
}