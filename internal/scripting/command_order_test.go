package scripting

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/dop251/goja"
)

// TestCommandCompletionOrder verifies that REPL commands complete in a stable order
// based on registration order, not in pseudo-random map iteration order.
func TestCommandCompletionOrder(t *testing.T) {
	ctx := context.Background()
	engine := mustNewEngine(t, ctx, io.Discard, io.Discard)

	tuiManager := engine.GetTUIManager()

	// Register commands in a specific order that would be different from alphabetical
	commands := []Command{
		{Name: "zebra", Description: "Test command Z", IsGoCommand: true, Handler: func([]string) error { return nil }},
		{Name: "alpha", Description: "Test command A", IsGoCommand: true, Handler: func([]string) error { return nil }},
		{Name: "beta", Description: "Test command B", IsGoCommand: true, Handler: func([]string) error { return nil }},
		{Name: "gamma", Description: "Test command G", IsGoCommand: true, Handler: func([]string) error { return nil }},
	}

	for _, cmd := range commands {
		tuiManager.RegisterCommand(cmd)
	}

	// Test ListCommands multiple times to ensure consistent order
	var orderSnapshots [][]string
	for i := 0; i < 10; i++ {
		commandList := tuiManager.ListCommands()
		var names []string
		for _, cmd := range commandList {
			names = append(names, cmd.Name)
		}
		orderSnapshots = append(orderSnapshots, names)
	}

	// Verify all snapshots are identical
	firstSnapshot := orderSnapshots[0]
	for i, snapshot := range orderSnapshots {
		if len(snapshot) != len(firstSnapshot) {
			t.Errorf("Run %d: length mismatch, expected %d got %d", i, len(firstSnapshot), len(snapshot))
			continue
		}

		for j, name := range snapshot {
			if name != firstSnapshot[j] {
				t.Errorf("Run %d: order mismatch at position %d, expected %s got %s", i, j, firstSnapshot[j], name)
			}
		}
	}

	// Verify the order matches registration order (after built-in commands)
	// Built-in commands are registered first during initialization
	// Note: "help", "exit", "quit" are handled specially in the executor, not registered as commands
	builtinCommands := []string{"mode", "modes", "state", "reset"}
	expectedOrder := append(builtinCommands, "zebra", "alpha", "beta", "gamma")

	if len(firstSnapshot) != len(expectedOrder) {
		t.Fatalf("Expected %d commands, got %d", len(expectedOrder), len(firstSnapshot))
	}

	for i, expected := range expectedOrder {
		if firstSnapshot[i] != expected {
			t.Errorf("Position %d: expected %s, got %s", i, expected, firstSnapshot[i])
		}
	}

	t.Logf("Command order is stable: %v", firstSnapshot)
}

