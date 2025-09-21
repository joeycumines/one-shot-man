package command

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/config"
	"github.com/joeycumines/one-shot-man/internal/scripting"
)

func TestCodeReviewCommand_LazyDiffBehavior(t *testing.T) {
	// Create a temporary git repository for testing
	tempDir := t.TempDir()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}
	defer os.Chdir(origDir)

	// Initialize git repo
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Failed to change to temp dir: %v", err)
	}

	// Create a test repository with some commits
	setupTestRepo(t, tempDir)

	// Test the code review command
	cfg := config.NewConfig()
	cmd := NewCodeReviewCommand(cfg)

	var stdout, stderr bytes.Buffer
	cmd.testMode = true
	cmd.interactive = false

	ctx := context.Background()
	engine := scripting.NewEngine(ctx, &stdout, &stderr)
	defer engine.Close()

	engine.SetTestMode(true)
	engine.SetGlobal("args", []string{})
	engine.SetGlobal("codeReviewTemplate", codeReviewTemplate)

	// Load the script
	script := engine.LoadScriptFromString("code-review", codeReviewScript)
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("Failed to execute script: %v", err)
	}

	// Test lazy diff behavior
	testScript := `
		// Test 1: Add a lazy diff (should not execute git diff yet)
		const initialItems = items().length;
		const commands = buildCommands();
		
		// Execute diff command with default (HEAD~1)
		commands.diff.handler([]);
		
		const afterDiff = items();
		if (afterDiff.length !== initialItems + 1) {
			throw new Error("Expected one new item after diff command");
		}
		
		const diffItem = afterDiff[afterDiff.length - 1];
		if (diffItem.type !== "lazy-diff") {
			throw new Error("Expected lazy-diff type, got: " + diffItem.type);
		}
		
		if (diffItem.payload !== "HEAD~1") {
			throw new Error("Expected HEAD~1 payload, got: " + diffItem.payload);
		}
		
		output.print("LAZY_DIFF_TEST_1_PASS");
		
		// Test 2: Add a lazy diff with custom spec
		commands.diff.handler(["HEAD~2", "--name-only"]);
		
		const afterCustomDiff = items();
		if (afterCustomDiff.length !== initialItems + 2) {
			throw new Error("Expected two items after second diff command");
		}
		
		const customDiffItem = afterCustomDiff[afterCustomDiff.length - 1];
		if (customDiffItem.type !== "lazy-diff") {
			throw new Error("Expected lazy-diff type for custom diff, got: " + customDiffItem.type);
		}
		
		if (customDiffItem.payload !== "HEAD~2 --name-only") {
			throw new Error("Expected 'HEAD~2 --name-only' payload, got: " + customDiffItem.payload);
		}
		
		output.print("LAZY_DIFF_TEST_2_PASS");
		
		// Test 3: Build prompt should execute the lazy diffs
		try {
			const prompt = buildPrompt();
			
			const afterBuild = items();
			let executedDiffs = 0;
			for (const item of afterBuild) {
				if (item.type === "diff" || item.type === "diff-error") {
					executedDiffs++;
				}
			}
			
			if (executedDiffs !== 2) {
				throw new Error("Expected 2 executed diffs after buildPrompt, got: " + executedDiffs);
			}
			
			output.print("LAZY_DIFF_TEST_3_PASS");
			
		} catch (e) {
			output.print("LAZY_DIFF_TEST_3_FAIL: " + e.message);
		}
	`

	testScriptObj := engine.LoadScriptFromString("lazy-diff-test", testScript)
	err = engine.ExecuteScript(testScriptObj)
	if err != nil {
		t.Fatalf("Test script execution failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "LAZY_DIFF_TEST_1_PASS") {
		t.Errorf("Lazy diff test 1 failed. Output: %s", output)
	}
	if !strings.Contains(output, "LAZY_DIFF_TEST_2_PASS") {
		t.Errorf("Lazy diff test 2 failed. Output: %s", output)
	}
	if !strings.Contains(output, "LAZY_DIFF_TEST_3_PASS") {
		t.Errorf("Lazy diff test 3 failed. Output: %s", output)
	}
}

func setupTestRepo(t *testing.T, dir string) {
	// Initialize git repo
	runGitCommand(t, dir, "init")
	runGitCommand(t, dir, "config", "user.name", "Test User")
	runGitCommand(t, dir, "config", "user.email", "test@example.com")

	// Create initial commit
	initialFile := filepath.Join(dir, "initial.txt")
	if err := os.WriteFile(initialFile, []byte("initial content\n"), 0644); err != nil {
		t.Fatalf("Failed to write initial file: %v", err)
	}
	runGitCommand(t, dir, "add", "initial.txt")
	runGitCommand(t, dir, "commit", "-m", "Initial commit")

	// Create second commit
	secondFile := filepath.Join(dir, "second.txt")
	if err := os.WriteFile(secondFile, []byte("second content\n"), 0644); err != nil {
		t.Fatalf("Failed to write second file: %v", err)
	}
	runGitCommand(t, dir, "add", "second.txt")
	runGitCommand(t, dir, "commit", "-m", "Second commit")

	// Create third commit
	thirdFile := filepath.Join(dir, "third.txt")
	if err := os.WriteFile(thirdFile, []byte("third content\n"), 0644); err != nil {
		t.Fatalf("Failed to write third file: %v", err)
	}
	runGitCommand(t, dir, "add", "third.txt")
	runGitCommand(t, dir, "commit", "-m", "Third commit")
}

func runGitCommand(t *testing.T, dir string, args ...string) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Git command failed: git %v, error: %v", args, err)
	}
}