package scripting

import (
	"context"
	"os"
	"testing"
)

// TestDocumentedAPIFunctionality tests the core APIs documented in docs/internal-api.md
func TestDocumentedAPIFunctionality(t *testing.T) {
	ctx := context.Background()
	engine := NewEngine(ctx, os.Stdout, os.Stderr)
	defer engine.Close()

	t.Run("GlobalObjects", func(t *testing.T) {
		// Test that all documented global objects are available
		script := engine.LoadScriptFromString("global-objects-test", `
			// Test ctx object
			if (typeof ctx === 'undefined') {
				throw new Error("ctx object not available");
			}
			if (typeof ctx.log !== 'function') {
				throw new Error("ctx.log not available");
			}
			if (typeof ctx.defer !== 'function') {
				throw new Error("ctx.defer not available");
			}
			
			// Test tui object
			if (typeof tui === 'undefined') {
				throw new Error("tui object not available");
			}
			if (typeof tui.registerMode !== 'function') {
				throw new Error("tui.registerMode not available");
			}
			if (typeof tui.registerCommand !== 'function') {
				throw new Error("tui.registerCommand not available");
			}
			
			// Test output object
			if (typeof output === 'undefined') {
				throw new Error("output object not available");
			}
			if (typeof output.print !== 'function') {
				throw new Error("output.print not available");
			}
			
			// Test log object (not console)
			if (typeof log === 'undefined') {
				throw new Error("log object not available");
			}
			if (typeof log.info !== 'function') {
				throw new Error("log.info not available");
			}
			
			ctx.log("All documented global objects are available");
		`)

		err := engine.ExecuteScript(script)
		if err != nil {
			t.Fatalf("Global objects test failed: %v", err)
		}
	})

	t.Run("NativeModules", func(t *testing.T) {
		// Test that all documented osm: modules are available
		script := engine.LoadScriptFromString("native-modules-test", `
			// Test osm:argv
			try {
				const argv = require('osm:argv');
				if (typeof argv.parseArgv !== 'function') {
					throw new Error("argv.parseArgv not available");
				}
				if (typeof argv.formatArgv !== 'function') {
					throw new Error("argv.formatArgv not available");
				}
				ctx.log("✓ osm:argv module working");
			} catch (e) {
				throw new Error("osm:argv module failed: " + e.message);
			}

			// Test osm:os
			try {
				const os = require('osm:os');
				if (typeof os.readFile !== 'function') {
					throw new Error("os.readFile not available");
				}
				if (typeof os.fileExists !== 'function') {
					throw new Error("os.fileExists not available");
				}
				if (typeof os.getenv !== 'function') {
					throw new Error("os.getenv not available");
				}
				ctx.log("✓ osm:os module working");
			} catch (e) {
				throw new Error("osm:os module failed: " + e.message);
			}

			// Test osm:exec
			try {
				const exec = require('osm:exec');
				if (typeof exec.exec !== 'function') {
					throw new Error("exec.exec not available");
				}
				if (typeof exec.execv !== 'function') {
					throw new Error("exec.execv not available");
				}
				ctx.log("✓ osm:exec module working");
			} catch (e) {
				throw new Error("osm:exec module failed: " + e.message);
			}

			// Test osm:time
			try {
				const time = require('osm:time');
				if (typeof time.sleep !== 'function') {
					throw new Error("time.sleep not available");
				}
				ctx.log("✓ osm:time module working");
			} catch (e) {
				throw new Error("osm:time module failed: " + e.message);
			}

			// Test osm:ctxutil
			try {
				const ctxutil = require('osm:ctxutil');
				if (typeof ctxutil.buildContext !== 'function') {
					throw new Error("ctxutil.buildContext not available");
				}
				ctx.log("✓ osm:ctxutil module working");
			} catch (e) {
				throw new Error("osm:ctxutil module failed: " + e.message);
			}

			// Test osm:nextIntegerId
			try {
				const nextId = require('osm:nextIntegerId');
				if (typeof nextId !== 'function') {
					throw new Error("nextIntegerId not available");
				}
				
				// Test with empty array
				var id1 = nextId([]);
				if (id1 !== 1) {
					throw new Error("nextIntegerId should return 1 for empty array, got: " + id1);
				}
				
				// Test with existing items
				var items = [{ id: 1 }, { id: 3 }];
				var id2 = nextId(items);
				if (id2 !== 4) {
					throw new Error("nextIntegerId should return 4 for max id 3, got: " + id2);
				}
				
				ctx.log("✓ osm:nextIntegerId module working");
			} catch (e) {
				throw new Error("osm:nextIntegerId module failed: " + e.message);
			}
		`)

		err := engine.ExecuteScript(script)
		if err != nil {
			t.Fatalf("Native modules test failed: %v", err)
		}
	})

	t.Run("TUIAPIs", func(t *testing.T) {
		// Test core TUI API functions
		script := engine.LoadScriptFromString("tui-apis-test", `
			// Test mode registration with full config
			tui.registerMode({
				name: "api-test-mode",
				tui: {
					title: "API Test Mode",
					prompt: "[api-test]> ",
					enableHistory: true,
					historyFile: ".api-test-history"
				},
				onEnter: function() {
					ctx.log("Entered API test mode");
				},
				onExit: function() {
					ctx.log("Exiting API test mode");
				},
				commands: {
					"test-cmd": {
						description: "A test command",
						usage: "test-cmd <arg>",
						handler: function(args) {
							ctx.log("Test command executed with args: " + args.join(" "));
						}
					}
				}
			});

			// Test mode listing
			var modes = tui.listModes();
			if (modes.indexOf("api-test-mode") === -1) {
				throw new Error("Registered mode not found in list");
			}
			ctx.log("Mode successfully registered and listed");

			// Test state management
			tui.switchMode("api-test-mode");
			tui.setState("testKey", "testValue");
			tui.setState("testObject", { foo: "bar", count: 42 });
			
			var stringValue = tui.getState("testKey");
			if (stringValue !== "testValue") {
				throw new Error("String state not preserved: " + stringValue);
			}
			
			var objectValue = tui.getState("testObject");
			if (!objectValue || objectValue.foo !== "bar" || objectValue.count !== 42) {
				throw new Error("Object state not preserved");
			}
			ctx.log("State management working correctly");

			// Test global command registration
			tui.registerCommand({
				name: "api-global-test",
				description: "Global API test command",
				usage: "api-global-test [args...]",
				handler: function(args) {
					ctx.log("Global test command executed");
				}
			});
			ctx.log("Global command registration working");
		`)

		err := engine.ExecuteScript(script)
		if err != nil {
			t.Fatalf("TUI APIs test failed: %v", err)
		}

		// Verify mode was actually registered
		tuiManager := engine.GetTUIManager()
		modes := tuiManager.ListModes()
		found := false
		for _, mode := range modes {
			if mode == "api-test-mode" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Mode 'api-test-mode' not found in registered modes: %v", modes)
		}
	})

	t.Run("ContextAPI", func(t *testing.T) {
		// Test context API with deferred execution and sub-contexts
		script := engine.LoadScriptFromString("context-api-test", `
			var executed = [];

			// Test basic logging
			ctx.log("Testing context API");

			// Test deferred execution
			ctx.defer(function() {
				executed.push("main-defer");
				ctx.log("Main deferred function executed");
			});

			// Test sub-contexts
			ctx.run("sub-test-1", function() {
				ctx.log("In sub-test-1");
				ctx.defer(function() {
					executed.push("sub-1-defer");
					ctx.log("Sub-test-1 deferred function executed");
				});
			});

			ctx.run("sub-test-2", function() {
				ctx.log("In sub-test-2");
				ctx.defer(function() {
					executed.push("sub-2-defer");
					ctx.log("Sub-test-2 deferred function executed");
				});
			});

			ctx.log("Context API test completed successfully");
		`)

		err := engine.ExecuteScript(script)
		if err != nil {
			t.Fatalf("Context API test failed: %v", err)
		}
	})

	t.Run("OutputAPI", func(t *testing.T) {
		// Test output and log APIs
		script := engine.LoadScriptFromString("output-api-test", `
			// Test output functions
			output.print("Test output.print message");
			output.printf("Test output.printf: %s = %d", "value", 123);

			// Test log functions
			log.info("Test log.info message");
			log.printf("Test log.printf: %s", "formatted");

			// Test log management
			var logsBefore = log.getLogs();
			log.info("New log message");
			var logsAfter = log.getLogs();
			
			if (logsAfter.length <= logsBefore.length) {
				throw new Error("log.info not adding to logs");
			}

			// Test log search
			var searchResults = log.searchLogs("New log message");
			if (searchResults.length === 0) {
				throw new Error("Log search not working");
			}

			ctx.log("Output and log APIs working correctly");
		`)

		err := engine.ExecuteScript(script)
		if err != nil {
			t.Fatalf("Output API test failed: %v", err)
		}
	})
}

