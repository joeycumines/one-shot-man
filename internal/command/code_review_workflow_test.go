package command

import (
	"bytes"
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/config"
)

func TestCodeReviewCommand_FullWorkflow(t *testing.T) {
	// Test the complete workflow to ensure nothing is broken
	cfg := config.NewConfig()
	cmd := NewCodeReviewCommand(cfg)

	if cmd == nil {
		t.Fatal("NewCodeReviewCommand returned nil")
	}

	// Test command properties
	if cmd.Name() != "code-review" {
		t.Errorf("Expected name 'code-review', got %s", cmd.Name())
	}

	if !strings.Contains(cmd.Description(), "Single-prompt code review") {
		t.Errorf("Unexpected description: %s", cmd.Description())
	}

	if cmd.Usage() != "code-review [options]" {
		t.Errorf("Expected usage 'code-review [options]', got %s", cmd.Usage())
	}

	// Test template content
	if len(codeReviewTemplate) == 0 {
		t.Fatal("codeReviewTemplate is empty")
	}

	expectedPhrases := []string{
		"GUARANTEE the correctness",
		"sink commensurate effort", 
		"think very VERY hard",
		"## IMPLEMENTATIONS/CONTEXT",
		"{{context_txtar}}",
	}

	for _, phrase := range expectedPhrases {
		if !strings.Contains(codeReviewTemplate, phrase) {
			t.Errorf("Template missing expected phrase: %s", phrase)
		}
	}

	// Test script content
	if len(codeReviewScript) == 0 {
		t.Fatal("codeReviewScript is empty")
	}

	expectedScriptElements := []string{
		"mode: \"review\"",
		"function buildPrompt()",
		"codeReviewTemplate",
		"pb.setTemplate(codeReviewTemplate)",
		"pb.setVariable(\"context_txtar\", txtar)",
	}

	for _, element := range expectedScriptElements {
		if !strings.Contains(codeReviewScript, element) {
			t.Errorf("Script missing expected element: %s", element)
		}
	}

	// Test actual execution using the command's Execute method (like other tests)
	var stdout, stderr bytes.Buffer
	cmd.testMode = true
	cmd.interactive = false
	
	err := cmd.Execute([]string{}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("Command execution failed: %v", err)
	}
	
	output := stdout.String()
	if !strings.Contains(output, "Code Review: context -> single prompt for PR review") {
		t.Errorf("Expected banner message in output, got: %s", output)
	}
	
	if !strings.Contains(output, "Commands: add, diff, note, list, edit, remove, show, copy, help, exit") {
		t.Errorf("Expected help message in output, got: %s", output)
	}

	// Basic test passed - the command can execute properly
	t.Log("Code review command executed successfully")
}