// TestCommandCompletionSuggestionOrder verifies that command completion suggestions
// also appear in a stable order for partial matches.
func TestCommandCompletionSuggestionOrder(t *testing.T) {
	// Create a minimal TUIManager for testing completion
	tm := &TUIManager{
		output:       io.Discard,
		commands:     make(map[string]Command),
		commandOrder: make([]string, 0),
		modes:        make(map[string]*ScriptMode),
	}

	// Register commands that all start with the same prefix in specific order
	commands := []string{"test_zebra", "test_alpha", "test_beta"}
	for _, name := range commands {
		tm.RegisterCommand(Command{
			Name:        name,
			Description: "Test command " + name,
			IsGoCommand: true,
			Handler:     func([]string) error { return nil },
		})
	}

	// Test completion suggestions for partial match multiple times
	var suggestionSnapshots [][]string
	for i := 0; i < 10; i++ {
		suggestions := tm.getDefaultCompletionSuggestionsFor("test_", "test_")
		var suggestionTexts []string
		for _, sugg := range suggestions {
			if strings.HasPrefix(sugg.Text, "test_") {
				suggestionTexts = append(suggestionTexts, sugg.Text)
			}
		}
		suggestionSnapshots = append(suggestionSnapshots, suggestionTexts)
	}

	// Verify all snapshots are identical
	firstSnapshot := suggestionSnapshots[0]
	for i, snapshot := range suggestionSnapshots {
		if len(snapshot) != len(firstSnapshot) {
			t.Errorf("Run %d: suggestion count mismatch, expected %d got %d", i, len(firstSnapshot), len(snapshot))
			continue
		}

		for j, text := range snapshot {
			if text != firstSnapshot[j] {
				t.Errorf("Run %d: suggestion order mismatch at position %d, expected %s got %s", i, j, firstSnapshot[j], text)
			}
		}
	}

	// Verify the suggestions match registration order
	expectedSuggestions := []string{"test_zebra", "test_alpha", "test_beta"}
	if len(firstSnapshot) != len(expectedSuggestions) {
		t.Fatalf("Expected %d suggestions, got %d", len(expectedSuggestions), len(firstSnapshot))
	}

	for i, expected := range expectedSuggestions {
		if firstSnapshot[i] != expected {
			t.Errorf("Position %d: expected %s, got %s", i, expected, firstSnapshot[i])
		}
	}

	t.Logf("Completion suggestion order is stable: %v", firstSnapshot)
}

// TestModeCommandOrder verifies that mode commands also maintain stable order
func TestModeCommandOrder(t *testing.T) {
	ctx := context.Background()
	engine := mustNewEngine(t, ctx, io.Discard, io.Discard)

	// Create a test mode with commands in specific order
	mode := &ScriptMode{
		Name:         "test-mode",
		Commands:     make(map[string]Command),
		CommandOrder: make([]string, 0),
		State:        make(map[goja.Value]interface{}),
	}

	// Add commands to mode in specific order
	modeCommands := []string{"zzz", "aaa", "mmm"}
	for _, name := range modeCommands {
		cmd := Command{
			Name:        name,
			Description: "Mode command " + name,
			IsGoCommand: true,
			Handler:     func([]string) error { return nil },
		}
		mode.Commands[name] = cmd
		mode.CommandOrder = append(mode.CommandOrder, name)
	}

	tuiManager := engine.GetTUIManager()
	err := tuiManager.RegisterMode(mode)
	if err != nil {
		t.Fatalf("Failed to register mode: %v", err)
	}

	err = tuiManager.SwitchMode("test-mode")
	if err != nil {
		t.Fatalf("Failed to switch to test mode: %v", err)
	}

	// Test ListCommands multiple times to ensure mode commands have stable order
	var orderSnapshots [][]string
	for i := 0; i < 10; i++ {
		commandList := tuiManager.ListCommands()
		var names []string
		for _, cmd := range commandList {
			names = append(names, cmd.Name)
		}
		orderSnapshots = append(orderSnapshots, names)
	}

	// Verify all snapshots are identical
	firstSnapshot := orderSnapshots[0]
	for i, snapshot := range orderSnapshots {
		if len(snapshot) != len(firstSnapshot) {
			t.Errorf("Run %d: length mismatch, expected %d got %d", i, len(firstSnapshot), len(snapshot))
			continue
		}

		for j, name := range snapshot {
			if name != firstSnapshot[j] {
				t.Errorf("Run %d: order mismatch at position %d, expected %s got %s", i, j, firstSnapshot[j], name)
			}
		}
	}

	// Verify mode commands appear in registration order (after global commands)
	// The command list should be: global commands first, then mode commands
	expectedModeCommandsAtEnd := []string{"zzz", "aaa", "mmm"}
	actualModeCommands := firstSnapshot[len(firstSnapshot)-len(expectedModeCommandsAtEnd):]

	for i, expected := range expectedModeCommandsAtEnd {
		if actualModeCommands[i] != expected {
			t.Errorf("Mode command at position %d: expected %s, got %s", i, expected, actualModeCommands[i])
		}
	}

	t.Logf("Mode command order is stable: %v", actualModeCommands)
}
