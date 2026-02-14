//go:build unix

package scripting

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/joeycumines/go-prompt/termtest"
	"github.com/joeycumines/one-shot-man/internal/testutil"
)

// NO HELPER FUNCTIONS FOR EXPECT!
//
// Every test MUST:
// 1. Capture startLen := cp.OutputLen() BEFORE the action
// 2. Perform the action that produces output
// 3. Call cp.ExpectSince(expected, startLen, timeout...) directly
//
// Any helper that captures offset internally is UNSAFE and creates race conditions!

func requireExpectExitCode(ctx context.Context, t *testing.T, p *termtest.Console, exitCode int) {
	t.Helper()
	code, err := p.WaitExit(ctx)
	if err != nil {
		t.Fatalf("Failed to wait for exit: %v", err)
	}
	if code != exitCode {
		t.Fatalf("Expected exit code %d, but got %d", exitCode, code)
	}
}

// newTestProcessEnv creates an isolated environment for subprocess tests.
// It provides:
//   - Unique session ID to prevent session state sharing
//   - Memory storage backend to avoid filesystem contention
//   - Unique clipboard file in a temporary directory
//
// Test-specific variables like EDITOR should be appended by the caller.
func newTestProcessEnv(tb testing.TB) []string {
	tb.Helper()
	sessionID := testutil.NewTestSessionID("test", tb.Name())
	clipboardFile := filepath.Join(tb.(*testing.T).TempDir(), sessionID+"-clipboard.txt")
	return []string{
		"OSM_SESSION=" + sessionID,
		"OSM_STORE=memory",
		"OSM_CLIPBOARD=cat > " + clipboardFile,
	}
}

func mustNewEngine(tb testing.TB, ctx context.Context, stdout, stderr io.Writer) *Engine {
	tb.Helper()

	// Create a context that will be cancelled when the test ends to prevent goroutine leaks
	ctx, cancel := context.WithCancel(ctx)
	tb.Cleanup(cancel)

	// Use explicit session configuration to prevent data races from env var mutation.
	sessionID := testutil.NewTestSessionID("test", tb.Name())
	engine, err := NewEngineWithConfig(ctx, stdout, stderr, sessionID, "memory")
	if err != nil {
		tb.Fatalf("NewEngineWithConfig failed: %v", err)
	}
	tb.Cleanup(func() {
		_ = engine.Close()
	})
	return engine
}

// TestFullLLMWorkflow tests a complete LLM prompt building workflow
func TestFullLLMWorkflow(t *testing.T) {
	binaryPath := buildTestBinary(t)

	t.Logf("Built binary at: %s", binaryPath)

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}
	projectDir := filepath.Clean(filepath.Join(wd, "..", ".."))
	scriptPath := filepath.Join(projectDir, "scripts", "example-01-llm-prompt-builder.js")

	t.Logf("Working directory: %s", wd)
	t.Logf("Project directory: %s", projectDir)
	t.Logf("Script path: %s", scriptPath)

	env := newTestProcessEnv(t)
	defaultTimeout := 60 * time.Second

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cp, err := termtest.NewConsole(ctx,
		termtest.WithCommand(binaryPath, "script", "-i", scriptPath),
		termtest.WithDefaultTimeout(defaultTimeout),
		termtest.WithEnv(env),
	)
	if err != nil {
		t.Fatalf("Failed to create termtest: %v", err)
	}
	defer cp.Close()

	// expect helper closure
	expect := func(timeout time.Duration, since termtest.Snapshot, cond termtest.Condition, description string) error {
		subCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		return cp.Expect(subCtx, since, cond, description)
	}

	// Capture initial offset for automatic startup output
	snap := cp.Snapshot()

	if err := expect(20*time.Second, snap, termtest.Contains(">>> "), "prompt"); err != nil {
		t.Fatalf("Expected prompt: %v", err)
	}

	// Drive go-prompt interactively
	t.Log("Sending help command...")
	snap = cp.Snapshot()
	{
		subCtx, cancel := context.WithTimeout(ctx, defaultTimeout)
		defer cancel()
		if err := cp.WriteSync(subCtx, "help"); err != nil {
			t.Fatalf("Failed to send 'help' command: %v", err)
		}
		if err := cp.SendSync(subCtx, "ctrl+j"); err != nil {
			t.Fatalf("Failed to send enter for 'help': %v", err)
		}
	}
	if err := expect(defaultTimeout, snap, termtest.Contains("Available commands:"), "help output"); err != nil {
		t.Fatalf("Expected help output: %v\nOUTPUT: %q", err, cp.String())
	}

	// Switch to the LLM prompt builder mode
	snap = cp.Snapshot()
	{
		subCtx, cancel := context.WithTimeout(ctx, defaultTimeout)
		defer cancel()
		if err := cp.WriteSync(subCtx, "mode llm-prompt-builder"); err != nil {
			t.Fatalf("Failed to send 'mode llm-prompt-builder' command: %v", err)
		}
		if err := cp.SendSync(subCtx, "ctrl+j"); err != nil {
			t.Fatalf("Failed to send enter for mode switch: %v", err)
		}
	}
	if err := expect(defaultTimeout, snap, termtest.Contains("Switched to mode: llm-prompt-builder"), "mode switch"); err != nil {
		t.Fatalf("Expected mode switch: %v", err)
	}
	if err := expect(defaultTimeout, snap, termtest.Contains("Welcome to LLM Prompt Builder!"), "welcome message"); err != nil {
		t.Fatalf("Expected welcome message: %v", err)
	}

	// Complete workflow: Create prompt, refine it, save versions, export
	testCompletePromptWorkflow(t, ctx, cp, expect, defaultTimeout)
}

