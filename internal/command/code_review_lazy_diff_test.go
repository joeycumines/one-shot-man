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

		if (!Array.isArray(diffItem.payload) || diffItem.payload.length !== 1 || diffItem.payload[0] !== "HEAD~1") {
			throw new Error("Expected payload [\"HEAD~1\"], got: " + JSON.stringify(diffItem.payload));
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

		if (!Array.isArray(customDiffItem.payload) || customDiffItem.payload.length !== 2 || customDiffItem.payload[0] !== "HEAD~2" || customDiffItem.payload[1] !== "--name-only") {
			throw new Error("Expected payload [\"HEAD~2\", \"--name-only\"], got: " + JSON.stringify(customDiffItem.payload));
		}

		output.print("LAZY_DIFF_TEST_2_PASS");

		// Test 3: Arguments with spaces (the main motivation for this PR)
		commands.diff.handler(["feature/my feature", "--stat"]);

		const afterSpaceDiff = items();
		const spaceDiffItem = afterSpaceDiff[afterSpaceDiff.length - 1];
		if (!Array.isArray(spaceDiffItem.payload) || spaceDiffItem.payload.length !== 2 || spaceDiffItem.payload[0] !== "feature/my feature" || spaceDiffItem.payload[1] !== "--stat") {
			throw new Error("Expected payload with spaces [\"feature/my feature\", \"--stat\"], got: " + JSON.stringify(spaceDiffItem.payload));
		}

		output.print("LAZY_DIFF_TEST_3_PASS");

		// Test 4: Empty quoted arguments
		commands.diff.handler(["HEAD~1", "--grep", "", "--author", "test"]);

		const afterEmptyDiff = items();
		const emptyDiffItem = afterEmptyDiff[afterEmptyDiff.length - 1];
		if (!Array.isArray(emptyDiffItem.payload) || emptyDiffItem.payload.length !== 5 ||
		    emptyDiffItem.payload[0] !== "HEAD~1" || emptyDiffItem.payload[1] !== "--grep" ||
		    emptyDiffItem.payload[2] !== "" || emptyDiffItem.payload[3] !== "--author" ||
		    emptyDiffItem.payload[4] !== "test") {
			throw new Error("Expected payload with empty string [\"HEAD~1\", \"--grep\", \"\", \"--author\", \"test\"], got: " + JSON.stringify(emptyDiffItem.payload));
		}

		output.print("LAZY_DIFF_TEST_4_PASS");

		// Test 5: Test parseArgv function directly with complex cases

		// Test empty quoted arguments
		const testEmptyQuotes = system.parseArgv('command --message ""');
		if (testEmptyQuotes.length !== 3 || testEmptyQuotes[0] !== "command" || testEmptyQuotes[1] !== "--message" || testEmptyQuotes[2] !== "") {
			throw new Error("parseArgv failed with empty quotes, expected [\"command\", \"--message\", \"\"], got: " + JSON.stringify(testEmptyQuotes));
		}

		// Test single quotes with empty
		const testEmptySingle = system.parseArgv("git diff ''");
		if (testEmptySingle.length !== 3 || testEmptySingle[0] !== "git" || testEmptySingle[1] !== "diff" || testEmptySingle[2] !== "") {
			throw new Error("parseArgv failed with empty single quotes, expected [\"git\", \"diff\", \"\"], got: " + JSON.stringify(testEmptySingle));
		}

		// Test arguments with spaces in quotes
		const testSpaces = system.parseArgv('git diff "feature/my feature" --stat');
		if (testSpaces.length !== 4 || testSpaces[0] !== "git" || testSpaces[1] !== "diff" || testSpaces[2] !== "feature/my feature" || testSpaces[3] !== "--stat") {
			throw new Error("parseArgv failed with spaces in quotes, expected [\"git\", \"diff\", \"feature/my feature\", \"--stat\"], got: " + JSON.stringify(testSpaces));
		}

		// Test escaped quotes
		const testEscaped = system.parseArgv('git log --grep "He said \\"hello\\""');
		if (testEscaped.length !== 4 || testEscaped[0] !== "git" || testEscaped[1] !== "log" || testEscaped[2] !== "--grep" || testEscaped[3] !== 'He said "hello"') {
			throw new Error("parseArgv failed with escaped quotes, expected [\"git\", \"log\", \"--grep\", 'He said \"hello\"'], got: " + JSON.stringify(testEscaped));
		}

		// Test mixed quotes and unquoted
		const testMixed = system.parseArgv("git diff HEAD~1 'file with spaces.txt' --name-only unquoted");
		if (testMixed.length !== 6 || testMixed[0] !== "git" || testMixed[1] !== "diff" || testMixed[2] !== "HEAD~1" ||
		    testMixed[3] !== "file with spaces.txt" || testMixed[4] !== "--name-only" || testMixed[5] !== "unquoted") {
			throw new Error("parseArgv failed with mixed quotes, expected [\"git\", \"diff\", \"HEAD~1\", \"file with spaces.txt\", \"--name-only\", \"unquoted\"], got: " + JSON.stringify(testMixed));
		}

		output.print("LAZY_DIFF_TEST_5_PASS");

		// Test 6: Test formatArgv function
		const formatTest1 = formatArgv(["git", "diff", "feature/my feature"]);
		if (formatTest1 !== 'git diff "feature/my feature"') {
			throw new Error("formatArgv failed to quote spaces, expected 'git diff \"feature/my feature\"', got: " + formatTest1);
		}

		const formatTest2 = formatArgv(["git", "log", "--grep", ""]);
		if (formatTest2 !== 'git log --grep ""') {
			throw new Error("formatArgv failed with empty string, expected 'git log --grep \"\"', got: " + formatTest2);
		}

		const formatTest3 = formatArgv(["git", "diff", 'He said "hello"']);
		if (formatTest3 !== 'git diff "He said \\"hello\\""') {
			throw new Error("formatArgv failed to escape quotes, expected 'git diff \"He said \\\"hello\\\"\"', got: " + formatTest3);
		}

		output.print("LAZY_DIFF_TEST_6_PASS");

		// Test 7: Build prompt should execute the lazy diffs (without mutating state)
		try {
			const prompt = buildPrompt();
			let executed = 0;
			if (prompt.includes("### Diff:")) executed++;
			if (prompt.includes("### Diff Error:")) executed++;
			if (executed < 1) {
				throw new Error("Expected at least 1 executed diff in prompt output");
			}
			const afterBuild = items();
			let mutated = false;
			for (const item of afterBuild) {
				if (item.type === "diff" || item.type === "diff-error") mutated = true;
			}
			if (mutated) {
				throw new Error("Items mutated unexpectedly; expected lazy-diff to remain");
			}
			output.print("LAZY_DIFF_TEST_7_PASS");
		} catch (e) {
			output.print("LAZY_DIFF_TEST_7_FAIL: " + e.message);
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
		t.Errorf("Lazy diff test 3 (spaces) failed. Output: %s", output)
	}
	if !strings.Contains(output, "LAZY_DIFF_TEST_4_PASS") {
		t.Errorf("Lazy diff test 4 (empty quotes) failed. Output: %s", output)
	}
	if !strings.Contains(output, "LAZY_DIFF_TEST_5_PASS") {
		t.Errorf("Lazy diff test 5 (parseArgv comprehensive) failed. Output: %s", output)
	}
	if !strings.Contains(output, "LAZY_DIFF_TEST_6_PASS") {
		t.Errorf("Lazy diff test 6 (formatArgv) failed. Output: %s", output)
	}
	if !strings.Contains(output, "LAZY_DIFF_TEST_7_PASS") {
		t.Errorf("Lazy diff test 7 (execution) failed. Output: %s", output)
	}
}

func setupTestRepo(t *testing.T, dir string) {
	// Initialize git repo
	runGitCommand(t, dir, "-c", "advice.defaultBranchName=false", "-c", "init.defaultBranch=main", "init", "-q")
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
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("Git command failed: git %v, error: %v\nstdout: %s\nstderr: %s", args, err, stdout.String(), stderr.String())
	}
}
