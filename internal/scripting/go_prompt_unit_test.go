package scripting

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/joeycumines/go-prompt"
	istrings "github.com/joeycumines/go-prompt/strings"
	"github.com/joeycumines/one-shot-man/internal/testutil"
)

func TestGoPromptIntegration(t *testing.T) {
	t.Run("CompletionFunctionality", func(t *testing.T) {
		// Create a TUI manager
		engine := &Engine{
			vm:      nil, // Not needed for this test
			ctx:     context.Background(),
			stdout:  os.Stdout,
			stderr:  os.Stderr,
			globals: make(map[string]interface{}),
		}

		tm := NewTUIManagerWithConfig(context.Background(), engine, os.Stdin, os.Stdout, testutil.NewTestSessionID("gopromptint1", t.Name()), "memory")

		// Register a test mode
		tm.RegisterMode(&ScriptMode{
			Name: "test-mode",
		})

		// Test the completion function
		completer := func(document prompt.Document) ([]prompt.Suggest, istrings.RuneNumber, istrings.RuneNumber) {
			suggestions := tm.getDefaultCompletionSuggestions(document)
			return suggestions, 0, istrings.RuneNumber(len(document.Text))
		}

		// Test completion with "he" -> should suggest "help"
		doc := prompt.Document{Text: "he"}
		suggestions, _, _ := completer(doc)

		if len(suggestions) == 0 {
			t.Error("Expected completion suggestions, got none")
		}

		foundHelp := false
		for _, suggestion := range suggestions {
			if suggestion.Text == "help" {
				foundHelp = true
				break
			}
		}

		if !foundHelp {
			t.Error("Expected 'help' suggestion for 'he' prefix")
		}
	})

	t.Run("ExecutorFunctionality", func(t *testing.T) {
		// Create a TUI manager
		engine := &Engine{
			vm:      nil,
			ctx:     context.Background(),
			stdout:  os.Stdout,
			stderr:  os.Stderr,
			globals: make(map[string]interface{}),
		}

		tm := NewTUIManagerWithConfig(context.Background(), engine, os.Stdin, os.Stdout, testutil.NewTestSessionID("gopromptint2", t.Name()), "memory")

		// Test the executor function behavior with help command
		var output strings.Builder
		tm.writer = NewTUIWriterFromIO(&output)

		// Test executor with "help" command
		_ = tm.executor("help")

		outputStr := output.String()
		if !strings.Contains(outputStr, "Available commands:") {
			t.Error("Expected help command to produce output")
		}

		// Test executor with "exit" command
		exitResult := tm.executor("exit")

		if exitResult {
			t.Error("Expected executor to return false for exit command")
		}
	})

	t.Run("PromptConfiguration", func(t *testing.T) {
		// Create a TUI manager
		engine := &Engine{
			vm:      nil,
			ctx:     context.Background(),
			stdout:  os.Stdout,
			stderr:  os.Stderr,
			globals: make(map[string]interface{}),
		}

		tm := NewTUIManagerWithConfig(context.Background(), engine, os.Stdin, os.Stdout, testutil.NewTestSessionID("gopromptint3", t.Name()), "memory")

		// Test prompt string generation
		promptString := tm.getPromptString()
		if promptString != ">>> " {
			t.Errorf("Expected default prompt '>>> ', got %q", promptString)
		}

		// Register a mode and test mode-specific prompt
		tm.RegisterMode(&ScriptMode{
			Name: "test-mode",
		})

		tm.SwitchMode("test-mode")

		modePromptString := tm.getPromptString()
		expectedModePrompt := "[test-mode]> "
		if modePromptString != expectedModePrompt {
			t.Errorf("Expected mode prompt %q, got %q", expectedModePrompt, modePromptString)
		}
	})
}

func TestFullGoPromptWorkflow(t *testing.T) {
	// This test validates the complete go-prompt workflow without actually running the interactive prompt

	// Create engine and TUI manager
	engine := &Engine{
		vm:      nil,
		ctx:     context.Background(),
		stdout:  os.Stdout,
		stderr:  os.Stderr,
		globals: make(map[string]interface{}),
	}

	tm := NewTUIManagerWithConfig(context.Background(), engine, os.Stdin, os.Stdout, testutil.NewTestSessionID("gopromptwflw", t.Name()), "memory")

	// Register commands and modes (same as in production)
	tm.RegisterMode(&ScriptMode{
		Name: "llm-prompt-builder",
	})

	// Test that all components work together
	t.Run("CompletionIntegration", func(t *testing.T) {
		// Test command completion
		doc := prompt.Document{Text: "help"}
		completer := func(document prompt.Document) ([]prompt.Suggest, istrings.RuneNumber, istrings.RuneNumber) {
			suggestions := tm.getDefaultCompletionSuggestions(document)
			return suggestions, 0, istrings.RuneNumber(len(document.Text))
		}

		suggestions, _, _ := completer(doc)

		// Should find exact match for "help"
		foundExactHelp := false
		for _, suggestion := range suggestions {
			if suggestion.Text == "help" {
				foundExactHelp = true
				break
			}
		}

		if !foundExactHelp {
			t.Error("Expected exact 'help' suggestion")
		}
	})

	t.Run("CommandExecution", func(t *testing.T) {
		// Test command execution workflow
		var output strings.Builder
		tm.writer = NewTUIWriterFromIO(&output)

		// Execute help command
		result := tm.executor("help")

		if !result {
			t.Error("Help command should return true")
		}

		outputStr := output.String()
		if !strings.Contains(outputStr, "Available commands:") {
			t.Error("Help output should contain 'Available commands:'")
		}
	})

	t.Run("ModeOperations", func(t *testing.T) {
		// Test mode switching
		tm.SwitchMode("llm-prompt-builder")

		if tm.currentMode == nil || tm.currentMode.Name != "llm-prompt-builder" {
			t.Error("Failed to switch to llm-prompt-builder mode")
		}

		// Test mode-specific prompt
		promptStr := tm.getPromptString()
		expected := "[llm-prompt-builder]> "
		if promptStr != expected {
			t.Errorf("Expected mode prompt %q, got %q", expected, promptStr)
		}
	})
}