func testCompletePromptWorkflow(
	t *testing.T,
	ctx context.Context,
	cp *termtest.Console,
	expect func(time.Duration, termtest.Snapshot, termtest.Condition, string) error,
	defaultTimeout time.Duration,
) {
	// Create a customer service prompt
	snap := cp.Snapshot()
	{
		subCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if err := cp.WriteSync(subCtx, "new customer-service A customer service assistant prompt"); err != nil {
			t.Fatalf("Failed to send 'new' command: %v", err)
		}
		if err := cp.SendSync(subCtx, "ctrl+j"); err != nil {
			t.Fatalf("Failed to send enter for 'new': %v", err)
		}
	}
	if err := expect(time.Second*5, snap, termtest.Contains("Created new prompt: customer-service"), "prompt creation"); err != nil {
		t.Fatalf("Expected prompt creation: %v\nOUTPUT: %q", err, cp.String())
	}

	// Set initial template
	snap = cp.Snapshot()
	cp.SendLine("template You are a {{role}} for {{company}}. You should be {{tone}} and {{helpfulLevel}}. Customer issue: {{issue}}")
	if err := expect(time.Second*5, snap, termtest.Contains("Template set:"), "template set"); err != nil {
		t.Fatalf("Expected template set: %v", err)
	}

	// Set variables for first version - PING-PONG: capture, send, wait for each
	snap = cp.Snapshot()
	cp.SendLine("var role customer service representative")
	if err := expect(defaultTimeout, snap, termtest.Contains("Set variable: role"), "role variable set"); err != nil {
		t.Fatalf("Expected role variable set: %v", err)
	}

	snap = cp.Snapshot()
	cp.SendLine("var company TechCorp Inc")
	if err := expect(defaultTimeout, snap, termtest.Contains("Set variable: company"), "company variable set"); err != nil {
		t.Fatalf("Expected company variable set: %v", err)
	}

	snap = cp.Snapshot()
	cp.SendLine("var tone professional and friendly")
	if err := expect(defaultTimeout, snap, termtest.Contains("Set variable: tone"), "tone variable set"); err != nil {
		t.Fatalf("Expected tone variable set: %v", err)
	}

	snap = cp.Snapshot()
	cp.SendLine("var helpfulLevel extremely helpful")
	if err := expect(defaultTimeout, snap, termtest.Contains("Set variable: helpfulLevel"), "helpfulLevel variable set"); err != nil {
		t.Fatalf("Expected helpfulLevel variable set: %v", err)
	}

	snap = cp.Snapshot()
	cp.SendLine("var issue I can't log into my account")
	if err := expect(defaultTimeout, snap, termtest.Contains("Set variable: issue"), "issue variable set"); err != nil {
		t.Fatalf("Expected issue variable set: %v", err)
	}

	// Build and preview
	snap = cp.Snapshot()
	{
		subCtx, cancel := context.WithTimeout(ctx, defaultTimeout)
		defer cancel()
		if err := cp.WriteSync(subCtx, "build"); err != nil {
			t.Fatalf("Failed to send 'build' command: %v", err)
		}
		if err := cp.SendSync(subCtx, "ctrl+j"); err != nil {
			t.Fatalf("Failed to send enter for 'build': %v", err)
		}
	}
	if err := expect(defaultTimeout, snap, termtest.Contains("You are a customer service representative for TechCorp Inc."), "build output"); err != nil {
		t.Fatalf("Expected build output: %v", err)
	}
	if err := expect(defaultTimeout, snap, termtest.Contains("I can't log into my account"), "issue in output"); err != nil {
		t.Fatalf("Expected issue in output: %v", err)
	}

	// Save first version
	snap = cp.Snapshot()
	{
		subCtx, cancel := context.WithTimeout(ctx, defaultTimeout)
		defer cancel()
		if err := cp.WriteSync(subCtx, "save Initial customer service template"); err != nil {
			t.Fatalf("Failed to send 'save' command: %v", err)
		}
		if err := cp.SendSync(subCtx, "ctrl+j"); err != nil {
			t.Fatalf("Failed to send enter for 'save': %v", err)
		}
	}
	if err := expect(defaultTimeout, snap, termtest.Contains("Saved version 1"), "save confirmation"); err != nil {
		t.Fatalf("Expected save confirmation: %v", err)
	}

	// Refine the prompt - make it more specific
	snap = cp.Snapshot()
	{
		subCtx, cancel := context.WithTimeout(ctx, defaultTimeout)
		defer cancel()
		if err := cp.WriteSync(subCtx, "template You are a {{role}} for {{company}}. You should be {{tone}} and {{helpfulLevel}}. When handling customer issues, always: 1. Acknowledge the customer's concern 2. Ask clarifying questions if needed 3. Provide step-by-step solutions 4. Offer additional assistance Customer issue: {{issue}}"); err != nil {
			t.Fatalf("Failed to send 'template' command: %v", err)
		}
		if err := cp.SendSync(subCtx, "ctrl+j"); err != nil {
			t.Fatalf("Failed to send enter for 'template': %v", err)
		}
	}
	if err := expect(defaultTimeout, snap, termtest.Contains("Template set:"), "template update"); err != nil {
		t.Fatalf("Expected template update: %v", err)
	}

	// Build the refined version
	snap = cp.Snapshot()
	{
		subCtx, cancel := context.WithTimeout(ctx, defaultTimeout)
		defer cancel()
		if err := cp.WriteSync(subCtx, "build"); err != nil {
			t.Fatalf("Failed to send 'build' command: %v", err)
		}
		if err := cp.SendSync(subCtx, "ctrl+j"); err != nil {
			t.Fatalf("Failed to send enter for 'build': %v", err)
		}
	}
	if err := expect(defaultTimeout, snap, termtest.Contains("Built prompt:"), "build output"); err != nil {
		t.Fatalf("Expected build output: %v", err)
	}
	if err := expect(defaultTimeout, snap, termtest.Contains("1. Acknowledge the customer's concern"), "structured response"); err != nil {
		t.Fatalf("Expected structured response: %v", err)
	}
	if err := expect(defaultTimeout, snap, termtest.Contains("2. Ask clarifying questions"), "clarifying questions"); err != nil {
		t.Fatalf("Expected clarifying questions: %v", err)
	}

	// Save refined version
	snap = cp.Snapshot()
	{
		subCtx, cancel := context.WithTimeout(ctx, defaultTimeout)
		defer cancel()
		if err := cp.WriteSync(subCtx, "save Added structured response format"); err != nil {
			t.Fatalf("Failed to send 'save' command: %v", err)
		}
		if err := cp.SendSync(subCtx, "ctrl+j"); err != nil {
			t.Fatalf("Failed to send enter for 'save': %v", err)
		}
	}
	if err := expect(defaultTimeout, snap, termtest.Contains("Saved version 2"), "save confirmation"); err != nil {
		t.Fatalf("Expected save confirmation: %v", err)
	}

	// Test different issue type
	snap = cp.Snapshot()
	{
		subCtx, cancel := context.WithTimeout(ctx, defaultTimeout)
		defer cancel()
		if err := cp.WriteSync(subCtx, "var issue My order hasn't arrived and it's been a week"); err != nil {
			t.Fatalf("Failed to send 'var issue' command: %v", err)
		}
		if err := cp.SendSync(subCtx, "ctrl+j"); err != nil {
			t.Fatalf("Failed to send enter for 'var issue': %v", err)
		}
	}
	if err := expect(defaultTimeout, snap, termtest.Contains("Set variable: issue"), "issue variable set"); err != nil {
		t.Fatalf("Expected issue variable set: %v", err)
	}

	snap = cp.Snapshot()
	{
		subCtx, cancel := context.WithTimeout(ctx, defaultTimeout)
		defer cancel()
		if err := cp.WriteSync(subCtx, "build"); err != nil {
			t.Fatalf("Failed to send 'build' command: %v", err)
		}
		if err := cp.SendSync(subCtx, "ctrl+j"); err != nil {
			t.Fatalf("Failed to send enter for 'build': %v", err)
		}
	}
	if err := expect(defaultTimeout, snap, termtest.Contains("My order hasn't arrived and it's been a week"), "issue in build output"); err != nil {
		t.Fatalf("Expected issue in build output: %v", err)
	}

	// Save version for different issue
	snap = cp.Snapshot()
	{
		subCtx, cancel := context.WithTimeout(ctx, defaultTimeout)
		defer cancel()
		if err := cp.WriteSync(subCtx, "save Shipping issue variant"); err != nil {
			t.Fatalf("Failed to send 'save' command: %v", err)
		}
		if err := cp.SendSync(subCtx, "ctrl+j"); err != nil {
			t.Fatalf("Failed to send enter for 'save': %v", err)
		}
	}
	if err := expect(defaultTimeout, snap, termtest.Contains("Saved version 3"), "save confirmation"); err != nil {
		t.Fatalf("Expected save confirmation: %v", err)
	}

	// List all versions
	snap = cp.Snapshot()
	{
		subCtx, cancel := context.WithTimeout(ctx, defaultTimeout)
		defer cancel()
		if err := cp.WriteSync(subCtx, "versions"); err != nil {
			t.Fatalf("Failed to send 'versions' command: %v", err)
		}
		if err := cp.SendSync(subCtx, "ctrl+j"); err != nil {
			t.Fatalf("Failed to send enter for 'versions': %v", err)
		}
	}
	if err := expect(defaultTimeout, snap, termtest.Contains("v1 -"), "v1 in versions"); err != nil {
		t.Fatalf("Expected v1 in versions: %v", err)
	}
	if err := expect(defaultTimeout, snap, termtest.Contains("Initial customer service template"), "v1 description"); err != nil {
		t.Fatalf("Expected v1 description: %v", err)
	}
	if err := expect(defaultTimeout, snap, termtest.Contains("v2 -"), "v2 in versions"); err != nil {
		t.Fatalf("Expected v2 in versions: %v", err)
	}
	if err := expect(defaultTimeout, snap, termtest.Contains("Added structured response format"), "v2 description"); err != nil {
		t.Fatalf("Expected v2 description: %v", err)
	}
	if err := expect(defaultTimeout, snap, termtest.Contains("v3 -"), "v3 in versions"); err != nil {
		t.Fatalf("Expected v3 in versions: %v", err)
	}
	if err := expect(defaultTimeout, snap, termtest.Contains("Shipping issue variant"), "v3 description"); err != nil {
		t.Fatalf("Expected v3 description: %v", err)
	}

	// Test restoration
	snap = cp.Snapshot()
	{
		subCtx, cancel := context.WithTimeout(ctx, defaultTimeout)
		defer cancel()
		if err := cp.WriteSync(subCtx, "restore 1"); err != nil {
			t.Fatalf("Failed to send 'restore' command: %v", err)
		}
		if err := cp.SendSync(subCtx, "ctrl+j"); err != nil {
			t.Fatalf("Failed to send enter for 'restore': %v", err)
		}
	}
	if err := expect(defaultTimeout, snap, termtest.Contains("Restored to version 1"), "restore confirmation"); err != nil {
		t.Fatalf("Expected restore confirmation: %v", err)
	}

	snap = cp.Snapshot()
	cp.SendLine("build")
	if err := expect(defaultTimeout, snap, termtest.Contains("Built prompt:"), "build output after restore"); err != nil {
		t.Fatalf("Expected build output after restore: %v", err)
	}

	// Create a second prompt to test multi-prompt management
	snap = cp.Snapshot()
	cp.SendLine("new technical-support Technical support prompt")
	if err := expect(defaultTimeout, snap, termtest.Contains("Created new prompt: technical-support"), "prompt creation"); err != nil {
		t.Fatalf("Expected prompt creation: %v", err)
	}

	// Switch back to customer service
	snap = cp.Snapshot()
	cp.SendLine("load customer-service")
	if err := expect(defaultTimeout, snap, termtest.Contains("Loaded prompt: customer-service"), "load confirmation"); err != nil {
		t.Fatalf("Expected load confirmation: %v", err)
	}

	cp.SendLine("exit")
	requireExpectExitCode(ctx, t, cp, 0)
}

