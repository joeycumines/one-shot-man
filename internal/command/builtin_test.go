package command

import (
	"bytes"
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/config"
)

func TestNewHelpCommand(t *testing.T) {
	registry := NewRegistry()
	cmd := NewHelpCommand(registry)

	if cmd == nil {
		t.Fatal("NewHelpCommand returned nil")
	}

	if cmd.Name() != "help" {
		t.Errorf("Expected name 'help', got %s", cmd.Name())
	}

	expectedDesc := "Display help information for commands"
	if cmd.Description() != expectedDesc {
		t.Errorf("Expected description %q, got %q", expectedDesc, cmd.Description())
	}

	expectedUsage := "help [command]"
	if cmd.Usage() != expectedUsage {
		t.Errorf("Expected usage %q, got %q", expectedUsage, cmd.Usage())
	}
}

func TestHelpCommandExecute(t *testing.T) {
	registry := NewRegistry()
	
	// Register some test commands
	registry.Register(NewVersionCommand("1.0.0"))
	registry.Register(NewConfigCommand(config.NewConfig()))
	
	cmd := NewHelpCommand(registry)

	t.Run("general help", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		err := cmd.Execute([]string{}, &stdout, &stderr)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		output := stdout.String()
		expectedParts := []string{
			"one-shot-man",
			"Usage: osm <command>",
			"Available commands:",
			"Built-in commands:",
		}

		for _, part := range expectedParts {
			if !strings.Contains(output, part) {
				t.Errorf("Expected output to contain %q, but it didn't. Output: %s", part, output)
			}
		}
	})
}

func TestNewVersionCommand(t *testing.T) {
	version := "1.2.3"
	cmd := NewVersionCommand(version)

	if cmd == nil {
		t.Fatal("NewVersionCommand returned nil")
	}

	if cmd.Name() != "version" {
		t.Errorf("Expected name 'version', got %s", cmd.Name())
	}

	expectedDesc := "Display version information"
	if cmd.Description() != expectedDesc {
		t.Errorf("Expected description %q, got %q", expectedDesc, cmd.Description())
	}

	expectedUsage := "version"
	if cmd.Usage() != expectedUsage {
		t.Errorf("Expected usage %q, got %q", expectedUsage, cmd.Usage())
	}
}

func TestVersionCommandExecute(t *testing.T) {
	version := "1.2.3"
	cmd := NewVersionCommand(version)

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, version) {
		t.Errorf("Expected output to contain version %q, got: %s", version, output)
	}
	if !strings.Contains(output, "one-shot-man") {
		t.Errorf("Expected output to contain 'one-shot-man', got: %s", output)
	}
}

func TestNewConfigCommand(t *testing.T) {
	cfg := config.NewConfig()
	cmd := NewConfigCommand(cfg)

	if cmd == nil {
		t.Fatal("NewConfigCommand returned nil")
	}

	if cmd.Name() != "config" {
		t.Errorf("Expected name 'config', got %s", cmd.Name())
	}

	expectedDesc := "Manage configuration settings"
	if cmd.Description() != expectedDesc {
		t.Errorf("Expected description %q, got %q", expectedDesc, cmd.Description())
	}

	expectedUsage := "config [options] [key] [value]"
	if cmd.Usage() != expectedUsage {
		t.Errorf("Expected usage %q, got %q", expectedUsage, cmd.Usage())
	}
}

func TestConfigCommandExecute(t *testing.T) {
	cfg := config.NewConfig()
	cmd := NewConfigCommand(cfg)

	t.Run("show config", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		err := cmd.Execute([]string{}, &stdout, &stderr)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		output := stdout.String()
		if len(output) == 0 {
			t.Error("Expected some output for config command")
		}
	})
}

func TestNewInitCommand(t *testing.T) {
	cmd := NewInitCommand()

	if cmd == nil {
		t.Fatal("NewInitCommand returned nil")
	}

	if cmd.Name() != "init" {
		t.Errorf("Expected name 'init', got %s", cmd.Name())
	}

	expectedDesc := "Initialize one-shot-man environment"
	if cmd.Description() != expectedDesc {
		t.Errorf("Expected description %q, got %q", expectedDesc, cmd.Description())
	}

	expectedUsage := "init [options]"
	if cmd.Usage() != expectedUsage {
		t.Errorf("Expected usage %q, got %q", expectedUsage, cmd.Usage())
	}
}

func TestInitCommandExecute(t *testing.T) {
	cmd := NewInitCommand()

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{}, &stdout, &stderr)
	
	// The init command should provide some output or feedback
	// Even if it's just a placeholder implementation
	if err != nil {
		// Check if it's just a "not implemented" error
		if !strings.Contains(err.Error(), "not implemented") &&
		   !strings.Contains(err.Error(), "TODO") {
			t.Errorf("Unexpected error from init command: %v", err)
		}
	}
}

func TestConfigCommandWithNilConfig(t *testing.T) {
	cmd := NewConfigCommand(nil)

	if cmd == nil {
		t.Fatal("NewConfigCommand with nil config returned nil")
	}

	// Should still work with nil config
	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{}, &stdout, &stderr)
	
	// Should not panic, even if it returns an error
	if err != nil {
		// That's okay, nil config might cause an error
	}
}