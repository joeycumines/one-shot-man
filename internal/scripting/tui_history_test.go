package scripting

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestSaveHistory tests the saveHistory function
func TestSaveHistory(t *testing.T) {
	// Create a temporary file
	tmpDir := t.TempDir()
	historyFile := filepath.Join(tmpDir, "test_history")

	// Test saving empty history
	err := saveHistory(historyFile, []string{})
	if err != nil {
		t.Fatalf("saveHistory failed for empty history: %v", err)
	}

	// Test saving with commands
	commands := []string{"command1", "command2", "command3"}
	err = saveHistory(historyFile, commands)
	if err != nil {
		t.Fatalf("saveHistory failed: %v", err)
	}

	// Verify file contents
	content, err := os.ReadFile(historyFile)
	if err != nil {
		t.Fatalf("Failed to read history file: %v", err)
	}

	expected := "command1\ncommand2\ncommand3\n"
	if string(content) != expected {
		t.Fatalf("History file content mismatch:\ngot: %q\nwant: %q", string(content), expected)
	}

	// Test saving to empty filename (should be no-op)
	err = saveHistory("", commands)
	if err != nil {
		t.Fatalf("saveHistory should not fail for empty filename: %v", err)
	}
}

// TestLoadHistory tests the loadHistory function  
func TestLoadHistory(t *testing.T) {
	// Create a temporary file with history
	tmpDir := t.TempDir()
	historyFile := filepath.Join(tmpDir, "test_history")

	content := "command1\ncommand2\n\n  command3  \n\n"
	err := os.WriteFile(historyFile, []byte(content), 0644)
	if err != nil {
		t.Fatalf("Failed to create test history file: %v", err)
	}

	// Load and verify
	history := loadHistory(historyFile)
	expected := []string{"command1", "command2", "command3"}
	
	if len(history) != len(expected) {
		t.Fatalf("History length mismatch: got %d, want %d", len(history), len(expected))
	}

	for i, cmd := range history {
		if cmd != expected[i] {
			t.Fatalf("History command mismatch at index %d: got %q, want %q", i, cmd, expected[i])
		}
	}

	// Test loading non-existent file (should return empty slice)
	history = loadHistory("nonexistent")
	if len(history) != 0 {
		t.Fatalf("loadHistory should return empty slice for non-existent file, got %v", history)
	}

	// Test loading empty filename (should return empty slice)
	history = loadHistory("")
	if len(history) != 0 {
		t.Fatalf("loadHistory should return empty slice for empty filename, got %v", history)
	}
}

// TestTUIManagerHistoryOperations tests the TUIManager history functions
func TestTUIManagerHistoryOperations(t *testing.T) {
	var out bytes.Buffer
	eng := NewEngine(context.Background(), &out, &out)
	defer eng.Close()

	// Create manager without loading existing history
	tm := NewTUIManagerForTesting(context.Background(), eng, nil, &out)

	// Create temporary history file and override the default
	tmpDir := t.TempDir()
	historyFile := filepath.Join(tmpDir, "test_history")
	tm.historyFile = historyFile

	// Test addToHistory
	tm.addToHistory("command1")
	tm.addToHistory("command2")
	tm.addToHistory("command2") // duplicate should be ignored
	tm.addToHistory("  command3  ") // should be trimmed

	history := tm.getHistoryForPrompt()
	expected := []string{"command1", "command2", "command3"}

	if len(history) != len(expected) {
		t.Fatalf("History length mismatch: got %d, want %d", len(history), len(expected))
	}

	for i, cmd := range history {
		if cmd != expected[i] {
			t.Fatalf("History command mismatch at index %d: got %q, want %q", i, cmd, expected[i])
		}
	}

	// Test saving history
	err := tm.saveCurrentHistory()
	if err != nil {
		t.Fatalf("saveCurrentHistory failed: %v", err)
	}

	// Verify file was created and content is correct
	content, err := os.ReadFile(historyFile)
	if err != nil {
		t.Fatalf("Failed to read saved history file: %v", err)
	}

	expectedContent := "command1\ncommand2\ncommand3\n"
	if string(content) != expectedContent {
		t.Fatalf("Saved history content mismatch:\ngot: %q\nwant: %q", string(content), expectedContent)
	}
}

