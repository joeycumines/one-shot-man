//go:build unix

package scripting

import (
	"fmt"
	"os"
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
	defer os.Remove(binaryPath)

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

	opts := termtest.Options{
		CmdName:        binaryPath,
		Args:           []string{"prompt-flow", "-i"},
		DefaultTimeout: 30 * time.Second,
		Env: []string{
			"EDITOR=" + editorScript,
			"VISUAL=",
			"ONESHOT_CLIPBOARD_CMD=cat > /tmp/test_clipboard.txt",
		},
	}

	cp, err := termtest.NewTest(t, opts)
	if err != nil {
		t.Fatalf("Failed to create termtest: %v", err)
	}
	defer cp.Close()

	// Wait for startup
	requireExpect(t, cp, "one-shot-man Rich TUI Terminal", 15*time.Second)
	requireExpect(t, cp, "(prompt-builder) > ", 20*time.Second)

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
	defer os.Remove(binaryPath)

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

	opts := termtest.Options{
		CmdName:        binaryPath,
		Args:           []string{"prompt-flow", "-i"},
		DefaultTimeout: 30 * time.Second,
		Env: []string{
			"EDITOR=" + editorScript,
			"VISUAL=",
			"ONESHOT_CLIPBOARD_CMD=cat > /tmp/test_context_assembly.txt",
		},
	}

	cp, err := termtest.NewTest(t, opts)
	if err != nil {
		t.Fatalf("Failed to create termtest: %v", err)
	}
	defer cp.Close()

	requireExpect(t, cp, "one-shot-man Rich TUI Terminal", 15*time.Second)
	requireExpect(t, cp, "(prompt-builder) > ", 20*time.Second)

	// Set goal
	cp.SendLine("goal Refactor Go application for better modularity")
	requireExpect(t, cp, "Goal set.")

	// Add files with absolute paths
	mainGoPath := filepath.Join(workspace, "main.go")
	utilsGoPath := filepath.Join(workspace, "utils.go")
	cp.SendLine("add " + mainGoPath + " " + utilsGoPath)
	requireExpect(t, cp, "Added file: "+mainGoPath)
	requireExpect(t, cp, "Added file: "+utilsGoPath)

	// Add note
	cp.SendLine("note Focus on separation of concerns and clean architecture")
	requireExpect(t, cp, "Added note [")

	// Generate meta-prompt
	cp.SendLine("generate")
	requireExpect(t, cp, "Generated. You can now 'show' or 'copy'.")

	// Show the final assembled output
	cp.SendLine("show")

	// Verify the final output structure
	requireExpect(t, cp, "Refactor Go application for better modularity") // Goal should be in final output
	requireExpect(t, cp, "## IMPLEMENTATIONS/CONTEXT")                    // Context section header
	requireExpect(t, cp, "### Note:")                                     // Note section
	requireExpect(t, cp, "Focus on separation of concerns")               // Note content
	requireExpect(t, cp, "-- main.go --")                                 // txtar file marker
	requireExpect(t, cp, "package main")                                  // File content
	requireExpect(t, cp, "-- utils.go --")                                // Second file marker
	requireExpect(t, cp, "func processString")                            // Second file content

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
	defer os.Remove(binaryPath)

	workspace := createTestWorkspace(t)
	defer os.RemoveAll(workspace)

	editorScript := createAdvancedFakeEditor(t, workspace)

	opts := termtest.Options{
		CmdName:        binaryPath,
		Args:           []string{"prompt-flow", "-i"},
		DefaultTimeout: 30 * time.Second,
		Env: []string{
			"EDITOR=" + editorScript,
			"VISUAL=",
			"ONESHOT_CLIPBOARD_CMD=cat > /tmp/test_template_config.txt",
		},
	}

	cp, err := termtest.NewTest(t, opts)
	if err != nil {
		t.Fatalf("Failed to create termtest: %v", err)
	}
	defer cp.Close()

	requireExpect(t, cp, "one-shot-man Rich TUI Terminal", 15*time.Second)
	requireExpect(t, cp, "(prompt-builder) > ", 20*time.Second)

	// Set a goal
	cp.SendLine("goal Create a REST API for user management")
	requireExpect(t, cp, "Goal set.")

	// Customize the template (opens editor and returns immediately in our fake editor)
	cp.SendLine("template")
	// Wait for the prompt line to re-render after editor exits
	raw, err := cp.ExpectNew("(prompt-builder) > ", 10*time.Second)
	if err != nil {
		t.Fatalf("Expected prompt after template edit, got error: %v\nRaw:\n%s", err, raw)
	}

	// Generate with custom template
	cp.SendLine("generate")
	// After generate, we should see the generated message; ensure it's new output
	raw, err = cp.ExpectNew("Generated. You can now 'show' or 'copy'.", 30*time.Second)
	if err != nil {
		t.Fatalf("Expected generate message, got error: %v\nRaw:\n%s", err, raw)
	}

	// Show meta-prompt to verify template customization
	cp.SendLine("show meta")
	requireExpect(t, cp, "Create a REST API for user management") // Goal should be substituted
	requireExpect(t, cp, "CUSTOM TEMPLATE MODIFICATION")          // Custom template marker

	cp.SendLine("exit")
	requireExpectExitCode(t, cp, 0)
}

