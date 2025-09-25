package command

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/config"
)

// TestCommand implements Command interface for testing
type TestCommand struct {
	*BaseCommand
}

func NewTestCommand(name, description, usage string) *TestCommand {
	return &TestCommand{
		BaseCommand: NewBaseCommand(name, description, usage),
	}
}

func (c *TestCommand) Execute(args []string, stdout, stderr io.Writer) error {
	return nil
}

func TestRegistry(t *testing.T) {
	cfg := config.NewConfig()
	registry := NewRegistryWithConfig(cfg)

	// Test registering built-in command
	testCmd := NewTestCommand("test", "Test command", "test [options]")
	registry.Register(testCmd)

	// Test getting built-in command
	cmd, err := registry.Get("test")
	if err != nil {
		t.Fatalf("Failed to get registered command: %v", err)
	}

	if cmd.Name() != "test" {
		t.Errorf("Expected command name 'test', got '%s'", cmd.Name())
	}

	// Test getting non-existent command
	_, err = registry.Get("nonexistent")
	if err == nil {
		t.Error("Expected error for non-existent command, got nil")
	}

	// Test listing commands
	commands := registry.ListBuiltin()
	found := false
	for _, name := range commands {
		if name == "test" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected 'test' command in builtin list")
	}
}

func TestScriptPathDuplication(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, "custom-config")
	scriptsDir := filepath.Join(configDir, "scripts")

	if err := os.MkdirAll(scriptsDir, 0755); err != nil {
		t.Fatalf("Failed to create scripts directory: %v", err)
	}

	configPath := filepath.Join(configDir, "config")
	t.Setenv("ONESHOTMAN_CONFIG", configPath)

	cfg := config.NewConfig()
	cfg.SetGlobalOption("script.paths", scriptsDir)

	registry := NewRegistryWithConfig(cfg)

	count := 0
	for _, path := range registry.scriptPaths {
		if path == scriptsDir {
			count++
		}
	}

	if count != 1 {
		t.Errorf("Expected scripts directory %s to be deduplicated, found %d entries", scriptsDir, count)
	}
}

func TestScriptCommand(t *testing.T) {
	// Create temporary script file
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "testscript")

	scriptContent := `#!/bin/bash
echo "Test script output"
`

	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0755); err != nil {
		t.Fatalf("Failed to create test script: %v", err)
	}

	// Test script command creation
	scriptCmd := NewScriptCommand("testscript", scriptPath)

	if scriptCmd.Name() != "testscript" {
		t.Errorf("Expected script name 'testscript', got '%s'", scriptCmd.Name())
	}

	if !strings.Contains(scriptCmd.Description(), "Script command") {
		t.Errorf("Expected description to contain 'Script command', got '%s'", scriptCmd.Description())
	}
}
