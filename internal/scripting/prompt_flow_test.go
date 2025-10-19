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

	// Capture initial offset BEFORE the process outputs
	startLen := cp.OutputLen()

	// Expect the prompt-flow banner/help emitted by onEnter after auto-switch
	if _, err := cp.ExpectSince("Prompt Flow: goal/context/template -> generate -> use -> assemble", startLen, opts.DefaultTimeout); err != nil {
		t.Fatalf("Expected banner: %v", err)
	}
	if _, err := cp.ExpectSince("Type 'help' for commands.", startLen, opts.DefaultTimeout); err != nil {
		t.Fatalf("Expected help text: %v", err)
	}
	if _, err := cp.ExpectSince("Commands: goal, add, diff, note, list, view, edit, remove, template, generate, use, show [meta|prompt], copy [meta|prompt], help, exit", startLen, opts.DefaultTimeout); err != nil {
		t.Fatalf("Expected commands list: %v", err)
	}

	// Process should terminate on its own (non-interactive mode)
	requireExpectExitCode(t, cp, 0)
}

func TestPromptFlow_GenerateRequiresGoal(t *testing.T) {
	binaryPath := buildTestBinary(t)
	defer os.Remove(binaryPath)

	opts := termtest.Options{
		CmdName:        binaryPath,
		Args:           []string{"prompt-flow", "-i"},
		DefaultTimeout: 30 * time.Second,
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

	startLen := cp.OutputLen()
	if _, err := cp.ExpectSince("one-shot-man Rich TUI Terminal", startLen, 15*time.Second); err != nil {
		t.Fatalf("Expected TUI startup: %v", err)
	}
	if _, err := cp.ExpectSince("(prompt-builder) > ", startLen, 20*time.Second); err != nil {
		t.Fatalf("Expected prompt: %v", err)
	}

	startLen = cp.OutputLen()
	cp.SendLine("generate")
	if _, err := cp.ExpectSince("Error: Please set a goal first using the 'goal' command.", startLen); err != nil {
		t.Fatalf("Expected error message: %v", err)
	}

	cp.SendLine("exit")
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
		DefaultTimeout: 30 * time.Second, // Increased from 60s - this comprehensive integration test needs more time
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
	startLen := cp.OutputLen()
	if _, err := cp.ExpectSince("one-shot-man Rich TUI Terminal", startLen, 15*time.Second); err != nil {
		t.Fatalf("Expected TUI startup: %v", err)
	}

	// The script auto-switches to its mode and prints banner/help.
	if _, err := cp.ExpectSince("Prompt Flow: goal/context/template -> generate -> use -> assemble", startLen); err != nil {
		t.Fatalf("Expected prompt flow banner: %v", err)
	}
	if _, err := cp.ExpectSince("Type 'help' for commands.", startLen); err != nil {
		t.Fatalf("Expected help text: %v", err)
	}

	// Wait for the prompt for this mode
	if _, err := cp.ExpectSince("(prompt-builder) > ", startLen, 20*time.Second); err != nil {
		t.Fatalf("Expected prompt: %v", err)
	}

	// Basic help - PING-PONG: capture offset, send command, wait for consequence
	startLen = cp.OutputLen()
	if err := cp.SendLine("help"); err != nil {
		t.Fatalf("Failed to send help: %v", err)
	}
	if _, err := cp.ExpectSince("Available commands:", startLen); err != nil {
		t.Fatalf("Expected help output: %v", err)
	}
	if _, err := cp.ExpectSince("Registered commands:", startLen); err != nil {
		t.Fatalf("Expected help output: %v", err)
	}
	if _, err := cp.ExpectSince("Current mode: flow", startLen); err != nil {
		t.Fatalf("Expected help output: %v", err)
	}
	if _, err := cp.ExpectSince("JavaScript API:", startLen); err != nil {
		t.Fatalf("Expected help output: %v", err)
	}
	if _, err := cp.ExpectSince("(prompt-builder) > ", startLen); err != nil {
		t.Fatalf("Expected prompt: %v", err)
	}

	// Set goal inline to avoid opening editor - PING-PONG
	startLen = cp.OutputLen()
	if err := cp.SendLine("goal my test goal"); err != nil {
		t.Fatalf("Failed to send goal: %v", err)
	}
	if _, err := cp.ExpectSince("Goal set.", startLen); err != nil {
		t.Fatalf("Expected goal set: %v", err)
	}

	// Add a known file to the context (README exists in repo). Use absolute path so the
	// engine (which sets basePath to its working directory) can stat it reliably.
	// PING-PONG: capture, send, wait
	readmeAbs := filepath.Join(projectDir, "README.md")
	startLen = cp.OutputLen()
	if err := cp.SendLine("add " + readmeAbs); err != nil {
		t.Fatalf("Failed to send add: %v", err)
	}
	// Expect the generic success prefix to avoid path differences
	if _, err := cp.ExpectSince("Added file: ", startLen); err != nil {
		t.Fatalf("Expected file added: %v", err)
	}

	// Add a simple note - PING-PONG
	startLen = cp.OutputLen()
	if err := cp.SendLine("note this is a note"); err != nil {
		t.Fatalf("Failed to send note: %v", err)
	}
	if _, err := cp.ExpectSince("Added note [", startLen); err != nil {
		t.Fatalf("Expected note added: %v", err)
	}

	// List current state - PING-PONG
	startLen = cp.OutputLen()
	if err := cp.SendLine("list"); err != nil {
		t.Fatalf("Failed to send list: %v", err)
	}
	if _, err := cp.ExpectSince("[goal] my test goal", startLen); err != nil {
		t.Fatalf("Expected goal in list: %v", err)
	}
	if _, err := cp.ExpectSince("[template] set", startLen); err != nil {
		t.Fatalf("Expected template in list: %v", err)
	}

	// Trigger generate to create meta-prompt, then inspect meta - PING-PONG
	startLen = cp.OutputLen()
	if err := cp.SendLine("generate"); err != nil {
		t.Fatalf("Failed to send generate: %v", err)
	}
	if _, err := cp.ExpectSince("Meta-prompt generated. You can 'show meta', 'copy meta', or provide the task prompt with 'use'.", startLen); err != nil {
		t.Fatalf("Expected meta-prompt generated: %v", err)
	}

	startLen = cp.OutputLen()
	if err := cp.SendLine("show meta"); err != nil {
		t.Fatalf("Failed to send show meta: %v", err)
	}
	if _, err := cp.ExpectSince("!! **GOAL:** !!", startLen); err != nil {
		t.Fatalf("Expected goal in meta: %v", err)
	}
	if _, err := cp.ExpectSince("!! **IMPLEMENTATIONS/CONTEXT:** !!", startLen); err != nil {
		t.Fatalf("Expected context in meta: %v", err)
	}
	// The meta prompt includes the txtar dump; check that README.md appears
	if _, err := cp.ExpectSince("README.md", startLen, 10*time.Second); err != nil {
		t.Fatalf("Expected README in meta: %v", err)
	}

	// Provide a task prompt so default show assembles final content - PING-PONG
	startLen = cp.OutputLen()
	if err := cp.SendLine("use edited prompt from test"); err != nil {
		t.Fatalf("Failed to send use: %v", err)
	}
	if _, err := cp.ExpectSince("Task prompt set.", startLen); err != nil {
		t.Fatalf("Expected task prompt set: %v", err)
	}

	// Show final (assembled) should now include IMPLEMENTATIONS/CONTEXT marker and edited prompt header - PING-PONG
	cp.ClearOutput()
	startLen = cp.OutputLen()
	if err := cp.SendLine("show"); err != nil {
		t.Fatalf("Failed to send show: %v", err)
	}
	if _, err := cp.ExpectSince("## IMPLEMENTATIONS/CONTEXT", startLen); err != nil {
		t.Fatalf("Expected context section: %v", err)
	}
	if out := cp.GetOutput(); !strings.Contains(out, "edited prompt from test") {
		t.Fatalf("expected final output to include task prompt, got: %s", out)
	} else if strings.Contains(out, "!! **TEMPLATE:** !!") {
		t.Fatalf("expected final output to omit template section, got: %s", out)
	}

	// Copy final output to clipboard (no-op override) - PING-PONG
	startLen = cp.OutputLen()
	if err := cp.SendLine("copy"); err != nil {
		t.Fatalf("Failed to send copy: %v", err)
	}
	// Expect the success confirmation message
	if _, err := cp.ExpectSince("Final output copied to clipboard.", startLen); err != nil {
		t.Fatalf("Expected copy confirmation: %v", err)
	}

	// Test remove synchronization: add then remove README and ensure it no longer appears in final - PING-PONG
	startLen = cp.OutputLen()
	if err := cp.SendLine("list"); err != nil {
		t.Fatalf("Failed to send list: %v", err)
	}
	if _, err := cp.ExpectSince("[file]", startLen); err != nil {
		t.Fatalf("Expected file in list: %v", err)
	}
	// Remove first non-note item id by re-listing and removing id 1 (we don't parse here; assume first add has id=1) - PING-PONG
	startLen = cp.OutputLen()
	if err := cp.SendLine("remove 1"); err != nil {
		t.Fatalf("Failed to send remove: %v", err)
	}
	if _, err := cp.ExpectSince("Removed [1]", startLen); err != nil {
		t.Fatalf("Expected remove confirmation: %v", err)
	}

	// Clear buffer, show final, and assert README.md is not present in the output - PING-PONG
	cp.ClearOutput()
	startLen = cp.OutputLen()
	if err := cp.SendLine("show"); err != nil {
		t.Fatalf("Failed to send show: %v", err)
	}
	if _, err := cp.ExpectSince("## IMPLEMENTATIONS/CONTEXT", startLen); err != nil {
		t.Fatalf("Expected context section: %v", err)
	}
	if out := cp.GetOutput(); strings.Contains(out, "README.md") {
		t.Fatalf("expected README.md to be removed from context, but it was present in output:\n%s", out)
	}

	// Re-run use to ensure repeated updates work without needing generate - PING-PONG
	startLen = cp.OutputLen()
	if err := cp.SendLine("use second prompt from test"); err != nil {
		t.Fatalf("Failed to send use: %v", err)
	}
	if _, err := cp.ExpectSince("Task prompt set.", startLen); err != nil {
		t.Fatalf("Expected task prompt set: %v", err)
	}
	cp.ClearOutput()
	startLen = cp.OutputLen()
	if err := cp.SendLine("show"); err != nil {
		t.Fatalf("Failed to send show: %v", err)
	}
	if _, err := cp.ExpectSince("## IMPLEMENTATIONS/CONTEXT", startLen); err != nil {
		t.Fatalf("Expected context section: %v", err)
	}
	if out := cp.GetOutput(); !strings.Contains(out, "second prompt from test") {
		t.Fatalf("expected final output to reflect updated task prompt, got: %s", out)
	} else if strings.Contains(out, "!! **TEMPLATE:** !!") {
		t.Fatalf("expected final output after update to omit template section, got: %s", out)
	}

	// Re-generating should clear the task prompt and revert show to meta output - PING-PONG
	cp.ClearOutput()
	startLen = cp.OutputLen()
	if err := cp.SendLine("generate"); err != nil {
		t.Fatalf("Failed to send generate: %v", err)
	}
	if _, err := cp.ExpectSince("Meta-prompt generated.", startLen); err != nil {
		t.Fatalf("Expected meta-prompt generated: %v", err)
	}
	cp.ClearOutput()
	startLen = cp.OutputLen()
	if err := cp.SendLine("show"); err != nil {
		t.Fatalf("Failed to send show: %v", err)
	}
	if _, err := cp.ExpectSince("!! **GOAL:** !!", startLen); err != nil {
		t.Fatalf("Expected goal in meta: %v", err)
	}
	if _, err := cp.ExpectSince("!! **TEMPLATE:** !!", startLen); err != nil {
		t.Fatalf("Expected goal in meta: %v", err)
	}
	if out := cp.GetOutput(); strings.Contains(out, "second prompt from test") {
		t.Fatalf("expected regenerate to clear task prompt, but output still contained task prompt text: %s", out)
	} else if !strings.Contains(out, "!! **TEMPLATE:** !!") {
		t.Fatalf("expected meta output to include template instructions, got: %s", out)
	}

	// Exit cleanly - PING-PONG
	if err := cp.SendLine("exit"); err != nil {
		t.Fatalf("Failed to send exit: %v", err)
	}
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
	startLen := cp.OutputLen()
	if _, err := cp.ExpectSince("one-shot-man Rich TUI Terminal", startLen, 15*time.Second); err != nil {
		t.Fatalf("Expected TUI startup: %v\nBuffer: %q", err, cp.GetOutput())
	}
	if _, err := cp.ExpectSince("(prompt-builder) > ", startLen, 20*time.Second); err != nil {
		t.Fatalf("Expected prompt: %v\nBuffer: %q", err, cp.GetOutput())
	}

	// Add both files (absolute paths)
	startLen = cp.OutputLen()
	if err := cp.SendLine("add " + f1); err != nil {
		t.Fatalf("Failed to send add: %v\nBuffer: %q", err, cp.GetOutput())
	}
	if _, err := cp.ExpectSince("Added file:", startLen, 2*time.Second); err != nil {
		t.Fatalf("Expected file added: %v\nBuffer: %q", err, cp.GetOutput())
	}
	startLen = cp.OutputLen()
	if err := cp.SendLine("add " + f2); err != nil {
		t.Fatalf("Failed to send add: %v\nBuffer: %q", err, cp.GetOutput())
	}
	if _, err := cp.ExpectSince("Added file:", startLen, 2*time.Second); err != nil {
		t.Fatalf("Expected file added: %v\nBuffer: %q", err, cp.GetOutput())
	}

	// Make the first item's label ambiguous by rewriting it to just the basename
	// so that removing by id=1 will attempt context.removePath("a.txt") which
	// should match both tracked files and yield an ambiguity error.
	// N.B. We mutate JS state via the dedicated test hook.
	if err := cp.SendLine(`(function(){ if (typeof __promptFlowTestHooks !== "undefined") { __promptFlowTestHooks.withState(function(h){ var items=h.state.get(h.StateKeys.contextItems); if(items.length>0){ items[0].label="a.txt"; h.state.set(h.StateKeys.contextItems, items); } }); } })()`); err != nil {
		t.Fatalf("Failed to send JS hook: %v\nBuffer: %q", err, cp.GetOutput())
	}
	// The JS expression executes but produces no output - just need to let it process
	time.Sleep(100 * time.Millisecond)
	// sanity: list shows label now as just a.txt
	startLen = cp.OutputLen()
	if err := cp.SendLine("list"); err != nil {
		t.Fatalf("Failed to send list: %v\nBuffer: %q", err, cp.GetOutput())
	}
	if _, err := cp.ExpectSince("[1] [file] a.txt", startLen, 2*time.Second); err != nil {
		t.Fatalf("Expected file in list: %v\nBuffer: %q", err, cp.GetOutput())
	}

	// Attempt removal; should print an Error and NOT say Removed [1]
	startLen = cp.OutputLen()
	if err := cp.SendLine("remove 1"); err != nil {
		t.Fatalf("Failed to send remove: %v\nBuffer: %q", err, cp.GetOutput())
	}
	if _, err := cp.ExpectSince("Error:", startLen, 2*time.Second); err != nil {
		t.Fatalf("Expected error: %v\nBuffer: %q", err, cp.GetOutput())
	}
	// Ensure UI still shows the item (id 1)
	startLen = cp.OutputLen()
	if err := cp.SendLine("list"); err != nil {
		t.Fatalf("Failed to send list: %v\nBuffer: %q", err, cp.GetOutput())
	}
	if _, err := cp.ExpectSince("[1] [file] a.txt", startLen, 2*time.Second); err != nil {
		t.Fatalf("Expected file still in list: %v\nBuffer: %q", err, cp.GetOutput())
	}

	if err := cp.SendLine("exit"); err != nil {
		t.Fatalf("Failed to send exit: %v\nBuffer: %q", err, cp.GetOutput())
	}
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

	startLen := cp.OutputLen()
	if _, err := cp.ExpectSince("one-shot-man Rich TUI Terminal", startLen, 15*time.Second); err != nil {
		t.Fatalf("Expected TUI startup: %v", err)
	}
	if _, err := cp.ExpectSince("(prompt-builder) > ", startLen, 20*time.Second); err != nil {
		t.Fatalf("Expected prompt: %v", err)
	}

	// Add one file
	startLen = cp.OutputLen()
	if err := cp.SendLine("add " + tmpFile); err != nil {
		t.Fatalf("Failed to send add: %v\nBuffer: %q", err, cp.GetOutput())
	}
	if _, err := cp.ExpectSince("Added file:", startLen, 2*time.Second); err != nil {
		t.Fatalf("Expected file added: %v\nBuffer: %q", err, cp.GetOutput())
	}

	// Make the UI label a path that isn't tracked in the backend to simulate not found
	if err := cp.SendLine(`(function(){ if (typeof __promptFlowTestHooks !== "undefined") { __promptFlowTestHooks.withState(function(h){ var items=h.state.get(h.StateKeys.contextItems); if(items.length>0){ items[0].label="nonexistent.txt"; h.state.set(h.StateKeys.contextItems, items); } }); } })()`); err != nil {
		t.Fatalf("Failed to send JS hook: %v\nBuffer: %q", err, cp.GetOutput())
	}
	// The JS expression executes but produces no output - just need to let it process
	time.Sleep(100 * time.Millisecond)

	// Now attempt to remove by id from UI; backend will say not found -> Error
	startLen = cp.OutputLen()
	if err := cp.SendLine("remove 1"); err != nil {
		t.Fatalf("Failed to send remove: %v\nBuffer: %q", err, cp.GetOutput())
	}
	if _, err := cp.ExpectSince("Error:", startLen, 2*time.Second); err != nil {
		t.Fatalf("Expected error: %v\nBuffer: %q", err, cp.GetOutput())
	}

	// Ensure UI still shows the item (id 1)
	startLen = cp.OutputLen()
	if err := cp.SendLine("list"); err != nil {
		t.Fatalf("Failed to send list: %v\nBuffer: %q", err, cp.GetOutput())
	}
	if _, err := cp.ExpectSince("[1] [file]", startLen, 2*time.Second); err != nil {
		t.Fatalf("Expected file in list: %v\nBuffer: %q", err, cp.GetOutput())
	}

	if err := cp.SendLine("exit"); err != nil {
		t.Fatalf("Failed to send exit: %v\nBuffer: %q", err, cp.GetOutput())
	}
	requireExpectExitCode(t, cp, 0)
}
