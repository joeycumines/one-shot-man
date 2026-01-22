//go:build unix

package command

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/joeycumines/go-prompt/termtest"
	"github.com/joeycumines/one-shot-man/internal/testutil"
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cp, err := termtest.NewConsole(ctx,
		termtest.WithCommand(binaryPath, "prompt-flow", "-i"),
		termtest.WithDefaultTimeout(50*time.Second),
		termtest.WithEnv([]string{
			"OSM_STORE=memory",
			"OSM_SESSION=" + uniqueSessionID(t),
			"EDITOR=" + editorScript,
			"VISUAL=",
			"OSM_CLIPBOARD=cat > /dev/null",
		}),
	)
	if err != nil {
		t.Fatalf("Failed to create termtest: %v", err)
	}
	defer cp.Close()

	// Wait for startup
	requirePromptFlowExpect(t, ctx, cp, "(prompt-flow) > ", 10*time.Second)

	// Call goal with no arguments - should trigger editor
	snap := cp.Snapshot()
	if err := cp.SendLine("goal"); err != nil {
		t.Fatalf("Failed to send 'goal' command: %v", err)
	}

	// Expect the editor to have written our test goal
	requirePromptFlowExpectSince(t, ctx, cp, "Goal updated.", snap, 20*time.Second)

	// Verify the goal was actually set by listing
	snap = cp.Snapshot()
	if err := cp.SendLine("list"); err != nil {
		t.Fatalf("Failed to send 'list' command: %v", err)
	}
	requirePromptFlowExpectSince(t, ctx, cp, "[goal] Test goal from editor", snap, 20*time.Second)

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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cp, err := termtest.NewConsole(ctx,
		termtest.WithCommand(binaryPath, "prompt-flow", "-i"),
		termtest.WithDefaultTimeout(25*time.Second),
		termtest.WithEnv([]string{
			"OSM_STORE=memory",
			"OSM_SESSION=" + uniqueSessionID(t),
			"EDITOR=" + editorScript,
			"VISUAL=",
			"OSM_CLIPBOARD=cat > /dev/null",
		}),
	)
	if err != nil {
		t.Fatalf("Failed to create termtest: %v", err)
	}
	defer cp.Close()

	// Wait for startup
	requirePromptFlowExpect(t, ctx, cp, "(prompt-flow) > ", 10*time.Second)

	// Set a goal first
	snap := cp.Snapshot()
	if err := cp.SendLine("goal Test goal for use command"); err != nil {
		t.Fatalf("Failed to send 'goal' command: %v", err)
	}
	requirePromptFlowExpectSince(t, ctx, cp, "Goal set.", snap, 20*time.Second)

	// Generate meta-prompt (required before use)
	snap = cp.Snapshot()
	if err := cp.SendLine("generate"); err != nil {
		t.Fatalf("Failed to send 'generate' command: %v", err)
	}
	requirePromptFlowExpectSince(t, ctx, cp, "Meta-prompt generated.", snap, 20*time.Second)

	// Call use with no arguments - should trigger editor
	snap = cp.Snapshot()
	if err := cp.SendLine("use"); err != nil {
		t.Fatalf("Failed to send 'use' command: %v", err)
	}

	// Expect the editor to have written our test task prompt
	requirePromptFlowExpectSince(t, ctx, cp, "Task prompt set.", snap, 20*time.Second)

	// Verify the task prompt was actually set by listing
	snap = cp.Snapshot()
	if err := cp.SendLine("list"); err != nil {
		t.Fatalf("Failed to send 'list' command: %v", err)
	}
	requirePromptFlowExpectSince(t, ctx, cp, "[prompt] Test task prompt from editor", snap, 20*time.Second)

	if err := cp.SendLine("exit"); err != nil {
		t.Error(err)
	}
}

// Helper functions

func buildPromptFlowTestBinary(t *testing.T) string {
	t.Helper()
	// Get the working directory and compute project root
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}
	projectDir := filepath.Clean(filepath.Join(wd, "..", ".."))

	binaryPath := filepath.Join(t.TempDir(), "osm-test")
	cmd := exec.Command("go", "build", "-tags=integration", "-o", binaryPath, "./cmd/osm")
	cmd.Dir = projectDir // Critical: set working directory to project root
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
	if err := os.Chmod(editorScript, 0755); err != nil {
		t.Fatalf("Failed to chmod goal editor script: %v", err)
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
	if err := os.Chmod(editorScript, 0755); err != nil {
		t.Fatalf("Failed to chmod use editor script: %v", err)
	}
	return editorScript
}

func requirePromptFlowExpect(t *testing.T, ctx context.Context, cp *termtest.Console, expected string, timeout time.Duration) {
	t.Helper()
	snap := cp.Snapshot()
	requirePromptFlowExpectSince(t, ctx, cp, expected, snap, timeout)
}

func requirePromptFlowExpectSince(t *testing.T, ctx context.Context, cp *termtest.Console, expected string, snap termtest.Snapshot, timeout time.Duration) {
	t.Helper()
	tCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if err := cp.Expect(tCtx, snap, termtest.Contains(expected), "expected output"); err != nil {
		// Include the full buffer in the failure message to assist debugging, matching legacy behavior intent
		t.Fatalf("Expected to find %q in new output, but got error: %v\nRaw:\n%s\n", expected, err, cp.String())
	}
}

// uniqueSessionID generates a unique session ID for a test to prevent session state sharing
func uniqueSessionID(t *testing.T) string {
	return testutil.NewTestSessionID("test", t.Name())
}
