package scripting

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ActiveState/termtest"
)

// TestTUIInteractiveMode tests the rich TUI system using PTY-based testing
func TestTUIInteractiveMode(t *testing.T) {
	// Build the binary for testing
	binaryPath := buildTestBinary(t)
	defer os.Remove(binaryPath)

	// Test basic interactive terminal startup
	t.Run("InteractiveStartup", func(t *testing.T) {
		testInteractiveStartup(t, binaryPath)
	})

	// Test mode registration and switching
	t.Run("ModeRegistrationAndSwitching", func(t *testing.T) {
		testModeRegistrationAndSwitching(t, binaryPath)
	})

	// Test command execution in modes
	t.Run("CommandExecutionInModes", func(t *testing.T) {
		testCommandExecutionInModes(t, binaryPath)
	})

	// Test state management
	t.Run("StateManagement", func(t *testing.T) {
		testStateManagement(t, binaryPath)
	})

	// Test LLM prompt builder mode
	t.Run("LLMPromptBuilder", func(t *testing.T) {
		testLLMPromptBuilder(t, binaryPath)
	})
}

// testInteractiveStartup tests basic interactive terminal startup
func testInteractiveStartup(t *testing.T, binaryPath string) {
	opts := termtest.Options{
		CmdName:        binaryPath,
		Args:           []string{"script", "-i"},
		DefaultTimeout: 5 * time.Second,
	}

	cp, err := termtest.NewTest(t, opts)
	if err != nil {
		t.Fatalf("Failed to create termtest: %v", err)
	}
	defer cp.Close()

	// Expect the welcome message
	requireExpect(t, cp, "one-shot-man Rich TUI Terminal")

	// Expect the help instruction
	requireExpect(t, cp, "Type 'help' for available commands")

	// Test help command
	cp.SendLine("help")
	requireExpect(t, cp, "Available commands:")
	requireExpect(t, cp, "mode <name>")

	// Test exit
	cp.SendLine("exit")
	requireExpect(t, cp, "Goodbye!")
	requireExpectExitCode(t, cp, 0)
}

// testModeRegistrationAndSwitching tests mode registration and switching
func testModeRegistrationAndSwitching(t *testing.T, binaryPath string) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}
	projectDir := filepath.Clean(filepath.Join(wd, "..", ".."))

	opts := termtest.Options{
		CmdName:        binaryPath,
		Args:           []string{"script", "-i", filepath.Join(projectDir, "scripts", "demo-mode.js")},
		DefaultTimeout: 5 * time.Second,
	}

	cp, err := termtest.NewTest(t, opts)
	if err != nil {
		t.Fatalf("Failed to create termtest: %v", err)
	}
	defer cp.Close()

	// Wait for startup
	requireExpect(t, cp, "Rich TUI Terminal")

	// The demo-mode.js script should have registered a mode
	cp.SendLine("modes")
	requireExpect(t, cp, "demo")

	// Switch to demo mode
	cp.SendLine("mode demo")
	requireExpect(t, cp, "Switched to mode: demo")
	requireExpect(t, cp, "Entered demo mode!")

	// Test mode-specific commands
	cp.SendLine("count")
	requireExpect(t, cp, "Counter: 1")

	// Test message command
	cp.SendLine("message Hello World")
	requireExpect(t, cp, "Added message: Hello World")

	// Test show command
	cp.SendLine("show")
	requireExpect(t, cp, "Counter: 1")
	requireExpect(t, cp, "Messages: 1")

	// Exit
	cp.SendLine("exit")
	requireExpect(t, cp, "Leaving demo mode...")
	requireExpect(t, cp, "Final counter value: 1")
	requireExpectExitCode(t, cp, 0)
}

// testCommandExecutionInModes tests command execution within modes
func testCommandExecutionInModes(t *testing.T, binaryPath string) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}
	projectDir := filepath.Clean(filepath.Join(wd, "..", ".."))

	opts := termtest.Options{
		CmdName:        binaryPath,
		Args:           []string{"script", "-i", filepath.Join(projectDir, "scripts", "demo-mode.js")},
		DefaultTimeout: 5 * time.Second,
	}

	cp, err := termtest.NewTest(t, opts)
	if err != nil {
		t.Fatalf("Failed to create termtest: %v", err)
	}
	defer cp.Close()

	// Wait for startup and switch to demo mode
	requireExpect(t, cp, "Rich TUI Terminal")
	cp.SendLine("mode demo")
	requireExpect(t, cp, "Entered demo mode!")

	// Test JavaScript execution in mode context
	cp.SendLine("js console.log('JavaScript execution test')")
	requireExpect(t, cp, "JavaScript execution test")

	// Test global echo command (registered globally)
	cp.SendLine("echo Test global command")
	requireExpect(t, cp, "Test global command")

	// Test multiple count commands
	cp.SendLine("count")
	cp.SendLine("count")
	cp.SendLine("count")
	cp.SendLine("show")
	requireExpect(t, cp, "Counter: 3")

	cp.SendLine("exit")
	requireExpectExitCode(t, cp, 0)
}

