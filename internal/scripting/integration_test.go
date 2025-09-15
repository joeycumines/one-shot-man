package scripting

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ActiveState/termtest"
)

func requireExpect(t *testing.T, p *termtest.ConsoleProcess, value string, timeout ...time.Duration) {
	t.Helper()
	rawString, err := p.Expect(value, timeout...)
	if err != nil {
		t.Fatalf("Expected to find %q in output, but got error: %v\nRaw:\n%s\n", value, err, rawString)
	}
}

func requireExpectExitCode(t *testing.T, p *termtest.ConsoleProcess, exitCode int, timeout ...time.Duration) {
	t.Helper()
	rawString, err := p.ExpectExitCode(exitCode, timeout...)
	if err != nil {
		t.Fatalf("Expected exit code %d, but got error: %v\nRaw:\n%s\n", exitCode, err, rawString)
	}
}

// TestFullLLMWorkflow tests a complete LLM prompt building workflow
func TestFullLLMWorkflow(t *testing.T) {
	binaryPath := buildTestBinary(t)
	defer os.Remove(binaryPath)

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}
	projectDir := filepath.Clean(filepath.Join(wd, "..", ".."))

	opts := termtest.Options{
		CmdName:        binaryPath,
		Args:           []string{"script", "-i", filepath.Join(projectDir, "scripts", "llm-prompt-builder.js")},
		DefaultTimeout: 5 * time.Second,
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
	requireExpect(t, cp, "Welcome to LLM Prompt Builder!")
	requireExpect(t, cp, "Switched to mode: llm-prompt-builder")

	// Complete workflow: Create prompt, refine it, save versions, export
	testCompletePromptWorkflow(t, cp)
}

func testCompletePromptWorkflow(t *testing.T, cp *termtest.ConsoleProcess) {
	// Create a customer service prompt
	cp.SendLine("new customer-service A customer service assistant prompt")
	requireExpect(t, cp, "Created new prompt: customer-service")

	// Set initial template
	cp.SendLine("template You are a {{role}} for {{company}}. You should be {{tone}} and {{helpful_level}}. Customer issue: {{issue}}")
	requireExpect(t, cp, "Template set:")

	// Set variables for first version
	cp.SendLine("var role customer service representative")
	cp.SendLine("var company TechCorp Inc")
	cp.SendLine("var tone professional and friendly")
	cp.SendLine("var helpful_level extremely helpful")
	cp.SendLine("var issue I can't log into my account")

	// Build and preview
	cp.SendLine("build")
	requireExpect(t, cp, "You are a customer service representative for TechCorp Inc.")
	requireExpect(t, cp, "I can't log into my account")

	// Save first version
	cp.SendLine("save Initial customer service template")
	requireExpect(t, cp, "Saved version 1")

	// Refine the prompt - make it more specific
	cp.SendLine("template You are a {{role}} for {{company}}. You should be {{tone}} and {{helpful_level}}. When handling customer issues, always:\n1. Acknowledge the customer's concern\n2. Ask clarifying questions if needed\n3. Provide step-by-step solutions\n4. Offer additional assistance\n\nCustomer issue: {{issue}}")
	requireExpect(t, cp, "Template set:")

	// Build the refined version
	cp.SendLine("build")
	requireExpect(t, cp, "Built prompt:")
	requireExpect(t, cp, "1. Acknowledge the customer's concern")
	requireExpect(t, cp, "2. Ask clarifying questions")

	// Save refined version
	cp.SendLine("save Added structured response format")
	requireExpect(t, cp, "Saved version 2")

	// Test different issue type
	cp.SendLine("var issue My order hasn't arrived and it's been a week")
	cp.SendLine("build")
	requireExpect(t, cp, "My order hasn't arrived and it's been a week")

	// Save version for different issue
	cp.SendLine("save Shipping issue variant")
	requireExpect(t, cp, "Saved version 3")

	// List all versions
	cp.SendLine("versions")
	requireExpect(t, cp, "v1 -")
	requireExpect(t, cp, "Initial customer service template")
	requireExpect(t, cp, "v2 -")
	requireExpect(t, cp, "Added structured response format")
	requireExpect(t, cp, "v3 -")
	requireExpect(t, cp, "Shipping issue variant")

	// Test restoration
	cp.SendLine("restore 1")
	requireExpect(t, cp, "Restored to version 1")

	cp.SendLine("build")

	// Export the prompt data
	cp.SendLine("export")
	requireExpect(t, cp, "\"title\": \"customer-service\"")
	requireExpect(t, cp, "\"versions\": 3")

	// Create a second prompt to test multi-prompt management
	cp.SendLine("new technical-support Technical support prompt")
	requireExpect(t, cp, "Created new prompt: technical-support")

	// List all prompts
	cp.SendLine("list")
	requireExpect(t, cp, "customer-service")
	requireExpect(t, cp, "technical-support")

	// Switch back to customer service
	cp.SendLine("load customer-service")
	requireExpect(t, cp, "Loaded prompt: customer-service")

	cp.SendLine("exit")
	requireExpectExitCode(t, cp, 0)
}