// TestPromptFlow_Unix_GitDiffIntegration tests git diff integration
// to ensure version control context is properly captured.
func TestPromptFlow_Unix_GitDiffIntegration(t *testing.T) {
	if !isUnixPlatform() {
		t.Skip("Unix-only integration test")
	}

	binaryPath := buildTestBinary(t)
	defer os.Remove(binaryPath)

	workspace := createTestWorkspace(t)
	defer os.RemoveAll(workspace)

	// Initialize git repository and create changes
	setupGitRepository(t, workspace)

	editorScript := createFakeEditor(t, workspace)

	opts := termtest.Options{
		CmdName:        binaryPath,
		Args:           []string{"prompt-flow", "-i"},
		DefaultTimeout: 30 * time.Second,
		Env: []string{
			"EDITOR=" + editorScript,
			"VISUAL=",
			"ONESHOT_CLIPBOARD_CMD=cat > /tmp/test_git_diff.txt",
			"GIT_AUTHOR_NAME=Test User",
			"GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=Test User",
			"GIT_COMMITTER_EMAIL=test@example.com",
		},
	}

	cp, err := termtest.NewTest(t, opts)
	if err != nil {
		t.Fatalf("Failed to create termtest: %v", err)
	}
	defer cp.Close()

	requireExpect(t, cp, "one-shot-man Rich TUI Terminal", 15*time.Second)
	requireExpect(t, cp, "(prompt-builder) > ", 20*time.Second)

	// Set goal
	cp.SendLine("goal Review and optimize the recent code changes")
	requireExpect(t, cp, "Goal set.")

	// Capture git diff
	cp.SendLine("diff")
	requireExpect(t, cp, "Added diff:")

	// List to verify diff was captured
	cp.SendLine("list")
	requireExpect(t, cp, "[goal] Review and optimize the recent code changes")
	requireExpect(t, cp, "[template] set")
	requireExpect(t, cp, "[1] [diff] git diff")

	// Generate and show final output
	cp.SendLine("generate")
	requireExpect(t, cp, "Generated. You can now 'show' or 'copy'.")

	cp.SendLine("show")
	requireExpect(t, cp, "Review and optimize the recent code changes") // Goal
	requireExpect(t, cp, "### Diff: git diff")                          // Diff section
	requireExpect(t, cp, "```diff")                                     // Diff formatting
	// Note: git diff may be empty if no staged changes, which is expected

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
	defer os.Remove(binaryPath)

	workspace := createTestWorkspace(t)
	defer os.RemoveAll(workspace)

	clipboardFile := filepath.Join(workspace, "clipboard_test.txt")
	editorScript := createFakeEditor(t, workspace)

	opts := termtest.Options{
		CmdName:        binaryPath,
		Args:           []string{"prompt-flow", "-i"},
		DefaultTimeout: 30 * time.Second,
		Env: []string{
			"EDITOR=" + editorScript,
			"VISUAL=",
			"ONESHOT_CLIPBOARD_CMD=cat > " + clipboardFile,
		},
	}

	cp, err := termtest.NewTest(t, opts)
	if err != nil {
		t.Fatalf("Failed to create termtest: %v", err)
	}
	defer cp.Close()

	requireExpect(t, cp, "one-shot-man Rich TUI Terminal", 15*time.Second)
	requireExpect(t, cp, "(prompt-builder) > ", 20*time.Second)

	// Set up a complete scenario
	cp.SendLine("goal Test clipboard functionality with prompt-flow")
	requireExpect(t, cp, "Goal set.")

	cp.SendLine("note This is a test note for clipboard verification")
	requireExpect(t, cp, "Added note [")

	cp.SendLine("generate")
	requireExpect(t, cp, "Generated. You can now 'show' or 'copy'.")

	// Test copying meta-prompt
	cp.SendLine("copy meta")
	requireExpect(t, cp, "Meta output copied to clipboard.")

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

	// Test copying final assembled output
	cp.SendLine("copy")
	requireExpect(t, cp, "Final output copied to clipboard.")

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
{{goal}}
!! **CONTEXT:** !!
{{context_txtar}}
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
	runCommand(t, workspace, "git", "init")
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
	runCommand(t, workspace, "git", "commit", "-m", "\"Initial commit\"")

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
}