func TestMultiModeWorkflow(t *testing.T) {
	// Create a script that registers multiple modes
	multiModeScript := `
// Multi-mode test script
ctx.log("Registering multiple modes...");

// Create state for calculator mode (command-specific state)
const CalculatorStateKeys = {
    result: Symbol("result")
};
const calculatorState = tui.createState("calculator", {
    [CalculatorStateKeys.result]: {defaultValue: 0}
});

// Create state for notes mode (command-specific state)
const NotesStateKeys = {
    notes: Symbol("notes")
};
const notesState = tui.createState("notes", {
    [NotesStateKeys.notes]: {defaultValue: []}
});

// Register a simple calculator mode
tui.registerMode({
    name: "calculator",
    tui: {
        title: "Simple Calculator",
        prompt: "[calc]> "
    },
    onEnter: function() {
        output.print("Calculator mode active");
        var current = calculatorState.get(CalculatorStateKeys.result);
        output.print("Current result: " + current);
    },
    commands: function() {
        return {
            "add": {
                description: "Add numbers",
                usage: "add <num1> <num2>",
                handler: function(args) {
                    if (args.length !== 2) {
                        output.print("Usage: add <num1> <num2>");
                        return;
                    }
                    var result = parseFloat(args[0]) + parseFloat(args[1]);
                    calculatorState.set(CalculatorStateKeys.result, result);
                    output.print("Result: " + result);
                }
            },
            "result": {
                description: "Show current result",
                handler: function() {
                    output.print("Current result: " + calculatorState.get(CalculatorStateKeys.result));
                }
            }
        };
    }
});

// Register a note-taking mode
tui.registerMode({
    name: "notes",
    tui: {
        title: "Note Taker",
        prompt: "[notes]> "
    },
    onEnter: function() {
        output.print("Note-taking mode active");
        output.print("Notes stored: " + notesState.get(NotesStateKeys.notes).length);
    },
    commands: function() {
        return {
            "add": {
                description: "Add a note",
                usage: "add <note text>",
                handler: function(args) {
                    var note = args.join(" ");
                    var notes = notesState.get(NotesStateKeys.notes);
                    notes.push(note);
                    notesState.set(NotesStateKeys.notes, notes);
                    output.print("Added note: " + note);
                }
            },
            "list": {
                description: "List all notes",
                handler: function() {
                    var notes = notesState.get(NotesStateKeys.notes);
                    if (notes.length === 0) {
                        output.print("No notes yet");
                        return;
                    }
                    var noteList = [];
                    for (var i = 0; i < notes.length; i++) {
                        noteList.push((i + 1) + ". " + notes[i]);
                    }
                    output.print("Notes: " + noteList.join(", "));
                }
            }
        };
    }
});

ctx.log("Modes registered: calculator, notes");
`

	// Write the test script
	scriptPath := "/tmp/multi-mode-test.js"
	err := os.WriteFile(scriptPath, []byte(multiModeScript), 0644)
	if err != nil {
		t.Fatalf("Failed to write test script: %v", err)
	}
	defer os.Remove(scriptPath)

	binaryPath := buildTestBinary(t)

	env := newTestProcessEnv(t)
	defaultTimeout := 60 * time.Second

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cp, err := termtest.NewConsole(ctx,
		termtest.WithCommand(binaryPath, "script", "-i", scriptPath),
		termtest.WithDefaultTimeout(defaultTimeout),
		termtest.WithEnv(env),
	)
	if err != nil {
		t.Fatalf("Failed to create termtest: %v", err)
	}
	defer cp.Close()

	// expect helper closure
	expect := func(timeout time.Duration, since termtest.Snapshot, cond termtest.Condition, description string) error {
		subCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		return cp.Expect(subCtx, since, cond, description)
	}

	// Wait for startup — script writes a registration message when ready
	snap := cp.Snapshot()
	if err := expect(10*time.Second, snap, termtest.Contains(">>> "), "multi-mode registration log"); err != nil {
		t.Fatalf("Expected multi-mode registration log: %v\nOUTPUT: %q", err, cp.String())
	}

	// Test calculator mode
	snap = cp.Snapshot()
	cp.SendLine("mode calculator")
	if err := expect(defaultTimeout, snap, termtest.Contains("Switched to mode: calculator"), "mode switch"); err != nil {
		t.Fatalf("Expected mode switch: %v", err)
	}
	if err := expect(defaultTimeout, snap, termtest.Contains("Calculator mode active"), "calculator active"); err != nil {
		t.Fatalf("Expected calculator active: %v", err)
	}

	snap = cp.Snapshot()
	cp.SendLine("add 5 3")
	if err := expect(defaultTimeout, snap, termtest.Contains("Result: 8"), "add result"); err != nil {
		t.Fatalf("Expected add result: %v", err)
	}

	snap = cp.Snapshot()
	cp.SendLine("add 2 7")
	if err := expect(defaultTimeout, snap, termtest.Contains("Result: 9"), "add result"); err != nil {
		t.Fatalf("Expected add result: %v", err)
	}

	snap = cp.Snapshot()
	cp.SendLine("result")
	if err := expect(defaultTimeout, snap, termtest.Contains("Current result: 9"), "current result"); err != nil {
		t.Fatalf("Expected current result: %v", err)
	}

	// Switch to notes mode
	snap = cp.Snapshot()
	cp.SendLine("mode notes")
	if err := expect(defaultTimeout, snap, termtest.Contains("Switched to mode: notes"), "mode switch"); err != nil {
		t.Fatalf("Expected mode switch: %v", err)
	}
	if err := expect(defaultTimeout, snap, termtest.Contains("Note-taking mode active"), "notes active"); err != nil {
		t.Fatalf("Expected notes active: %v", err)
	}

	snap = cp.Snapshot()
	cp.SendLine("add This is my first note")
	if err := expect(defaultTimeout, snap, termtest.Contains("Added note: This is my first note"), "note added"); err != nil {
		t.Fatalf("Expected note added: %v", err)
	}

	snap = cp.Snapshot()
	cp.SendLine("add Another important note")
	if err := expect(defaultTimeout, snap, termtest.Contains("Added note: Another important note"), "note added"); err != nil {
		t.Fatalf("Expected note added: %v", err)
	}

	snap = cp.Snapshot()
	cp.SendLine("list")
	if err := expect(defaultTimeout, snap, termtest.Contains("Notes:"), "notes list"); err != nil {
		t.Fatalf("Expected notes list: %v", err)
	}

	// Switch back to calculator
	snap = cp.Snapshot()
	cp.SendLine("mode calculator")
	if err := expect(defaultTimeout, snap, termtest.Contains("Switched to mode: calculator"), "mode switch"); err != nil {
		t.Fatalf("Expected mode switch: %v", err)
	}
	if err := expect(defaultTimeout, snap, termtest.Contains("Calculator mode active"), "calculator active"); err != nil {
		t.Fatalf("Expected calculator active: %v", err)
	}

	// Result should persist across mode switches
	snap = cp.Snapshot()
	cp.SendLine("result")
	if err := expect(defaultTimeout, snap, termtest.Contains("Current result: 9"), "persisted result"); err != nil {
		t.Fatalf("Expected persisted result: %v", err)
	}

	// Switch back to notes
	snap = cp.Snapshot()
	cp.SendLine("mode notes")
	if err := expect(defaultTimeout, snap, termtest.Contains("Switched to mode: notes"), "mode switch"); err != nil {
		t.Fatalf("Expected mode switch: %v", err)
	}
	if err := expect(defaultTimeout, snap, termtest.Contains("Note-taking mode active"), "notes active"); err != nil {
		t.Fatalf("Expected notes active: %v", err)
	}

	// Notes should still be there
	snap = cp.Snapshot()
	cp.SendLine("list")
	if err := expect(defaultTimeout, snap, termtest.Contains("Notes:"), "notes list"); err != nil {
		t.Fatalf("Expected notes list: %v", err)
	}

	snap = cp.Snapshot()
	cp.SendLine("exit")
	if err := expect(defaultTimeout, snap, termtest.Contains("Goodbye!"), "goodbye"); err != nil {
		t.Fatalf("Expected goodbye: %v", err)
	}
	requireExpectExitCode(ctx, t, cp, 0)
}

