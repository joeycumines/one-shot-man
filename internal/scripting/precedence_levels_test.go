package scripting

import (
	"context"
	"os"
	"testing"

	"github.com/elk-language/go-prompt"
)

func TestCompletionPrecedenceLevels(t *testing.T) {
	ctx := context.Background()
	engine := mustNewEngine(t, ctx, os.Stdin, os.Stdout)

	tuiManager := engine.GetTUIManager()

	// Test all three levels of precedence
	testScript := engine.LoadScriptFromString("precedence-test", `
		// Register a global command that should override built-in "help"
		tui.registerCommand({
			name: "help",
			description: "Global registered help",
			handler: function() {
				output.print("Global help");
			}
		});

		// Register a mode that also has a help command (should override both)
		tui.registerMode({
			name: "precedence-mode",
			commands: {
				help: {
					description: "Mode help command",
					handler: function() {
						output.print("Mode help");
					}
				}
			}
		});
	`)

	err := engine.ExecuteScript(testScript)
	if err != nil {
		t.Fatalf("Failed to register commands and mode: %v", err)
	}

	t.Run("GlobalCommandOverridesBuiltIn", func(t *testing.T) {
		// Test without being in any mode - registered command should override built-in
		doc := prompt.Document{Text: "he"}
		suggestions := tuiManager.getDefaultCompletionSuggestions(doc)

		var helpSuggestions []prompt.Suggest
		for _, s := range suggestions {
			if s.Text == "help" {
				helpSuggestions = append(helpSuggestions, s)
			}
		}

		if len(helpSuggestions) != 1 {
			t.Errorf("Expected exactly 1 help suggestion, got %d: %+v", len(helpSuggestions), helpSuggestions)
		} else if helpSuggestions[0].Description != "Global registered help" {
			t.Errorf("Expected 'Global registered help', got '%s'", helpSuggestions[0].Description)
		}
	})

	t.Run("ModeCommandOverridesEverything", func(t *testing.T) {
		// Switch to the mode
		switchScript := engine.LoadScriptFromString("switch-mode", `
			tui.switchMode("precedence-mode");
		`)
		err := engine.ExecuteScript(switchScript)
		if err != nil {
			t.Fatalf("Failed to switch mode: %v", err)
		}

		// Test in mode - mode command should override everything
		doc := prompt.Document{Text: "he"}
		suggestions := tuiManager.getDefaultCompletionSuggestions(doc)

		var helpSuggestions []prompt.Suggest
		for _, s := range suggestions {
			if s.Text == "help" {
				helpSuggestions = append(helpSuggestions, s)
			}
		}

		if len(helpSuggestions) != 1 {
			t.Errorf("Expected exactly 1 help suggestion, got %d: %+v", len(helpSuggestions), helpSuggestions)
		} else if helpSuggestions[0].Description != "Mode help command" {
			t.Errorf("Expected 'Mode help command', got '%s'", helpSuggestions[0].Description)
		}
	})
}
