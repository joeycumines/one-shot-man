package scripting

// Testing patterns for TUI/terminal tests:
//
// 1. ALWAYS capture buffer position BEFORE sending input/keys:
//    startLen := test.GetPTY().OutputLen()
//    test.SendInput("command")
//    test.GetPTY().WaitForOutputSince("expected", startLen, timeout)
//
// 2. NEVER use time.Sleep for orchestration - use channels instead:
//    executorIn := make(chan executorCall)
//    executorOut := make(chan executorResult)
//    // In executor: send to executorIn, wait for executorOut
//    // In test: wait for executorIn with timeout, then send to executorOut
//
// 3. Use WaitForOutputSince with offsets, not AssertOutput on full buffer.
//    This prevents matching stale data from previous commands.
//
// 4. For race detector safety, check bounds before accessing slices:
//    if len(cell.Runes) > 0 { content.WriteRune(cell.Runes[0]) }

import (
	"context"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/joeycumines/go-prompt"
	"github.com/joeycumines/one-shot-man/internal/termtest"
)

var stripANSIColor = regexp.MustCompile(`\x1B\[[0-9;]+[A-Za-z]`)

func TestTUIInteractiveMode(t *testing.T) {
	ctx := context.Background()

	t.Run("CommandExecution", func(t *testing.T) {
		testCommandExecution(ctx, t)
	})

	t.Run("ModeSwitching", func(t *testing.T) {
		testModeSwitching(ctx, t)
	})
}

func testCommandExecution(ctx context.Context, t *testing.T) {
	// Test that command execution works in the new implementation
	engine := mustNewEngine(t, ctx, os.Stdin, os.Stdout)

	tuiManager := engine.GetTUIManager()

	// Test built-in command registration
	commands := tuiManager.ListCommands()
	if len(commands) == 0 {
		t.Error("no built-in commands found")
	}

	// Verify specific commands exist
	expectedCommands := []string{"mode", "modes", "state"}
	for _, expected := range expectedCommands {
		found := false
		for _, cmd := range commands {
			if cmd.Name == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected command %s not found", expected)
		}
	}

	// Test command execution directly
	if err := tuiManager.ExecuteCommand("modes", []string{}); err != nil {
		t.Errorf("failed to execute modes command: %v", err)
	}

	if err := tuiManager.ExecuteCommand("state", []string{}); err != nil {
		t.Errorf("failed to execute state command: %v", err)
	}

	t.Logf("Command execution test passed - found %d commands", len(commands))
}

func testModeSwitching(ctx context.Context, t *testing.T) {
	// Test mode registration and switching functionality
	engine := mustNewEngine(t, ctx, os.Stdin, os.Stdout)

	// Register a test mode
	testScript := engine.LoadScriptFromString("test-mode", `
        tui.registerMode({
            name: "test-mode",
            tui: {
                title: "Test Mode",
                prompt: "[test]> "
            },
            onEnter: function() {
                output.print("Entered test mode!");
            }
        });
    `)

	err := engine.ExecuteScript(testScript)
	if err != nil {
		t.Fatalf("Failed to execute test script: %v", err)
	}

	tuiManager := engine.GetTUIManager()

	// Verify mode was registered
	modes := tuiManager.ListModes()
	found := false
	for _, mode := range modes {
		if mode == "test-mode" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("test-mode not found in registered modes: %v", modes)
	}

	// Test mode switching
	if err := tuiManager.SwitchMode("test-mode"); err != nil {
		t.Fatalf("failed to switch to test-mode: %v", err)
	}

	// Verify current mode
	currentMode := tuiManager.GetCurrentMode()
	if currentMode == nil {
		t.Error("no current mode after switching")
	} else if currentMode.Name != "test-mode" {
		t.Errorf("expected current mode to be test-mode, got %s", currentMode.Name)
	}

	// Note: Direct state testing now requires formal state contracts.
	// Mode switching itself is sufficient for this test.

	t.Logf("Mode switching test passed - successfully registered and switched to test-mode")
}

func TestTUIAdvancedPrompt(t *testing.T) {
	ctx := context.Background()

	t.Run("PromptCompletion", func(t *testing.T) {
		testPromptCompletion(ctx, t)
	})

	t.Run("KeyBindings", func(t *testing.T) {
		testKeyBindings(ctx, t)
	})
}