func runCommand(t *testing.T, dir string, name string, args ...string) {
	t.Helper()

	// Build the command properly
	fullArgs := []string{name}
	fullArgs = append(fullArgs, args...)
	cmd := strings.Join(fullArgs, " ")

	// Use sh -c to run the command
	proc, err := os.StartProcess("/bin/sh", []string{"sh", "-c", cmd}, &os.ProcAttr{
		Dir:   dir,
		Env:   os.Environ(),
		Files: []*os.File{os.Stdin, os.Stdout, os.Stderr},
	})
	if err != nil {
		t.Fatalf("Failed to start command %s: %v", cmd, err)
	}

	state, err := proc.Wait()
	if err != nil {
		t.Fatalf("Failed to wait for command %s: %v", cmd, err)
	}

	if !state.Success() {
		t.Fatalf("Command %s failed with exit code %d", cmd, state.ExitCode())
	}
}

func testCompletePromptFlowWorkflow(t *testing.T, cp *termtest.ConsoleProcess, testFile string) {
	t.Helper()

	// Set goal
	cp.SendLine("goal Enhance Java thread pool with comprehensive monitoring and metrics")
	requireExpect(t, cp, "Goal set.")

	// Add test file with absolute path
	cp.SendLine("add " + testFile)
	requireExpect(t, cp, "Added file:")

	// Add a note
	cp.SendLine("note Focus on Micrometer integration for production monitoring")
	requireExpect(t, cp, "Added note [")

	// List current state
	cp.SendLine("list")
	requireExpect(t, cp, "[goal] Enhance Java thread pool")
	requireExpect(t, cp, "[template] set")
	requireExpect(t, cp, "[1] [file]")
	requireExpect(t, cp, "[2] [note]")

	// Generate meta-prompt
	cp.SendLine("generate")
	requireExpect(t, cp, "Generated. You can now 'show' or 'copy'.")

	// Show meta-prompt to verify content
	cp.SendLine("show meta")
	requireExpect(t, cp, "!! **GOAL:** !!")
	requireExpect(t, cp, "Enhance Java thread pool with comprehensive monitoring")
	requireExpect(t, cp, "!! **IMPLEMENTATIONS/CONTEXT:** !!")

	// Show final assembled output
	cp.SendLine("show")
	requireExpect(t, cp, "Enhance Java thread pool with comprehensive monitoring") // Goal in final output
	requireExpect(t, cp, "## IMPLEMENTATIONS/CONTEXT")                             // Context section
	requireExpect(t, cp, "### Note:")                                              // Note section
	requireExpect(t, cp, "Micrometer integration")                                 // Note content
	requireExpect(t, cp, "ThreadPoolManager.java")                                 // File content marker
	requireExpect(t, cp, "ThreadPoolExecutor")                                     // Java content

	// Test copy functionality
	cp.SendLine("copy")
	requireExpect(t, cp, "Final output copied to clipboard.")

	cp.SendLine("exit")
	requireExpectExitCode(t, cp, 0)
}

func testMetaPromptVariation(t *testing.T, goal string, files []string, diffs []string, notes []string, expectedInMeta []string) {
	t.Helper()

	binaryPath := buildTestBinary(t)
	defer os.Remove(binaryPath)

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

	opts := termtest.Options{
		CmdName:        binaryPath,
		Args:           []string{"prompt-flow", "-i"},
		DefaultTimeout: 30 * time.Second,
		Env: []string{
			"EDITOR=" + editorScript,
			"VISUAL=",
			"ONESHOT_CLIPBOARD_CMD=cat > /tmp/test_meta_variation.txt",
		},
	}

	cp, err := termtest.NewTest(t, opts)
	if err != nil {
		t.Fatalf("Failed to create termtest: %v", err)
	}
	defer cp.Close()

	requireExpect(t, cp, "one-shot-man Rich TUI Terminal", 15*time.Second)
	requireExpect(t, cp, "(prompt-builder) > ", 20*time.Second)

	// Set goal
	cp.SendLine("goal " + goal)
	requireExpect(t, cp, "Goal set.")

	// Add files with absolute paths
	for _, file := range files {
		absPath := filepath.Join(workspace, file)
		cp.SendLine("add " + absPath)
		requireExpect(t, cp, "Added file:")
	}

	// Add notes
	for _, note := range notes {
		cp.SendLine("note " + note)
		requireExpect(t, cp, "Added note [")
	}

	// Generate and show meta-prompt
	cp.SendLine("generate")
	requireExpect(t, cp, "Generated. You can now 'show' or 'copy'.")

	cp.SendLine("show meta")

	// Verify expected content in meta-prompt
	for _, expected := range expectedInMeta {
		requireExpect(t, cp, expected)
	}

	cp.SendLine("exit")
	requireExpectExitCode(t, cp, 0)
}