// TestErrorHandling tests error conditions and edge cases
func TestErrorHandling(t *testing.T) {
	binaryPath := buildTestBinary(t)

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}
	projectDir := filepath.Clean(filepath.Join(wd, "..", ".."))

	env := newTestProcessEnv(t)
	defaultTimeout := 60 * time.Second

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cp, err := termtest.NewConsole(ctx,
		termtest.WithCommand(binaryPath, "script", "-i", filepath.Join(projectDir, "scripts", "test-01-register-mode.js")),
		termtest.WithDefaultTimeout(defaultTimeout),
		termtest.WithEnv(env),
	)
	if err != nil {
		t.Fatalf("Failed to create termtest: %v", err)
	}
	defer cp.Close()

	// expect helper closure
	expect := func(timeout time.Duration, since termtest.Snapshot, cond termtest.Condition, description string) error {
		subCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		return cp.Expect(subCtx, since, cond, description)
	}

	// Wait for startup — demo-mode script logs when it registers
	snap := cp.Snapshot()
	if err := expect(time.Second*10, snap, termtest.Contains(">>> "), "demo script readiness"); err != nil {
		t.Fatalf("Expected demo script readiness (available modes): %v\nOUTPUT: %q", err, cp.String())
	}

	// Test switching to non-existent mode
	snap = cp.Snapshot()
	cp.SendLine("mode nonexistent")
	if err := expect(defaultTimeout, snap, termtest.Contains("mode nonexistent not found"), "error message"); err != nil {
		t.Fatalf("Expected error message: %v", err)
	}

	// Test unknown command
	snap = cp.Snapshot()
	cp.SendLine("unknowncommand")
	if err := expect(defaultTimeout, snap, termtest.Contains("Command not found: unknowncommand"), "error message"); err != nil {
		t.Fatalf("Expected error message: %v", err)
	}

	// Switch to demo mode
	snap = cp.Snapshot()
	cp.SendLine("mode demo")
	if err := expect(defaultTimeout, snap, termtest.Contains("Switched to mode: demo"), "mode switch"); err != nil {
		t.Fatalf("Expected mode switch: %v", err)
	}
	if err := expect(defaultTimeout, snap, termtest.Contains("Entered demo mode!"), "demo mode active"); err != nil {
		t.Fatalf("Expected demo mode active: %v", err)
	}

	// Test command with wrong usage
	snap = cp.Snapshot()
	cp.SendLine("js")
	if err := expect(defaultTimeout, snap, termtest.Contains("Usage: js <code>"), "usage message"); err != nil {
		t.Fatalf("Expected usage message: %v", err)
	}

	// Test JavaScript syntax error
	snap = cp.Snapshot()
	cp.SendLine("js this is not valid javascript syntax +++")
	if err := expect(defaultTimeout, snap, termtest.Contains("Error:"), "error message"); err != nil {
		t.Fatalf("Expected error message: %v", err)
	}

	cp.SendLine("exit")
	requireExpectExitCode(ctx, t, cp, 0)
}

