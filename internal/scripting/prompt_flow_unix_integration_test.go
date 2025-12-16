//go:build unix

package scripting

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/joeycumines/go-prompt/termtest"
	"github.com/joeycumines/one-shot-man/internal/testutil"
)

// Example demonstrates a complete prompt-flow workflow
// from setting a goal to generating and copying the final prompt.
func Example() {
	// This example shows the complete workflow for using prompt-flow
	// to build a prompt for implementing metrics in a Java application

	// Step 1: Start prompt-flow
	// osm prompt-flow

	// Step 2: Set a goal
	// goal "Add JMX metrics to monitor thread pool performance"

	// Step 3: Add context files
	// add src/main/java/ThreadPoolManager.java

	// Step 4: Add git diff
	// diff --staged

	// Step 5: Add a note
	// note "Focus on Micrometer integration for metrics collection"

	// Step 6: Generate the meta-prompt
	// generate

	// Step 7: Show and copy the final assembled prompt
	// show
	// copy

	fmt.Println("Prompt Flow workflow completed successfully")
	// Output: Prompt Flow workflow completed successfully
}

// TestPromptFlow_Unix_Integration_CompleteWorkflow tests a full prompt-flow workflow
// with real file operations, testing meta-prompt generation and final output assembly.
func TestPromptFlow_Unix_Integration_CompleteWorkflow(t *testing.T) {
	if !isUnixPlatform() {
		t.Skip("Unix-only integration test")
	}

	binaryPath := buildTestBinary(t)

	// Create a temporary workspace with test files
	workspace := createTestWorkspace(t)
	defer os.RemoveAll(workspace)

	// Create test files with realistic content
	testJavaFile := filepath.Join(workspace, "ThreadPoolManager.java")
	testJavaContent := `package com.example.metrics;

import java.util.concurrent.ThreadPoolExecutor;
import java.util.concurrent.TimeUnit;

/**
 * Manages thread pool operations and performance monitoring.
 */
public class ThreadPoolManager {
    private final ThreadPoolExecutor executor;

    public ThreadPoolManager(int corePoolSize, int maxPoolSize) {
        this.executor = new ThreadPoolExecutor(
            corePoolSize, maxPoolSize, 60L, TimeUnit.SECONDS,
            new LinkedBlockingQueue<>()
        );
    }

    public void submitTask(Runnable task) {
        executor.submit(task);
    }

    public int getActiveCount() {
        return executor.getActiveCount();
    }

    public long getCompletedTaskCount() {
        return executor.getCompletedTaskCount();
    }
}`

	if err := os.WriteFile(testJavaFile, []byte(testJavaContent), 0644); err != nil {
		t.Fatalf("Failed to create test Java file: %v", err)
	}

	// Create fake editor script
	editorScript := createFakeEditor(t, workspace)

	env := newTestProcessEnv(t)
	env = append(env, "EDITOR="+editorScript, "VISUAL=")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cp, err := termtest.NewConsole(ctx,
		termtest.WithCommand(binaryPath, "prompt-flow", "-i"),
		termtest.WithDefaultTimeout(30*time.Second),
		termtest.WithEnv(env),
	)
	if err != nil {
		t.Fatalf("Failed to create termtest: %v", err)
	}
	defer cp.Close()

	// Helper to reduce boilerplate
	expect := func(snap termtest.Snapshot, target string, timeout time.Duration) {
		t.Helper()
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		if err := cp.Expect(ctx, snap, termtest.Contains(target), fmt.Sprintf("wait for %q", target)); err != nil {
			t.Fatalf("Expected %q: %v\nBuffer: %q", target, err, cp.String())
		}
	}

	// Wait for startup — prompt-flow emits an initial mode switch on enter
	snap := cp.Snapshot()
	expect(snap, "Switched to mode: prompt-flow", 15*time.Second)
	expect(snap, "(prompt-flow) > ", 20*time.Second)

	// Test the complete workflow
	testCompletePromptFlowWorkflow(t, ctx, cp, testJavaFile)
}

// TestPromptFlow_Unix_MetaPromptVariations tests different meta-prompt configurations
// to verify template variable substitution and output format consistency.
func TestPromptFlow_Unix_MetaPromptVariations(t *testing.T) {
	if !isUnixPlatform() {
		t.Skip("Unix-only integration test")
	}

	testCases := []struct {
		name           string
		goal           string
		files          []string
		diffs          []string
		notes          []string
		expectedInMeta []string
	}{
		{
			name: "SimpleGoalOnly",
			goal: "Implement basic authentication",
			expectedInMeta: []string{
				"!! **GOAL:** !!",
				"Implement basic authentication",
				"!! **IMPLEMENTATIONS/CONTEXT:** !!",
			},
		},
		{
			name:  "GoalWithFiles",
			goal:  "Add database connection pooling",
			files: []string{"database.go", "config.go"},
			expectedInMeta: []string{
				"Add database connection pooling",
				"database.go",
				"config.go",
			},
		},
		{
			name:  "ComplexScenario",
			goal:  "Implement comprehensive logging system",
			files: []string{"logger.go"},
			notes: []string{"Use structured logging", "Include log levels"},
			expectedInMeta: []string{
				"Implement comprehensive logging system",
				"logger.go",
				"Use structured logging",
				"Include log levels",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			testMetaPromptVariation(t, tc.goal, tc.files, tc.diffs, tc.notes, tc.expectedInMeta)
		})
	}
}