func TestExecutorTokenization_QuotedArgs(t *testing.T) {
	ctx := context.Background()
	var out strings.Builder
	engine := mustNewEngine(t, ctx, &out, &out)

	tm := engine.GetTUIManager()
	received := make([][]string, 0)
	tm.RegisterCommand(Command{
		Name:        "add",
		Description: "Add files",
		IsGoCommand: true,
		Handler: func(args []string) error {
			cp := make([]string, len(args))
			copy(cp, args)
			received = append(received, cp)
			return nil
		},
	})

	cases := []struct {
		line string
		want []string
	}{
		{`add "my report.docx"`, []string{"my report.docx"}},
		{`add 'My Folder'/file.txt`, []string{"My Folder/file.txt"}},
		{`add path\ with\ spaces.txt`, []string{"path with spaces.txt"}},
		{`add one two\ three "four five"`, []string{"one", "two three", "four five"}},
		{`add "embedded \"quote\".txt"`, []string{`embedded "quote".txt`}},
	}

	for _, tc := range cases {
		if !tm.executor(tc.line) {
			t.Fatalf("executor indicated exit for line: %s", tc.line)
		}
		if len(received) == 0 {
			t.Fatalf("no handler calls for line: %s", tc.line)
		}
		got := received[len(received)-1]
		if len(got) != len(tc.want) {
			t.Fatalf("args len mismatch: got %v want %v", got, tc.want)
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Fatalf("arg[%d] mismatch: got %q want %q (line: %s)", i, got[i], tc.want[i], tc.line)
			}
		}
	}
}

// waitForPromptIdle waits for the prompt to be in a stable state, meaning no new
// output has been generated for a short, consistent period. This is crucial for
// ensuring that subsequent input (like an "enter" key press) is processed as a
// new event and not bundled with the previous render's output.
func waitForPromptIdle(t *testing.T, test *termtest.GoPromptTest, timeout time.Duration) {
	t.Helper()
	if err := test.GetPTY().WaitIdleOutput(t.Context(), timeout); err != nil {
		t.Fatalf("timed out waiting for prompt to become idle: %s\nOutput: %q\n---\n%s",
			err,
			test.GetOutput(),
			stripANSIColor.ReplaceAllString(test.GetOutput(), ""))
	}
}

func testPromptCompletion(ctx context.Context, t *testing.T) {
	test, err := termtest.NewGoPromptTest(ctx)
	if err != nil {
		t.Fatalf("failed to create go-prompt test: %v", err)
	}
	defer test.Close()

	// Define orchestration channels for executor
	type executorCall struct {
		cmd string
	}
	type executorResult struct{}

	executorIn := make(chan executorCall)
	executorOut := make(chan executorResult)
	defer close(executorIn)
	defer close(executorOut)

	// Orchestrated executor wrapping test.Executor
	executor := func(cmd string) {
		// Send args (ping)
		select {
		case executorIn <- executorCall{cmd: cmd}:
			// Wait for result (pong)
			<-executorOut
		case <-ctx.Done():
			return
		}
		// Call the actual executor to record the command
		test.Executor(cmd)
	}

	completer := termtest.TestCompleter("help", "exit", "modes", "state")

	// Start prompt with completer and prefix
	test.RunPrompt(executor, prompt.WithPrefix("> "), prompt.WithCompleter(completer))

	// Wait for initial prompt to be ready, indicated by the final cursor show.
	initialLen := test.GetPTY().OutputLen()
	if err := test.GetPTY().WaitForRawOutputSince(1*time.Second, initialLen, "\x1b[?25h"); err != nil {
		t.Fatalf("prompt not ready: %v", err)
	}

	// Capture offset BEFORE typing
	startLen := test.GetPTY().OutputLen()

	// Test completion by typing partial command
	if err := test.SendInput("he"); err != nil {
		t.Fatalf("failed to send input: %v", err)
	}

	// Wait for the render cycle that echoes "he" to complete.
	if err := test.GetPTY().WaitForRawOutputSince(time.Second*2, startLen, "\x1b[?25h"); err != nil {
		t.Fatalf("input echo render not complete: %v", err)
	}

	// Capture offset BEFORE sending tab
	tabStartLen := test.GetPTY().OutputLen()
	if err := test.SendKeys("tab"); err != nil {
		t.Fatalf("failed to send tab: %v", err)
	}

	// Wait for the render cycle from tab completion to finish. The cursor will be
	// shown after the line is updated to "help". This is the sync point.
	if err := test.GetPTY().WaitForRawOutputSince(1*time.Second, tabStartLen, "\x1b[?25h"); err != nil {
		output := test.GetOutput()
		t.Fatalf("completion render not complete: %v\nOutput from %d: %q",
			err, tabStartLen, stripANSIColor.ReplaceAllString(output[tabStartLen:], ""))
	}

	// Wait for the prompt to be idle before sending enter to prevent race conditions.
	waitForPromptIdle(t, test, 0)

	// The tab key completed "he" to "help", so we can now execute
	if err := test.SendKeys("enter"); err != nil {
		t.Fatalf("failed to send enter: %v", err)
	}

	// Orchestrate: wait for executor call with timeout
	// Use a longer timeout to account for go-prompt's internal processing
	select {
	case <-ctx.Done():
		t.Fatalf("context done before command received")
	case <-time.After(10 * time.Second):
		output := test.GetOutput()
		t.Fatalf("timeout waiting for help command\nFull normalized output: %s\nRaw output: %q",
			stripANSIColor.ReplaceAllString(output, ""), output)
	case call := <-executorIn:
		if call.cmd != "help" {
			t.Fatalf("expected 'help', got %q", call.cmd)
		}
		// Send response unconditionally
		executorOut <- executorResult{}
	}

	// Close the prompt (ExitChecker is disabled, so we must close explicitly)
	if err := test.Close(); err != nil {
		t.Fatalf("close error: %v", err)
	}

	// Verify command was recorded
	commands := test.Commands()
	if len(commands) != 1 {
		t.Fatalf("expected 1 command, got %d: %v", len(commands), commands)
	}
	if commands[0] != "help" {
		t.Errorf("expected command to be 'help', got %q", commands[0])
	}
}

