package scripting

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/joeycumines/one-shot-man/internal/termtest"
)

// TestPromptFlow_NonInteractive ensures the prompt-flow script registers and enters its mode
// when executed in non-interactive test mode, and emits the expected banner/help text.
func TestPromptFlow_NonInteractive(t *testing.T) {
	binaryPath := buildTestBinary(t)
	defer os.Remove(binaryPath)

	// Run the CLI in non-interactive test mode to evaluate the built-in prompt-flow command
	opts := termtest.Options{
		CmdName:        binaryPath,
		Args:           []string{"prompt-flow", "--test"},
		DefaultTimeout: 20 * time.Second,
	}

	cp, err := termtest.NewTest(t, opts)
	if err != nil {
		t.Fatalf("Failed to start test process: %v", err)
	}
	defer cp.Close()

	// Expect the prompt-flow banner/help emitted by onEnter after auto-switch
	requireExpect(t, cp, "Prompt Flow: goal/context/template -> generate -> use -> assemble")
	requireExpect(t, cp, "Type 'help' for commands.")
	requireExpect(t, cp, "Commands: goal, add, diff, note, list, edit, remove, template, generate, use, show [meta|prompt], copy [meta|prompt], help, exit")

	// Process should terminate on its own (non-interactive mode)
	requireExpectExitCode(t, cp, 0)
}

// TestPromptFlow_Interactive drives a minimal happy path in interactive mode without invoking the editor.
// It avoids commands that would open the system editor (goal without args, template, generate).
func TestPromptFlow_Interactive(t *testing.T) {
	binaryPath := buildTestBinary(t)
	defer os.Remove(binaryPath)

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}
	projectDir := filepath.Clean(filepath.Join(wd, "..", ".."))

	// Create a tiny shell to act as EDITOR that appends a suffix to the file; use sh -c echo > "$1"
	// We'll simulate editor by using /bin/sh -c 'printf ... > file' via setting EDITOR to /bin/sh and VISUAL to empty,
	// but since system.openEditor passes the file as single arg, we instead point EDITOR to a helper script path.
	// Create helper script in temp dir
	tmpDir := t.TempDir()
	editorScript := filepath.Join(tmpDir, "fake-editor.sh")
	scriptContent := `#!/bin/sh
# overwrite file with provided content
# $1 is the path
# For goal/template/prompt, write a deterministic string
if [ -n "$1" ]; then
  echo "edited: $(basename \"$1\")" > "$1"
fi
`
	if err := os.WriteFile(editorScript, []byte(scriptContent), 0755); err != nil {
		t.Fatalf("failed to write fake editor: %v", err)
	}

	opts := termtest.Options{
		CmdName:        binaryPath,
		Args:           []string{"prompt-flow", "-i"},
		DefaultTimeout: 60 * time.Second,
		Env: []string{
			"EDITOR=" + editorScript,
			"VISUAL=",
			// ensure clipboard cmd is harmless and fast
			"ONESHOT_CLIPBOARD_CMD=cat >/dev/null",
		},
	}

	cp, err := termtest.NewTest(t, opts)
	if err != nil {
		t.Fatalf("Failed to create termtest: %v", err)
	}
	defer cp.Close()

	// Startup of the TUI
	requireExpect(t, cp, "one-shot-man Rich TUI Terminal", 15*time.Second)

	// The script auto-switches to its mode and prints banner/help.
	requireExpect(t, cp, "Prompt Flow: goal/context/template -> generate -> use -> assemble")
	requireExpect(t, cp, "Type 'help' for commands.")

	// Wait for the prompt for this mode
	requireExpect(t, cp, "(prompt-builder) > ", 20*time.Second)

	// Basic help
	cp.SendLine("help")
	requireExpect(t, cp, "Commands: goal, add, diff, note, list")

	// Set goal inline to avoid opening editor
	cp.SendLine("goal my test goal")
	requireExpect(t, cp, "Goal set.")

	// Add a known file to the context (README exists in repo). Use absolute path so the
	// engine (which sets basePath to its working directory) can stat it reliably.
	readmeAbs := filepath.Join(projectDir, "README.md")
	cp.SendLine("add " + readmeAbs)
	// Expect the generic success prefix to avoid path differences
	requireExpect(t, cp, "Added file: ")

	// Add a simple note
	cp.SendLine("note this is a note")
	requireExpect(t, cp, "Added note [")

	// List current state
	cp.SendLine("list")
	requireExpect(t, cp, "[goal] my test goal")
	requireExpect(t, cp, "[template] set")

	// Trigger generate to create meta-prompt, then inspect meta
	cp.SendLine("generate")
	requireExpect(t, cp, "Meta-prompt generated. You can 'show meta', 'copy meta', or provide the task prompt with 'use'.")

	cp.SendLine("show meta")
	requireExpect(t, cp, "!! **GOAL:** !!")
	requireExpect(t, cp, "!! **IMPLEMENTATIONS/CONTEXT:** !!")
	// The meta prompt includes the txtar dump; check that README.md appears
	requireExpect(t, cp, "README.md", 10*time.Second)

	// Provide a task prompt so default show assembles final content
	cp.SendLine("use edited prompt from test")
	requireExpect(t, cp, "Task prompt set.")

	// Show final (assembled) should now include IMPLEMENTATIONS/CONTEXT marker and edited prompt header
	cp.SendLine("show")
	requireExpect(t, cp, "## IMPLEMENTATIONS/CONTEXT")

	// Copy final output to clipboard (no-op override)
	cp.SendLine("copy")
	// Expect the success confirmation message
	requireExpect(t, cp, "Final output copied to clipboard.")

	// Test remove synchronization: add then remove README and ensure it no longer appears in final
	cp.SendLine("list")
	requireExpect(t, cp, "[file]")
	// Remove first non-note item id by re-listing and removing id 1 (we don't parse here; assume first add has id=1)
	cp.SendLine("remove 1")
	requireExpect(t, cp, "Removed [1]")

	// Clear buffer, show final, and assert README.md is not present in the output
	cp.ClearOutput()
	cp.SendLine("show")
	requireExpect(t, cp, "## IMPLEMENTATIONS/CONTEXT")
	if out := cp.GetOutput(); strings.Contains(out, "README.md") {
		t.Fatalf("expected README.md to be removed from context, but it was present in output:\n%s", out)
	}

	// Exit cleanly
	cp.SendLine("exit")
	requireExpectExitCode(t, cp, 0)
}