// TestAPIExamples tests that the examples in the documentation actually work
func TestAPIExamples(t *testing.T) {
	ctx := context.Background()
	engine := NewEngine(ctx, os.Stdout, os.Stderr)
	defer engine.Close()

	t.Run("BasicModeExample", func(t *testing.T) {
		// Test the basic mode example from the documentation
		script := engine.LoadScriptFromString("basic-mode-example", `
			tui.registerMode({
				name: "my-mode",
				tui: {
					title: "My Mode",
					prompt: "[my]> ",
					enableHistory: true
				},
				onEnter: function() {
					output.print("Welcome to my mode!");
					tui.setState("counter", 0);
				},
				onExit: function() {
					output.print("Goodbye from my mode!");
				},
				commands: {
					"hello": {
						description: "Say hello",
						usage: "hello [name]",
						handler: function(args) {
							var name = args.length > 0 ? args[0] : "World";
							output.print("Hello, " + name + "!");
						}
					}
				}
			});
			ctx.log("Basic mode example works");
		`)

		err := engine.ExecuteScript(script)
		if err != nil {
			t.Fatalf("Basic mode example failed: %v", err)
		}
	})

	t.Run("NativeModuleExample", func(t *testing.T) {
		// Test native module examples from the documentation
		script := engine.LoadScriptFromString("native-module-example", `
			// Test osm:argv example
			const argv = require('osm:argv');
			var args = argv.parseArgv('git commit -m "Initial commit"');
			if (args.length !== 4 || args[0] !== 'git' || args[3] !== 'Initial commit') {
				throw new Error("parseArgv example failed");
			}
			var formatted = argv.formatArgv(["git", "commit", "-m", "Initial commit"]);
			if (!formatted.includes('git') || !formatted.includes('"Initial commit"')) {
				throw new Error("formatArgv example failed");
			}

			// Test osm:exec example
			const exec = require('osm:exec');
			var result = exec.exec('echo', 'hello');
			if (result.error || !result.stdout.includes('hello')) {
				throw new Error("exec example failed: " + result.message);
			}

			// Test osm:time example
			const time = require('osm:time');
			var start = Date.now();
			time.sleep(10);
			var elapsed = Date.now() - start;
			if (elapsed < 8) { // Allow some tolerance
				throw new Error("sleep example failed - too short: " + elapsed);
			}

			ctx.log("Native module examples work correctly");
		`)

		err := engine.ExecuteScript(script)
		if err != nil {
			t.Fatalf("Native module example failed: %v", err)
		}
	})
}

