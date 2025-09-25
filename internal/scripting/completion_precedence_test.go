package scripting

import (
	"context"
	"os"
	"testing"

	"github.com/elk-language/go-prompt"
)

func TestCompletionPrecedence(t *testing.T) {
	ctx := context.Background()
	engine := mustNewEngine(t, ctx, os.Stdin, os.Stdout)

	tuiManager := engine.GetTUIManager()

	// Register a mode with a help command that should override the built-in help
	testScript := engine.LoadScriptFromString("test-mode-help", `
		tui.registerMode({
			name: "test-mode",
			tui: {
				title: "Test Mode",
				prompt: "[test]> "
			},
			commands: {
				help: {
					description: "Show help", // This should take precedence over "Built-in command"
					handler: function() {
						output.print("Mode-specific help");
					}
				}
			}
		});

		// Switch to the test mode
		tui.switchMode("test-mode");
	`)

	err := engine.ExecuteScript(testScript)
	if err != nil {
		t.Fatalf("Failed to register mode and switch: %v", err)
	}

	t.Run("DefaultPromptShouldRespectModeCommands", func(t *testing.T) {
		// Test that when in a mode with commands, those commands should take
		// precedence over built-in commands with the same name

		// Create a document with "he" - this should match "help"
		doc := prompt.Document{Text: "he"}

		// Get completions from default completion function
		suggestions := tuiManager.getDefaultCompletionSuggestions(doc)

		// Check what completions we get for "help"
		var allHelp []prompt.Suggest

		for i := range suggestions {
			if suggestions[i].Text == "help" {
				allHelp = append(allHelp, suggestions[i])
			}
		}

		t.Logf("All help completions found: %+v", allHelp)

		if len(allHelp) > 1 {
			t.Errorf("Multiple help completions found - mode command should take precedence. Found: %+v", allHelp)
		} else if len(allHelp) == 1 {
			if allHelp[0].Description == "Show help" {
				t.Logf("Correct: Mode help command takes precedence: %+v", allHelp[0])
			} else {
				t.Errorf("Built-in help found instead of mode help: %+v", allHelp[0])
			}
		} else {
			t.Error("No help completion found at all")
		}
	})

	t.Run("CustomPromptWithJSCompleter", func(t *testing.T) {
		// Test that custom prompts can use JS completers
		// This should continue to work as before

		// Register a JS completer for the custom prompt
		completersScript := engine.LoadScriptFromString("test-completer", `
			tui.registerCompleter('helpCompleter', function(document) {
				var word = document.getWordBeforeCursor();
				if ('help'.startsWith(word)) {
					return [{
						text: 'help',
						description: 'JS completer help' // Different from mode help
					}];
				}
				return [];
			});
		`)

		err := engine.ExecuteScript(completersScript)
		if err != nil {
			t.Fatalf("Failed to register JS completer: %v", err)
		}

		// Create a custom prompt and test its completion behavior
		promptConfig := map[string]interface{}{
			"name":   "testPrompt",
			"title":  "Test Prompt",
			"prefix": "test> ",
		}

		promptName, err := tuiManager.jsCreateAdvancedPrompt(promptConfig)
		if err != nil {
			t.Fatalf("Failed to create custom prompt: %v", err)
		}

		// Set the JS completer for this prompt
		err = tuiManager.jsSetCompleter(promptName, "helpCompleter")
		if err != nil {
			t.Fatalf("Failed to set completer: %v", err)
		}

		// Test the completer logic used by jsCreateAdvancedPrompt
		tuiManager.mu.RLock()
		jsCompleter := tuiManager.completers["helpCompleter"]
		tuiManager.mu.RUnlock()

		if jsCompleter == nil {
			t.Fatal("JS completer not found")
		}

		doc := prompt.Document{Text: "he"}
		suggestions, err := tuiManager.tryCallJSCompleter(jsCompleter, doc)
		if err != nil {
			t.Fatalf("JS completer call failed: %v", err)
		}

		if len(suggestions) != 1 || suggestions[0].Text != "help" || suggestions[0].Description != "JS completer help" {
			t.Errorf("Expected JS completion, got: %+v", suggestions)
		}

		t.Logf("Custom prompt correctly uses JS completion: %+v", suggestions[0])
	})
}
