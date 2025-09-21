package command

import (
	"bytes"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/config"
)

func TestCodeReviewCommand_DiffDefaultBehavior(t *testing.T) {
	cfg := config.NewConfig()
	cmd := NewCodeReviewCommand(cfg)

	var stdout, stderr bytes.Buffer
	cmd.testMode = true
	cmd.interactive = false

	err := cmd.Execute([]string{}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	output := stdout.String()

	// Verify banner was printed
	if !contains(output, "Code Review: context -> single prompt for PR review") {
		t.Errorf("Expected banner message in output, got: %s", output)
	}

	// Verify help message is shown (indicating commands are available)
	if !contains(output, "Commands: add, diff, note, list, edit, remove, show, copy, help, exit") {
		t.Errorf("Expected command list in output, got: %s", output)
	}

	// Verify both script sub-tests passed (register-mode and enter-review)
	if !contains(output, "Sub-test register-mode passed") {
		t.Errorf("Expected register-mode sub-test to pass, got: %s", output)
	}

	if !contains(output, "Sub-test enter-review passed") {
		t.Errorf("Expected enter-review sub-test to pass, got: %s", output)
	}
}