// TestConcurrentAccess tests the thread safety of the TUI system
func TestConcurrentAccess(t *testing.T) {
	ctx := context.Background()
	engine := mustNewEngine(t, ctx, os.Stdout, os.Stderr)

	tuiManager := engine.GetTUIManager()

	// Register a test mode with state
	script := engine.LoadScriptFromString("concurrent-test", `
        const stateKeys = {
            counter: Symbol("counter")
        };
        const state = tui.createState("concurrent-test", {
            [stateKeys.counter]: {defaultValue: 0}
        });

        tui.registerMode({
            name: "concurrent-test",
            commands: function() {
                return {
                    "increment": {
                        description: "Increment counter",
                        handler: function(args) {
                            var current = state.get(stateKeys.counter);
                            state.set(stateKeys.counter, current + 1);
                        }
                    },
                    "get": {
                        description: "Get counter value",
                        handler: function(args) {
                            output.print("Counter: " + state.get(stateKeys.counter));
                        }
                    }
                };
            }
        });
    `)

	err := engine.ExecuteScript(script)
	if err != nil {
		t.Fatalf("Script execution failed: %v", err)
	}

	// Switch to test mode
	err = tuiManager.SwitchMode("concurrent-test")
	if err != nil {
		t.Fatalf("Initial mode switch failed: %v", err)
	}

	// Important note: goja.Runtime is NOT thread-safe!
	// We cannot execute JavaScript from multiple goroutines concurrently.
	// This test focuses on exercising the Go-level synchronization primitives that
	// guard state access, mode information, and command registration.

	// Phase 1: concurrent reads of mode information (safe operation)
	done := make(chan bool, 10)
	errors := make(chan error, 20)

	for i := 0; i < 10; i++ {
		go func(id int) {
			defer func() { done <- true }()

			for j := 0; j < 100; j++ {
				mode := tuiManager.GetCurrentMode()
				if mode == nil {
					errors <- fmt.Errorf("goroutine %d: current mode is nil at iteration %d", id, j)
					return
				}
				if mode.Name != "concurrent-test" {
					errors <- fmt.Errorf("goroutine %d: wrong mode at iteration %d: expected concurrent-test, got %s", id, j, mode.Name)
					return
				}

				commands := tuiManager.ListCommands()
				if len(commands) == 0 {
					errors <- fmt.Errorf("goroutine %d: no commands found at iteration %d", id, j)
					return
				}

				modes := tuiManager.ListModes()
				found := false
				for _, modeName := range modes {
					if modeName == "concurrent-test" {
						found = true
						break
					}
				}
				if !found {
					errors <- fmt.Errorf("goroutine %d: concurrent-test mode not found in modes list at iteration %d", id, j)
					return
				}
			}
		}(i)
	}

	for i := 0; i < 10; i++ {
		select {
		case <-done:
		case err := <-errors:
			t.Fatal(err)
		case <-time.After(10 * time.Second):
			t.Fatal("Concurrent access test timed out during read phase")
		}
	}

	select {
	case err := <-errors:
		t.Fatal(err)
	default:
	}

	// Phase 2: concurrent read/write access via test helpers to validate mutex protection
	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			for j := 0; j < 100; j++ {
				valueToSet := int64(id*100 + j)
				if err := tuiManager.SetStateViaJS("concurrent-test:counter", valueToSet); err != nil {
					errors <- fmt.Errorf("goroutine %d: failed to set state at iteration %d: %v", id, j, err)
					return
				}

				val, err := tuiManager.GetStateViaJS("concurrent-test:counter")
				if err != nil {
					errors <- fmt.Errorf("goroutine %d: failed to get state at iteration %d: %v", id, j, err)
					return
				}

				if val == nil {
					errors <- fmt.Errorf("goroutine %d: nil state value at iteration %d", id, j)
					return
				}

				switch typed := val.(type) {
				case int64:
					// OK - expected type
				default:
					errors <- fmt.Errorf("goroutine %d: unexpected state type %T at iteration %d", id, typed, j)
					return
				}
			}
		}(i)
	}

	doneWriting := make(chan struct{})
	go func() {
		wg.Wait()
		close(doneWriting)
	}()

	select {
	case <-doneWriting:
	case err := <-errors:
		t.Fatal(err)
	case <-time.After(10 * time.Second):
		t.Fatal("Concurrent access test timed out during read/write phase")
	}

	select {
	case err := <-errors:
		t.Fatal(err)
	default:
	}

	finalVal, err := tuiManager.GetStateViaJS("concurrent-test:counter")
	if err != nil {
		t.Fatalf("Failed to retrieve final counter value: %v", err)
	}

	if finalVal == nil {
		t.Fatal("Expected final counter value to be set")
	}

	if _, ok := finalVal.(int64); !ok {
		t.Fatalf("Expected final counter value to be int64, got %T", finalVal)
	}
}

// TestJavaScriptInteroperability tests complex JavaScript integration
func TestJavaScriptInteroperability(t *testing.T) {
	ctx := context.Background()
	engine := mustNewEngine(t, ctx, os.Stdout, os.Stderr)

	// Test complex JavaScript integration with the TUI system
	script := engine.LoadScriptFromString("interop-test", `
        // Create state for complex mode
        var stateKeys = {
            config: Symbol("config"),
            testArray: Symbol("testArray")
        };
        var state = tui.createState("complex-mode", {
            [stateKeys.config]: {defaultValue: null},
            [stateKeys.testArray]: {defaultValue: []}
        });

        // Test complex object handling
        var complexConfig = {
            name: "complex-mode",
            tui: {
                title: "Complex Mode",
                prompt: "[complex]> "
            },
            onEnter: function() {
                output.print("Complex mode entered");
                // Test object state storage
                state.set(stateKeys.config, {
                    nested: {
                        value: 42,
                        array: [1, 2, 3]
                    }
                });
            },
            commands: function() {
                return {
                    "test-object": {
                        description: "Test object handling",
                        handler: function(args) {
                            var config = state.get(stateKeys.config);
                            output.print("Nested value: " + config.nested.value);
                            output.print("Array length: " + config.nested.array.length);
                        }
                    },
                    "test-array": {
                        description: "Test array operations",
                        handler: function(args) {
                            var arr = state.get(stateKeys.testArray);
                            arr.push(args.join(" "));
                            state.set(stateKeys.testArray, arr);
                            output.print("Array now has " + arr.length + " items");
                        }
                    }
                };
            }
        };

        // Register the complex mode
        tui.registerMode(complexConfig);

        // Test immediate switching and operation
        tui.switchMode("complex-mode");
    `)

	err := engine.ExecuteScript(script)
	if err != nil {
		t.Fatalf("Complex JavaScript interop failed: %v", err)
	}

	// Verify the mode was created and switched to
	tuiManager := engine.GetTUIManager()
	currentMode := tuiManager.GetCurrentMode()
	if currentMode == nil {
		t.Fatal("No current mode after complex script execution")
	}

	if currentMode.Name != "complex-mode" {
		t.Fatalf("Expected complex-mode, got %s", currentMode.Name)
	}

	// Verify state was set correctly using test helper
	configValue, err := tuiManager.GetStateViaJS("complex-mode:config")
	if err != nil {
		t.Fatalf("Failed to get config state: %v", err)
	}
	if configValue == nil {
		t.Error("Expected config to be set, but got nil")
	}
	// Verify it's an object with nested.value = 42
	if configMap, ok := configValue.(map[string]interface{}); ok {
		if nested, ok := configMap["nested"].(map[string]interface{}); ok {
			if value, ok := nested["value"].(int64); !ok || value != 42 {
				t.Errorf("Expected nested.value to be 42, got %v", nested["value"])
			}
		} else {
			t.Error("Expected nested object in config")
		}
	} else {
		t.Errorf("Expected config to be a map, got %T", configValue)
	}
}

// BenchmarkTUIPerformance benchmarks the TUI system performance
func BenchmarkTUIPerformance(b *testing.B) {
	ctx := context.Background()
	engine := mustNewEngine(b, ctx, os.Stdout, os.Stderr)

	// Register a test mode
	script := engine.LoadScriptFromString("perf-test", `
        const stateKeys = {
            counter: Symbol("counter")
        };
        const state = tui.createState("perf-test", {
            [stateKeys.counter]: {defaultValue: 0}
        });

        tui.registerMode({
            name: "perf-test",
            commands: function() {
                return {
                    "perf": {
                        description: "Performance test command",
                        handler: function() {
                            var current = state.get(stateKeys.counter);
                            state.set(stateKeys.counter, current + 1);
                        }
                    }
                };
            }
        });
    `)

	err := engine.ExecuteScript(script)
	if err != nil {
		b.Fatalf("Script execution failed: %v", err)
	}

	tuiManager := engine.GetTUIManager()
	err = tuiManager.SwitchMode("perf-test")
	if err != nil {
		b.Fatalf("Mode switching failed: %v", err)
	}

	b.ResetTimer()

	// Benchmark command execution
	b.Run("CommandExecution", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			err := tuiManager.ExecuteCommand("perf", []string{})
			if err != nil {
				b.Fatalf("Command execution failed: %v", err)
			}
		}
	})

	// Note: State operations benchmark removed as state now requires formal contracts
}