func TestMultiModeWorkflow(t *testing.T) {
	// Create a script that registers multiple modes
	multiModeScript := `
// Multi-mode test script
ctx.log("Registering multiple modes...");

// Register a simple calculator mode
tui.registerMode({
    name: "calculator",
    tui: {
        title: "Simple Calculator",
        prompt: "[calc]> "
    },
    onEnter: function() {
        output.print("Calculator mode active");
        if (tui.getState("result") === undefined) {
            tui.setState("result", 0);
        }
    },
    commands: {
        "add": {
            description: "Add numbers",
            usage: "add <num1> <num2>",
            handler: function(args) {
                if (args.length !== 2) {
                    output.print("Usage: add <num1> <num2>");
                    return;
                }
                var result = parseFloat(args[0]) + parseFloat(args[1]);
                tui.setState("result", result);
                output.print("Result: " + result);
            }
        },
        "result": {
            description: "Show current result",
            handler: function(args) {
                output.print("Current result: " + tui.getState("result"));
            }
        }
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
        if (!tui.getState("notes")) {
            tui.setState("notes", []);
        }
    },
    commands: {
        "add": {
            description: "Add a note",
            usage: "add <note text>",
            handler: function(args) {
                var note = args.join(" ");
                var notes = tui.getState("notes") || [];
                notes.push(note);
                tui.setState("notes", notes);
                output.print("Added note: " + note);
            }
        },
        "list": {
            description: "List all notes",
            handler: function(args) {
                var notes = tui.getState("notes") || [];
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
	defer os.Remove(binaryPath)

	opts := termtest.Options{
		CmdName:        binaryPath,
		Args:           []string{"script", "-i", scriptPath},
		DefaultTimeout: 5 * time.Second,
	}

	cp, err := termtest.NewTest(t, opts)
	if err != nil {
		t.Fatalf("Failed to create termtest: %v", err)
	}
	defer cp.Close()

	// Wait for startup
	requireExpect(t, cp, "one-shot-man Rich TUI Terminal", 10*time.Second)

	// Test calculator mode
	cp.SendLine("mode calculator")
	requireExpect(t, cp, "Calculator mode active")

	cp.SendLine("add 5 3")
	requireExpect(t, cp, "Result: 8")

	cp.SendLine("add 2 7")
	requireExpect(t, cp, "Result: 9")

	cp.SendLine("result")
	requireExpect(t, cp, "Current result: 9")

	// Switch to notes mode
	cp.SendLine("mode notes")
	requireExpect(t, cp, "Note-taking mode active")

	cp.SendLine("add This is my first note")
	requireExpect(t, cp, "Added note: This is my first note")

	cp.SendLine("add Another important note")
	requireExpect(t, cp, "Added note: Another important note")

	cp.SendLine("list")
	requireExpect(t, cp, "Notes:")

	// Switch back to calculator
	cp.SendLine("mode calculator")
	requireExpect(t, cp, "Calculator mode active")
	requireExpect(t, cp, "Switched to mode: calculator")

	// Result should persist across mode switches
	cp.SendLine("result")
	requireExpect(t, cp, "Current result: 9")

	// Switch back to notes
	cp.SendLine("mode notes")
	requireExpect(t, cp, "Note-taking mode active")

	// Notes should still be there
	cp.SendLine("list")
	requireExpect(t, cp, "Notes:")

	cp.SendLine("exit")
	requireExpect(t, cp, "Goodbye!")
	requireExpectExitCode(t, cp, 0)
}

// TestErrorHandling tests error conditions and edge cases
func TestErrorHandling(t *testing.T) {
	binaryPath := buildTestBinary(t)
	defer os.Remove(binaryPath)

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}
	projectDir := filepath.Clean(filepath.Join(wd, "..", ".."))

	opts := termtest.Options{
		CmdName:        binaryPath,
		Args:           []string{"script", "-i", filepath.Join(projectDir, "scripts", "demo-mode.js")},
		DefaultTimeout: 10 * time.Second,
	}

	cp, err := termtest.NewTest(t, opts)
	if err != nil {
		t.Fatalf("Failed to create termtest: %v", err)
	}
	defer cp.Close()

	// Wait for startup
	requireExpect(t, cp, "Rich TUI Terminal")

	// Test switching to non-existent mode
	cp.SendLine("mode nonexistent")
	requireExpect(t, cp, "mode nonexistent not found")

	// Test unknown command
	cp.SendLine("unknowncommand")
	requireExpect(t, cp, "Command not found: unknowncommand")

	// Switch to demo mode
	cp.SendLine("mode demo")
	requireExpect(t, cp, "Entered demo mode!")
	requireExpect(t, cp, "Switched to mode: demo")

	// Test command with wrong usage
	cp.SendLine("js")
	requireExpect(t, cp, "Usage: js <code>")

	// Test JavaScript syntax error
	cp.SendLine("js this is not valid javascript syntax +++")
	requireExpect(t, cp, "Error:")

	cp.SendLine("exit")
	requireExpectExitCode(t, cp, 0)
}

// TestConcurrentAccess tests the thread safety of the TUI system
func TestConcurrentAccess(t *testing.T) {
	ctx := context.Background()
	engine := NewEngine(ctx, os.Stdout, os.Stderr)
	defer engine.Close()

	tuiManager := engine.GetTUIManager()

	// Register a test mode
	script := engine.LoadScriptFromString("concurrent-test", `
		tui.registerMode({
			name: "concurrent-test",
			commands: {}
		});
	`)

	err := engine.ExecuteScript(script)
	if err != nil {
		t.Fatalf("Script execution failed: %v", err)
	}

	// Test concurrent state access
	done := make(chan bool, 10)

	// Start multiple goroutines accessing state
	for i := 0; i < 10; i++ {
		go func(id int) {
			defer func() { done <- true }()

			// Switch to the mode
			err := tuiManager.SwitchMode("concurrent-test")
			if err != nil {
				t.Errorf("Mode switching failed in goroutine %d: %v", id, err)
				return
			}

			// Set and get state values
			for j := 0; j < 100; j++ {
				key := fmt.Sprintf("key-%d-%d", id, j)
				value := fmt.Sprintf("value-%d-%d", id, j)

				tuiManager.SetState(key, value)
				retrieved := tuiManager.GetState(key)

				if retrieved != value {
					t.Errorf("State mismatch in goroutine %d: expected %s, got %v", id, value, retrieved)
					return
				}
			}
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		select {
		case <-done:
			// Goroutine completed
		case <-time.After(10 * time.Second):
			t.Fatal("Concurrent access test timed out")
		}
	}
}

// TestJavaScriptInteroperability tests complex JavaScript integration
func TestJavaScriptInteroperability(t *testing.T) {
	ctx := context.Background()
	engine := NewEngine(ctx, os.Stdout, os.Stderr)
	defer engine.Close()

	// Test complex JavaScript integration with the TUI system
	script := engine.LoadScriptFromString("interop-test", `
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
				tui.setState("config", {
					nested: {
						value: 42,
						array: [1, 2, 3],
						func: function() { return "test"; }
					}
				});
			},
			commands: {
				"test-object": {
					description: "Test object handling",
					handler: function(args) {
						var config = tui.getState("config");
						output.print("Nested value: " + config.nested.value);
						output.print("Array length: " + config.nested.array.length);
					}
				},
				"test-array": {
					description: "Test array operations",
					handler: function(args) {
						var arr = tui.getState("testArray") || [];
						arr.push(args.join(" "));
						tui.setState("testArray", arr);
						output.print("Array now has " + arr.length + " items");
					}
				}
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

	// Test that complex state was set
	config := tuiManager.GetState("config")
	if config == nil {
		t.Fatal("Complex config state not set")
	}
}

