package command

import (
	"bytes"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/config"
)

func TestCodeReviewCommand_Integration(t *testing.T) {
	cfg := config.NewConfig()
	cmd := NewCodeReviewCommand(cfg)

	var stdout, stderr bytes.Buffer

	// Test with test mode enabled
	cmd.testMode = true
	cmd.interactive = false

	// Use in-memory storage to avoid polluting real sessions
	cmd.storageBackend = "memory"
	cmd.session = t.Name()

	err := cmd.Execute([]string{}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Check for expected output from the script execution
	output := stdout.String()

	// Verify the mode registration was successful
	if !contains(output, "Sub-test enter-review passed") {
		t.Errorf("Expected enter-review sub-test to pass, got: %s", output)
	}

	// Verify we entered the review mode
	if !contains(output, "Sub-test register-mode passed") {
		t.Errorf("Expected register-mode sub-test to pass, got: %s", output)
	}

	// Verify the compact initial message is displayed
	if !contains(output, "Type 'help' for commands. Tip: Try 'note --goals'.") {
		t.Errorf("Expected compact initial message in output, got: %s", output)
	}

	// Verify the enter-review test passed - removed duplicate check
}
