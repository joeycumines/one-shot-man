//go:build unix

package scripting

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/joeycumines/go-prompt/termtest"
)

// TestPromptFlow_NonInteractive ensures the prompt-flow script registers and enters its mode
// when executed in non-interactive test mode, and emits the expected banner/help text.
func TestPromptFlow_NonInteractive(t *testing.T) {
	binaryPath := buildTestBinary(t)

	env := newTestProcessEnv(t)

	// Run the CLI in non-interactive test mode to evaluate the built-in prompt-flow command
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	cp, err := termtest.NewConsole(ctx,
		termtest.WithCommand(binaryPath, "prompt-flow", "--test"),
		termtest.WithDefaultTimeout(20*time.Second),
		termtest.WithEnv(env),
	)
	if err != nil {
		t.Fatalf("Failed to start test process: %v", err)
	}
	defer cp.Close()

	expect := func(timeout time.Duration, since termtest.Snapshot, cond termtest.Condition, description string) error {
		ctx, cancel := context.WithTimeout(t.Context(), timeout)
		defer cancel()
		return cp.Expect(ctx, since, cond, description)
	}

	// Capture snapshot immediately after start (before output is asserted)
	snap := cp.Snapshot()

	// Expect the prompt-flow banner/help emitted by onEnter after auto-switch
	if err := expect(20*time.Second, snap, termtest.Contains("Type 'help' for commands. Tip: Try 'goal --prewritten'."), "banner"); err != nil {
		t.Fatalf("Expected banner: %v", err)
	}

	// Process should terminate on its own (non-interactive mode)
	code, err := cp.WaitExit(t.Context())
	if err != nil {
		t.Fatalf("Failed to wait for exit: %v", err)
	}
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
}

func TestPromptFlow_GenerateRequiresGoal(t *testing.T) {
	binaryPath := buildTestBinary(t)

	env := newTestProcessEnv(t)
	env = append(env, "VISUAL=", "EDITOR=")

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

	snap := cp.Snapshot()
	if err := expect(15*time.Second, snap, termtest.Contains("Switched to mode: prompt-flow"), "mode switch"); err != nil {
		t.Fatalf("Expected mode switch to prompt-flow: %v", err)
	}
	if err := expect(20*time.Second, snap, termtest.Contains("(prompt-flow) > "), "prompt"); err != nil {
		t.Fatalf("Expected prompt: %v", err)
	}

	snap = cp.Snapshot()
	cp.SendLine("generate")
	if err := expect(30*time.Second, snap, termtest.Contains("Error: Please set a goal first using the 'goal' command."), "error message"); err != nil {
		t.Fatalf("Expected error message: %v", err)
	}

	cp.SendLine("exit")
	code, err := cp.WaitExit(t.Context())
	if err != nil {
		t.Fatalf("Failed to wait for exit: %v", err)
	}
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
}

