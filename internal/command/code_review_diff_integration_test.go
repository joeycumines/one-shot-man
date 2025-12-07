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
	// avoid polluting real session storage
	cmd.store = "memory"
	cmd.session = t.Name()

	err := cmd.Execute([]string{}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	output := stdout.String()

	// Verify compact initial message was printed
	if !contains(output, "Type 'help' for commands. Tip: Try 'note --goals'.") {
		t.Errorf("Expected compact initial message in output, got: %s", output)
	}

	// Verify both script sub-tests passed (register-mode and enter-code-review)
	if !contains(output, "Sub-test register-mode passed") {
		t.Errorf("Expected register-mode sub-test to pass, got: %s", output)
	}

	if !contains(output, "Sub-test enter-code-review passed") {
		t.Errorf("Expected enter-code-review sub-test to pass, got: %s", output)
	}
}
