//go:build unix

package scripting

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/joeycumines/one-shot-man/internal/termtest"
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

	opts := termtest.Options{
		CmdName:        binaryPath,
		Args:           []string{"prompt-flow", "-i"},
		DefaultTimeout: 30 * time.Second,
		Env:            env,
	}

	cp, err := termtest.NewTest(t, opts)
	if err != nil {
		t.Fatalf("Failed to create termtest: %v", err)
	}
	defer cp.Close()

	// Wait for startup — prompt-flow emits an initial mode switch on enter
	startLen := cp.OutputLen()
	if _, err := cp.ExpectSince("Switched to mode: prompt-flow", startLen, 15*time.Second); err != nil {
		t.Fatalf("Expected mode switch to prompt-flow: %v", err)
	}
	if _, err := cp.ExpectSince("(prompt-flow) > ", startLen, 20*time.Second); err != nil {
		t.Fatalf("Expected prompt: %v", err)
	}

	// Test the complete workflow
	testCompletePromptFlowWorkflow(t, cp, testJavaFile)
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

	opts := termtest.Options{
		CmdName:        binaryPath,
		Args:           []string{"prompt-flow", "-i"},
		DefaultTimeout: 30 * time.Second,
		Env:            env,
	}

	cp, err := termtest.NewTest(t, opts)
	if err != nil {
		t.Fatalf("Failed to create termtest: %v", err)
	}
	defer cp.Close()

	startLen := cp.OutputLen()
	if _, err := cp.ExpectSince("Switched to mode: prompt-flow", startLen, 15*time.Second); err != nil {
		t.Fatalf("Expected mode switch to prompt-flow: %v", err)
	}
	if _, err := cp.ExpectSince("(prompt-flow) > ", startLen, 20*time.Second); err != nil {
		t.Fatalf("Expected prompt: %v", err)
	}

	// Set goal
	startLen = cp.OutputLen()
	if err := cp.SendLine("goal Refactor Go application for better modularity"); err != nil {
		t.Fatalf("Failed to send goal: %v\nBuffer: %q", err, cp.GetOutput())
	}
	if _, err := cp.ExpectSince("Goal set.", startLen, 2*time.Second); err != nil {
		t.Fatalf("Expected goal set: %v\nBuffer: %q", err, cp.GetOutput())
	}

	// Add files with absolute paths
	mainGoPath := filepath.Join(workspace, "main.go")
	utilsGoPath := filepath.Join(workspace, "utils.go")
	startLen = cp.OutputLen()
	if err := cp.SendLine("add " + mainGoPath + " " + utilsGoPath); err != nil {
		t.Fatalf("Failed to send add: %v\nBuffer: %q", err, cp.GetOutput())
	}
	if _, err := cp.ExpectSince("Added file: "+mainGoPath, startLen, 2*time.Second); err != nil {
		t.Fatalf("Expected file added: %v\nBuffer: %q", err, cp.GetOutput())
	}
	if _, err := cp.ExpectSince("Added file: "+utilsGoPath, startLen, 2*time.Second); err != nil {
		t.Fatalf("Expected file added: %v\nBuffer: %q", err, cp.GetOutput())
	}

	// Add note
	startLen = cp.OutputLen()
	if err := cp.SendLine("note Focus on separation of concerns and clean architecture"); err != nil {
		t.Fatalf("Failed to send note: %v\nBuffer: %q", err, cp.GetOutput())
	}
	if _, err := cp.ExpectSince("Added note [", startLen, 2*time.Second); err != nil {
		t.Fatalf("Expected note added: %v\nBuffer: %q", err, cp.GetOutput())
	}

	// Generate meta-prompt
	startLen = cp.OutputLen()
	if err := cp.SendLine("generate"); err != nil {
		t.Fatalf("Failed to send generate: %v\nBuffer: %q", err, cp.GetOutput())
	}
	if _, err := cp.ExpectSince("Meta-prompt generated. You can 'show meta', 'copy meta', or provide the task prompt with 'use'.", startLen, 2*time.Second); err != nil {
		t.Fatalf("Expected meta generated: %v\nBuffer: %q", err, cp.GetOutput())
	}

	// Show the final assembled output
	// Provide a simple task prompt then show final
	startLen = cp.OutputLen()
	if err := cp.SendLine("use final output please"); err != nil {
		t.Fatalf("Failed to send use: %v\nBuffer: %q", err, cp.GetOutput())
	}
	if _, err := cp.ExpectSince("Task prompt set.", startLen, 2*time.Second); err != nil {
		t.Fatalf("Expected task prompt set: %v\nBuffer: %q", err, cp.GetOutput())
	}
	startLen = cp.OutputLen()
	if err := cp.SendLine("show"); err != nil {
		t.Fatalf("Failed to send show: %v\nBuffer: %q", err, cp.GetOutput())
	}

	// Verify the final output structure (goal is NOT in final output, only task prompt + context)
	if _, err := cp.ExpectSince("final output please", startLen, 2*time.Second); err != nil {
		t.Fatalf("Expected task prompt in final: %v\nBuffer: %q", err, cp.GetOutput())
	}
	if _, err := cp.ExpectSince("## IMPLEMENTATIONS/CONTEXT", startLen, 2*time.Second); err != nil {
		t.Fatalf("Expected context section: %v\nBuffer: %q", err, cp.GetOutput())
	}
	if _, err := cp.ExpectSince("### Note:", startLen, 2*time.Second); err != nil {
		t.Fatalf("Expected note section: %v\nBuffer: %q", err, cp.GetOutput())
	}
	if _, err := cp.ExpectSince("Focus on separation of concerns", startLen, 2*time.Second); err != nil {
		t.Fatalf("Expected note content: %v\nBuffer: %q", err, cp.GetOutput())
	}
	if _, err := cp.ExpectSince("-- main.go --", startLen, 2*time.Second); err != nil {
		t.Fatalf("Expected txtar marker: %v\nBuffer: %q", err, cp.GetOutput())
	}
	if _, err := cp.ExpectSince("package main", startLen, 2*time.Second); err != nil {
		t.Fatalf("Expected file content: %v\nBuffer: %q", err, cp.GetOutput())
	}
	if _, err := cp.ExpectSince("-- utils.go --", startLen, 2*time.Second); err != nil {
		t.Fatalf("Expected second file marker: %v\nBuffer: %q", err, cp.GetOutput())
	}
	if _, err := cp.ExpectSince("func processString", startLen, 2*time.Second); err != nil {
		t.Fatalf("Expected second file content: %v\nBuffer: %q", err, cp.GetOutput())
	}

	if err := cp.SendLine("exit"); err != nil {
		t.Fatalf("Failed to send exit: %v\nBuffer: %q", err, cp.GetOutput())
	}
	requireExpectExitCode(t, cp, 0)
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
	env = append(env, "EDITOR="+editorScript, "VISUAL=", "ONESHOT_CLIPBOARD_CMD=cat > /dev/null")

	opts := termtest.Options{
		CmdName:        binaryPath,
		Args:           []string{"prompt-flow", "-i"},
		DefaultTimeout: 30 * time.Second,
		Env:            env,
	}

	cp, err := termtest.NewTest(t, opts)
	if err != nil {
		t.Fatalf("Failed to create termtest: %v", err)
	}
	defer cp.Close()

	startLen := cp.OutputLen()
	if _, err := cp.ExpectSince("Switched to mode: prompt-flow", startLen, 15*time.Second); err != nil {
		t.Fatalf("Expected mode switch to prompt-flow: %v", err)
	}
	if _, err := cp.ExpectSince("(prompt-flow) > ", startLen, 20*time.Second); err != nil {
		t.Fatalf("Expected prompt: %v", err)
	}

	// Add the file and verify it's listed
	startLen = cp.OutputLen()
	cp.SendLine("add " + filePath)
	if _, err := cp.ExpectSince("Added file:", startLen); err != nil {
		t.Fatalf("Expected file added: %v", err)
	}

	// Remove the file from disk
	if err := os.Remove(filePath); err != nil {
		t.Fatalf("Failed to remove file: %v", err)
	}

	// List should now show (missing)
	startLen = cp.OutputLen()
	cp.SendLine("list")
	if _, err := cp.ExpectSince("(missing)", startLen); err != nil {
		t.Fatalf("Expected missing indicator: %v", err)
	}

	cp.SendLine("exit")
	requireExpectExitCode(t, cp, 0)
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
	env = append(env, "EDITOR="+editorScript, "VISUAL=", "ONESHOT_CLIPBOARD_CMD=cat > /dev/null")

	opts := termtest.Options{
		CmdName:        binaryPath,
		Args:           []string{"prompt-flow", "-i"},
		DefaultTimeout: 30 * time.Second,
		Env:            env,
	}

	cp, err := termtest.NewTest(t, opts)
	if err != nil {
		t.Fatalf("Failed to create termtest: %v", err)
	}
	defer cp.Close()

	startLen := cp.OutputLen()
	if _, err := cp.ExpectSince("Switched to mode: prompt-flow", startLen, 15*time.Second); err != nil {
		t.Fatalf("Expected mode switch to prompt-flow: %v", err)
	}
	if _, err := cp.ExpectSince("(prompt-flow) > ", startLen, 20*time.Second); err != nil {
		t.Fatalf("Expected prompt: %v", err)
	}

	// Set goal to reach meta generation phase
	startLen = cp.OutputLen()
	cp.SendLine("goal test live updates")
	if _, err := cp.ExpectSince("Goal set.", startLen); err != nil {
		t.Fatalf("Expected goal set: %v", err)
	}

	// Add file
	startLen = cp.OutputLen()
	cp.SendLine("add " + filePath)
	if _, err := cp.ExpectSince("Added file:", startLen); err != nil {
		t.Fatalf("Expected file added: %v", err)
	}

	// Update file content on disk
	if err := os.WriteFile(filePath, []byte("v2-updated\n"), 0644); err != nil {
		t.Fatalf("Failed to update file: %v", err)
	}

	// Generate meta and expect updated content to appear via txtar
	startLen = cp.OutputLen()
	cp.SendLine("generate")
	if _, err := cp.ExpectSince("Meta-prompt generated.", startLen); err != nil {
		t.Fatalf("Expected meta generated: %v", err)
	}
	startLen = cp.OutputLen()
	cp.SendLine("show meta")
	if _, err := cp.ExpectSince("v2-updated", startLen); err != nil {
		t.Fatalf("Expected updated content: %v", err)
	}

	cp.SendLine("exit")
	requireExpectExitCode(t, cp, 0)
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

	opts := termtest.Options{
		CmdName:        binaryPath,
		Args:           []string{"prompt-flow", "-i"},
		DefaultTimeout: 30 * time.Second,
		Env:            env,
	}

	cp, err := termtest.NewTest(t, opts)
	if err != nil {
		t.Fatalf("Failed to create termtest: %v", err)
	}
	defer cp.Close()

	startLen := cp.OutputLen()
	if _, err := cp.ExpectSince("Switched to mode: prompt-flow", startLen, 15*time.Second); err != nil {
		t.Fatalf("Expected mode switch to prompt-flow: %v", err)
	}
	if _, err := cp.ExpectSince("(prompt-flow) > ", startLen, 20*time.Second); err != nil {
		t.Fatalf("Expected prompt: %v", err)
	}

	// Set a goal
	startLen = cp.OutputLen()
	if err := cp.SendLine("goal Create a REST API for user management"); err != nil {
		t.Fatalf("Failed to send goal: %v\nBuffer: %q", err, cp.GetOutput())
	}
	if _, err := cp.ExpectSince("Goal set.", startLen, 2*time.Second); err != nil {
		t.Fatalf("Expected goal set: %v\nBuffer: %q", err, cp.GetOutput())
	}

	// Customize the template (opens editor and returns immediately in our fake editor)
	startTemplate := cp.OutputLen()
	if err := cp.SendLine("template edit"); err != nil {
		t.Fatalf("Failed to send template edit: %v\nBuffer: %q", err, cp.GetOutput())
	}
	// Wait for "Template updated." message after editor exits
	if _, err := cp.ExpectSince("Template updated.", startTemplate, 5*time.Second); err != nil {
		t.Fatalf("Expected template updated message: %v\nBuffer: %q", err, cp.GetOutput())
	}

	// Generate with custom template
	startGenerate := cp.OutputLen()
	if err := cp.SendLine("generate"); err != nil {
		t.Fatalf("Failed to send generate: %v\nBuffer: %q", err, cp.GetOutput())
	}
	if _, err := cp.ExpectSince("Meta-prompt generated.", startGenerate, 10*time.Second); err != nil {
		t.Fatalf("Expected meta generated: %v\nBuffer: %q", err, cp.GetOutput())
	}

	// Show meta-prompt to verify template customization
	startShowMeta := cp.OutputLen()
	if err := cp.SendLine("show meta"); err != nil {
		t.Fatalf("Failed to send show meta: %v\nBuffer: %q", err, cp.GetOutput())
	}
	if _, err := cp.ExpectSince("Create a REST API for user management", startShowMeta, 2*time.Second); err != nil {
		t.Fatalf("Expected goal in meta: %v\nBuffer: %q", err, cp.GetOutput())
	}
	if _, err := cp.ExpectSince("CUSTOM TEMPLATE MODIFICATION", startShowMeta, 2*time.Second); err != nil {
		t.Fatalf("Expected custom template marker: %v\nBuffer: %q", err, cp.GetOutput())
	}

	if err := cp.SendLine("exit"); err != nil {
		t.Fatalf("Failed to send exit: %v\nBuffer: %q", err, cp.GetOutput())
	}
	requireExpectExitCode(t, cp, 0)
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

	opts := termtest.Options{
		CmdName:        binaryPath,
		Args:           []string{"prompt-flow", "-i"},
		DefaultTimeout: 30 * time.Second,
		Env:            env,
		Dir:            workspace,
	}

	cp, err := termtest.NewTest(t, opts)
	if err != nil {
		t.Fatalf("Failed to create termtest: %v", err)
	}
	defer cp.Close()

	startLen := cp.OutputLen()
	if _, err := cp.ExpectSince("Switched to mode: prompt-flow", startLen, 15*time.Second); err != nil {
		t.Fatalf("Expected mode switch to prompt-flow: %v", err)
	}
	if _, err := cp.ExpectSince("(prompt-flow) > ", startLen, 20*time.Second); err != nil {
		t.Fatalf("Expected prompt: %v", err)
	}

	// Set goal
	startLen = cp.OutputLen()
	if err := cp.SendLine("goal Review and optimize the recent code changes"); err != nil {
		t.Fatalf("Failed to send goal: %v\nBuffer: %q", err, cp.GetOutput())
	}
	if _, err := cp.ExpectSince("Goal set.", startLen, 2*time.Second); err != nil {
		t.Fatalf("Expected goal set: %v\nBuffer: %q", err, cp.GetOutput())
	}

	// Capture git diff
	startLen = cp.OutputLen()
	if err := cp.SendLine("diff"); err != nil {
		t.Fatalf("Failed to send diff: %v\nBuffer: %q", err, cp.GetOutput())
	}
	if _, err := cp.ExpectSince("Added diff:", startLen, 2*time.Second); err != nil {
		t.Fatalf("Expected diff added: %v\nBuffer: %q", err, cp.GetOutput())
	}

	// List to verify diff was captured (lazy-diff semantics)
	startLen = cp.OutputLen()
	if err := cp.SendLine("list"); err != nil {
		t.Fatalf("Failed to send list: %v\nBuffer: %q", err, cp.GetOutput())
	}
	if _, err := cp.ExpectSince("[goal] Review and optimize the recent code changes", startLen, 2*time.Second); err != nil {
		t.Fatalf("Expected goal in list: %v\nBuffer: %q", err, cp.GetOutput())
	}
	if _, err := cp.ExpectSince("[template] set", startLen, 2*time.Second); err != nil {
		t.Fatalf("Expected template in list: %v\nBuffer: %q", err, cp.GetOutput())
	}
	if _, err := cp.ExpectSince("[1] [lazy-diff] git diff", startLen, 2*time.Second); err != nil {
		t.Fatalf("Expected diff in list: %v\nBuffer: %q", err, cp.GetOutput())
	}

	// Generate and show final output
	startLen = cp.OutputLen()
	if err := cp.SendLine("generate"); err != nil {
		t.Fatalf("Failed to send generate: %v\nBuffer: %q", err, cp.GetOutput())
	}
	if _, err := cp.ExpectSince("Meta-prompt generated. You can 'show meta', 'copy meta', or provide the task prompt with 'use'.", startLen, 2*time.Second); err != nil {
		t.Fatalf("Expected meta generated: %v\nBuffer: %q", err, cp.GetOutput())
	}

	startLen = cp.OutputLen()
	if err := cp.SendLine("use ready to review"); err != nil {
		t.Fatalf("Failed to send use: %v\nBuffer: %q", err, cp.GetOutput())
	}
	if _, err := cp.ExpectSince("Task prompt set.", startLen, 2*time.Second); err != nil {
		t.Fatalf("Expected task prompt set: %v\nBuffer: %q", err, cp.GetOutput())
	}
	startLen = cp.OutputLen()
	if err := cp.SendLine("show"); err != nil {
		t.Fatalf("Failed to send show: %v\nBuffer: %q", err, cp.GetOutput())
	}
	// After 'use', final output shows task prompt + context, NOT the goal
	if _, err := cp.ExpectSince("ready to review", startLen, 2*time.Second); err != nil {
		t.Fatalf("Expected task prompt in final: %v\nBuffer: %q", err, cp.GetOutput())
	}
	if _, err := cp.ExpectSince("### Diff: git diff", startLen, 2*time.Second); err != nil {
		t.Fatalf("Expected diff section: %v\nBuffer: %q", err, cp.GetOutput())
	}
	if _, err := cp.ExpectSince("```diff", startLen, 2*time.Second); err != nil {
		t.Fatalf("Expected diff formatting: %v\nBuffer: %q", err, cp.GetOutput())
	}
	// Note: git diff may be empty if no staged changes, which is expected

	if err := cp.SendLine("exit"); err != nil {
		t.Fatalf("Failed to send exit: %v\nBuffer: %q", err, cp.GetOutput())
	}
	requireExpectExitCode(t, cp, 0)
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

	opts := termtest.Options{
		CmdName:        binaryPath,
		Args:           []string{"prompt-flow", "-i"},
		DefaultTimeout: 30 * time.Second,
		Env:            env,
		Dir:            workspace,
	}

	cp, err := termtest.NewTest(t, opts)
	if err != nil {
		t.Fatalf("Failed to create termtest: %v", err)
	}
	defer cp.Close()

	// Wait for startup — prompt-flow emits an initial mode switch on enter
	startLen := cp.OutputLen()
	if _, err := cp.ExpectSince("Switched to mode: prompt-flow", startLen, 15*time.Second); err != nil {
		t.Fatalf("Expected mode switch to prompt-flow: %v", err)
	}
	if _, err := cp.ExpectSince("(prompt-flow) > ", startLen, 20*time.Second); err != nil {
		t.Fatalf("Expected prompt: %v", err)
	}

	// Set a goal before generating
	startLen = cp.OutputLen()
	cp.SendLine("goal Summarise repository history")
	if _, err := cp.ExpectSince("Goal set.", startLen); err != nil {
		t.Fatalf("Expected goal set: %v", err)
	}

	// Add a lazy diff with no arguments, triggering the default behavior
	startLen = cp.OutputLen()
	cp.SendLine("diff")
	if _, err := cp.ExpectSince("Added diff:", startLen); err != nil {
		t.Fatalf("Expected diff added: %v", err)
	}

	// Generate and show output
	startLen = cp.OutputLen()
	cp.SendLine("generate")
	if _, err := cp.ExpectSince("Meta-prompt generated.", startLen); err != nil {
		t.Fatalf("Expected meta generated: %v", err)
	}
	startLen = cp.OutputLen()
	cp.SendLine("show")

	// Verify that the diff was successful and not an error, and shows an initial commit file as new
	if _, err := cp.ExpectSince("### Diff: git diff", startLen); err != nil {
		t.Fatalf("Expected diff section: %v", err)
	}
	if _, err := cp.ExpectSince("diff --git a/example.go b/example.go", startLen); err != nil {
		t.Fatalf("Expected diff content: %v", err)
	}
	if _, err := cp.ExpectSince("new file mode 100644", startLen); err != nil {
		t.Fatalf("Expected new file: %v", err)
	}

	cp.SendLine("exit")
	requireExpectExitCode(t, cp, 0)
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

	opts := termtest.Options{
		CmdName:        binaryPath,
		Args:           []string{"prompt-flow", "-i"},
		DefaultTimeout: 30 * time.Second,
		Env:            env,
		Dir:            workspace,
	}

	cp, err := termtest.NewTest(t, opts)
	if err != nil {
		t.Fatalf("Failed to create termtest: %v", err)
	}
	defer cp.Close()

	startLen := cp.OutputLen()
	if _, err := cp.ExpectSince("Switched to mode: prompt-flow", startLen, 15*time.Second); err != nil {
		t.Fatalf("Expected mode switch to prompt-flow: %v", err)
	}
	if _, err := cp.ExpectSince("(prompt-flow) > ", startLen, 20*time.Second); err != nil {
		t.Fatalf("Expected prompt: %v", err)
	}

	// Ensure a goal is present for generate usage
	startLen = cp.OutputLen()
	if err := cp.SendLine("goal Handle malformed payload test cases"); err != nil {
		t.Fatalf("Failed to send goal: %v\nBuffer: %q", err, cp.GetOutput())
	}
	if _, err := cp.ExpectSince("Goal set.", startLen, 2*time.Second); err != nil {
		t.Fatalf("Expected goal set: %v\nBuffer: %q", err, cp.GetOutput())
	}

	// Helper to test malformed payloads via JS REPL (functions are global in script)
	testMalformed := func(payloadJS string) {
		// addItem("lazy-diff", "malformed", <payloadJS>)
		// SendLine sends all chars at once -> paste detection -> multi-line mode
		// So we send empty line to exit multi-line mode
		startLen := cp.OutputLen()
		if err := cp.SendLine("addItem(\"lazy-diff\", \"malformed\", " + payloadJS + ")"); err != nil {
			t.Fatalf("Failed to send addItem: %v\nBuffer: %q", err, cp.GetOutput())
		}
		// Send empty line to exit multi-line mode
		if err := cp.SendLine(""); err != nil {
			t.Fatalf("Failed to send empty line: %v\nBuffer: %q", err, cp.GetOutput())
		}
		// Wait for the JS command to complete and prompt to return
		if _, err := cp.ExpectSince("(prompt-flow) > ", startLen, 2*time.Second); err != nil {
			t.Fatalf("Expected prompt after addItem: %v\nBuffer: %q", err, cp.GetOutput())
		}

		startLen = cp.OutputLen()
		if err := cp.SendLine("generate"); err != nil {
			t.Fatalf("Failed to send generate: %v\nBuffer: %q", err, cp.GetOutput())
		}
		if _, err := cp.ExpectSince("Meta-prompt generated.", startLen, 2*time.Second); err != nil {
			t.Fatalf("Expected meta generated: %v\nBuffer: %q", err, cp.GetOutput())
		}
		startLen = cp.OutputLen()
		if err := cp.SendLine("show"); err != nil {
			t.Fatalf("Failed to send show: %v\nBuffer: %q", err, cp.GetOutput())
		}

		if _, err := cp.ExpectSince("### Diff Error: malformed", startLen, 2*time.Second); err != nil {
			t.Fatalf("Expected diff error: %v\nBuffer: %q", err, cp.GetOutput())
		}
		if _, err := cp.ExpectSince("Invalid payload: expected a string or string array", startLen, 2*time.Second); err != nil {
			t.Fatalf("Expected error message: %v\nBuffer: %q", err, cp.GetOutput())
		}

		// Clear items list for next sub-case
		startLen = cp.OutputLen()
		if err := cp.SendLine("setItems([])"); err != nil {
			t.Fatalf("Failed to send setItems: %v\nBuffer: %q", err, cp.GetOutput())
		}
		// Send empty line to exit multi-line mode
		if err := cp.SendLine(""); err != nil {
			t.Fatalf("Failed to send empty line: %v\nBuffer: %q", err, cp.GetOutput())
		}
		if _, err := cp.ExpectSince("(prompt-flow) > ", startLen, 2*time.Second); err != nil {
			t.Fatalf("Expected prompt after setItems: %v\nBuffer: %q", err, cp.GetOutput())
		}
	}

	// Case 1: number
	testMalformed("12345")
	// Case 2: boolean
	testMalformed("true")
	// Case 3: object
	testMalformed("({key: 'value'})")
	// Case 4: array of non-strings should trigger array-specific error
	startLen = cp.OutputLen()
	if err := cp.SendLine("addItem(\"lazy-diff\", \"malformed\", [1,2,3])"); err != nil {
		t.Fatalf("Failed to send addItem: %v\nBuffer: %q", err, cp.GetOutput())
	}
	// Send empty line to exit multi-line mode
	if err := cp.SendLine(""); err != nil {
		t.Fatalf("Failed to send empty line: %v\nBuffer: %q", err, cp.GetOutput())
	}
	// Wait for the JS command to complete and prompt to return
	if _, err := cp.ExpectSince("(prompt-flow) > ", startLen, 2*time.Second); err != nil {
		t.Fatalf("Expected prompt after addItem: %v\nBuffer: %q", err, cp.GetOutput())
	}
	startLen = cp.OutputLen()
	if err := cp.SendLine("generate"); err != nil {
		t.Fatalf("Failed to send generate: %v\nBuffer: %q", err, cp.GetOutput())
	}
	if _, err := cp.ExpectSince("Meta-prompt generated.", startLen, 2*time.Second); err != nil {
		t.Fatalf("Expected meta generated: %v\nBuffer: %q", err, cp.GetOutput())
	}
	startLen = cp.OutputLen()
	if err := cp.SendLine("show"); err != nil {
		t.Fatalf("Failed to send show: %v\nBuffer: %q", err, cp.GetOutput())
	}
	if _, err := cp.ExpectSince("### Diff Error: malformed", startLen, 2*time.Second); err != nil {
		t.Fatalf("Expected diff error: %v\nBuffer: %q", err, cp.GetOutput())
	}
	if _, err := cp.ExpectSince("Invalid payload: expected a string array", startLen, 2*time.Second); err != nil {
		t.Fatalf("Expected error message: %v\nBuffer: %q", err, cp.GetOutput())
	}

	if err := cp.SendLine("exit"); err != nil {
		t.Fatalf("Failed to send exit: %v\nBuffer: %q", err, cp.GetOutput())
	}
	requireExpectExitCode(t, cp, 0)
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
		"ONESHOT_CLIPBOARD_CMD=cat > /dev/null",
		"GIT_AUTHOR_NAME=Test User",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test User",
		"GIT_COMMITTER_EMAIL=test@example.com",
	)

	opts := termtest.Options{
		CmdName:        binaryPath,
		Args:           []string{"prompt-flow", "-i"},
		DefaultTimeout: 30 * time.Second,
		Env:            env,
		Dir:            workspace,
	}

	cp, err := termtest.NewTest(t, opts)
	if err != nil {
		t.Fatalf("Failed to create termtest: %v", err)
	}
	defer cp.Close()

	// Wait for startup and prompt — prompt-flow prints a mode switch when entering
	startLen := cp.OutputLen()
	if _, err := cp.ExpectSince("Switched to mode: prompt-flow", startLen, 15*time.Second); err != nil {
		t.Fatalf("Expected mode switch to prompt-flow: %v", err)
	}
	if _, err := cp.ExpectSince("(prompt-flow) > ", startLen, 20*time.Second); err != nil {
		t.Fatalf("Expected prompt: %v", err)
	}

	// Set goal
	startLen = cp.OutputLen()
	cp.SendLine("goal Summarise changes in the diff")
	if _, err := cp.ExpectSince("Goal set.", startLen); err != nil {
		t.Fatalf("Expected goal set: %v", err)
	}

	// Capture git diff (no args -> working tree diff)
	startLen = cp.OutputLen()
	cp.SendLine("diff")
	if _, err := cp.ExpectSince("Added diff:", startLen); err != nil {
		t.Fatalf("Expected diff added: %v", err)
	}

	// Generate meta and show it
	startLen = cp.OutputLen()
	cp.SendLine("generate")
	if _, err := cp.ExpectSince("Meta-prompt generated.", startLen); err != nil {
		t.Fatalf("Expected meta generated: %v", err)
	}

	startLen = cp.OutputLen()
	cp.SendLine("show meta")

	// The meta should include the diff section with diff fencing
	if _, err := cp.ExpectSince("### Diff: git diff", startLen); err != nil {
		t.Fatalf("Expected diff section: %v", err)
	}
	if _, err := cp.ExpectSince("```diff", startLen); err != nil {
		t.Fatalf("Expected diff formatting: %v", err)
	}
	// And it should include at least some content from our modified file
	// created by setupGitRepository (Modified version with new features)
	if _, err := cp.ExpectSince("Modified version with new features", startLen); err != nil {
		t.Fatalf("Expected modified content: %v", err)
	}

	cp.SendLine("exit")
	requireExpectExitCode(t, cp, 0)
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
		"OSM_SESSION_ID=" + fmt.Sprintf("test-%s-%d", t.Name(), time.Now().UnixNano()),
		"OSM_STORAGE_BACKEND=memory",
		"ONESHOT_CLIPBOARD_CMD=cat > " + clipboardFile,
		"EDITOR=" + editorScript,
		"VISUAL=",
	}

	opts := termtest.Options{
		CmdName:        binaryPath,
		Args:           []string{"prompt-flow", "-i"},
		DefaultTimeout: 30 * time.Second,
		Env:            env,
	}

	cp, err := termtest.NewTest(t, opts)
	if err != nil {
		t.Fatalf("Failed to create termtest: %v", err)
	}
	defer cp.Close()

	startLen := cp.OutputLen()
	if _, err := cp.ExpectSince("Switched to mode: prompt-flow", startLen, 15*time.Second); err != nil {
		t.Fatalf("Expected mode switch to prompt-flow: %v", err)
	}
	if _, err := cp.ExpectSince("(prompt-flow) > ", startLen, 20*time.Second); err != nil {
		t.Fatalf("Expected prompt: %v", err)
	}

	// Set up a complete scenario
	startLen = cp.OutputLen()
	cp.SendLine("goal Test clipboard functionality with prompt-flow")
	if _, err := cp.ExpectSince("Goal set.", startLen); err != nil {
		t.Fatalf("Expected goal set: %v", err)
	}

	startLen = cp.OutputLen()
	cp.SendLine("note This is a test note for clipboard verification")
	if _, err := cp.ExpectSince("Added note [", startLen); err != nil {
		t.Fatalf("Expected note added: %v", err)
	}

	startLen = cp.OutputLen()
	cp.SendLine("generate")
	if _, err := cp.ExpectSince("Meta-prompt generated. You can 'show meta', 'copy meta', or provide the task prompt with 'use'.", startLen); err != nil {
		t.Fatalf("Expected meta generated: %v", err)
	}

	// Test copying meta-prompt
	startLen = cp.OutputLen()
	cp.SendLine("copy meta")
	if _, err := cp.ExpectSince("Meta prompt copied to clipboard.", startLen); err != nil {
		t.Fatalf("Expected copy confirmation: %v", err)
	}

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
	startLen = cp.OutputLen()
	cp.SendLine("use Test goal for automated testing")
	if _, err := cp.ExpectSince("Task prompt set.", startLen); err != nil {
		t.Fatalf("Expected task prompt set: %v", err)
	}
	startLen = cp.OutputLen()
	cp.SendLine("copy")
	if _, err := cp.ExpectSince("Final output copied to clipboard.", startLen); err != nil {
		t.Fatalf("Expected copy confirmation: %v", err)
	}

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
	requireExpectExitCode(t, cp, 0)
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
		echo "Context: {{context_txtar}}" >> "$1"
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
{{.context_txtar}}
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
	cmd.Env = os.Environ()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("Command %s %s failed: %v\nstdout: %s\nstderr: %s", name, strings.Join(args, " "), err, stdout.String(), stderr.String())
	}
}

