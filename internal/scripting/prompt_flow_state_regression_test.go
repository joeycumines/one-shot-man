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
	defer os.Remove(binaryPath)

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

	opts := termtest.Options{
		CmdName:        binaryPath,
		Args:           []string{"prompt-flow", "-i"},
		DefaultTimeout: 30 * time.Second,
		Env: []string{
			"EDITOR=" + editor,
			"VISUAL=", // force EDITOR usage
			"ONESHOT_CLIPBOARD_CMD=cat >/dev/null",
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

	// Minimal setup: goal and generate meta-prompt
	cp.SendLine("goal My goal for testing phase reversion")
	requireExpect(t, cp, "Goal set.")

	cp.SendLine("generate")
	// New flow prints this message; wait for any part of it
	requireExpect(t, cp, "Meta-prompt generated.")

	// Set task prompt using inline text (no editor)
	cp.SendLine("use hello world")
	requireExpect(t, cp, "Task prompt set.")

	// Sanity: default show now assembles final output
	cp.SendLine("show")
	requireExpect(t, cp, "## IMPLEMENTATIONS/CONTEXT")

	// Now clear the task prompt via edit -> empty
	cp.SendLine("edit prompt")
	// Expect acknowledgement matching the non-destructive workflow
	requireExpect(t, cp, "Task prompt not updated (no content provided).")

	// Default show should still render the final assembled prompt
	cp.SendLine("show")
	requireExpect(t, cp, "## IMPLEMENTATIONS/CONTEXT")
	requireExpect(t, cp, "hello world")

	cp.SendLine("exit")
	requireExpectExitCode(t, cp, 0)
}
