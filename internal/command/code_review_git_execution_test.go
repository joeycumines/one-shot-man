package command

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/scripting"
)

func TestCodeReviewCommand_ActualGitDiffExecution(t *testing.T) {
	// Only run this test if we're in a git repository
	if !isGitRepository() {
		t.Skip("Skipping git diff test - not in a git repository")
	}

	// Test with real git diff execution
	ctx := context.Background()
	var stdout, stderr bytes.Buffer
	// Use an in-memory storage backend with a test-scoped session so the test
	// doesn't write to the user's session directory when executing git diffs.
	engine, err := scripting.NewEngineWithConfig(ctx, &stdout, &stderr, t.Name(), "memory")
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}
	defer engine.Close()

	engine.SetTestMode(true)
	engine.SetGlobal("args", []string{})
	engine.SetGlobal("codeReviewTemplate", codeReviewTemplate)
	// Inject config object with name field
	engine.SetGlobal("config", map[string]interface{}{"name": "code-review"})

	// Load the script
	script := engine.LoadScriptFromString("code-review", codeReviewScript)
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("Failed to execute script: %v", err)
	}

	// Explicitly switch to code-review mode using Go-level call to ensure mode is fully initialized
	tuiManager := engine.GetTUIManager()
	if err := tuiManager.SwitchMode("code-review"); err != nil {
		t.Fatalf("Failed to switch to code-review mode: %v", err)
	}

	// Test actual git diff execution
	testScript := `
		// Add a lazy diff (commands is a global set when mode is entered)
		commands.diff.handler([]);

		// Verify it's lazy
		const contextItems = items();
		const diffItem = contextItems[contextItems.length - 1];
		if (diffItem.type !== "lazy-diff") {
			throw new Error("Expected lazy-diff, got: " + diffItem.type);
		}

		output.print("ADDED_LAZY_DIFF");

		// Build prompt which should execute the git diff
		try {
			const prompt = buildPrompt();

			// Check if the lazy diff was executed within the prompt output
			if (prompt.includes("### Diff:")) {
				output.print("GIT_DIFF_EXECUTED_SUCCESS");
			} else if (prompt.includes("### Diff Error:")) {
				output.print("GIT_DIFF_EXECUTED_ERROR");
			} else {
				output.print("GIT_DIFF_NOT_EXECUTED_IN_PROMPT");
			}

			// Ensure state remains lazy (non-mutating behavior)
			const stillLazy = items()[items().length - 1];
			if (stillLazy && stillLazy.type === 'lazy-diff') {
				output.print("STATE_REMAINS_LAZY");
			} else {
				output.print("STATE_MUTATED_UNEXPECTEDLY: " + (stillLazy ? stillLazy.type : 'missing'));
			}

			// Check that prompt contains the template
			if (prompt.includes("GUARANTEE the correctness")) {
				output.print("PROMPT_CONTAINS_TEMPLATE");
			}

		} catch (e) {
			output.print("BUILD_PROMPT_ERROR: " + e.message);
		}
	`

	testScriptObj := engine.LoadScriptFromString("git-diff-test", testScript)
	err = engine.ExecuteScript(testScriptObj)
	if err != nil {
		t.Fatalf("Test script execution failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "ADDED_LAZY_DIFF") {
		t.Errorf("Failed to add lazy diff. Output: %s", output)
	}

	if strings.Contains(output, "GIT_DIFF_EXECUTED_SUCCESS") {
		t.Log("Git diff executed successfully")
	} else if strings.Contains(output, "GIT_DIFF_EXECUTED_ERROR") {
		t.Log("Git diff executed but returned error (expected in some cases)")
	} else {
		t.Errorf("Git diff was not executed properly. Output: %s", output)
	}

	if !strings.Contains(output, "PROMPT_CONTAINS_TEMPLATE") {
		t.Errorf("Prompt does not contain expected template. Output: %s", output)
	}
}

func isGitRepository() bool {
	cmd := exec.Command("git", "rev-parse", "--git-dir")
	return cmd.Run() == nil
}