// TestPromptFlow_Unix_ContextAssembly tests the final context assembly process
// ensuring that meta-prompts are properly converted to final prompts with context.
func TestPromptFlow_Unix_ContextAssembly(t *testing.T) {
	if !isUnixPlatform() {
		t.Skip("Unix-only integration test")
	}

	binaryPath := buildTestBinary(t)

	workspace := createTestWorkspace(t)
	defer os.RemoveAll(workspace)

	// Create multiple test files to verify context assembly
	files := map[string]string{
		"main.go": `package main

import "fmt"

func main() {
    fmt.Println("Hello, World!")
}`,
		"utils.go": `package main

import "strings"

func processString(s string) string {
    return strings.ToUpper(s)
}`,
	}

	for filename, content := range files {
		filepath := filepath.Join(workspace, filename)
		if err := os.WriteFile(filepath, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create test file %s: %v", filename, err)
		}
	}

	editorScript := createFakeEditor(t, workspace)

	env := newTestProcessEnv(t)
	env = append(env, "EDITOR="+editorScript, "VISUAL=")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cp, err := termtest.NewConsole(ctx,
		termtest.WithCommand(binaryPath, "prompt-flow", "-i"),
		termtest.WithDefaultTimeout(30*time.Second),
		termtest.WithEnv(env),
	)
	if err != nil {
		t.Fatalf("Failed to create termtest: %v", err)
	}
	defer cp.Close()

	expect := func(snap termtest.Snapshot, target string, timeout time.Duration) {
		t.Helper()
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		if err := cp.Expect(ctx, snap, termtest.Contains(target), fmt.Sprintf("wait for %q", target)); err != nil {
			t.Fatalf("Expected %q: %v\nBuffer: %q", target, err, cp.String())
		}
	}

	snap := cp.Snapshot()
	expect(snap, "Switched to mode: prompt-flow", 15*time.Second)
	expect(snap, "(prompt-flow) > ", 20*time.Second)

	// Set goal
	snap = cp.Snapshot()
	if err := cp.SendLine("goal Refactor Go application for better modularity"); err != nil {
		t.Fatalf("Failed to send goal: %v\nBuffer: %q", err, cp.String())
	}
	expect(snap, "Goal set.", 2*time.Second)

	// Add files with absolute paths
	mainGoPath := filepath.Join(workspace, "main.go")
	utilsGoPath := filepath.Join(workspace, "utils.go")
	snap = cp.Snapshot()
	if err := cp.SendLine("add " + mainGoPath + " " + utilsGoPath); err != nil {
		t.Fatalf("Failed to send add: %v\nBuffer: %q", err, cp.String())
	}
	expect(snap, "Added file: "+mainGoPath, 2*time.Second)
	expect(snap, "Added file: "+utilsGoPath, 2*time.Second)

	// Add note
	snap = cp.Snapshot()
	if err := cp.SendLine("note Focus on separation of concerns and clean architecture"); err != nil {
		t.Fatalf("Failed to send note: %v\nBuffer: %q", err, cp.String())
	}
	expect(snap, "Added note [", 2*time.Second)

	// Generate meta-prompt
	snap = cp.Snapshot()
	if err := cp.SendLine("generate"); err != nil {
		t.Fatalf("Failed to send generate: %v\nBuffer: %q", err, cp.String())
	}
	expect(snap, "Meta-prompt generated. You can 'show meta', 'copy meta', or provide the task prompt with 'use'.", 2*time.Second)

	// Show the final assembled output
	// Provide a simple task prompt then show final
	snap = cp.Snapshot()
	if err := cp.SendLine("use final output please"); err != nil {
		t.Fatalf("Failed to send use: %v\nBuffer: %q", err, cp.String())
	}
	expect(snap, "Task prompt set.", 2*time.Second)

	snap = cp.Snapshot()
	if err := cp.SendLine("show"); err != nil {
		t.Fatalf("Failed to send show: %v\nBuffer: %q", err, cp.String())
	}

	// Verify the final output structure (goal is NOT in final output, only task prompt + context)
	expect(snap, "final output please", 2*time.Second)
	expect(snap, "## IMPLEMENTATIONS/CONTEXT", 2*time.Second)
	expect(snap, "### Note:", 2*time.Second)
	expect(snap, "Focus on separation of concerns", 2*time.Second)
	expect(snap, "-- main.go --", 2*time.Second)
	expect(snap, "package main", 2*time.Second)
	expect(snap, "-- utils.go --", 2*time.Second)
	expect(snap, "func processString", 2*time.Second)

	if err := cp.SendLine("exit"); err != nil {
		t.Fatalf("Failed to send exit: %v\nBuffer: %q", err, cp.String())
	}
	if code, err := cp.WaitExit(ctx); err != nil || code != 0 {
		t.Fatalf("Expected exit code 0, got %d (err: %v)", code, err)
	}
}

// TestPromptFlow_Unix_ListShowsMissing verifies that when a tracked file is removed from disk,
// the prompt-flow list command shows a "(missing)" indicator next to the file item label.
func TestPromptFlow_Unix_ListShowsMissing(t *testing.T) {
	if !isUnixPlatform() {
		t.Skip("Unix-only integration test")
	}

	binaryPath := buildTestBinary(t)

	workspace := createTestWorkspace(t)
	defer os.RemoveAll(workspace)

	// Create a file, add it, then remove it and ensure list shows (missing)
	filePath := filepath.Join(workspace, "gone.txt")
	if err := os.WriteFile(filePath, []byte("hello\n"), 0644); err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}

	editorScript := createFakeEditor(t, workspace)

	env := newTestProcessEnv(t)
	env = append(env, "EDITOR="+editorScript, "VISUAL=", "OSM_CLIPBOARD=cat > /dev/null")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cp, err := termtest.NewConsole(ctx,
		termtest.WithCommand(binaryPath, "prompt-flow", "-i"),
		termtest.WithDefaultTimeout(30*time.Second),
		termtest.WithEnv(env),
	)
	if err != nil {
		t.Fatalf("Failed to create termtest: %v", err)
	}
	defer cp.Close()

	expect := func(snap termtest.Snapshot, target string, timeout time.Duration) {
		t.Helper()
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		if err := cp.Expect(ctx, snap, termtest.Contains(target), fmt.Sprintf("wait for %q", target)); err != nil {
			t.Fatalf("Expected %q: %v\nBuffer: %q", target, err, cp.String())
		}
	}

	snap := cp.Snapshot()
	expect(snap, "Switched to mode: prompt-flow", 15*time.Second)
	expect(snap, "(prompt-flow) > ", 20*time.Second)

	// Add the file and verify it's listed
	snap = cp.Snapshot()
	cp.SendLine("add " + filePath)
	expect(snap, "Added file:", 30*time.Second)

	// Remove the file from disk
	if err := os.Remove(filePath); err != nil {
		t.Fatalf("Failed to remove file: %v", err)
	}

	// List should now show (missing)
	snap = cp.Snapshot()
	cp.SendLine("list")
	expect(snap, "(missing)", 30*time.Second)

	cp.SendLine("exit")
	if code, err := cp.WaitExit(ctx); err != nil || code != 0 {
		t.Fatalf("Expected exit code 0, got %d (err: %v)", code, err)
	}
}

