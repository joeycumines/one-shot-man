//go:build unix

package scripting

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/joeycumines/go-prompt/termtest"
)

// TestPromptFlow_EditPromptClearingPreservesPhase ensures that clearing the task prompt via
// `edit prompt` behaves like the `use` command and leaves the existing prompt/state intact.
func TestPromptFlow_EditPromptClearingPreservesPhase(t *testing.T) {
	if !isUnixPlatform() {
		t.Skip("Unix-only integration test")
	}

	binaryPath := buildTestBinary(t)

	// Create a temp dir and a fake editor that clears task-prompt content
	tmp := t.TempDir()
	editor := filepath.Join(tmp, "empty-task-prompt-editor.sh")
	script := `#!/bin/sh
# If invoked for task-prompt, clear the file; otherwise leave as-is
case "$(basename "$1")" in
    "task-prompt")
        : > "$1"
        ;;
    *)
        # no-op; ensure file exists
        :
        ;;
esac
`
	if err := os.WriteFile(editor, []byte(script), 0o755); err != nil {
		t.Fatalf("failed to write editor: %v", err)
	}
	if err := os.Chmod(editor, 0o755); err != nil {
		t.Fatalf("failed to chmod editor: %v", err)
	}

	env := newTestProcessEnv(t)
	env = append(env, "EDITOR="+editor, "VISUAL=")

	ctx, cancel := context.WithCancel(t.Context())
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

	expect := func(timeout time.Duration, since termtest.Snapshot, cond termtest.Condition, description string) error {
		ctx, cancel := context.WithTimeout(t.Context(), timeout)
		defer cancel()
		return cp.Expect(ctx, since, cond, description)
	}

	// Wait for startup — prompt-flow prints a mode switch on enter
	snap := cp.Snapshot()
	if err := expect(15*time.Second, snap, termtest.Contains("Switched to mode: prompt-flow"), "mode switch"); err != nil {
		t.Fatalf("Expected mode switch to prompt-flow: %v", err)
	}
	if err := expect(20*time.Second, snap, termtest.Contains("(prompt-flow) > "), "prompt"); err != nil {
		t.Fatalf("Expected prompt: %v", err)
	}

	// Minimal setup: goal and generate meta-prompt
	snap = cp.Snapshot()
	cp.SendLine("goal My goal for testing phase reversion")
	if err := expect(30*time.Second, snap, termtest.Contains("Goal set."), "goal set"); err != nil {
		t.Fatalf("Expected goal set: %v", err)
	}

	snap = cp.Snapshot()
	cp.SendLine("generate")
	// New flow prints this message; wait for any part of it
	if err := expect(30*time.Second, snap, termtest.Contains("Meta-prompt generated."), "meta-prompt generated"); err != nil {
		t.Fatalf("Expected meta-prompt generated: %v", err)
	}

	// Set task prompt using inline text (no editor)
	snap = cp.Snapshot()
	cp.SendLine("use hello world")
	if err := expect(30*time.Second, snap, termtest.Contains("Task prompt set."), "task prompt set"); err != nil {
		t.Fatalf("Expected task prompt set: %v", err)
	}

	// Sanity: default show now assembles final output
	snap = cp.Snapshot()
	cp.SendLine("show")
	if err := expect(30*time.Second, snap, termtest.Contains("## IMPLEMENTATIONS/CONTEXT"), "context section"); err != nil {
		t.Fatalf("Expected context section: %v", err)
	}

	// Now clear the task prompt via edit -> empty
	snap = cp.Snapshot()
	cp.SendLine("edit prompt")
	// Expect acknowledgement matching the non-destructive workflow
	if err := expect(30*time.Second, snap, termtest.Contains("Task prompt not updated (no content provided)."), "task prompt not updated"); err != nil {
		t.Fatalf("Expected task prompt not updated: %v", err)
	}

	// Default show should still render the final assembled prompt
	snap = cp.Snapshot()
	cp.SendLine("show")
	if err := expect(30*time.Second, snap, termtest.Contains("## IMPLEMENTATIONS/CONTEXT"), "context section"); err != nil {
		t.Fatalf("Expected context section: %v", err)
	}
	if err := expect(30*time.Second, snap, termtest.Contains("hello world"), "task prompt text"); err != nil {
		t.Fatalf("Expected task prompt text: %v", err)
	}

	cp.SendLine("exit")
	requireExpectExitCode(t.Context(), t, cp, 0)
}
