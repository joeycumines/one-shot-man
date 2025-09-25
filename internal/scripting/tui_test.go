package scripting

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/elk-language/go-prompt"

	"github.com/joeycumines/one-shot-man/internal/termtest"
)

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

	// Test mode state
	tuiManager.SetState("test-key", "test-value")
	value := tuiManager.GetState("test-key")
	if value != "test-value" {
		t.Errorf("expected state value test-value, got %v", value)
	}

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

func testPromptCompletion(ctx context.Context, t *testing.T) {
	test, err := termtest.NewGoPromptTest(ctx)
	if err != nil {
		t.Fatalf("failed to create go-prompt test: %v", err)
	}
	defer test.Close()

	var commands []string
	executor := termtest.TestExecutor(&commands)
	completer := termtest.TestCompleter("help", "exit", "modes", "state")

	// Start prompt with completer
	test.RunPrompt(executor, prompt.WithCompleter(completer))

	// Test completion by typing partial command and pressing tab
	if err := test.SendInput("he"); err != nil {
		t.Fatalf("failed to send input: %v", err)
	}

	if err := test.SendKeys("tab"); err != nil {
		t.Fatalf("failed to send tab: %v", err)
	}

	// Wait for completion to appear
	if err := test.WaitForOutput("help", 1*time.Second); err != nil {
		t.Errorf("completion not shown: %v", err)
	}

	// Send enter to execute
	if err := test.SendKeys("enter"); err != nil {
		t.Fatalf("failed to send enter: %v", err)
	}

	// Send exit to finish
	if err := test.SendLine("exit"); err != nil {
		t.Fatalf("failed to send exit: %v", err)
	}

	// Wait for prompt to exit
	if err := test.WaitForExit(2 * time.Second); err != nil {
		t.Errorf("prompt did not exit cleanly: %v", err)
	}

	// Verify commands were captured
	if len(commands) == 0 {
		t.Error("no commands were executed")
	}
}

func testKeyBindings(ctx context.Context, t *testing.T) {
	test, err := termtest.NewGoPromptTest(ctx)
	if err != nil {
		t.Fatalf("failed to create go-prompt test: %v", err)
	}
	defer test.Close()

	var commands []string
	executor := termtest.TestExecutor(&commands)

	// Start prompt
	test.RunPrompt(executor)

	// Test basic key sequences
	if err := test.SendInput("test command"); err != nil {
		t.Fatalf("failed to send input: %v", err)
	}

	// Test backspace
	if err := test.SendKeys("backspace"); err != nil {
		t.Fatalf("failed to send backspace: %v", err)
	}

	if err := test.SendKeys("enter"); err != nil {
		t.Fatalf("failed to send enter: %v", err)
	}

	if err := test.SendLine("exit"); err != nil {
		t.Fatalf("failed to send exit: %v", err)
	}

	// Wait for prompt to exit
	if err := test.WaitForExit(2 * time.Second); err != nil {
		t.Errorf("prompt did not exit cleanly: %v", err)
	}

	// Check that input was processed
	output := test.GetOutput()
	if len(output) == 0 {
		t.Error("no output captured from prompt")
	}
}