// TestContextManagement tests the context manager functionality
func TestContextManagement(t *testing.T) {
	ctx := context.Background()
	engine := mustNewEngine(t, ctx, os.Stdout, os.Stderr)

	// Test adding paths to context
	testScript := engine.LoadScriptFromString("context-test", `
        // Test context operations
        log.info("Testing context management");

        // Add current directory to context
        var err = context.addPath(".");
        if (err) {
            log.error("Failed to add path: " + err);
            throw new Error("Failed to add path");
        }

        // List paths
        var paths = context.listPaths();
        log.info("Tracked paths: " + paths.length);

        // Get stats
        var stats = context.getStats();
        log.info("Context stats: " + JSON.stringify(stats));

        // Generate txtar format
        var txtar = context.toTxtar();
        log.info("Generated txtar format (length: " + txtar.length + ")");

        output.print("Context test completed successfully");
    `)

	err := engine.ExecuteScript(testScript)
	if err != nil {
		t.Fatalf("Context management test failed: %v", err)
	}

	// Verify context manager has paths
	paths := engine.contextManager.ListPaths()
	if len(paths) == 0 {
		t.Error("Expected context manager to have paths, but got none")
	}

	// Test txtar generation
	txtarString := engine.contextManager.GetTxtarString()
	if len(txtarString) == 0 {
		t.Error("Expected txtar string to be generated, but got empty string")
	}
}

// TestLoggingSystem tests the structured logging system
func TestLoggingSystem(t *testing.T) {
	ctx := context.Background()

	// Capture output
	var logOutput strings.Builder
	engine := mustNewEngine(t, ctx, &logOutput, &logOutput)

	// Test logging operations
	testScript := engine.LoadScriptFromString("logging-test", `
        // Test different log levels
        log.debug("Debug message");
        log.info("Info message");
        log.warn("Warning message");
        log.error("Error message");

        // Test formatted logging
        log.printf("Formatted message: %s %d", "test", 42);

        // Test output (terminal output)
        output.print("Terminal output message");
        output.printf("Formatted terminal output: %s", "success");

        // Get logs
        var logs = log.getLogs();
        output.print("Total logs: " + logs.length);

        // Search logs
        var searchResults = log.searchLogs("info");
        output.print("Info logs found: " + searchResults.length);
    `)

	err := engine.ExecuteScript(testScript)
	if err != nil {
		t.Fatalf("Logging system test failed: %v", err)
	}

	// Verify logs were created
	logs := engine.logger.GetLogs()
	if len(logs) == 0 {
		t.Error("Expected logs to be created, but got none")
	}

	// Verify different log levels
	stats := engine.logger.GetLogStats()
	if stats["total"] == 0 {
		t.Error("Expected log statistics, but got zero total")
	}

	// Check if terminal output was written
	output := logOutput.String()
	if !strings.Contains(output, "Terminal output message") {
		t.Error("Expected terminal output message in output")
	}
}

// TestTUIModeSystem tests the TUI mode system functionality
func TestTUIModeSystem(t *testing.T) {
	ctx := context.Background()
	engine := mustNewEngine(t, ctx, os.Stdout, os.Stderr)

	// Test mode registration and switching
	testScript := engine.LoadScriptFromString("tui-mode-test", `
        // Create state contract for test mode
        var stateKeys = {
            testValue: Symbol("testValue"),
            lastCommand: Symbol("lastCommand")
        };
        var state = tui.createState("test-mode", {
            [stateKeys.testValue]: {defaultValue: null},
            [stateKeys.lastCommand]: {defaultValue: ""}
        });

        // Register a test mode
        tui.registerMode({
            name: "test-mode",
            tui: {
                title: "Test Mode",
                prompt: "[test]> "
            },
            onEnter: function() {
                log.info("Entered test mode");
                    state.set(stateKeys.testValue, "initialized");
            },
            onExit: function() {
                log.info("Exited test mode");
            },
            commands: function() {
                return {
                    "test-cmd": {
                        description: "A test command",
                        handler: function(args) {
                            state.set(stateKeys.lastCommand, args.join(" "));
                            output.print("Test command executed with: " + args.join(" "));
                        }
                    },
                    "get-state": {
                        description: "Get test state",
                        handler: function(args) {
                            var value = state.get(stateKeys.testValue);
                            output.print("Test value: " + value);
                        }
                    }
                };
            }
        });

        // Switch to the test mode
        tui.switchMode("test-mode");

        // Verify current mode
        var currentMode = tui.getCurrentMode();
        if (currentMode !== "test-mode") {
            throw new Error("Expected test-mode, got: " + currentMode);
        }

        output.print("TUI mode test completed successfully");
    `)

	err := engine.ExecuteScript(testScript)
	if err != nil {
		t.Fatalf("TUI mode system test failed: %v", err)
	}

	// Verify mode was registered and is current
	tuiManager := engine.GetTUIManager()
	currentMode := tuiManager.GetCurrentMode()
	if currentMode == nil {
		t.Fatal("Expected current mode to be set")
	}
	if currentMode.Name != "test-mode" {
		t.Fatalf("Expected test-mode, got %s", currentMode.Name)
	}

	// Test command execution
	err = tuiManager.ExecuteCommand("test-cmd", []string{"arg1", "arg2"})
	if err != nil {
		t.Fatalf("Command execution failed: %v", err)
	}

	// Verify state was set correctly using test helper
	testValue, err := tuiManager.GetStateViaJS("test-mode:testValue")
	if err != nil {
		t.Fatalf("Failed to get testValue: %v", err)
	}
	if testValue != "initialized" {
		t.Errorf("Expected testValue to be 'initialized', got %v", testValue)
	}

	lastCommand, err := tuiManager.GetStateViaJS("test-mode:lastCommand")
	if err != nil {
		t.Fatalf("Failed to get lastCommand: %v", err)
	}
	if lastCommand != "arg1 arg2" {
		t.Errorf("Expected lastCommand to be 'arg1 arg2', got %v", lastCommand)
	}
}

