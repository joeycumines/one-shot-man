package command

import (
	"testing"
)

func TestCodeReviewTemplate_Content(t *testing.T) {
	// Test the embedded template contains the exact prompt text as specified
	expectedPhrases := []string{
		"GUARANTEE the correctness of my PR",
		"sink commensurate effort",
		"you care deeply about keeping your word",
		"think very VERY hard",
		"there's _always_ another problem",
		"Question all information provided",
		"_only_ if it is simply impossible to verify are you allowed to trust",
		"Provide a succinct summary then more detailed analysis",
		"removing any single part, or applying any transformation or adjustment of wording, would make it materially worse",
		"## IMPLEMENTATIONS/CONTEXT",
		"{{.context_txtar}}",
	}

	for _, phrase := range expectedPhrases {
		if !contains(codeReviewTemplate, phrase) {
			t.Errorf("Expected template to contain phrase: %s", phrase)
		}
	}
}

func TestCodeReviewScript_Content(t *testing.T) {
	// Test the script contains the expected function and structure
	expectedElements := []string{
		"Code Review: Single-prompt code review with context",
		"const COMMAND_NAME = config.name",
		"function buildCommands(stateArg)",
		"codeReviewTemplate",
		"context.toTxtar()",
		"template.execute(codeReviewTemplate",
		"context_txtar: fullContext",
	}

	for _, element := range expectedElements {
		if !contains(codeReviewScript, element) {
			t.Errorf("Expected script to contain element: %s", element)
		}
	}
}

func TestCodeReviewScript_Commands(t *testing.T) {
	// Test that the script includes the necessary commands for the code review workflow
	// Commands are built by buildCommands() and inherit from ctxmgr.commands

	// Verify the script spreads ctxmgr.commands and defines additional commands
	if !contains(codeReviewScript, "...ctxmgr.commands") {
		t.Error("Expected script to spread ctxmgr.commands")
	}

	// Verify specific commands are defined or referenced
	commandChecks := map[string]string{
		"note command": "note:",
		"show command": "show:",
	}

	for name, pattern := range commandChecks {
		if !contains(codeReviewScript, pattern) {
			t.Errorf("Expected script to contain %s (pattern: %s)", name, pattern)
		}
	}
}