// BenchmarkTUIPerformance benchmarks the TUI system performance
func BenchmarkTUIPerformance(b *testing.B) {
	ctx := context.Background()
	engine := NewEngine(ctx, os.Stdout, os.Stderr)
	defer engine.Close()

	// Register a test mode
	script := engine.LoadScriptFromString("perf-test", `
		tui.registerMode({
			name: "perf-test",
			commands: {
				"perf": {
					description: "Performance test command",
					handler: function(args) {
						tui.setState("counter", (tui.getState("counter") || 0) + 1);
					}
				}
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

	// Benchmark state operations
	b.Run("StateOperations", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			key := fmt.Sprintf("key-%d", i)
			value := fmt.Sprintf("value-%d", i)
			tuiManager.SetState(key, value)
			retrieved := tuiManager.GetState(key)
			if retrieved != value {
				b.Fatalf("State operation failed")
			}
		}
	})
}

// TestContextManagement tests the context manager functionality
func TestContextManagement(t *testing.T) {
	ctx := context.Background()
	engine := NewEngine(ctx, os.Stdout, os.Stderr)
	defer engine.Close()

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
	engine := NewEngine(ctx, &logOutput, &logOutput)
	defer engine.Close()

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
	engine := NewEngine(ctx, os.Stdout, os.Stderr)
	defer engine.Close()

	// Test mode registration and switching
	testScript := engine.LoadScriptFromString("tui-mode-test", `
		// Register a test mode
		tui.registerMode({
			name: "test-mode",
			tui: {
				title: "Test Mode",
				prompt: "[test]> "
			},
			onEnter: function() {
				log.info("Entered test mode");
				tui.setState("testValue", "initialized");
			},
			onExit: function() {
				log.info("Exited test mode");
			},
			commands: {
				"test-cmd": {
					description: "A test command",
					handler: function(args) {
						tui.setState("lastCommand", args.join(" "));
						output.print("Test command executed with: " + args.join(" "));
					}
				},
				"get-state": {
					description: "Get test state",
					handler: function(args) {
						var value = tui.getState("testValue");
						output.print("Test value: " + value);
					}
				}
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

	// Verify state was set
	state := tuiManager.GetState("lastCommand")
	if state != "arg1 arg2" {
		t.Fatalf("Expected 'arg1 arg2', got %v", state)
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
	engine := NewEngine(ctx, &output, &output)
	defer engine.Close()

	// Integration test script
	testScript := engine.LoadScriptFromString("full-integration", fmt.Sprintf(`
		// Full integration test combining all systems
		log.info("Starting full integration test");

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

				// Initialize state
				tui.setState("filesProcessed", 0);
			},
			commands: {
				"process-files": {
					description: "Process files in context",
					handler: function(args) {
						var goFiles = context.getFilesByExt("go");
						var txtFiles = context.getFilesByExt("txt");

						log.info("Found " + goFiles.length + " Go files");
						log.info("Found " + txtFiles.length + " text files");

						var processed = tui.getState("filesProcessed") + goFiles.length + txtFiles.length;
						tui.setState("filesProcessed", processed);

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
						var processed = tui.getState("filesProcessed");

						output.print("Context: " + contextStats.files + " files, " + contextStats.totalSize + " bytes");
						output.print("Processed: " + processed + " files");

						var logs = log.getLogs(5);
						output.print("Recent logs: " + logs.length + " entries");
					}
				}
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
	engine := NewEngine(ctx, &output, &output)
	defer engine.Close()

	// Test script with various command types
	testScript := engine.LoadScriptFromString("command-verification", `
		// Register mode with comprehensive command testing
		tui.registerMode({
			name: "command-test",
			tui: {
				title: "Command Test Mode",
				prompt: "[cmd-test]> "
			},
			onEnter: function() {
				tui.setState("commandResults", []);
				log.info("Command test mode initialized");
			},
			commands: {
				"simple": {
					description: "Simple command test",
					handler: function(args) {
						var results = tui.getState("commandResults");
						results.push("simple:" + args.join(","));
						tui.setState("commandResults", results);
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
						var state = tui.getState("complexState") || {};

						switch (action) {
							case "set":
								if (!value) {
									output.print("Usage: complex set <value>");
									return;
								}
								state[value] = new Date().getTime();
								break;
							case "get":
								if (!value) {
									output.print("Usage: complex get <value>");
									return;
								}
								if (state[value]) {
									output.print("Value " + value + " set at: " + state[value]);
								} else {
									output.print("Value " + value + " not found");
								}
								return;
							case "list":
								var keys = Object.keys(state);
								output.print("Keys: " + keys.join(", "));
								return;
						}

						tui.setState("complexState", state);
						output.print("Complex command " + action + " completed");
					}
				},
				"verify": {
					description: "Verify all commands worked",
					handler: function(args) {
						var results = tui.getState("commandResults");
						var complexState = tui.getState("complexState") || {};

						output.print("Command results: " + results.length + " simple commands executed");
						output.print("Complex state keys: " + Object.keys(complexState).length);

						// Log verification
						log.info("Command verification completed");
						var logs = log.getLogs();
						output.print("Total logs: " + logs.length);
					}
				}
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
	engine := NewEngine(ctx, &output, &output)
	defer engine.Close()

	// Test state persistence and manipulation
	testScript := engine.LoadScriptFromString("state-verification", `
		// Register mode with comprehensive state testing
		tui.registerMode({
			name: "state-test",
			tui: {
				title: "State Test Mode",
				prompt: "[state-test]> "
			},
			onEnter: function() {
				// Initialize complex state structure
				tui.setState("counter", 0);
				tui.setState("messages", []);
				tui.setState("config", {
					name: "test-config",
					features: ["logging", "state", "commands"],
					settings: {
						debug: true,
						maxItems: 100
					}
				});
				log.info("State test mode initialized with complex state");
			},
			commands: {
				"increment": {
					description: "Increment counter",
					handler: function(args) {
						var counter = tui.getState("counter");
						counter++;
						tui.setState("counter", counter);
						output.print("Counter: " + counter);
					}
				},
				"add-message": {
					description: "Add message to array",
					usage: "add-message <text>",
					handler: function(args) {
						var messages = tui.getState("messages");
						var text = args.join(" ");
						messages.push({
							text: text,
							timestamp: new Date().getTime(),
							id: messages.length + 1
						});
						tui.setState("messages", messages);
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

						var config = tui.getState("config");
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

						tui.setState("config", config);
						output.print("Updated config." + key + " = " + value);
					}
				},
				"show-state": {
					description: "Show all state",
					handler: function(args) {
						var counter = tui.getState("counter");
						var messages = tui.getState("messages");
						var config = tui.getState("config");

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

	// Verify state persistence by directly checking TUI manager
	counter := tuiManager.GetState("counter")
	// JavaScript numbers might be stored as various numeric types
	var counterInt int
	switch v := counter.(type) {
	case int:
		counterInt = v
	case int64:
		counterInt = int(v)
	case float64:
		counterInt = int(v)
	default:
		t.Fatalf("Expected counter to be numeric, got %T: %v", counter, counter)
	}

	if counterInt != 5 {
		t.Fatalf("Expected counter to be 5, got %v", counterInt)
	}

	messagesState := tuiManager.GetState("messages")
	if messagesState == nil {
		t.Fatal("Expected messages state to exist")
	}

	configState := tuiManager.GetState("config")
	if configState == nil {
		t.Fatal("Expected config state to exist")
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