// TestPromptFlow_Remove_Ambiguous_AbortsUI verifies that when the backend reports
// an ambiguous removal (e.g., two tracked files end with the same name), the JS
// UI does NOT remove the item and surfaces an error message.
func TestPromptFlow_Remove_Ambiguous_AbortsUI(t *testing.T) {
	binaryPath := buildTestBinary(t)
	defer os.Remove(binaryPath)

	// Create two files with the same basename in different directories
	tmpDir := t.TempDir()
	dir1 := filepath.Join(tmpDir, "foo")
	dir2 := filepath.Join(tmpDir, "bar")
	if err := os.MkdirAll(dir1, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir2, 0o755); err != nil {
		t.Fatal(err)
	}
	f1 := filepath.Join(dir1, "a.txt")
	f2 := filepath.Join(dir2, "a.txt")
	if err := os.WriteFile(f1, []byte("file one"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f2, []byte("file two"), 0o644); err != nil {
		t.Fatal(err)
	}

	opts := termtest.Options{
		CmdName:        binaryPath,
		Args:           []string{"prompt-flow", "-i"},
		DefaultTimeout: 60 * time.Second,
		Env: []string{
			"VISUAL=",
			"EDITOR=",
			"ONESHOT_CLIPBOARD_CMD=cat >/dev/null",
		},
	}

	cp, err := termtest.NewTest(t, opts)
	if err != nil {
		t.Fatalf("Failed to create termtest: %v", err)
	}
	defer cp.Close()

	// Wait for prompt
	requireExpect(t, cp, "one-shot-man Rich TUI Terminal", 15*time.Second)
	requireExpect(t, cp, "(prompt-builder) > ", 20*time.Second)

	// Add both files (absolute paths)
	cp.SendLine("add " + f1)
	requireExpect(t, cp, "Added file:")
	cp.SendLine("add " + f2)
	requireExpect(t, cp, "Added file:")

	// Make the first item's label ambiguous by rewriting it to just the basename
	// so that removing by id=1 will attempt context.removePath("a.txt") which
	// should match both tracked files and yield an ambiguity error.
	// N.B. We mutate JS state via inline JS execution.
	cp.SendLine(`(function(){ var l=tui.getState("contextItems")||[]; if(l.length>0){ l[0].label="a.txt"; } tui.setState("contextItems", l); })()`)
	// sanity: list shows label now as just a.txt
	cp.SendLine("list")
	requireExpect(t, cp, "[1] [file] a.txt")

	// Attempt removal; should print an Error and NOT say Removed [1]
	cp.SendLine("remove 1")
	requireExpect(t, cp, "Error:")
	// Ensure not removed from UI
	cp.SendLine("list")
	requireExpect(t, cp, "[1] [file] a.txt")

	// The backend still has both; meta should include both files
	cp.SendLine("show meta")
	requireExpect(t, cp, "a.txt")

	cp.SendLine("exit")
	requireExpectExitCode(t, cp, 0)
}

// TestPromptFlow_Remove_NotFound_AbortsUI verifies that when the backend does not
// have the file (path not found), the JS UI does not remove the item and surfaces
// an error. This ensures we do not silently desynchronize UI from backend.
func TestPromptFlow_Remove_NotFound_AbortsUI(t *testing.T) {
	binaryPath := buildTestBinary(t)
	defer os.Remove(binaryPath)

	tmpFile := filepath.Join(t.TempDir(), "lonely.txt")
	if err := os.WriteFile(tmpFile, []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}

	opts := termtest.Options{
		CmdName:        binaryPath,
		Args:           []string{"prompt-flow", "-i"},
		DefaultTimeout: 60 * time.Second,
		Env: []string{
			"VISUAL=",
			"EDITOR=",
			"ONESHOT_CLIPBOARD_CMD=cat >/dev/null",
		},
	}

	cp, err := termtest.NewTest(t, opts)
	if err != nil {
		t.Fatalf("Failed to create termtest: %v", err)
	}
	defer cp.Close()

	requireExpect(t, cp, "one-shot-man Rich TUI Terminal", 15*time.Second)
	requireExpect(t, cp, "(prompt-builder) > ", 20*time.Second)

	// Add one file
	cp.SendLine("add " + tmpFile)
	requireExpect(t, cp, "Added file:")

	// Make the UI label a path that isn't tracked in the backend to simulate not found
	cp.SendLine(`(function(){ var l=tui.getState("contextItems")||[]; if(l.length>0){ l[0].label="nonexistent.txt"; } tui.setState("contextItems", l); })()`)

	// Now attempt to remove by id from UI; backend will say not found -> Error
	cp.SendLine("remove 1")
	requireExpect(t, cp, "Error:")

	// Ensure UI still shows the item (id 1)
	cp.SendLine("list")
	requireExpect(t, cp, "[1] [file]")

	cp.SendLine("exit")
	requireExpectExitCode(t, cp, 0)
}
