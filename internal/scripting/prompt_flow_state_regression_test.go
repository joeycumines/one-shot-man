//go:build unix

package scripting

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/joeycumines/one-shot-man/internal/termtest"
)

// TestPromptFlow_EditPromptClearingRevertsPhase validates that when the task prompt
// is cleared via `edit prompt` (empty editor result), the phase reverts to META_GENERATED
// and default `show` switches back to meta-prompt output.
func TestPromptFlow_EditPromptClearingRevertsPhase(t *testing.T) {
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
	// Expect acknowledgement of clearing and phase reversion
	requireExpect(t, cp, "Task prompt cleared. Reverted to meta-prompt phase.")

	// Default show should now print meta-prompt, not the assembled final
	cp.SendLine("show")
	requireExpect(t, cp, "!! **GOAL:** !!")

	cp.SendLine("exit")
	requireExpectExitCode(t, cp, 0)
}