// TestTUIManagerHistoryLoading tests loading existing history
func TestTUIManagerHistoryLoading(t *testing.T) {
	var out bytes.Buffer
	eng := NewEngine(context.Background(), &out, &out)
	defer eng.Close()

	// Create a history file with existing content
	tmpDir := t.TempDir()
	historyFile := filepath.Join(tmpDir, "existing_history")
	existingContent := "old_command1\nold_command2\nold_command3\n"
	err := os.WriteFile(historyFile, []byte(existingContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create existing history file: %v", err)
	}

	// Create manager with custom history file
	tm := NewTUIManager(context.Background(), eng, nil, &out)
	tm.historyFile = historyFile
	
	// Load existing history
	tm.loadExistingHistory()

	history := tm.getHistoryForPrompt()
	expected := []string{"old_command1", "old_command2", "old_command3"}

	if len(history) != len(expected) {
		t.Fatalf("Loaded history length mismatch: got %d, want %d", len(history), len(expected))
	}

	for i, cmd := range history {
		if cmd != expected[i] {
			t.Fatalf("Loaded history command mismatch at index %d: got %q, want %q", i, cmd, expected[i])
		}
	}

	// Add new commands and verify they're appended
	tm.addToHistory("new_command")
	history = tm.getHistoryForPrompt()
	
	if len(history) != 4 {
		t.Fatalf("History should have 4 commands after adding new one, got %d", len(history))
	}
	
	if history[3] != "new_command" {
		t.Fatalf("New command not properly added: got %q, want 'new_command'", history[3])
	}
}

// TestTUIManagerHistoryMaxSize tests history size limiting
func TestTUIManagerHistoryMaxSize(t *testing.T) {
	var out bytes.Buffer
	eng := NewEngine(context.Background(), &out, &out)
	defer eng.Close()

	tm := NewTUIManagerForTesting(context.Background(), eng, nil, &out)
	
	// Set custom settings
	tmpDir := t.TempDir()
	historyFile := filepath.Join(tmpDir, "test_history")
	tm.historyFile = historyFile
	tm.historyMaxSize = 3 // Set small limit for testing

	// Add more commands than the limit
	commands := []string{"cmd1", "cmd2", "cmd3", "cmd4", "cmd5"}
	for _, cmd := range commands {
		tm.addToHistory(cmd)
	}

	history := tm.getHistoryForPrompt()
	
	// Should only keep the last 3 commands
	if len(history) != 3 {
		t.Fatalf("History should be limited to 3 commands, got %d", len(history))
	}

	expected := []string{"cmd3", "cmd4", "cmd5"}
	for i, cmd := range history {
		if cmd != expected[i] {
			t.Fatalf("History command mismatch at index %d: got %q, want %q", i, cmd, expected[i])
		}
	}
}

// TestTUIManagerExecutorHistoryCapture tests that executor captures commands in history
func TestTUIManagerExecutorHistoryCapture(t *testing.T) {
	var out bytes.Buffer
	eng := NewEngine(context.Background(), &out, &out)
	defer eng.Close()

	tm := NewTUIManagerForTesting(context.Background(), eng, nil, &out)

	// Use temp file
	tmpDir := t.TempDir()
	historyFile := filepath.Join(tmpDir, "test_history")
	tm.historyFile = historyFile

	// Execute some commands through the executor
	tm.executor("help")
	tm.executor("  mode  ") // should be trimmed
	tm.executor("") // empty should be ignored
	
	history := tm.getHistoryForPrompt()
	expected := []string{"help", "mode"}

	if len(history) != len(expected) {
		t.Fatalf("Executor history capture length mismatch: got %d, want %d", len(history), len(expected))
	}

	for i, cmd := range history {
		if cmd != expected[i] {
			t.Fatalf("Executor history command mismatch at index %d: got %q, want %q", i, cmd, expected[i])
		}
	}
}