// testStateManagement tests state management between commands
func testStateManagement(t *testing.T, binaryPath string) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}
	projectDir := filepath.Clean(filepath.Join(wd, "..", ".."))

	opts := termtest.Options{
		CmdName:        binaryPath,
		Args:           []string{"script", "-i", filepath.Join(projectDir, "scripts", "demo-mode.js")},
		DefaultTimeout: 5 * time.Second,
	}

	cp, err := termtest.NewTest(t, opts)
	if err != nil {
		t.Fatalf("Failed to create termtest: %v", err)
	}
	defer cp.Close()

	// Setup
	requireExpect(t, cp, "Rich TUI Terminal")
	cp.SendLine("mode demo")
	requireExpect(t, cp, "Entered demo mode!")

	// Test state persistence across multiple commands
	cp.SendLine("message First message")
	cp.SendLine("message Second message")
	cp.SendLine("message Third message")

	cp.SendLine("show")
	requireExpect(t, cp, "Messages: 3")

	// Should show recent messages
	requireExpect(t, cp, "1. First message")
	requireExpect(t, cp, "2. Second message")
	requireExpect(t, cp, "3. Third message")

	// Test global state command
	cp.SendLine("state")
	requireExpect(t, cp, "Mode: demo")

	cp.SendLine("exit")
	requireExpectExitCode(t, cp, 0)
}

// testLLMPromptBuilder tests the LLM prompt builder functionality
func testLLMPromptBuilder(t *testing.T, binaryPath string) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}
	projectDir := filepath.Clean(filepath.Join(wd, "..", ".."))

	opts := termtest.Options{
		CmdName:        binaryPath,
		Args:           []string{"script", "-i", filepath.Join(projectDir, "scripts", "llm-prompt-builder.js")},
		DefaultTimeout: 30 * time.Second,
	}

	cp, err := termtest.NewTest(t, opts)
	if err != nil {
		t.Fatalf("Failed to create termtest: %v", err)
	}
	defer cp.Close()

	// Wait for TUI startup
	requireExpect(t, cp, "one-shot-man Rich TUI Terminal")
	requireExpect(t, cp, "Available modes: llm-prompt-builder")

	// Switch to the LLM prompt builder mode
	cp.SendLine("mode llm-prompt-builder")
	requireExpect(t, cp, "Switched to mode: llm-prompt-builder")
	requireExpect(t, cp, "Welcome to LLM Prompt Builder!")

	// Test creating a new prompt
	cp.SendLine("new test-prompt A test prompt for AI")
	requireExpect(t, cp, "Created new prompt: test-prompt")

	// Test setting a template
	cp.SendLine("template You are a helpful assistant. Answer the following question: {{question}}")
	requireExpect(t, cp, "Template set:")

	// Test setting variables
	cp.SendLine("var question What is the capital of France?")
	requireExpect(t, cp, "Set variable: question = What is the capital of France?")

	// Test building the prompt
	cp.SendLine("build")
	requireExpect(t, cp, "Built prompt:")

	requireExpect(t, cp, "You are a helpful assistant. Answer the following question: What is the capital of France?")

	// Test saving a version
	cp.SendLine("save Initial version with France question")
	requireExpect(t, cp, "Saved version 1")

	// Test listing versions
	cp.SendLine("versions")
	requireExpect(t, cp, "v1 -")
	requireExpect(t, cp, "Initial version with France question")

	// Test preview
	cp.SendLine("preview")
	requireExpect(t, cp, "Title: test-prompt")

	// Test export
	cp.SendLine("export")
	requireExpect(t, cp, "Prompt export:")

	// Test list command
	cp.SendLine("list")
	requireExpect(t, cp, "test-prompt")

	cp.SendLine("exit")
	requireExpect(t, cp, "Exiting LLM Prompt Builder mode")
	requireExpectExitCode(t, cp, 0)
}