// TestPromptFlow_Unix_DiskReadInMeta ensures that context.toTxtar() reflects latest disk content
// by modifying a file after adding it, then generating meta and verifying updated content appears.
func TestPromptFlow_Unix_DiskReadInMeta(t *testing.T) {
	if !isUnixPlatform() {
		t.Skip("Unix-only integration test")
	}

	binaryPath := buildTestBinary(t)

	workspace := createTestWorkspace(t)
	defer os.RemoveAll(workspace)

	filePath := filepath.Join(workspace, "live.txt")
	if err := os.WriteFile(filePath, []byte("v1\n"), 0644); err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}

	editorScript := createFakeEditor(t, workspace)

	env := newTestProcessEnv(t)
	env = append(env, "EDITOR="+editorScript, "VISUAL=", "OSM_CLIPBOARD=cat > /dev/null")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cp, err := termtest.NewConsole(ctx,
		termtest.WithCommand(binaryPath, "prompt-flow", "-i"),
		termtest.WithDefaultTimeout(30*time.Second),
		termtest.WithEnv(env),
	)
	if err != nil {
		t.Fatalf("Failed to create termtest: %v", err)
	}
	defer cp.Close()

	expect := func(snap termtest.Snapshot, target string, timeout time.Duration) {
		t.Helper()
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		if err := cp.Expect(ctx, snap, termtest.Contains(target), fmt.Sprintf("wait for %q", target)); err != nil {
			t.Fatalf("Expected %q: %v\nBuffer: %q", target, err, cp.String())
		}
	}

	snap := cp.Snapshot()
	expect(snap, "Switched to mode: prompt-flow", 15*time.Second)
	expect(snap, "(prompt-flow) > ", 20*time.Second)

	// Set goal to reach meta generation phase
	snap = cp.Snapshot()
	cp.SendLine("goal test live updates")
	expect(snap, "Goal set.", 30*time.Second)

	// Add file
	snap = cp.Snapshot()
	cp.SendLine("add " + filePath)
	expect(snap, "Added file:", 30*time.Second)

	// Update file content on disk
	if err := os.WriteFile(filePath, []byte("v2-updated\n"), 0644); err != nil {
		t.Fatalf("Failed to update file: %v", err)
	}

	// Generate meta and expect updated content to appear via txtar
	snap = cp.Snapshot()
	cp.SendLine("generate")
	expect(snap, "Meta-prompt generated.", 30*time.Second)

	snap = cp.Snapshot()
	cp.SendLine("show meta")
	expect(snap, "v2-updated", 30*time.Second)

	cp.SendLine("exit")
	if code, err := cp.WaitExit(ctx); err != nil || code != 0 {
		t.Fatalf("Expected exit code 0, got %d (err: %v)", code, err)
	}
}

// TestPromptFlow_Unix_DifferentTemplateConfigurations tests various template
// customizations to ensure the templating system works correctly.
func TestPromptFlow_Unix_DifferentTemplateConfigurations(t *testing.T) {
	if !isUnixPlatform() {
		t.Skip("Unix-only integration test")
	}

	binaryPath := buildTestBinary(t)

	workspace := createTestWorkspace(t)
	defer os.RemoveAll(workspace)

	editorScript := createAdvancedFakeEditor(t, workspace)

	env := newTestProcessEnv(t)
	env = append(env, "EDITOR="+editorScript, "VISUAL=")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cp, err := termtest.NewConsole(ctx,
		termtest.WithCommand(binaryPath, "prompt-flow", "-i"),
		termtest.WithDefaultTimeout(30*time.Second),
		termtest.WithEnv(env),
	)
	if err != nil {
		t.Fatalf("Failed to create termtest: %v", err)
	}
	defer cp.Close()

	expect := func(snap termtest.Snapshot, target string, timeout time.Duration) {
		t.Helper()
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		if err := cp.Expect(ctx, snap, termtest.Contains(target), fmt.Sprintf("wait for %q", target)); err != nil {
			t.Fatalf("Expected %q: %v\nBuffer: %q", target, err, cp.String())
		}
	}

	snap := cp.Snapshot()
	expect(snap, "Switched to mode: prompt-flow", 15*time.Second)
	expect(snap, "(prompt-flow) > ", 20*time.Second)

	// Set a goal
	snap = cp.Snapshot()
	if err := cp.SendLine("goal Create a REST API for user management"); err != nil {
		t.Fatalf("Failed to send goal: %v\nBuffer: %q", err, cp.String())
	}
	expect(snap, "Goal set.", 2*time.Second)

	// Customize the template (opens editor and returns immediately in our fake editor)
	snap = cp.Snapshot()
	if err := cp.SendLine("template edit"); err != nil {
		t.Fatalf("Failed to send template edit: %v\nBuffer: %q", err, cp.String())
	}
	// Wait for "Template updated." message after editor exits
	expect(snap, "Template updated.", 5*time.Second)

	// Generate with custom template
	snap = cp.Snapshot()
	if err := cp.SendLine("generate"); err != nil {
		t.Fatalf("Failed to send generate: %v\nBuffer: %q", err, cp.String())
	}
	expect(snap, "Meta-prompt generated.", 10*time.Second)

	// Show meta-prompt to verify template customization
	snap = cp.Snapshot()
	if err := cp.SendLine("show meta"); err != nil {
		t.Fatalf("Failed to send show meta: %v\nBuffer: %q", err, cp.String())
	}
	expect(snap, "Create a REST API for user management", 2*time.Second)
	expect(snap, "CUSTOM TEMPLATE MODIFICATION", 2*time.Second)

	if err := cp.SendLine("exit"); err != nil {
		t.Fatalf("Failed to send exit: %v\nBuffer: %q", err, cp.String())
	}
	if code, err := cp.WaitExit(ctx); err != nil || code != 0 {
		t.Fatalf("Expected exit code 0, got %d (err: %v)", code, err)
	}
}

