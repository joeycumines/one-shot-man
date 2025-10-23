//go:build unix

package scripting

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/joeycumines/one-shot-man/internal/termtest"
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

	env := newTestProcessEnv(t)
	env = append(env, "EDITOR="+editor, "VISUAL=")

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

	// Wait for startup
	startLen := cp.OutputLen()
	if _, err := cp.ExpectSince("one-shot-man Rich TUI Terminal", startLen, 15*time.Second); err != nil {
		t.Fatalf("Expected TUI startup: %v", err)
	}
	if _, err := cp.ExpectSince("(prompt-builder) > ", startLen, 20*time.Second); err != nil {
		t.Fatalf("Expected prompt: %v", err)
	}

	// Minimal setup: goal and generate meta-prompt
	startLen = cp.OutputLen()
	cp.SendLine("goal My goal for testing phase reversion")
	if _, err := cp.ExpectSince("Goal set.", startLen); err != nil {
		t.Fatalf("Expected goal set: %v", err)
	}

	startLen = cp.OutputLen()
	cp.SendLine("generate")
	// New flow prints this message; wait for any part of it
	if _, err := cp.ExpectSince("Meta-prompt generated.", startLen); err != nil {
		t.Fatalf("Expected meta-prompt generated: %v", err)
	}

	// Set task prompt using inline text (no editor)
	startLen = cp.OutputLen()
	cp.SendLine("use hello world")
	if _, err := cp.ExpectSince("Task prompt set.", startLen); err != nil {
		t.Fatalf("Expected task prompt set: %v", err)
	}

	// Sanity: default show now assembles final output
	startLen = cp.OutputLen()
	cp.SendLine("show")
	if _, err := cp.ExpectSince("## IMPLEMENTATIONS/CONTEXT", startLen); err != nil {
		t.Fatalf("Expected context section: %v", err)
	}

	// Now clear the task prompt via edit -> empty
	startLen = cp.OutputLen()
	cp.SendLine("edit prompt")
	// Expect acknowledgement matching the non-destructive workflow
	if _, err := cp.ExpectSince("Task prompt not updated (no content provided).", startLen); err != nil {
		t.Fatalf("Expected task prompt not updated: %v", err)
	}

	// Default show should still render the final assembled prompt
	startLen = cp.OutputLen()
	cp.SendLine("show")
	if _, err := cp.ExpectSince("## IMPLEMENTATIONS/CONTEXT", startLen); err != nil {
		t.Fatalf("Expected context section: %v", err)
	}
	if _, err := cp.ExpectSince("hello world", startLen); err != nil {
		t.Fatalf("Expected task prompt text: %v", err)
	}

	cp.SendLine("exit")
	requireExpectExitCode(t, cp, 0)
}
