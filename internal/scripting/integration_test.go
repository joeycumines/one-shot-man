package scripting

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