// TestPromptFlow_Unix_GitDiffIntegration tests git diff integration
// to ensure version control context is properly captured.
func TestPromptFlow_Unix_GitDiffIntegration(t *testing.T) {
	if !isUnixPlatform() {
		t.Skip("Unix-only integration test")
	}

	binaryPath := buildTestBinary(t)

	workspace := createTestWorkspace(t)
	defer os.RemoveAll(workspace)

	// Initialize git repository and create changes
	setupGitRepository(t, workspace)

	editorScript := createFakeEditor(t, workspace)

	env := newTestProcessEnv(t)
	env = append(env,
		"EDITOR="+editorScript,
		"VISUAL=",
		"GIT_AUTHOR_NAME=Test User",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test User",
		"GIT_COMMITTER_EMAIL=test@example.com",
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cp, err := termtest.NewConsole(ctx,
		termtest.WithCommand(binaryPath, "prompt-flow", "-i"),
		termtest.WithDefaultTimeout(30*time.Second),
		termtest.WithEnv(env),
		termtest.WithDir(workspace),
	)
	if err != nil {
		t.Fatalf("Failed to create termtest: %v", err)
	}
	defer cp.Close()

	expect := func(snap termtest.Snapshot, target string, timeout time.Duration) {
		t.Helper()
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		if err := cp.Expect(ctx, snap, termtest.Contains(target), fmt.Sprintf("wait for %q", target)); err != nil {
			t.Fatalf("Expected %q: %v\nBuffer: %q", target, err, cp.String())
		}
	}

	snap := cp.Snapshot()
	expect(snap, "Switched to mode: prompt-flow", 15*time.Second)
	expect(snap, "(prompt-flow) > ", 20*time.Second)

	// Set goal
	snap = cp.Snapshot()
	if err := cp.SendLine("goal Review and optimize the recent code changes"); err != nil {
		t.Fatalf("Failed to send goal: %v\nBuffer: %q", err, cp.String())
	}
	expect(snap, "Goal set.", 2*time.Second)

	// Capture git diff
	snap = cp.Snapshot()
	if err := cp.SendLine("diff"); err != nil {
		t.Fatalf("Failed to send diff: %v\nBuffer: %q", err, cp.String())
	}
	expect(snap, "Added diff:", 2*time.Second)

	// List to verify diff was captured (lazy-diff semantics)
	snap = cp.Snapshot()
	if err := cp.SendLine("list"); err != nil {
		t.Fatalf("Failed to send list: %v\nBuffer: %q", err, cp.String())
	}
	expect(snap, "[goal] Review and optimize the recent code changes", 2*time.Second)
	expect(snap, "[template] set", 2*time.Second)
	expect(snap, "[1] [lazy-diff] git diff", 2*time.Second)

	// Generate and show final output
	snap = cp.Snapshot()
	if err := cp.SendLine("generate"); err != nil {
		t.Fatalf("Failed to send generate: %v\nBuffer: %q", err, cp.String())
	}
	expect(snap, "Meta-prompt generated. You can 'show meta', 'copy meta', or provide the task prompt with 'use'.", 2*time.Second)

	snap = cp.Snapshot()
	if err := cp.SendLine("use ready to review"); err != nil {
		t.Fatalf("Failed to send use: %v\nBuffer: %q", err, cp.String())
	}
	expect(snap, "Task prompt set.", 2*time.Second)

	snap = cp.Snapshot()
	if err := cp.SendLine("show"); err != nil {
		t.Fatalf("Failed to send show: %v\nBuffer: %q", err, cp.String())
	}
	// After 'use', final output shows task prompt + context, NOT the goal
	expect(snap, "ready to review", 2*time.Second)
	expect(snap, "### Diff: git diff", 2*time.Second)
	expect(snap, "```diff", 2*time.Second)
	// Note: git diff may be empty if no staged changes, which is expected

	if err := cp.SendLine("exit"); err != nil {
		t.Fatalf("Failed to send exit: %v\nBuffer: %q", err, cp.String())
	}
	if code, err := cp.WaitExit(ctx); err != nil || code != 0 {
		t.Fatalf("Expected exit code 0, got %d (err: %v)", code, err)
	}
}

// Ensures that in a repository with only a single commit, the default git diff
// used by lazy-diff resolves to a valid comparison (empty tree vs HEAD) rather
// than failing due to missing HEAD~1.
func TestPromptFlow_Unix_GitDiffSingleCommit(t *testing.T) {
	if !isUnixPlatform() {
		t.Skip("Unix-only integration test")
	}

	binaryPath := buildTestBinary(t)

	// Setup a git repo with only one commit
	workspace := createTestWorkspace(t)
	defer os.RemoveAll(workspace)

	// Initialize repo with a single commit
	runCommand(t, workspace, "git", "-c", "advice.defaultBranchName=false", "-c", "init.defaultBranch=main", "init", "-q")
	runCommand(t, workspace, "git", "config", "user.email", "test@example.com")
	runCommand(t, workspace, "git", "config", "user.name", "Test User")

	initialContent := `package main

import "fmt"

func main() {
    fmt.Println("Initial version")
}
`
	initialFile := filepath.Join(workspace, "example.go")
	if err := os.WriteFile(initialFile, []byte(initialContent), 0644); err != nil {
		t.Fatalf("Failed to write initial file: %v", err)
	}
	runCommand(t, workspace, "git", "add", "example.go")
	runCommand(t, workspace, "git", "commit", "-m", "Initial commit")

	env := newTestProcessEnv(t)
	env = append(env,
		"EDITOR=",
		"VISUAL=",
		"GIT_AUTHOR_NAME=Test User",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test User",
		"GIT_COMMITTER_EMAIL=test@example.com",
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cp, err := termtest.NewConsole(ctx,
		termtest.WithCommand(binaryPath, "prompt-flow", "-i"),
		termtest.WithDefaultTimeout(30*time.Second),
		termtest.WithEnv(env),
		termtest.WithDir(workspace),
	)
	if err != nil {
		t.Fatalf("Failed to create termtest: %v", err)
	}
	defer cp.Close()

	expect := func(snap termtest.Snapshot, target string, timeout time.Duration) {
		t.Helper()
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		if err := cp.Expect(ctx, snap, termtest.Contains(target), fmt.Sprintf("wait for %q", target)); err != nil {
			t.Fatalf("Expected %q: %v\nBuffer: %q", target, err, cp.String())
		}
	}

	// Wait for startup — prompt-flow emits an initial mode switch on enter
	snap := cp.Snapshot()
	expect(snap, "Switched to mode: prompt-flow", 15*time.Second)
	expect(snap, "(prompt-flow) > ", 20*time.Second)

	// Set a goal before generating
	snap = cp.Snapshot()
	cp.SendLine("goal Summarise repository history")
	expect(snap, "Goal set.", 30*time.Second)

	// Add a lazy diff with no arguments, triggering the default behavior
	snap = cp.Snapshot()
	cp.SendLine("diff")
	expect(snap, "Added diff:", 30*time.Second)

	// Generate and show output
	snap = cp.Snapshot()
	cp.SendLine("generate")
	expect(snap, "Meta-prompt generated.", 30*time.Second)

	snap = cp.Snapshot()
	cp.SendLine("show")

	// Verify that the diff was successful and not an error, and shows an initial commit file as new
	expect(snap, "### Diff: git diff", 30*time.Second)
	expect(snap, "diff --git a/example.go b/example.go", 30*time.Second)
	expect(snap, "new file mode 100644", 30*time.Second)

	cp.SendLine("exit")
	if code, err := cp.WaitExit(ctx); err != nil || code != 0 {
		t.Fatalf("Expected exit code 0, got %d (err: %v)", code, err)
	}
}

// Validates that malformed lazy-diff payloads are handled gracefully and produce
// descriptive Diff Error messages rather than panicking or silently falling back.
func TestPromptFlow_Unix_GitDiffMalformedPayload(t *testing.T) {
	if !isUnixPlatform() {
		t.Skip("Unix-only integration test")
	}

	binaryPath := buildTestBinary(t)

	workspace := createTestWorkspace(t)
	defer os.RemoveAll(workspace)

	// Initialize a standard repo with 2 commits to ensure diffs are available
	setupGitRepository(t, workspace)

	env := newTestProcessEnv(t)
	env = append(env,
		"EDITOR=",
		"VISUAL=",
		"GIT_AUTHOR_NAME=Test User",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test User",
		"GIT_COMMITTER_EMAIL=test@example.com",
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cp, err := termtest.NewConsole(ctx,
		termtest.WithCommand(binaryPath, "prompt-flow", "-i"),
		termtest.WithDefaultTimeout(30*time.Second),
		termtest.WithEnv(env),
		termtest.WithDir(workspace),
	)
	if err != nil {
		t.Fatalf("Failed to create termtest: %v", err)
	}
	defer cp.Close()

	expect := func(snap termtest.Snapshot, target string, timeout time.Duration) {
		t.Helper()
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		if err := cp.Expect(ctx, snap, termtest.Contains(target), fmt.Sprintf("wait for %q", target)); err != nil {
			t.Fatalf("Expected %q: %v\nBuffer: %q", target, err, cp.String())
		}
	}

	snap := cp.Snapshot()
	expect(snap, "Switched to mode: prompt-flow", 15*time.Second)
	expect(snap, "(prompt-flow) > ", 20*time.Second)

	// Ensure a goal is present for generate usage
	snap = cp.Snapshot()
	if err := cp.SendLine("goal Handle malformed payload test cases"); err != nil {
		t.Fatalf("Failed to send goal: %v\nBuffer: %q", err, cp.String())
	}
	expect(snap, "Goal set.", 2*time.Second)

	// Helper to test malformed payloads via JS REPL (functions are global in script)
	testMalformed := func(payloadJS string) {
		// addItem("lazy-diff", "malformed", <payloadJS>)
		// SendLine sends all chars at once -> paste detection -> multi-line mode
		// So we send empty line to exit multi-line mode
		snap := cp.Snapshot()
		if err := cp.SendLine("addItem(\"lazy-diff\", \"malformed\", " + payloadJS + ")"); err != nil {
			t.Fatalf("Failed to send addItem: %v\nBuffer: %q", err, cp.String())
		}
		// Send empty line to exit multi-line mode
		if err := cp.SendLine(""); err != nil {
			t.Fatalf("Failed to send empty line: %v\nBuffer: %q", err, cp.String())
		}
		// Wait for the JS command to complete and prompt to return
		expect(snap, "(prompt-flow) > ", 2*time.Second)

		snap = cp.Snapshot()
		if err := cp.SendLine("generate"); err != nil {
			t.Fatalf("Failed to send generate: %v\nBuffer: %q", err, cp.String())
		}
		expect(snap, "Meta-prompt generated.", 2*time.Second)

		snap = cp.Snapshot()
		if err := cp.SendLine("show"); err != nil {
			t.Fatalf("Failed to send show: %v\nBuffer: %q", err, cp.String())
		}

		expect(snap, "### Diff Error: malformed", 2*time.Second)
		expect(snap, "Invalid payload: expected a string or string array", 2*time.Second)

		// Clear items list for next sub-case
		snap = cp.Snapshot()
		if err := cp.SendLine("setItems([])"); err != nil {
			t.Fatalf("Failed to send setItems: %v\nBuffer: %q", err, cp.String())
		}
		// Send empty line to exit multi-line mode
		if err := cp.SendLine(""); err != nil {
			t.Fatalf("Failed to send empty line: %v\nBuffer: %q", err, cp.String())
		}
		expect(snap, "(prompt-flow) > ", 2*time.Second)
	}

	// Case 1: number
	testMalformed("12345")
	// Case 2: boolean
	testMalformed("true")
	// Case 3: object
	testMalformed("({key: 'value'})")
	// Case 4: array of non-strings should trigger array-specific error
	snap = cp.Snapshot()
	if err := cp.SendLine("addItem(\"lazy-diff\", \"malformed\", [1,2,3])"); err != nil {
		t.Fatalf("Failed to send addItem: %v\nBuffer: %q", err, cp.String())
	}
	// Send empty line to exit multi-line mode
	if err := cp.SendLine(""); err != nil {
		t.Fatalf("Failed to send empty line: %v\nBuffer: %q", err, cp.String())
	}
	// Wait for the JS command to complete and prompt to return
	expect(snap, "(prompt-flow) > ", 2*time.Second)

	snap = cp.Snapshot()
	if err := cp.SendLine("generate"); err != nil {
		t.Fatalf("Failed to send generate: %v\nBuffer: %q", err, cp.String())
	}
	expect(snap, "Meta-prompt generated.", 2*time.Second)

	snap = cp.Snapshot()
	if err := cp.SendLine("show"); err != nil {
		t.Fatalf("Failed to send show: %v\nBuffer: %q", err, cp.String())
	}
	expect(snap, "### Diff Error: malformed", 2*time.Second)
	expect(snap, "Invalid payload: expected a string array", 2*time.Second)

	if err := cp.SendLine("exit"); err != nil {
		t.Fatalf("Failed to send exit: %v\nBuffer: %q", err, cp.String())
	}
	if code, err := cp.WaitExit(ctx); err != nil || code != 0 {
		t.Fatalf("Expected exit code 0, got %d (err: %v)", code, err)
	}
}

// TestPromptFlow_Unix_MetaIncludesGitDiff is a regression test ensuring that the
// meta-prompt generated by prompt-flow includes any captured git diff output.
// Prior to the fix, meta only included the txtar dump and omitted diffs.
func TestPromptFlow_Unix_MetaIncludesGitDiff(t *testing.T) {
	if !isUnixPlatform() {
		t.Skip("Unix-only integration test")
	}

	binaryPath := buildTestBinary(t)

	workspace := createTestWorkspace(t)
	defer os.RemoveAll(workspace)

	// Initialize git repo and create a working tree diff
	setupGitRepository(t, workspace)

	editorScript := createFakeEditor(t, workspace)

	env := newTestProcessEnv(t)
	env = append(env,
		"EDITOR="+editorScript,
		"VISUAL=",
		"OSM_CLIPBOARD=cat > /dev/null",
		"GIT_AUTHOR_NAME=Test User",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test User",
		"GIT_COMMITTER_EMAIL=test@example.com",
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cp, err := termtest.NewConsole(ctx,
		termtest.WithCommand(binaryPath, "prompt-flow", "-i"),
		termtest.WithDefaultTimeout(30*time.Second),
		termtest.WithEnv(env),
		termtest.WithDir(workspace),
	)
	if err != nil {
		t.Fatalf("Failed to create termtest: %v", err)
	}
	defer cp.Close()

	expect := func(snap termtest.Snapshot, target string, timeout time.Duration) {
		t.Helper()
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		if err := cp.Expect(ctx, snap, termtest.Contains(target), fmt.Sprintf("wait for %q", target)); err != nil {
			t.Fatalf("Expected %q: %v\nBuffer: %q", target, err, cp.String())
		}
	}

	// Wait for startup and prompt — prompt-flow prints a mode switch when entering
	snap := cp.Snapshot()
	expect(snap, "Switched to mode: prompt-flow", 15*time.Second)
	expect(snap, "(prompt-flow) > ", 20*time.Second)

	// Set goal
	snap = cp.Snapshot()
	cp.SendLine("goal Summarise changes in the diff")
	expect(snap, "Goal set.", 30*time.Second)

	// Capture git diff (no args -> working tree diff)
	snap = cp.Snapshot()
	cp.SendLine("diff")
	expect(snap, "Added diff:", 30*time.Second)

	// Generate meta and show it
	snap = cp.Snapshot()
	cp.SendLine("generate")
	expect(snap, "Meta-prompt generated.", 30*time.Second)

	snap = cp.Snapshot()
	cp.SendLine("show meta")

	// The meta should include the diff section with diff fencing
	expect(snap, "### Diff: git diff", 30*time.Second)
	expect(snap, "```diff", 30*time.Second)
	// And it should include at least some content from our modified file
	// created by setupGitRepository (Modified version with new features)
	expect(snap, "Modified version with new features", 30*time.Second)

	cp.SendLine("exit")
	if code, err := cp.WaitExit(ctx); err != nil || code != 0 {
		t.Fatalf("Expected exit code 0, got %d (err: %v)", code, err)
	}
}

// TestPromptFlow_Unix_ClipboardIntegration tests clipboard operations
// to ensure copy functionality works correctly in Unix environments.
func TestPromptFlow_Unix_ClipboardIntegration(t *testing.T) {
	if !isUnixPlatform() {
		t.Skip("Unix-only integration test")
	}

	binaryPath := buildTestBinary(t)

	workspace := createTestWorkspace(t)
	defer os.RemoveAll(workspace)

	editorScript := createFakeEditor(t, workspace)

	// This test needs to verify clipboard file contents, so create explicit clipboard file
	clipboardFile := filepath.Join(t.TempDir(), "clipboard.txt")
	env := []string{
		"OSM_SESSION=" + testutil.NewTestSessionID("test", t.Name()),
		"OSM_STORE=memory",
		"OSM_CLIPBOARD=cat > " + clipboardFile,
		"EDITOR=" + editorScript,
		"VISUAL=",
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cp, err := termtest.NewConsole(ctx,
		termtest.WithCommand(binaryPath, "prompt-flow", "-i"),
		termtest.WithDefaultTimeout(30*time.Second),
		termtest.WithEnv(env),
	)
	if err != nil {
		t.Fatalf("Failed to create termtest: %v", err)
	}
	defer cp.Close()

	expect := func(snap termtest.Snapshot, target string, timeout time.Duration) {
		t.Helper()
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		if err := cp.Expect(ctx, snap, termtest.Contains(target), fmt.Sprintf("wait for %q", target)); err != nil {
			t.Fatalf("Expected %q: %v\nBuffer: %q", target, err, cp.String())
		}
	}

	snap := cp.Snapshot()
	expect(snap, "Switched to mode: prompt-flow", 15*time.Second)
	expect(snap, "(prompt-flow) > ", 20*time.Second)

	// Set up a complete scenario
	snap = cp.Snapshot()
	cp.SendLine("goal Test clipboard functionality with prompt-flow")
	expect(snap, "Goal set.", 30*time.Second)

	snap = cp.Snapshot()
	cp.SendLine("note This is a test note for clipboard verification")
	expect(snap, "Added note [", 30*time.Second)

	snap = cp.Snapshot()
	cp.SendLine("generate")
	expect(snap, "Meta-prompt generated. You can 'show meta', 'copy meta', or provide the task prompt with 'use'.", 30*time.Second)

	// Test copying meta-prompt
	snap = cp.Snapshot()
	cp.SendLine("copy meta")
	expect(snap, "Meta prompt copied to clipboard.", 30*time.Second)

	// Verify meta-prompt was copied
	metaContent, err := os.ReadFile(clipboardFile)
	if err != nil {
		t.Fatalf("Failed to read clipboard file: %v", err)
	}
	metaContentStr := string(metaContent)
	if !strings.Contains(metaContentStr, "Test clipboard functionality with prompt-flow") {
		t.Errorf("Meta-prompt should contain goal text, got: %s", metaContentStr)
	}
	if !strings.Contains(metaContentStr, "!! **GOAL:** !!") {
		t.Errorf("Meta-prompt should contain template instructions, got: %s", metaContentStr)
	}

	// Clear clipboard file for final test
	os.WriteFile(clipboardFile, []byte(""), 0644)

	// Provide task prompt then copy final assembled output
	snap = cp.Snapshot()
	cp.SendLine("use Test goal for automated testing")
	expect(snap, "Task prompt set.", 30*time.Second)
	snap = cp.Snapshot()
	cp.SendLine("copy")
	expect(snap, "Final output copied to clipboard.", 30*time.Second)

	// Verify final output was copied
	finalContent, err := os.ReadFile(clipboardFile)
	if err != nil {
		t.Fatalf("Failed to read clipboard file for final output: %v", err)
	}
	finalContentStr := string(finalContent)
	if !strings.Contains(finalContentStr, "Test goal for automated testing") {
		t.Errorf("Final output should contain generated goal text, got: %s", finalContentStr)
	}
	if !strings.Contains(finalContentStr, "## IMPLEMENTATIONS/CONTEXT") {
		t.Errorf("Final output should contain context section, got: %s", finalContentStr)
	}
	if !strings.Contains(finalContentStr, "This is a test note for clipboard verification") {
		t.Errorf("Final output should contain note content, got: %s", finalContentStr)
	}

	cp.SendLine("exit")
	if code, err := cp.WaitExit(ctx); err != nil || code != 0 {
		t.Fatalf("Expected exit code 0, got %d (err: %v)", code, err)
	}
}

// Helper functions for Unix-only tests

func isUnixPlatform() bool {
	// Check if we're on a Unix-like platform
	_, err := os.Stat("/bin/sh")
	return err == nil
}

func createTestWorkspace(t *testing.T) string {
	t.Helper()
	workspace, err := os.MkdirTemp("", "prompt-flow-test-*")
	if err != nil {
		t.Fatalf("Failed to create test workspace: %v", err)
	}
	return workspace
}

func createFakeEditor(t *testing.T, workspace string) string {
	t.Helper()
	editorScript := filepath.Join(workspace, "fake-editor.sh")
	scriptContent := `#!/bin/sh
# Fake editor for testing
# $1 is the file path
case "$(basename "$1")" in
    "goal")
        echo "Test goal for automated testing" > "$1"
        ;;
    "template")
        echo "!! Custom template for testing !!" > "$1"
        echo "Goal: {{goal}}" >> "$1"
        echo "Context: {{contextTxtar}}" >> "$1"
        ;;
    *generated-prompt*)
        echo "Test goal for automated testing" > "$1"
        ;;
    *)
        echo "edited: $(basename "$1")" > "$1"
        ;;
esac
`
	if err := os.WriteFile(editorScript, []byte(scriptContent), 0755); err != nil {
		t.Fatalf("Failed to write fake editor: %v", err)
	}
	if err := os.Chmod(editorScript, 0755); err != nil {
		t.Fatalf("Failed to chmod fake editor: %v", err)
	}
	return editorScript
}

func createAdvancedFakeEditor(t *testing.T, workspace string) string {
	t.Helper()
	editorScript := filepath.Join(workspace, "advanced-fake-editor.sh")
	scriptContent := `#!/bin/sh
# Advanced fake editor for template testing
case "$(basename "$1")" in
    "template")
        cat > "$1" << 'EOF'
!! CUSTOM TEMPLATE MODIFICATION !!
!! Generate a specialized prompt for the following goal: !!
!! **GOAL:** !!
{{.goal}}
!! **CONTEXT:** !!
{{.contextTxtar}}
!! End of custom template !!
EOF
        ;;
    "generated-prompt")
        echo "Custom generated prompt with goal: Create a REST API for user management" > "$1"
        ;;
    *)
        echo "edited: $(basename "$1")" > "$1"
        ;;
esac
`
	if err := os.WriteFile(editorScript, []byte(scriptContent), 0755); err != nil {
		t.Fatalf("Failed to write advanced fake editor: %v", err)
	}
	if err := os.Chmod(editorScript, 0755); err != nil {
		t.Fatalf("Failed to chmod advanced fake editor: %v", err)
	}
	return editorScript
}

func setupGitRepository(t *testing.T, workspace string) {
	t.Helper()

	// Initialize git repo
	// Suppress default-branch hints and keep quiet to avoid polluting PTY/test output
	runCommand(t, workspace, "git", "-c", "advice.defaultBranchName=false", "-c", "init.defaultBranch=main", "init", "-q")
	runCommand(t, workspace, "git", "config", "user.name", "Test User")
	runCommand(t, workspace, "git", "config", "user.email", "test@example.com")

	// Create initial file and commit
	initialFile := filepath.Join(workspace, "example.go")
	initialContent := `package main

import "fmt"

func main() {
    fmt.Println("Initial version")
}
`
	if err := os.WriteFile(initialFile, []byte(initialContent), 0644); err != nil {
		t.Fatalf("Failed to create initial file: %v", err)
	}

	runCommand(t, workspace, "git", "add", "example.go")
	runCommand(t, workspace, "git", "commit", "-m", "Initial commit")

	// Modify file to create diff
	modifiedContent := `package main

import "fmt"

func main() {
    fmt.Println("Modified version with new features")
    fmt.Println("Additional functionality added")
}
`
	if err := os.WriteFile(initialFile, []byte(modifiedContent), 0644); err != nil {
		t.Fatalf("Failed to modify file: %v", err)
	}
	// Commit the modification to ensure HEAD~1 exists and diffs are available
	runCommand(t, workspace, "git", "add", "example.go")
	runCommand(t, workspace, "git", "commit", "-m", "Second commit")
}

func runCommand(t *testing.T, dir string, name string, args ...string) {
	t.Helper()

	if name == "git" {
		if _, err := exec.LookPath("git"); err != nil {
			t.Skipf("git executable not found in PATH; skipping git-dependent test: %v", err)
		}
	}

	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(append([]string(nil), os.Environ()...), "OSM_SESSION="+testutil.NewTestSessionID("test", t.Name()), "OSM_STORE=memory")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("Command %s %s failed: %v\nstdout: %s\nstderr: %s", name, strings.Join(args, " "), err, stdout.String(), stderr.String())
	}
}

func testCompletePromptFlowWorkflow(t *testing.T, ctx context.Context, cp *termtest.Console, testFile string) {
	t.Helper()

	expect := func(snap termtest.Snapshot, target string, timeout time.Duration) {
		t.Helper()
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		if err := cp.Expect(ctx, snap, termtest.Contains(target), fmt.Sprintf("wait for %q", target)); err != nil {
			t.Fatalf("Expected %q: %v\nBuffer: %q", target, err, cp.String())
		}
	}

	// Set goal
	snap := cp.Snapshot()
	if err := cp.SendLine("goal Enhance Java thread pool with comprehensive monitoring and metrics"); err != nil {
		t.Fatalf("Failed to send goal: %v\nBuffer: %q", err, cp.String())
	}
	expect(snap, "Goal set.", 2*time.Second)

	// Add test file with absolute path
	snap = cp.Snapshot()
	if err := cp.SendLine("add " + testFile); err != nil {
		t.Fatalf("Failed to send add: %v\nBuffer: %q", err, cp.String())
	}
	expect(snap, "Added file:", 2*time.Second)

	// Add a note
	snap = cp.Snapshot()
	if err := cp.SendLine("note Focus on Micrometer integration for production monitoring"); err != nil {
		t.Fatalf("Failed to send note: %v\nBuffer: %q", err, cp.String())
	}
	expect(snap, "Added note [", 2*time.Second)

	// List current state
	snap = cp.Snapshot()
	if err := cp.SendLine("list"); err != nil {
		t.Fatalf("Failed to send list: %v\nBuffer: %q", err, cp.String())
	}
	expect(snap, "[goal] Enhance Java thread pool", 2*time.Second)
	expect(snap, "[template] set", 2*time.Second)
	expect(snap, "[1] [file]", 2*time.Second)
	expect(snap, "[2] [note]", 2*time.Second)

	// Generate meta-prompt
	snap = cp.Snapshot()
	if err := cp.SendLine("generate"); err != nil {
		t.Fatalf("Failed to send generate: %v\nBuffer: %q", err, cp.String())
	}
	expect(snap, "Meta-prompt generated. You can 'show meta', 'copy meta', or provide the task prompt with 'use'.", 2*time.Second)

	// Show meta-prompt to verify content
	snap = cp.Snapshot()
	if err := cp.SendLine("show meta"); err != nil {
		t.Fatalf("Failed to send show meta: %v\nBuffer: %q", err, cp.String())
	}
	expect(snap, "!! **GOAL:** !!", 2*time.Second)
	expect(snap, "Enhance Java thread pool with comprehensive monitoring", 2*time.Second)
	expect(snap, "!! **IMPLEMENTATIONS/CONTEXT:** !!", 2*time.Second)

	// Provide task prompt then show final assembled output
	snap = cp.Snapshot()
	if err := cp.SendLine("use ready to assemble"); err != nil {
		t.Fatalf("Failed to send use: %v\nBuffer: %q", err, cp.String())
	}
	expect(snap, "Task prompt set.", 2*time.Second)

	snap = cp.Snapshot()
	if err := cp.SendLine("show"); err != nil {
		t.Fatalf("Failed to send show: %v\nBuffer: %q", err, cp.String())
	}
	// After 'use', the final output shows the task prompt + context, but NOT the goal
	expect(snap, "ready to assemble", 2*time.Second)
	expect(snap, "## IMPLEMENTATIONS/CONTEXT", 2*time.Second)
	expect(snap, "### Note:", 2*time.Second)
	expect(snap, "Micrometer integration", 2*time.Second)
	expect(snap, "ThreadPoolManager.java", 2*time.Second)
	expect(snap, "ThreadPoolExecutor", 2*time.Second)

	// Test copy functionality
	snap = cp.Snapshot()
	if err := cp.SendLine("copy"); err != nil {
		t.Fatalf("Failed to send copy: %v\nBuffer: %q", err, cp.String())
	}
	expect(snap, "Final output copied to clipboard.", 2*time.Second)

	if err := cp.SendLine("exit"); err != nil {
		t.Fatalf("Failed to send exit: %v\nBuffer: %q", err, cp.String())
	}
	if code, err := cp.WaitExit(ctx); err != nil || code != 0 {
		t.Fatalf("Expected exit code 0, got %d (err: %v)", code, err)
	}
}

func testMetaPromptVariation(t *testing.T, goal string, files []string, diffs []string, notes []string, expectedInMeta []string) {
	t.Helper()

	binaryPath := buildTestBinary(t)

	workspace := createTestWorkspace(t)
	defer os.RemoveAll(workspace)

	// Create test files
	for _, file := range files {
		filePath := filepath.Join(workspace, file)
		content := fmt.Sprintf("// Test content for %s\npackage main\n", file)
		if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create test file %s: %v", file, err)
		}
	}

	editorScript := createFakeEditor(t, workspace)

	env := newTestProcessEnv(t)
	env = append(env, "EDITOR="+editorScript, "VISUAL=")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cp, err := termtest.NewConsole(ctx,
		termtest.WithCommand(binaryPath, "prompt-flow", "-i"),
		termtest.WithDefaultTimeout(30*time.Second),
		termtest.WithEnv(env),
	)
	if err != nil {
		t.Fatalf("Failed to create termtest: %v", err)
	}
	defer cp.Close()

	expect := func(snap termtest.Snapshot, target string, timeout time.Duration) {
		t.Helper()
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		if err := cp.Expect(ctx, snap, termtest.Contains(target), fmt.Sprintf("wait for %q", target)); err != nil {
			t.Fatalf("Expected %q: %v\nBuffer: %q", target, err, cp.String())
		}
	}

	snap := cp.Snapshot()
	expect(snap, "Switched to mode: prompt-flow", 15*time.Second)
	expect(snap, "(prompt-flow) > ", 20*time.Second)

	// Set goal
	snap = cp.Snapshot()
	if err := cp.SendLine("goal " + goal); err != nil {
		t.Fatalf("Failed to send goal: %v\nBuffer: %q", err, cp.String())
	}
	expect(snap, "Goal set.", 2*time.Second)

	// Add files with absolute paths
	for _, file := range files {
		absPath := filepath.Join(workspace, file)
		snap = cp.Snapshot()
		if err := cp.SendLine("add " + absPath); err != nil {
			t.Fatalf("Failed to send add: %v\nBuffer: %q", err, cp.String())
		}
		expect(snap, "Added file:", 2*time.Second)
	}

	// Add notes
	for _, note := range notes {
		snap = cp.Snapshot()
		if err := cp.SendLine("note " + note); err != nil {
			t.Fatalf("Failed to send note: %v\nBuffer: %q", err, cp.String())
		}
		expect(snap, "Added note [", 2*time.Second)
	}

	// Generate and show meta-prompt
	snap = cp.Snapshot()
	if err := cp.SendLine("generate"); err != nil {
		t.Fatalf("Failed to send generate: %v\nBuffer: %q", err, cp.String())
	}
	expect(snap, "Meta-prompt generated. You can 'show meta', 'copy meta', or provide the task prompt with 'use'.", 2*time.Second)

	snap = cp.Snapshot()
	if err := cp.SendLine("show meta"); err != nil {
		t.Fatalf("Failed to send show meta: %v\nBuffer: %q", err, cp.String())
	}

	// Verify expected content in meta-prompt
	for _, expected := range expectedInMeta {
		expect(snap, expected, 2*time.Second)
	}

	if err := cp.SendLine("exit"); err != nil {
		t.Fatalf("Failed to send exit: %v\nBuffer: %q", err, cp.String())
	}
	if code, err := cp.WaitExit(ctx); err != nil || code != 0 {
		t.Fatalf("Expected exit code 0, got %d (err: %v)", code, err)
	}
}