func testCompletePromptFlowWorkflow(t *testing.T, cp *termtest.ConsoleProcess, testFile string) {
	t.Helper()

	// Set goal
	startLen := cp.OutputLen()
	if err := cp.SendLine("goal Enhance Java thread pool with comprehensive monitoring and metrics"); err != nil {
		t.Fatalf("Failed to send goal: %v\nBuffer: %q", err, cp.GetOutput())
	}
	if _, err := cp.ExpectSince("Goal set.", startLen, 2*time.Second); err != nil {
		t.Fatalf("Expected goal set: %v\nBuffer: %q", err, cp.GetOutput())
	}

	// Add test file with absolute path
	startLen = cp.OutputLen()
	if err := cp.SendLine("add " + testFile); err != nil {
		t.Fatalf("Failed to send add: %v\nBuffer: %q", err, cp.GetOutput())
	}
	if _, err := cp.ExpectSince("Added file:", startLen, 2*time.Second); err != nil {
		t.Fatalf("Expected file added: %v\nBuffer: %q", err, cp.GetOutput())
	}

	// Add a note
	startLen = cp.OutputLen()
	if err := cp.SendLine("note Focus on Micrometer integration for production monitoring"); err != nil {
		t.Fatalf("Failed to send note: %v\nBuffer: %q", err, cp.GetOutput())
	}
	if _, err := cp.ExpectSince("Added note [", startLen, 2*time.Second); err != nil {
		t.Fatalf("Expected note added: %v\nBuffer: %q", err, cp.GetOutput())
	}

	// List current state
	startLen = cp.OutputLen()
	if err := cp.SendLine("list"); err != nil {
		t.Fatalf("Failed to send list: %v\nBuffer: %q", err, cp.GetOutput())
	}
	if _, err := cp.ExpectSince("[goal] Enhance Java thread pool", startLen, 2*time.Second); err != nil {
		t.Fatalf("Expected goal in list: %v\nBuffer: %q", err, cp.GetOutput())
	}
	if _, err := cp.ExpectSince("[template] set", startLen, 2*time.Second); err != nil {
		t.Fatalf("Expected template in list: %v\nBuffer: %q", err, cp.GetOutput())
	}
	if _, err := cp.ExpectSince("[1] [file]", startLen, 2*time.Second); err != nil {
		t.Fatalf("Expected file in list: %v\nBuffer: %q", err, cp.GetOutput())
	}
	if _, err := cp.ExpectSince("[2] [note]", startLen, 2*time.Second); err != nil {
		t.Fatalf("Expected note in list: %v\nBuffer: %q", err, cp.GetOutput())
	}

	// Generate meta-prompt
	startLen = cp.OutputLen()
	if err := cp.SendLine("generate"); err != nil {
		t.Fatalf("Failed to send generate: %v\nBuffer: %q", err, cp.GetOutput())
	}
	if _, err := cp.ExpectSince("Meta-prompt generated. You can 'show meta', 'copy meta', or provide the task prompt with 'use'.", startLen, 2*time.Second); err != nil {
		t.Fatalf("Expected meta generated: %v\nBuffer: %q", err, cp.GetOutput())
	}

	// Show meta-prompt to verify content
	startLen = cp.OutputLen()
	if err := cp.SendLine("show meta"); err != nil {
		t.Fatalf("Failed to send show meta: %v\nBuffer: %q", err, cp.GetOutput())
	}
	if _, err := cp.ExpectSince("!! **GOAL:** !!", startLen, 2*time.Second); err != nil {
		t.Fatalf("Expected goal marker: %v\nBuffer: %q", err, cp.GetOutput())
	}
	if _, err := cp.ExpectSince("Enhance Java thread pool with comprehensive monitoring", startLen, 2*time.Second); err != nil {
		t.Fatalf("Expected goal text: %v\nBuffer: %q", err, cp.GetOutput())
	}
	if _, err := cp.ExpectSince("!! **IMPLEMENTATIONS/CONTEXT:** !!", startLen, 2*time.Second); err != nil {
		t.Fatalf("Expected context marker: %v\nBuffer: %q", err, cp.GetOutput())
	}

	// Provide task prompt then show final assembled output
	startLen = cp.OutputLen()
	if err := cp.SendLine("use ready to assemble"); err != nil {
		t.Fatalf("Failed to send use: %v\nBuffer: %q", err, cp.GetOutput())
	}
	if _, err := cp.ExpectSince("Task prompt set.", startLen, 2*time.Second); err != nil {
		t.Fatalf("Expected task prompt set: %v\nBuffer: %q", err, cp.GetOutput())
	}
	startLen = cp.OutputLen()
	if err := cp.SendLine("show"); err != nil {
		t.Fatalf("Failed to send show: %v\nBuffer: %q", err, cp.GetOutput())
	}
	// After 'use', the final output shows the task prompt + context, but NOT the goal
	if _, err := cp.ExpectSince("ready to assemble", startLen, 2*time.Second); err != nil {
		t.Fatalf("Expected task prompt in final: %v\nBuffer: %q", err, cp.GetOutput())
	}
	if _, err := cp.ExpectSince("## IMPLEMENTATIONS/CONTEXT", startLen, 2*time.Second); err != nil {
		t.Fatalf("Expected context section: %v\nBuffer: %q", err, cp.GetOutput())
	}
	if _, err := cp.ExpectSince("### Note:", startLen, 2*time.Second); err != nil {
		t.Fatalf("Expected note section: %v\nBuffer: %q", err, cp.GetOutput())
	}
	if _, err := cp.ExpectSince("Micrometer integration", startLen, 2*time.Second); err != nil {
		t.Fatalf("Expected note content: %v\nBuffer: %q", err, cp.GetOutput())
	}
	if _, err := cp.ExpectSince("ThreadPoolManager.java", startLen, 2*time.Second); err != nil {
		t.Fatalf("Expected file marker: %v\nBuffer: %q", err, cp.GetOutput())
	}
	if _, err := cp.ExpectSince("ThreadPoolExecutor", startLen, 2*time.Second); err != nil {
		t.Fatalf("Expected java content: %v\nBuffer: %q", err, cp.GetOutput())
	}

	// Test copy functionality
	startLen = cp.OutputLen()
	if err := cp.SendLine("copy"); err != nil {
		t.Fatalf("Failed to send copy: %v\nBuffer: %q", err, cp.GetOutput())
	}
	if _, err := cp.ExpectSince("Final output copied to clipboard.", startLen, 2*time.Second); err != nil {
		t.Fatalf("Expected copy confirmation: %v\nBuffer: %q", err, cp.GetOutput())
	}

	if err := cp.SendLine("exit"); err != nil {
		t.Fatalf("Failed to send exit: %v\nBuffer: %q", err, cp.GetOutput())
	}
	requireExpectExitCode(t, cp, 0)
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

	opts := termtest.Options{
		CmdName:        binaryPath,
		Args:           []string{"prompt-flow", "-i"},
		DefaultTimeout: 30 * time.Second,
		Env:            env,
	}

	cp, err := termtest.NewTest(t, opts)
	if err != nil {
		t.Fatalf("Failed to create termtest: %v", err)
	}
	defer cp.Close()

	startLen := cp.OutputLen()
	if _, err := cp.ExpectSince("Switched to mode: prompt-flow", startLen, 15*time.Second); err != nil {
		t.Fatalf("Expected mode switch to prompt-flow: %v", err)
	}
	if _, err := cp.ExpectSince("(prompt-flow) > ", startLen, 20*time.Second); err != nil {
		t.Fatalf("Expected prompt: %v", err)
	}

	// Set goal
	startLen = cp.OutputLen()
	if err := cp.SendLine("goal " + goal); err != nil {
		t.Fatalf("Failed to send goal: %v\nBuffer: %q", err, cp.GetOutput())
	}
	if _, err := cp.ExpectSince("Goal set.", startLen, 2*time.Second); err != nil {
		t.Fatalf("Expected goal set: %v\nBuffer: %q", err, cp.GetOutput())
	}

	// Add files with absolute paths
	for _, file := range files {
		absPath := filepath.Join(workspace, file)
		startLen = cp.OutputLen()
		if err := cp.SendLine("add " + absPath); err != nil {
			t.Fatalf("Failed to send add: %v\nBuffer: %q", err, cp.GetOutput())
		}
		if _, err := cp.ExpectSince("Added file:", startLen, 2*time.Second); err != nil {
			t.Fatalf("Expected file added: %v\nBuffer: %q", err, cp.GetOutput())
		}
	}

	// Add notes
	for _, note := range notes {
		startLen = cp.OutputLen()
		if err := cp.SendLine("note " + note); err != nil {
			t.Fatalf("Failed to send note: %v\nBuffer: %q", err, cp.GetOutput())
		}
		if _, err := cp.ExpectSince("Added note [", startLen, 2*time.Second); err != nil {
			t.Fatalf("Expected note added: %v\nBuffer: %q", err, cp.GetOutput())
		}
	}

	// Generate and show meta-prompt
	startLen = cp.OutputLen()
	if err := cp.SendLine("generate"); err != nil {
		t.Fatalf("Failed to send generate: %v\nBuffer: %q", err, cp.GetOutput())
	}
	if _, err := cp.ExpectSince("Meta-prompt generated. You can 'show meta', 'copy meta', or provide the task prompt with 'use'.", startLen, 2*time.Second); err != nil {
		t.Fatalf("Expected meta generated: %v\nBuffer: %q", err, cp.GetOutput())
	}

	startLen = cp.OutputLen()
	if err := cp.SendLine("show meta"); err != nil {
		t.Fatalf("Failed to send show meta: %v\nBuffer: %q", err, cp.GetOutput())
	}

	// Verify expected content in meta-prompt
	for _, expected := range expectedInMeta {
		if _, err := cp.ExpectSince(expected, startLen, 2*time.Second); err != nil {
			t.Fatalf("Expected %q in meta: %v\nBuffer: %q", expected, err, cp.GetOutput())
		}
	}

	if err := cp.SendLine("exit"); err != nil {
		t.Fatalf("Failed to send exit: %v\nBuffer: %q", err, cp.GetOutput())
	}
	requireExpectExitCode(t, cp, 0)
}
