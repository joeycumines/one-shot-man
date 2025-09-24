package main

import (
	"os"
	"strings"
	"testing"
)

func TestMain(t *testing.T) {
	// Test that main doesn't panic with valid input
	// We'll capture os.Exit calls by temporarily overriding them
	
	// This is a basic smoke test for the main function
	// We can't easily test main() directly since it calls os.Exit,
	// but we can test the run() function
}

func TestRun(t *testing.T) {
	// Save original args and restore after test
	originalArgs := os.Args
	defer func() { os.Args = originalArgs }()

	t.Run("help command", func(t *testing.T) {
		os.Args = []string{"osm", "help"}
		err := run()
		if err != nil {
			t.Errorf("Expected no error for help command, got: %v", err)
		}
	})

	t.Run("version command", func(t *testing.T) {
		os.Args = []string{"osm", "version"}
		err := run()
		if err != nil {
			t.Errorf("Expected no error for version command, got: %v", err)
		}
	})

	t.Run("no command shows help", func(t *testing.T) {
		os.Args = []string{"osm"}
		err := run()
		// Should not error when showing help
		if err != nil {
			t.Errorf("Expected no error when no command specified, got: %v", err)
		}
	})

	t.Run("help flag", func(t *testing.T) {
		os.Args = []string{"osm", "--help"}
		err := run()
		if err != nil {
			t.Errorf("Expected no error for --help flag, got: %v", err)
		}
	})

	t.Run("short help flag", func(t *testing.T) {
		os.Args = []string{"osm", "-h"}
		err := run()
		if err != nil {
			t.Errorf("Expected no error for -h flag, got: %v", err)
		}
	})

	t.Run("unknown command", func(t *testing.T) {
		os.Args = []string{"osm", "nonexistent"}
		err := run()
		if err == nil {
			t.Error("Expected error for unknown command")
		}
	})
}

func TestRunWithConfigError(t *testing.T) {
	// Save and restore environment
	configPath := os.Getenv("XDG_CONFIG_HOME")
	defer func() {
		if configPath == "" {
			os.Unsetenv("XDG_CONFIG_HOME")
		} else {
			os.Setenv("XDG_CONFIG_HOME", configPath)
		}
	}()

	// Save original args and restore after test
	originalArgs := os.Args
	defer func() { os.Args = originalArgs }()

	// Set an invalid config path that will cause permission issues
	os.Setenv("XDG_CONFIG_HOME", "/root/nonexistent")
	os.Args = []string{"osm", "help"}
	
	// Should not error even with config load failure (falls back to default config)
	err := run()
	if err != nil {
		t.Errorf("Expected no error even with config failure, got: %v", err)
	}
}

func TestRunRegistersAllCommands(t *testing.T) {
	// This is more of an integration test to ensure all commands are registered
	originalArgs := os.Args
	defer func() { os.Args = originalArgs }()

	// Test that built-in commands can be accessed (but not run, due to os.Exit issues)
	expectedCommands := []string{"help", "version", "config", "init", "script", "prompt-flow", "code-review"}
	
	// Instead of testing --help (which causes os.Exit), just test that commands exist
	for _, cmdName := range expectedCommands {
		t.Run("command_exists_"+cmdName, func(t *testing.T) {
			os.Args = []string{"osm", cmdName}
			
			// Try to run and expect it to fail with flag parsing error or command-specific error,
			// but not "unknown command" error
			err := run()
			if err != nil {
				errStr := err.Error()
				if strings.Contains(errStr, "Unknown command") {
					t.Errorf("Command %s should be registered but got 'Unknown command' error", cmdName)
				}
				// Other errors are expected (missing flags, etc.)
			}
		})
	}
}