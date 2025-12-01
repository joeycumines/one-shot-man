//go:build unix

package command

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/joeycumines/one-shot-man/internal/termtest"
)

// TestPromptFlow_GoalCommandOpensEditor verifies that the goal command
// opens the editor when invoked with no arguments.
func TestPromptFlow_GoalCommandOpensEditor(t *testing.T) {
	binaryPath := buildPromptFlowTestBinary(t)
	defer os.Remove(binaryPath)

	workspace := createPromptFlowTestWorkspace(t)
	defer os.RemoveAll(workspace)

	// Create fake editor script that writes a known goal
	editorScript := createGoalEditorScript(t, workspace)

	opts := termtest.Options{
		CmdName:        binaryPath,
		Args:           []string{"prompt-flow", "-i"},
		DefaultTimeout: 50 * time.Second,
		Env: []string{
			"OSM_STORAGE_BACKEND=memory",
			"OSM_SESSION_ID=" + uniqueSessionID(t),
			"EDITOR=" + editorScript,
			"VISUAL=",
			"ONESHOT_CLIPBOARD_CMD=cat > /dev/null",
		},
	}

	cp, err := termtest.NewTest(t, opts)
	if err != nil {
		t.Fatalf("Failed to create termtest: %v", err)
	}
	defer cp.Close()

	// Wait for startup
	requirePromptFlowExpect(t, cp, "(prompt-flow) > ", 10*time.Second)

	// Call goal with no arguments - should trigger editor
	start := cp.OutputLen()
	if err := cp.SendLine("goal"); err != nil {
		t.Fatalf("Failed to send 'goal' command: %v", err)
	}

	// Expect the editor to have written our test goal
	requirePromptFlowExpectSince(t, cp, "Goal updated.", start, 5*time.Second)

	// Verify the goal was actually set by listing
	start = cp.OutputLen()
	if err := cp.SendLine("list"); err != nil {
		t.Fatalf("Failed to send 'list' command: %v", err)
	}
	requirePromptFlowExpectSince(t, cp, "[goal] Test goal from editor", start, 5*time.Second)

	cp.SendLine("exit")
}

// TestPromptFlow_UseCommandOpensEditor verifies that the use command
// opens the editor when invoked with no arguments.
func TestPromptFlow_UseCommandOpensEditor(t *testing.T) {
	binaryPath := buildPromptFlowTestBinary(t)
	defer os.Remove(binaryPath)

	workspace := createPromptFlowTestWorkspace(t)
	defer os.RemoveAll(workspace)

	// Create fake editor script that writes a known task prompt
	editorScript := createUseEditorScript(t, workspace)

	opts := termtest.Options{
		CmdName:        binaryPath,
		Args:           []string{"prompt-flow", "-i"},
		DefaultTimeout: 15 * time.Second,
		Env: []string{
			"OSM_STORAGE_BACKEND=memory",
			"OSM_SESSION_ID=" + uniqueSessionID(t),
			"EDITOR=" + editorScript,
			"VISUAL=",
			"ONESHOT_CLIPBOARD_CMD=cat > /dev/null",
		},
	}

	cp, err := termtest.NewTest(t, opts)
	if err != nil {
		t.Fatalf("Failed to create termtest: %v", err)
	}
	defer cp.Close()

	// Wait for startup
	requirePromptFlowExpect(t, cp, "(prompt-flow) > ", 10*time.Second)

	// Set a goal first
	start := cp.OutputLen()
	if err := cp.SendLine("goal Test goal for use command"); err != nil {
		t.Fatalf("Failed to send 'goal' command: %v", err)
	}
	requirePromptFlowExpectSince(t, cp, "Goal set.", start, 5*time.Second)

	// Generate meta-prompt (required before use)
	start = cp.OutputLen()
	if err := cp.SendLine("generate"); err != nil {
		t.Fatalf("Failed to send 'generate' command: %v", err)
	}
	requirePromptFlowExpectSince(t, cp, "Meta-prompt generated.", start, 5*time.Second)

	// Call use with no arguments - should trigger editor
	start = cp.OutputLen()
	if err := cp.SendLine("use"); err != nil {
		t.Fatalf("Failed to send 'use' command: %v", err)
	}

	// Expect the editor to have written our test task prompt
	requirePromptFlowExpectSince(t, cp, "Task prompt set.", start, 5*time.Second)

	// Verify the task prompt was actually set by listing
	start = cp.OutputLen()
	if err := cp.SendLine("list"); err != nil {
		t.Fatalf("Failed to send 'list' command: %v", err)
	}
	requirePromptFlowExpectSince(t, cp, "[prompt] Test task prompt from editor", start, 5*time.Second)

	cp.SendLine("exit")
}

// Helper functions

func buildPromptFlowTestBinary(t *testing.T) string {
	t.Helper()
	binaryPath := filepath.Join(t.TempDir(), "osm-test")
	cmd := exec.Command("go", "build", "-o", binaryPath, "../../cmd/osm")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to build test binary: %v\nStderr: %s", err, stderr.String())
	}
	return binaryPath
}

func createPromptFlowTestWorkspace(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	return dir
}

func createGoalEditorScript(t *testing.T, workspace string) string {
	t.Helper()
	editorScript := filepath.Join(workspace, "goal-editor.sh")
	scriptContent := `#!/bin/sh
# Fake editor for testing goal command
case "$(basename "$1")" in
	goal)
		echo "Test goal from editor" > "$1"
		;;
	*)
		echo "unexpected file: $(basename "$1")" > "$1"
		;;
esac
`
	if err := os.WriteFile(editorScript, []byte(scriptContent), 0755); err != nil {
		t.Fatalf("Failed to write goal editor script: %v", err)
	}
	return editorScript
}

func createUseEditorScript(t *testing.T, workspace string) string {
	t.Helper()
	editorScript := filepath.Join(workspace, "use-editor.sh")
	scriptContent := `#!/bin/sh
# Fake editor for testing use command
case "$(basename "$1")" in
	task-prompt)
		echo "Test task prompt from editor" > "$1"
		;;
	*)
		echo "unexpected file: $(basename "$1")" > "$1"
		;;
esac
`
	if err := os.WriteFile(editorScript, []byte(scriptContent), 0755); err != nil {
		t.Fatalf("Failed to write use editor script: %v", err)
	}
	return editorScript
}

func requirePromptFlowExpect(t *testing.T, cp *termtest.ConsoleProcess, expected string, timeout time.Duration) {
	t.Helper()
	startLen := cp.OutputLen()
	if raw, err := cp.ExpectSince(expected, startLen, timeout); err != nil {
		t.Fatalf("Expected to find %q in output, but got error: %v\nRaw:\n%s\n", expected, err, raw)
	}
}

func requirePromptFlowExpectSince(t *testing.T, cp *termtest.ConsoleProcess, expected string, start int, timeout time.Duration) {
	t.Helper()
	if raw, err := cp.ExpectSince(expected, start, timeout); err != nil {
		t.Fatalf("Expected to find %q in new output since offset %d, but got error: %v\nRaw:\n%s\n", expected, start, err, raw)
	}
}

// uniqueSessionID generates a unique session ID for a test to prevent session state sharing
func uniqueSessionID(t *testing.T) string {
	return fmt.Sprintf("test-%s-%d", t.Name(), time.Now().UnixNano())
}