// TestFullIntegration tests complete integration of all systems
func TestFullIntegration(t *testing.T) {
	ctx := context.Background()

	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "osm-integration-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create test files
	testFile1 := filepath.Join(tempDir, "test1.txt")
	testFile2 := filepath.Join(tempDir, "test2.go")

	err = os.WriteFile(testFile1, []byte("This is test file 1"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file 1: %v", err)
	}

	err = os.WriteFile(testFile2, []byte("package main\nfunc main() { println(\"test\") }"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file 2: %v", err)
	}

	var output strings.Builder
	engine := mustNewEngine(t, ctx, &output, &output)

	// Integration test script
	testScript := engine.LoadScriptFromString("full-integration", fmt.Sprintf(`
        // Full integration test combining all systems
        log.info("Starting full integration test");

        // Create state for integration mode
        var stateKeys = {
            filesProcessed: Symbol("filesProcessed")
        };
        var state = tui.createState("integration-mode", {
            [stateKeys.filesProcessed]: {defaultValue: 0}
        });

        // Register a comprehensive mode
        tui.registerMode({
            name: "integration-mode",
            tui: {
                title: "Integration Test Mode",
                prompt: "[integration]> "
            },
            onEnter: function() {
                log.info("Entered integration mode");

                // Add test files to context
                context.addPath("%s");
                context.addPath("%s");

                var paths = context.listPaths();
                log.info("Added " + paths.length + " paths to context");

                // Initialize state (already done by default value, but we can verify)
                if (state.get(stateKeys.filesProcessed) === null) {
                    state.set(stateKeys.filesProcessed, 0);
                }
            },
            commands: function() {
                return {
                    "process-files": {
                        description: "Process files in context",
                        handler: function(args) {
                            var goFiles = context.getFilesByExt("go");
                            var txtFiles = context.getFilesByExt("txt");

                            log.info("Found " + goFiles.length + " Go files");
                            log.info("Found " + txtFiles.length + " text files");

                            var processed = state.get(stateKeys.filesProcessed) + goFiles.length + txtFiles.length;
                            state.set(stateKeys.filesProcessed, processed);

                            output.print("Processed " + processed + " files total");
                        }
                    },
                    "export-context": {
                        description: "Export context as txtar",
                        handler: function(args) {
                            var txtar = context.toTxtar();
                            log.info("Generated txtar with " + txtar.length + " characters");

                            // Test filtering
                            var goFiles = context.filterPaths("*.go");
                            log.info("Filtered " + goFiles.length + " Go files");

                            output.print("Context exported successfully");
                        }
                    },
                    "show-stats": {
                        description: "Show context and mode statistics",
                        handler: function(args) {
                            var contextStats = context.getStats();
                            var processed = state.get(stateKeys.filesProcessed);

                            output.print("Context: " + contextStats.files + " files, " + contextStats.totalSize + " bytes");
                            output.print("Processed: " + processed + " files");

                            var logs = log.getLogs(5);
                            output.print("Recent logs: " + logs.length + " entries");
                        }
                    }
                };
            }
        });

        // Switch to the mode
        tui.switchMode("integration-mode");

        log.info("Integration test setup completed");
        output.print("Integration test ready");
    `, testFile1, testFile2))

	err = engine.ExecuteScript(testScript)
	if err != nil {
		t.Fatalf("Full integration test failed: %v", err)
	}

	// Test the registered commands
	tuiManager := engine.GetTUIManager()

	// Test file processing
	err = tuiManager.ExecuteCommand("process-files", []string{})
	if err != nil {
		t.Fatalf("process-files command failed: %v", err)
	}

	// Test context export
	err = tuiManager.ExecuteCommand("export-context", []string{})
	if err != nil {
		t.Fatalf("export-context command failed: %v", err)
	}

	// Test statistics
	err = tuiManager.ExecuteCommand("show-stats", []string{})
	if err != nil {
		t.Fatalf("show-stats command failed: %v", err)
	}

	// Verify output contains expected results
	outputStr := output.String()
	if !strings.Contains(outputStr, "Integration test ready") {
		t.Error("Expected integration test ready message")
	}
	if !strings.Contains(outputStr, "Processed") {
		t.Error("Expected file processing output")
	}
	if !strings.Contains(outputStr, "Context exported successfully") {
		t.Error("Expected context export success message")
	}

	// Verify context manager has the test files
	paths := engine.contextManager.ListPaths()
	foundFiles := 0
	for _, path := range paths {
		if strings.Contains(path, "test1.txt") || strings.Contains(path, "test2.go") {
			foundFiles++
		}
	}
	if foundFiles != 2 {
		t.Errorf("Expected 2 test files in context, found %d", foundFiles)
	}
}

// TestScriptCommandVerification tests script-defined commands work correctly
func TestScriptCommandVerification(t *testing.T) {
	ctx := context.Background()

	var output strings.Builder
	engine := mustNewEngine(t, ctx, &output, &output)

	// Test script with various command types
	testScript := engine.LoadScriptFromString("command-verification", `
        // Create state for command test mode
        var stateKeys = {
            commandResults: Symbol("commandResults"),
            complexState: Symbol("complexState")
        };
        var state = tui.createState("command-test", {
            [stateKeys.commandResults]: {defaultValue: []},
            [stateKeys.complexState]: {defaultValue: {}}
        });

        // Register mode with comprehensive command testing
        tui.registerMode({
            name: "command-test",
            tui: {
                title: "Command Test Mode",
                prompt: "[cmd-test]> "
            },
            onEnter: function() {
                log.info("Command test mode initialized");
            },
            commands: function() {
                return {
                    "simple": {
                        description: "Simple command test",
                        handler: function(args) {
                            var results = state.get(stateKeys.commandResults);
                            results.push("simple:" + args.join(","));
                            state.set(stateKeys.commandResults, results);
                            output.print("Simple command executed");
                        }
                    },
                    "complex": {
                        description: "Complex command with state manipulation",
                        usage: "complex <action> [value]",
                        handler: function(args) {
                            if (args.length < 1) {
                                output.print("Usage: complex <action> [value]");
                                return;
                            }

                            var action = args[0];
                            var value = args.length > 1 ? args[1] : "";
                            var complexObj = state.get(stateKeys.complexState);

                            switch (action) {
                                case "set":
                                    if (!value) {
                                        output.print("Usage: complex set <value>");
                                        return;
                                    }
                                    complexObj[value] = new Date().getTime();
                                    break;
                                case "get":
                                    if (!value) {
                                        output.print("Usage: complex get <value>");
                                        return;
                                    }
                                    if (complexObj[value]) {
                                        output.print("Value " + value + " set at: " + complexObj[value]);
                                    } else {
                                        output.print("Value " + value + " not found");
                                    }
                                    return;
                                case "list":
                                    var keys = Object.keys(complexObj);
                                    output.print("Keys: " + keys.join(", "));
                                    return;
                            }

                            state.set(stateKeys.complexState, complexObj);
                            output.print("Complex command " + action + " completed");
                        }
                    },
                    "verify": {
                        description: "Verify all commands worked",
                        handler: function(args) {
                            var results = state.get(stateKeys.commandResults);
                            var complexObj = state.get(stateKeys.complexState);

                            output.print("Command results: " + results.length + " simple commands executed");
                            output.print("Complex state keys: " + Object.keys(complexObj).length);

                            // Log verification
                            log.info("Command verification completed");
                            var logs = log.getLogs();
                            output.print("Total logs: " + logs.length);
                        }
                    }
                };
            }
        });

        tui.switchMode("command-test");
        output.print("Command test mode ready");
    `)

	err := engine.ExecuteScript(testScript)
	if err != nil {
		t.Fatalf("Command verification script failed: %v", err)
	}

	tuiManager := engine.GetTUIManager()

	// Test simple command
	err = tuiManager.ExecuteCommand("simple", []string{"arg1", "arg2", "arg3"})
	if err != nil {
		t.Fatalf("Simple command failed: %v", err)
	}

	// Test complex command - set operations
	err = tuiManager.ExecuteCommand("complex", []string{"set", "testkey1"})
	if err != nil {
		t.Fatalf("Complex set command failed: %v", err)
	}

	err = tuiManager.ExecuteCommand("complex", []string{"set", "testkey2"})
	if err != nil {
		t.Fatalf("Complex set command 2 failed: %v", err)
	}

	// Test complex command - get operation
	err = tuiManager.ExecuteCommand("complex", []string{"get", "testkey1"})
	if err != nil {
		t.Fatalf("Complex get command failed: %v", err)
	}

	// Test complex command - list operation
	err = tuiManager.ExecuteCommand("complex", []string{"list"})
	if err != nil {
		t.Fatalf("Complex list command failed: %v", err)
	}

	// Test verification
	err = tuiManager.ExecuteCommand("verify", []string{})
	if err != nil {
		t.Fatalf("Verify command failed: %v", err)
	}

	// Verify the output contains expected results
	outputStr := output.String()
	if !strings.Contains(outputStr, "Simple command executed") {
		t.Error("Expected simple command execution output")
	}
	if !strings.Contains(outputStr, "Complex command set completed") {
		t.Error("Expected complex command execution output")
	}
	if !strings.Contains(outputStr, "Value testkey1 set at:") {
		t.Errorf("Expected complex get command output. Actual output:\n%s", outputStr)
	}
	if !strings.Contains(outputStr, "Keys: testkey1, testkey2") && !strings.Contains(outputStr, "Keys: testkey2, testkey1") {
		t.Errorf("Expected complex list command output. Actual output:\n%s", outputStr)
	}
}

// TestScriptStateVerification tests script-defined state functionality
func TestScriptStateVerification(t *testing.T) {
	ctx := context.Background()

	var output strings.Builder
	engine := mustNewEngine(t, ctx, &output, &output)

	// Test state persistence and manipulation
	testScript := engine.LoadScriptFromString("state-verification", `
        // Create state for state-test mode
        var stateKeys = {
            counter: Symbol("counter"),
            messages: Symbol("messages"),
            config: Symbol("config")
        };
        var state = tui.createState("state-test", {
            [stateKeys.counter]: {defaultValue: 0},
            [stateKeys.messages]: {defaultValue: []},
            [stateKeys.config]: {
                defaultValue: {
                    name: "test-config",
                    features: ["logging", "state", "commands"],
                    settings: {
                        debug: true,
                        maxItems: 100
                    }
                }
            }
        });

        // Register mode with comprehensive state testing
        tui.registerMode({
            name: "state-test",
            tui: {
                title: "State Test Mode",
                prompt: "[state-test]> "
            },
            onEnter: function() {
                // State is auto-initialized from defaults
                log.info("State test mode initialized with complex state");
            },
            commands: function() {
                return {
                    "increment": {
                        description: "Increment counter",
                        handler: function(args) {
                            var counter = state.get(stateKeys.counter);
                            counter++;
                            state.set(stateKeys.counter, counter);
                            output.print("Counter: " + counter);
                        }
                    },
                    "add-message": {
                        description: "Add message to array",
                        usage: "add-message <text>",
                        handler: function(args) {
                            var messages = state.get(stateKeys.messages);
                            var text = args.join(" ");
                            messages.push({
                                text: text,
                                timestamp: new Date().getTime(),
                                id: messages.length + 1
                            });
                            state.set(stateKeys.messages, messages);
                            output.print("Added message: " + text + " (total: " + messages.length + ")");
                        }
                    },
                        "update-config": {
                            description: "Update configuration",
                            usage: "update-config <key> <value>",
                        handler: function(args) {
                            if (args.length < 2) {
                                output.print("Usage: update-config <key> <value>");
                                return;
                            }

                            var config = state.get(stateKeys.config);
                            var key = args[0];
                            var value = args[1];

                            // Handle nested updates
                            if (key.indexOf(".") !== -1) {
                                var parts = key.split(".");
                                var obj = config;
                                for (var i = 0; i < parts.length - 1; i++) {
                                    if (!obj[parts[i]]) {
                                        obj[parts[i]] = {};
                                    }
                                    obj = obj[parts[i]];
                                }
                                obj[parts[parts.length - 1]] = value;
                            } else {
                                config[key] = value;
                            }

                            state.set(stateKeys.config, config);
                            output.print("Updated config." + key + " = " + value);
                        }
                    },
                    "show-state": {
                        description: "Show all state",
                        handler: function(args) {
                            var counter = state.get(stateKeys.counter);
                            var messages = state.get(stateKeys.messages);
                            var config = state.get(stateKeys.config);

                            output.print("=== State Summary ===");
                            output.print("Counter: " + counter);
                            output.print("Messages: " + messages.length + " items");
                            output.print("Config name: " + config.name);
                            output.print("Config features: " + config.features.join(", "));
                            output.print("Config debug: " + config.settings.debug);
                            output.print("Config maxItems: " + config.settings.maxItems);

                            if (messages.length > 0) {
                                output.print("Recent messages:");
                                var recent = messages.slice(-3);
                                for (var i = 0; i < recent.length; i++) {
                                    output.print("  " + recent[i].id + ": " + recent[i].text);
                                }
                            }
                        }
                    }
                };
            }
        });

        tui.switchMode("state-test");
        output.print("State test mode ready");
    `)

	err := engine.ExecuteScript(testScript)
	if err != nil {
		t.Fatalf("State verification script failed: %v", err)
	}

	tuiManager := engine.GetTUIManager()

	// Test counter operations
	for i := 0; i < 5; i++ {
		err = tuiManager.ExecuteCommand("increment", []string{})
		if err != nil {
			t.Fatalf("Increment command failed at iteration %d: %v", i, err)
		}
	}

	// Test message additions
	messages := []string{"First message", "Second message", "Third message with spaces"}
	for _, msg := range messages {
		err = tuiManager.ExecuteCommand("add-message", strings.Split(msg, " "))
		if err != nil {
			t.Fatalf("Add message command failed for '%s': %v", msg, err)
		}
	}

	// Test config updates
	configUpdates := map[string]string{
		"version":           "1.0",
		"settings.debug":    "false",
		"settings.maxItems": "200",
	}

	for key, value := range configUpdates {
		err = tuiManager.ExecuteCommand("update-config", []string{key, value})
		if err != nil {
			t.Fatalf("Update config command failed for %s=%s: %v", key, value, err)
		}
	}

	// Show final state
	err = tuiManager.ExecuteCommand("show-state", []string{})
	if err != nil {
		t.Fatalf("Show state command failed: %v", err)
	}

	// Verify state using test helpers
	counterValue, err := tuiManager.GetStateViaJS("state-test:counter")
	if err != nil {
		t.Fatalf("Failed to get counter state: %v", err)
	}
	if counterInt, ok := counterValue.(int64); !ok || counterInt != 5 {
		t.Errorf("Expected counter to be 5, got %v (type %T)", counterValue, counterValue)
	}

	messagesValue, err := tuiManager.GetStateViaJS("state-test:messages")
	if err != nil {
		t.Fatalf("Failed to get messages state: %v", err)
	}
	if messages, ok := messagesValue.([]interface{}); !ok || len(messages) != 3 {
		t.Errorf("Expected 3 messages, got %v (type %T)", messagesValue, messagesValue)
	}

	configValue, err := tuiManager.GetStateViaJS("state-test:config")
	if err != nil {
		t.Fatalf("Failed to get config state: %v", err)
	}
	if configMap, ok := configValue.(map[string]interface{}); ok {
		if version, exists := configMap["version"]; !exists || version != "1.0" {
			t.Errorf("Expected config.version to be '1.0', got %v", configMap["version"])
		}
		if settings, ok := configMap["settings"].(map[string]interface{}); ok {
			if debug, ok := settings["debug"].(string); !ok || debug != "false" {
				t.Errorf("Expected config.settings.debug to be 'false', got %v", settings["debug"])
			}
			if maxItems, ok := settings["maxItems"].(string); !ok || maxItems != "200" {
				t.Errorf("Expected config.settings.maxItems to be '200', got %v", settings["maxItems"])
			}
		} else {
			t.Error("Expected config.settings to be a map")
		}
	} else {
		t.Errorf("Expected config to be a map, got %T", configValue)
	}

	// Verify output contains expected state information
	outputStr := output.String()
	if !strings.Contains(outputStr, "Counter: 5") {
		t.Error("Expected final counter value of 5 in output")
	}
	if !strings.Contains(outputStr, "Messages: 3 items") {
		t.Error("Expected 3 messages in output")
	}
	if !strings.Contains(outputStr, "Updated config.version = 1.0") {
		t.Error("Expected config version update in output")
	}
	if !strings.Contains(outputStr, "Config debug: false") {
		t.Error("Expected config debug to be false in output")
	}
}