// TestAPICompleteness ensures all documented APIs are actually implemented
func TestAPICompleteness(t *testing.T) {
	ctx := context.Background()
	engine := NewEngine(ctx, os.Stdout, os.Stderr)
	defer engine.Close()

	// Define all APIs that should be available based on documentation
	requiredAPIs := map[string][]string{
		"ctx": {
			"log", "logf", "defer", "run",
		},
		"tui": {
			"registerMode", "switchMode", "getCurrentMode", "listModes",
			"setState", "getState", "registerCommand", "createAdvancedPrompt",
			"runPrompt", "registerCompleter", "setCompleter", "registerKeyBinding",
		},
		"output": {
			"print", "printf",
		},
		"log": {
			"info", "debug", "warn", "error", "printf", "getLogs", "clearLogs", "searchLogs",
		},
	}

	for objName, methods := range requiredAPIs {
		t.Run(objName, func(t *testing.T) {
			for _, method := range methods {
				script := engine.LoadScriptFromString("api-check-"+objName+"-"+method, `
					if (typeof `+objName+` === 'undefined') {
						throw new Error("`+objName+` object not available");
					}
					if (typeof `+objName+`.`+method+` !== 'function') {
						throw new Error("`+objName+`.`+method+` not available or not a function");
					}
				`)
				err := engine.ExecuteScript(script)
				if err != nil {
					t.Errorf("API %s.%s not available: %v", objName, method, err)
				}
			}
		})
	}

	// Test that all osm: modules from documentation are available
	requiredModules := []string{
		"osm:argv", "osm:os", "osm:exec", "osm:time", "osm:ctxutil", "osm:nextIntegerId",
	}

	for _, module := range requiredModules {
		t.Run("module-"+module, func(t *testing.T) {
			script := engine.LoadScriptFromString("module-check-"+module, `
				try {
					var mod = require('`+module+`');
					if (typeof mod === 'undefined') {
						throw new Error("Module `+module+` is undefined");
					}
				} catch (e) {
					throw new Error("Failed to load module `+module+`: " + e.message);
				}
			`)
			err := engine.ExecuteScript(script)
			if err != nil {
				t.Errorf("Module %s not available: %v", module, err)
			}
		})
	}
}