package scripting

import (
	"os"
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
		CmdName: binaryPath,
		Args:    []string{"script", "-i"},
	}

	cp, err := termtest.NewTest(t, opts)
	if err != nil {
		t.Fatalf("Failed to create termtest: %v", err)
	}
	defer cp.Close()

	// Expect the welcome message
	_, err = cp.Expect("one-shot-man Rich TUI Terminal", 10*time.Second)
	if err != nil {
		t.Fatalf("Expected welcome message not found: %v", err)
	}

	// Expect the help instruction
	_, err = cp.Expect("Type 'help' for available commands", 5*time.Second)
	if err != nil {
		t.Fatalf("Expected help instruction not found: %v", err)
	}

	// Test help command
	cp.SendLine("help")
	_, err = cp.Expect("Available commands:", 5*time.Second)
	if err != nil {
		t.Fatalf("Expected help output not found: %v", err)
	}

	_, err = cp.Expect("mode <name>", 5*time.Second)
	if err != nil {
		t.Fatalf("Expected mode command in help not found: %v", err)
	}

	// Test exit
	cp.SendLine("exit")
	_, err = cp.Expect("Goodbye!", 5*time.Second)
	if err != nil {
		t.Fatalf("Expected goodbye message not found: %v", err)
	}

	// Wait for the process to finish
	cp.Wait(5 * time.Second)
}

// testModeRegistrationAndSwitching tests mode registration and switching
func testModeRegistrationAndSwitching(t *testing.T, binaryPath string) {
	opts := termtest.Options{
		CmdName: binaryPath,
		Args:    []string{"script", "-i", "scripts/demo-mode.js"},
	}

	cp, err := termtest.NewTest(t, opts)
	if err != nil {
		t.Fatalf("Failed to create termtest: %v", err)
	}
	defer cp.Close()

	// Wait for startup
	_, err = cp.Expect("Rich TUI Terminal", 10*time.Second)
	if err != nil {
		t.Fatalf("Terminal startup failed: %v", err)
	}

	// The demo-mode.js script should have registered a mode
	cp.SendLine("modes")
	_, err = cp.Expect("demo", 5*time.Second)
	if err != nil {
		t.Fatalf("Demo mode not found in modes list: %v", err)
	}

	// Switch to demo mode
	cp.SendLine("mode demo")
	_, err = cp.Expect("Switched to mode: demo", 5*time.Second)
	if err != nil {
		t.Fatalf("Mode switching failed: %v", err)
	}

	_, err = cp.Expect("Entered demo mode!", 5*time.Second)
	if err != nil {
		t.Fatalf("Demo mode onEnter callback not executed: %v", err)
	}

	// Test mode-specific commands
	cp.SendLine("count")
	_, err = cp.Expect("Counter: 1", 5*time.Second)
	if err != nil {
		t.Fatalf("Count command failed: %v", err)
	}

	// Test message command
	cp.SendLine("message Hello World")
	_, err = cp.Expect("Added message: Hello World", 5*time.Second)
	if err != nil {
		t.Fatalf("Message command failed: %v", err)
	}

	// Test show command
	cp.SendLine("show")
	_, err = cp.Expect("Counter: 1", 5*time.Second)
	if err != nil {
		t.Fatalf("Show command failed: %v", err)
	}

	_, err = cp.Expect("Messages: 1", 5*time.Second)
	if err != nil {
		t.Fatalf("Show command messages count failed: %v", err)
	}

	// Exit
	cp.SendLine("exit")
	_, err = cp.Expect("Leaving demo mode...", 5*time.Second)
	if err != nil {
		t.Fatalf("Demo mode onExit callback not executed: %v", err)
	}

	_, err = cp.Expect("Final counter value: 1", 5*time.Second)
	if err != nil {
		t.Fatalf("Demo mode final state not shown: %v", err)
	}

	cp.Wait(5 * time.Second)
}

// testCommandExecutionInModes tests command execution within modes
func testCommandExecutionInModes(t *testing.T, binaryPath string) {
	opts := &termtest.Options{
		CmdName: binaryPath,
		Args:    []string{"script", "-i", "scripts/demo-mode.js"},
		Timeout: 30 * time.Second,
	}

	cp, err := termtest.NewTest(opts)
	if err != nil {
		t.Fatalf("Failed to create termtest: %v", err)
	}
	defer cp.Close()

	// Wait for startup and switch to demo mode
	cp.ExpectString("Rich TUI Terminal")
	cp.SendLine("mode demo")
	cp.ExpectString("Entered demo mode!")

	// Test JavaScript execution in mode context
	cp.SendLine("js console.log('JavaScript execution test')")
	err = cp.ExpectString("JavaScript execution test")
	if err != nil {
		t.Fatalf("JavaScript execution in mode failed: %v", err)
	}

	// Test global echo command (registered globally)
	cp.SendLine("echo Test global command")
	err = cp.ExpectString("Test global command")
	if err != nil {
		t.Fatalf("Global echo command failed: %v", err)
	}

	// Test multiple count commands
	cp.SendLine("count")
	cp.SendLine("count")
	cp.SendLine("count")
	cp.SendLine("show")
	err = cp.ExpectString("Counter: 4") // Should be incremented from previous test + 3
	if err != nil {
		t.Fatalf("Multiple count commands failed: %v", err)
	}

	cp.SendLine("exit")
	cp.ExpectEOF()
}