func testKeyBindings(ctx context.Context, t *testing.T) {
	test, err := termtest.NewGoPromptTest(ctx)
	if err != nil {
		t.Fatalf("failed to create go-prompt test: %v", err)
	}
	defer test.Close()

	// Define orchestration channels for executor
	type executorCall struct {
		cmd string
	}
	type executorResult struct{}

	executorIn := make(chan executorCall)
	executorOut := make(chan executorResult)
	defer close(executorIn)
	defer close(executorOut)

	// Orchestrated executor wrapping test.Executor
	executor := func(cmd string) {
		// Send args (ping)
		select {
		case executorIn <- executorCall{cmd: cmd}:
			// Wait for result (pong)
			<-executorOut
		case <-ctx.Done():
			return
		}
		// Call the actual executor to record the command
		test.Executor(cmd)
	}

	// Start prompt with prefix
	test.RunPrompt(executor, prompt.WithPrefix("> "))

	// Wait for initial prompt to be ready, indicated by the final cursor show.
	initialLen := test.GetPTY().OutputLen()
	if err := test.GetPTY().WaitForRawOutputSince(1*time.Second, initialLen, "\x1b[?25h"); err != nil {
		t.Fatalf("prompt not ready: %v", err)
	}

	// Test basic key sequences
	inputStartLen := test.GetPTY().OutputLen()
	if err := test.SendInput("test command"); err != nil {
		t.Fatalf("failed to send input: %v", err)
	}

	// Wait for the render cycle that echoes the input to complete.
	if err := test.GetPTY().WaitForRawOutputSince(1*time.Second, inputStartLen, "\x1b[?25h"); err != nil {
		t.Fatalf("input render not complete: %v", err)
	}

	// Test backspace
	backspaceStartLen := test.GetPTY().OutputLen()
	if err := test.SendKeys("backspace"); err != nil {
		t.Fatalf("failed to send backspace: %v", err)
	}

	// Wait for the rerender after backspace to complete, indicated by cursor show.
	if err := test.GetPTY().WaitForRawOutputSince(1*time.Second, backspaceStartLen, "\x1b[?25h"); err != nil {
		t.Fatalf("render after backspace not complete: %v\n%q", err, test.GetOutput())
	}

	// Wait for the prompt to be idle before sending enter to prevent race conditions.
	waitForPromptIdle(t, test, 0)

	if err := test.SendKeys("enter"); err != nil {
		t.Fatalf("failed to send enter: %v", err)
	}

	// Orchestrate: wait for executor call with timeout
	// Use a longer timeout to account for go-prompt's internal processing
	select {
	case <-ctx.Done():
		t.Fatalf("context done before command received")
	case <-time.After(10 * time.Second):
		output := test.GetOutput()
		t.Fatalf("timeout waiting for test command\nOutput: %q",
			stripANSIColor.ReplaceAllString(output, ""))
	case call := <-executorIn:
		if call.cmd != "test comman" { // backspace removed 'd'
			t.Errorf("expected 'test comman', got %q", call.cmd)
		}
		// Send response unconditionally
		executorOut <- executorResult{}
	}

	// Close the prompt (ExitChecker is disabled, so we must close explicitly)
	if err := test.Close(); err != nil {
		t.Fatalf("close error: %v", err)
	}

	// Check that input was processed
	output := test.GetOutput()
	if len(output) == 0 {
		t.Error("no output captured from prompt")
	}

	// Verify command was recorded
	commands := test.Commands()
	if len(commands) != 1 {
		t.Fatalf("expected 1 command, got %d: %v", len(commands), commands)
	}
	if commands[0] != "test comman" { // backspace removed 'd'
		t.Errorf("expected command to be 'test comman', got %q", commands[0])
	}
}