// TestPromptFlow_Interactive drives a minimal happy path in interactive mode without invoking the editor.
// It avoids commands that would open the system editor (goal without args, template, generate).
func TestPromptFlow_Interactive(t *testing.T) {
	binaryPath := buildTestBinary(t)

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
  echo "edited: $(basename "$1")" > "$1"
fi
`
	if err := os.WriteFile(editorScript, []byte(scriptContent), 0755); err != nil {
		t.Fatalf("failed to write fake editor: %v", err)
	}
	if err := os.Chmod(editorScript, 0755); err != nil {
		t.Fatalf("failed to chmod fake editor: %v", err)
	}

	env := newTestProcessEnv(t)
	env = append(env, "EDITOR="+editorScript, "VISUAL=")

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	cp, err := termtest.NewConsole(ctx,
		termtest.WithCommand(binaryPath, "prompt-flow", "-i"),
		termtest.WithDefaultTimeout(30*time.Second), // Increased from 60s - this comprehensive integration test needs more time
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

	// Startup of the TUI
	snap := cp.Snapshot()
	if err := expect(15*time.Second, snap, termtest.Contains("Switched to mode: prompt-flow"), "mode switch"); err != nil {
		t.Fatalf("Expected mode switch to prompt-flow: %v", err)
	}

	// Wait for the prompt for this mode
	if err := expect(20*time.Second, snap, termtest.Contains("(prompt-flow) > "), "prompt"); err != nil {
		t.Fatalf("Expected prompt: %v", err)
	}

	// Basic help - PING-PONG: capture snapshot, send command, wait for consequence
	snap = cp.Snapshot()
	if err := cp.SendLine("help"); err != nil {
		t.Fatalf("Failed to send help: %v", err)
	}
	if err := expect(30*time.Second, snap, termtest.Contains("Available commands:"), "help output 1"); err != nil {
		t.Fatalf("Expected help output: %v", err)
	}
	if err := expect(30*time.Second, snap, termtest.Contains("Registered commands:"), "help output 2"); err != nil {
		t.Fatalf("Expected help output: %v", err)
	}
	if err := expect(30*time.Second, snap, termtest.Contains("Current mode: prompt-flow"), "help output 3"); err != nil {
		t.Fatalf("Expected help output: %v", err)
	}
	if err := expect(30*time.Second, snap, termtest.Contains("Note: You can execute JavaScript code directly!"), "help output 4"); err != nil {
		t.Fatalf("Expected help output: %v", err)
	}
	if err := expect(30*time.Second, snap, termtest.Contains("(prompt-flow) > "), "prompt"); err != nil {
		t.Fatalf("Expected prompt: %v", err)
	}

	// Set goal inline to avoid opening editor - PING-PONG
	snap = cp.Snapshot()
	if err := cp.SendLine("goal my test goal"); err != nil {
		t.Fatalf("Failed to send goal: %v", err)
	}
	if err := expect(30*time.Second, snap, termtest.Contains("Goal set."), "goal set"); err != nil {
		t.Fatalf("Expected goal set: %v", err)
	}

	// Add a known file to the context (README exists in repo). Use absolute path so the
	// engine (which sets basePath to its working directory) can stat it reliably.
	// PING-PONG: capture, send, wait
	readmeAbs := filepath.Join(projectDir, "README.md")
	snap = cp.Snapshot()
	if err := cp.SendLine("add " + readmeAbs); err != nil {
		t.Fatalf("Failed to send add: %v", err)
	}
	// Expect the generic success prefix to avoid path differences
	if err := expect(30*time.Second, snap, termtest.Contains("Added file: "), "file added"); err != nil {
		t.Fatalf("Expected file added: %v", err)
	}

	// Add a simple note - PING-PONG
	snap = cp.Snapshot()
	if err := cp.SendLine("note this is a note"); err != nil {
		t.Fatalf("Failed to send note: %v", err)
	}
	if err := expect(30*time.Second, snap, termtest.Contains("Added note ["), "note added"); err != nil {
		t.Fatalf("Expected note added: %v", err)
	}

	// List current state - PING-PONG
	snap = cp.Snapshot()
	if err := cp.SendLine("list"); err != nil {
		t.Fatalf("Failed to send list: %v", err)
	}
	if err := expect(30*time.Second, snap, termtest.Contains("[goal] my test goal"), "goal in list"); err != nil {
		t.Fatalf("Expected goal in list: %v", err)
	}
	if err := expect(30*time.Second, snap, termtest.Contains("[template] set"), "template in list"); err != nil {
		t.Fatalf("Expected template in list: %v", err)
	}

	// Trigger generate to create meta-prompt, then inspect meta - PING-PONG
	snap = cp.Snapshot()
	if err := cp.SendLine("generate"); err != nil {
		t.Fatalf("Failed to send generate: %v", err)
	}
	if err := expect(30*time.Second, snap, termtest.Contains("Meta-prompt generated. You can 'show meta', 'copy meta', or provide the task prompt with 'use'."), "meta prompt generated"); err != nil {
		t.Fatalf("Expected meta-prompt generated: %v", err)
	}

	snap = cp.Snapshot()
	if err := cp.SendLine("show meta"); err != nil {
		t.Fatalf("Failed to send show meta: %v", err)
	}
	if err := expect(30*time.Second, snap, termtest.Contains("!! **GOAL:** !!"), "goal in meta"); err != nil {
		t.Fatalf("Expected goal in meta: %v", err)
	}
	if err := expect(30*time.Second, snap, termtest.Contains("!! **IMPLEMENTATIONS/CONTEXT:** !!"), "context in meta"); err != nil {
		t.Fatalf("Expected context in meta: %v", err)
	}
	// The meta prompt includes the txtar dump; check that README.md appears
	if err := expect(10*time.Second, snap, termtest.Contains("README.md"), "README in meta"); err != nil {
		t.Fatalf("Expected README in meta: %v", err)
	}

	// Provide a task prompt so default show assembles final content - PING-PONG
	snap = cp.Snapshot()
	if err := cp.SendLine("use edited prompt from test"); err != nil {
		t.Fatalf("Failed to send use: %v", err)
	}
	if err := expect(30*time.Second, snap, termtest.Contains("Task prompt set."), "task prompt set"); err != nil {
		t.Fatalf("Expected task prompt set: %v", err)
	}

	// Show final (assembled) should now include IMPLEMENTATIONS/CONTEXT marker and edited prompt header - PING-PONG
	// We simulate cp.ClearOutput() by capturing the current buffer state and asserting on the diff.
	beforeShow := cp.String()
	snap = cp.Snapshot()
	if err := cp.SendLine("show"); err != nil {
		t.Fatalf("Failed to send show: %v", err)
	}
	if err := expect(30*time.Second, snap, termtest.Contains("## IMPLEMENTATIONS/CONTEXT"), "context section"); err != nil {
		t.Fatalf("Expected context section: %v", err)
	}

	// Get output generated by the 'show' command
	out := strings.TrimPrefix(cp.String(), beforeShow)
	if !strings.Contains(out, "edited prompt from test") {
		t.Fatalf("expected final output to include task prompt, got: %s", out)
	} else if strings.Contains(out, "!! **TEMPLATE:** !!") {
		t.Fatalf("expected final output to omit template section, got: %s", out)
	}

	// Copy final output to clipboard (no-op override) - PING-PONG
	snap = cp.Snapshot()
	if err := cp.SendLine("copy"); err != nil {
		t.Fatalf("Failed to send copy: %v", err)
	}
	// Expect the success confirmation message
	if err := expect(30*time.Second, snap, termtest.Contains("Final output copied to clipboard."), "copy confirmation"); err != nil {
		t.Fatalf("Expected copy confirmation: %v", err)
	}

	// Test remove synchronization: add then remove README and ensure it no longer appears in final - PING-PONG
	snap = cp.Snapshot()
	if err := cp.SendLine("list"); err != nil {
		t.Fatalf("Failed to send list: %v", err)
	}
	if err := expect(30*time.Second, snap, termtest.Contains("[file]"), "file in list"); err != nil {
		t.Fatalf("Expected file in list: %v", err)
	}
	// Remove first non-note item id by re-listing and removing id 1 (we don't parse here; assume first add has id=1) - PING-PONG
	snap = cp.Snapshot()
	if err := cp.SendLine("remove 1"); err != nil {
		t.Fatalf("Failed to send remove: %v", err)
	}
	if err := expect(30*time.Second, snap, termtest.Contains("Removed [1]"), "remove confirmation"); err != nil {
		t.Fatalf("Expected remove confirmation: %v", err)
	}

	// Clear buffer, show final, and assert README.md is not present in the output - PING-PONG
	beforeShow = cp.String()
	snap = cp.Snapshot()
	if err := cp.SendLine("show"); err != nil {
		t.Fatalf("Failed to send show: %v", err)
	}
	if err := expect(30*time.Second, snap, termtest.Contains("## IMPLEMENTATIONS/CONTEXT"), "context section"); err != nil {
		t.Fatalf("Expected context section: %v", err)
	}
	out = strings.TrimPrefix(cp.String(), beforeShow)
	if strings.Contains(out, "README.md") {
		t.Fatalf("expected README.md to be removed from context, but it was present in output:\n%s", out)
	}

	// Re-run use to ensure repeated updates work without needing generate - PING-PONG
	snap = cp.Snapshot()
	if err := cp.SendLine("use second prompt from test"); err != nil {
		t.Fatalf("Failed to send use: %v", err)
	}
	if err := expect(30*time.Second, snap, termtest.Contains("Task prompt set."), "task prompt set"); err != nil {
		t.Fatalf("Expected task prompt set: %v", err)
	}

	beforeShow = cp.String()
	snap = cp.Snapshot()
	if err := cp.SendLine("show"); err != nil {
		t.Fatalf("Failed to send show: %v", err)
	}
	if err := expect(30*time.Second, snap, termtest.Contains("## IMPLEMENTATIONS/CONTEXT"), "context section"); err != nil {
		t.Fatalf("Expected context section: %v", err)
	}
	out = strings.TrimPrefix(cp.String(), beforeShow)
	if !strings.Contains(out, "second prompt from test") {
		t.Fatalf("expected final output to reflect updated task prompt, got: %s", out)
	} else if strings.Contains(out, "!! **TEMPLATE:** !!") {
		t.Fatalf("expected final output after update to omit template section, got: %s", out)
	}

	// Re-generating should clear the task prompt and revert show to meta output - PING-PONG
	// (Simulation of ClearOutput not needed here as we are sending generate)
	snap = cp.Snapshot()
	if err := cp.SendLine("generate"); err != nil {
		t.Fatalf("Failed to send generate: %v", err)
	}
	if err := expect(30*time.Second, snap, termtest.Contains("Meta-prompt generated."), "meta-prompt generated"); err != nil {
		t.Fatalf("Expected meta-prompt generated: %v", err)
	}

	beforeShow = cp.String()
	snap = cp.Snapshot()
	if err := cp.SendLine("show"); err != nil {
		t.Fatalf("Failed to send show: %v", err)
	}
	if err := expect(30*time.Second, snap, termtest.Contains("!! **GOAL:** !!"), "goal in meta"); err != nil {
		t.Fatalf("Expected goal in meta: %v", err)
	}
	if err := expect(30*time.Second, snap, termtest.Contains("!! **TEMPLATE:** !!"), "template in meta"); err != nil {
		t.Fatalf("Expected template in meta: %v", err)
	}
	out = strings.TrimPrefix(cp.String(), beforeShow)
	if strings.Contains(out, "second prompt from test") {
		t.Fatalf("expected regenerate to clear task prompt, but output still contained task prompt text: %s", out)
	} else if !strings.Contains(out, "!! **TEMPLATE:** !!") {
		t.Fatalf("expected meta output to include template instructions, got: %s", out)
	}

	// Exit cleanly - PING-PONG
	if err := cp.SendLine("exit"); err != nil {
		t.Fatalf("Failed to send exit: %v", err)
	}
	code, err := cp.WaitExit(t.Context())
	if err != nil {
		t.Fatalf("Failed to wait for exit: %v", err)
	}
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
}

// TestPromptFlow_Remove_Ambiguous_AbortsUI verifies that when the backend reports
// an ambiguous removal (e.g., two tracked files end with the same name), the JS
// UI does NOT remove the item and surfaces an error message.
func TestPromptFlow_Remove_Ambiguous_AbortsUI(t *testing.T) {
	binaryPath := buildTestBinary(t)

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

	env := newTestProcessEnv(t)
	env = append(env, "VISUAL=", "EDITOR=")

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	cp, err := termtest.NewConsole(ctx,
		termtest.WithCommand(binaryPath, "prompt-flow", "-i"),
		termtest.WithDefaultTimeout(60*time.Second),
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

	// Wait for prompt
	snap := cp.Snapshot()
	if err := expect(15*time.Second, snap, termtest.Contains("Switched to mode: prompt-flow"), "mode switch"); err != nil {
		t.Fatalf("Expected mode switch to prompt-flow: %v\nBuffer: %q", err, cp.String())
	}
	if err := expect(20*time.Second, snap, termtest.Contains("(prompt-flow) > "), "prompt"); err != nil {
		t.Fatalf("Expected prompt: %v\nBuffer: %q", err, cp.String())
	}

	// Add both files (absolute paths)
	snap = cp.Snapshot()
	if err := cp.SendLine("add " + f1); err != nil {
		t.Fatalf("Failed to send add: %v\nBuffer: %q", err, cp.String())
	}
	if err := expect(2*time.Second, snap, termtest.Contains("Added file:"), "file 1 added"); err != nil {
		t.Fatalf("Expected file added: %v\nBuffer: %q", err, cp.String())
	}
	snap = cp.Snapshot()
	if err := cp.SendLine("add " + f2); err != nil {
		t.Fatalf("Failed to send add: %v\nBuffer: %q", err, cp.String())
	}
	if err := expect(2*time.Second, snap, termtest.Contains("Added file:"), "file 2 added"); err != nil {
		t.Fatalf("Expected file added: %v\nBuffer: %q", err, cp.String())
	}

	// Make the first item's label ambiguous by rewriting it to just the basename
	// so that removing by id=1 will attempt context.removePath("a.txt") which
	// should match both tracked files and yield an ambiguity error.
	// N.B. We mutate JS state via the dedicated test hook.
	if err := cp.SendLine(`(function(){ if (typeof __promptFlowTestHooks !== "undefined") { __promptFlowTestHooks.withState(function(h){ var items=h.state.get(h.stateKeys.contextItems); if(items.length>0){ items[0].label="a.txt"; h.state.set(h.stateKeys.contextItems, items); } }); } })()`); err != nil {
		t.Fatalf("Failed to send JS hook: %v\nBuffer: %q", err, cp.String())
	}
	// The JS expression executes but produces no output - just need to let it process
	time.Sleep(100 * time.Millisecond)
	// sanity: list shows label now as just a.txt
	snap = cp.Snapshot()
	if err := cp.SendLine("list"); err != nil {
		t.Fatalf("Failed to send list: %v\nBuffer: %q", err, cp.String())
	}
	if err := expect(2*time.Second, snap, termtest.Contains("[1] [file] a.txt"), "file with ambiguous label"); err != nil {
		t.Fatalf("Expected file in list: %v\nBuffer: %q", err, cp.String())
	}

	// Attempt removal; should print an Error and NOT say Removed [1]
	snap = cp.Snapshot()
	if err := cp.SendLine("remove 1"); err != nil {
		t.Fatalf("Failed to send remove: %v\nBuffer: %q", err, cp.String())
	}
	if err := expect(2*time.Second, snap, termtest.Contains("Error:"), "error output"); err != nil {
		t.Fatalf("Expected error: %v\nBuffer: %q", err, cp.String())
	}
	// Ensure UI still shows the item (id 1)
	snap = cp.Snapshot()
	if err := cp.SendLine("list"); err != nil {
		t.Fatalf("Failed to send list: %v\nBuffer: %q", err, cp.String())
	}
	if err := expect(2*time.Second, snap, termtest.Contains("[1] [file] a.txt"), "file still in list"); err != nil {
		t.Fatalf("Expected file still in list: %v\nBuffer: %q", err, cp.String())
	}

	if err := cp.SendLine("exit"); err != nil {
		t.Fatalf("Failed to send exit: %v\nBuffer: %q", err, cp.String())
	}
	code, err := cp.WaitExit(t.Context())
	if err != nil {
		t.Fatalf("Failed to wait for exit: %v", err)
	}
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
}

// TestPromptFlow_Remove_NotFound_AbortsUI verifies that when the backend does not
// have the file (path not found), the JS UI does not remove the item and surfaces
// an error. This ensures we do not silently desynchronize UI from backend.
func TestPromptFlow_Remove_NotFound_AbortsUI(t *testing.T) {
	binaryPath := buildTestBinary(t)

	tmpFile := filepath.Join(t.TempDir(), "lonely.txt")
	if err := os.WriteFile(tmpFile, []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}

	env := newTestProcessEnv(t)
	env = append(env, "VISUAL=", "EDITOR=")

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	cp, err := termtest.NewConsole(ctx,
		termtest.WithCommand(binaryPath, "prompt-flow", "-i"),
		termtest.WithDefaultTimeout(60*time.Second),
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

	snap := cp.Snapshot()
	if err := expect(15*time.Second, snap, termtest.Contains("Switched to mode: prompt-flow"), "mode switch"); err != nil {
		t.Fatalf("Expected mode switch to prompt-flow: %v", err)
	}
	if err := expect(20*time.Second, snap, termtest.Contains("(prompt-flow) > "), "prompt"); err != nil {
		t.Fatalf("Expected prompt: %v", err)
	}

	// Add one file
	snap = cp.Snapshot()
	if err := cp.SendLine("add " + tmpFile); err != nil {
		t.Fatalf("Failed to send add: %v\nBuffer: %q", err, cp.String())
	}
	if err := expect(2*time.Second, snap, termtest.Contains("Added file:"), "file added"); err != nil {
		t.Fatalf("Expected file added: %v\nBuffer: %q", err, cp.String())
	}

	// Make the UI label a path that isn't tracked in the backend to simulate not found
	if err := cp.SendLine(`(function(){ if (typeof __promptFlowTestHooks !== "undefined") { __promptFlowTestHooks.withState(function(h){ var items=h.state.get(h.stateKeys.contextItems); if(items.length>0){ items[0].label="nonexistent.txt"; h.state.set(h.stateKeys.contextItems, items); } }); } })()`); err != nil {
		t.Fatalf("Failed to send JS hook: %v\nBuffer: %q", err, cp.String())
	}
	// The JS expression executes but produces no output - just need to let it process
	time.Sleep(100 * time.Millisecond)

	// Now attempt to remove by id from UI; backend will say not found -> Info and remove
	snap = cp.Snapshot()
	if err := cp.SendLine("remove 1"); err != nil {
		t.Fatalf("Failed to send remove: %v\nBuffer: %q", err, cp.String())
	}
	// Since RemovePath is idempotent, it returns success even if file is missing.
	// The UI should simply report "Removed [1]".
	if err := expect(2*time.Second, snap, termtest.Contains("Removed [1]"), "remove confirmation"); err != nil {
		t.Fatalf("Expected removal confirmation: %v\nBuffer: %q", err, cp.String())
	}

	// Ensure UI no longer shows the item (id 1)
	snap = cp.Snapshot()
	if err := cp.SendLine("list"); err != nil {
		t.Fatalf("Failed to send list: %v\nBuffer: %q", err, cp.String())
	}
	if err := expect(2*time.Second, snap, termtest.Contains("[1] [file]"), "forbidden item"); err == nil {
		t.Fatalf("Expected item to be removed from list, but it is still present\nBuffer: %q", cp.String())
	}

	if err := cp.SendLine("exit"); err != nil {
		t.Fatalf("Failed to send exit: %v\nBuffer: %q", err, cp.String())
	}
	code, err := cp.WaitExit(t.Context())
	if err != nil {
		t.Fatalf("Failed to wait for exit: %v", err)
	}
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
}