// testStateManagement tests state management between commands
func testStateManagement(t *testing.T, binaryPath string) {
	opts := &termtest.Options{
		CmdName: binaryPath,
		Args:    []string{"script", "-i", "scripts/demo-mode.js"},
		Timeout: 30 * time.Second,
	}

	cp, err := termtest.NewTest(opts)
	if err != nil {
		t.Fatalf("Failed to create termtest: %v", err)
	}
	defer cp.Close()

	// Setup
	cp.ExpectString("Rich TUI Terminal")
	cp.SendLine("mode demo")
	cp.ExpectString("Entered demo mode!")

	// Test state persistence across multiple commands
	cp.SendLine("message First message")
	cp.SendLine("message Second message")
	cp.SendLine("message Third message")

	cp.SendLine("show")
	err = cp.ExpectString("Messages: 3")
	if err != nil {
		t.Fatalf("State persistence failed: %v", err)
	}

	// Should show recent messages
	err = cp.ExpectString("1. First message")
	if err != nil {
		t.Fatalf("First message not found in state: %v", err)
	}

	err = cp.ExpectString("2. Second message")
	if err != nil {
		t.Fatalf("Second message not found in state: %v", err)
	}

	err = cp.ExpectString("3. Third message")
	if err != nil {
		t.Fatalf("Third message not found in state: %v", err)
	}

	// Test global state command
	cp.SendLine("state")
	err = cp.ExpectString("Mode: demo")
	if err != nil {
		t.Fatalf("State command mode display failed: %v", err)
	}

	cp.SendLine("exit")
	cp.ExpectEOF()
}

// testLLMPromptBuilder tests the LLM prompt builder functionality
func testLLMPromptBuilder(t *testing.T, binaryPath string) {
	opts := &termtest.Options{
		CmdName: binaryPath,
		Args:    []string{"script", "-i", "scripts/llm-prompt-builder.js"},
		Timeout: 30 * time.Second,
	}

	cp, err := termtest.NewTest(opts)
	if err != nil {
		t.Fatalf("Failed to create termtest: %v", err)
	}
	defer cp.Close()

	// Wait for LLM prompt builder mode to be activated
	err = cp.ExpectString("Welcome to LLM Prompt Builder!")
	if err != nil {
		t.Fatalf("LLM Prompt Builder welcome not found: %v", err)
	}

	// Test creating a new prompt
	cp.SendLine("new test-prompt A test prompt for AI")
	err = cp.ExpectString("Created new prompt: test-prompt")
	if err != nil {
		t.Fatalf("Prompt creation failed: %v", err)
	}

	// Test setting a template
	cp.SendLine("template You are a helpful assistant. Answer the following question: {{question}}")
	err = cp.ExpectString("Template set:")
	if err != nil {
		t.Fatalf("Template setting failed: %v", err)
	}

	// Test setting variables
	cp.SendLine("var question What is the capital of France?")
	err = cp.ExpectString("Set variable: question = What is the capital of France?")
	if err != nil {
		t.Fatalf("Variable setting failed: %v", err)
	}

	// Test building the prompt
	cp.SendLine("build")
	err = cp.ExpectString("Built prompt:")
	if err != nil {
		t.Fatalf("Prompt building failed: %v", err)
	}

	err = cp.ExpectString("You are a helpful assistant. Answer the following question: What is the capital of France?")
	if err != nil {
		t.Fatalf("Variable substitution failed: %v", err)
	}

	// Test saving a version
	cp.SendLine("save Initial version with France question")
	err = cp.ExpectString("Saved version 1")
	if err != nil {
		t.Fatalf("Version saving failed: %v", err)
	}

	// Test listing versions
	cp.SendLine("versions")
	err = cp.ExpectString("v1 -")
	if err != nil {
		t.Fatalf("Version listing failed: %v", err)
	}

	err = cp.ExpectString("Initial version with France question")
	if err != nil {
		t.Fatalf("Version notes not found: %v", err)
	}

	// Test preview
	cp.SendLine("preview")
	err = cp.ExpectString("Title: test-prompt")
	if err != nil {
		t.Fatalf("Preview title failed: %v", err)
	}

	// Test export
	cp.SendLine("export")
	err = cp.ExpectString("Prompt export:")
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	// Test list command
	cp.SendLine("list")
	err = cp.ExpectString("test-prompt")
	if err != nil {
		t.Fatalf("List prompts failed: %v", err)
	}

	cp.SendLine("exit")
	err = cp.ExpectString("Exiting LLM Prompt Builder mode")
	if err != nil {
		t.Fatalf("LLM mode exit failed: %v", err)
	}

	cp.ExpectEOF()
}
