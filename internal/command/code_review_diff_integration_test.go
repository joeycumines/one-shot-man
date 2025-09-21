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

	// Verify the script contains lazy diff functionality
	if !contains(output, "Code Review: context -> single prompt for PR review") {
		t.Errorf("Expected banner message in output, got: %s", output)
	}

	// Test script should have the expected diff command description
	expectedElements := []string{
		"lazy-diff",
		"HEAD~1",
		"will be executed when generating prompt",
	}

	for _, element := range expectedElements {
		if !contains(codeReviewScript, element) {
			t.Errorf("Expected script to contain element: %s", element)
		}
	}
}
