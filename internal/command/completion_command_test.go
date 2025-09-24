package command

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompletionCommand(t *testing.T) {
	// Create a test registry with some commands
	registry := NewRegistry()
	registry.Register(NewHelpCommand(registry))
	registry.Register(NewVersionCommand("1.0.0"))

	tests := []struct {
		name           string
		args           []string
		expectError    bool
		expectedOutput string
	}{
		{
			name:           "bash completion",
			args:           []string{"bash"},
			expectError:    false,
			expectedOutput: "_osm_completion",
		},
		{
			name:           "zsh completion",
			args:           []string{"zsh"},
			expectError:    false,
			expectedOutput: "#compdef osm",
		},
		{
			name:           "fish completion",
			args:           []string{"fish"},
			expectError:    false,
			expectedOutput: "complete -c osm",
		},
		{
			name:           "powershell completion",
			args:           []string{"powershell"},
			expectError:    false,
			expectedOutput: "Register-ArgumentCompleter",
		},
		{
			name:        "unsupported shell",
			args:        []string{"unsupported"},
			expectError: true,
		},
		{
			name:        "too many args",
			args:        []string{"bash", "zsh"},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output strings.Builder
			var stderr strings.Builder

			completionCmd := NewCompletionCommand(registry)
			err := completionCmd.Execute(tt.args, &output, &stderr)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error for args %v, but got none", tt.args)
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			outputStr := output.String()
			if !strings.Contains(outputStr, tt.expectedOutput) {
				to := outputStr
				if len(to) > 1024 {
					to = to[:1024]
				}
				t.Errorf("Expected output to contain %q, but got:\n%s", tt.expectedOutput, to)
			}

			// Verify that the completion includes the commands from the registry
			if !strings.Contains(outputStr, "help") {
				t.Errorf("Expected completion to include 'help' command")
			}
			if !strings.Contains(outputStr, "version") {
				t.Errorf("Expected completion to include 'version' command")
			}

			// Sanity check: 'completion' subcommand should expose shell names in scripts
			if tt.name != "unsupported shell" && tt.name != "too many args" {
				for _, w := range []string{"bash", "zsh", "fish", "powershell"} {
					if !strings.Contains(outputStr, w) {
						t.Errorf("Expected completion script to include %q option for 'completion' subcommand", w)
					}
				}
			}
		})
	}
}

func TestCompletionCommandExecuteWithArgs(t *testing.T) {
	registry := NewRegistry()
	registry.Register(NewHelpCommand(registry))

	completionCmd := NewCompletionCommand(registry)

	var output strings.Builder
	var stderr strings.Builder

	// Test that shell can be specified as an argument
	err := completionCmd.Execute([]string{"zsh"}, &output, &stderr)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	outputStr := output.String()
	if !strings.Contains(outputStr, "#compdef osm") {
		t.Errorf("Expected zsh completion output, but got:\n%s", outputStr)
	}
}

func TestCompletionCommandDefaultToBash(t *testing.T) {
	registry := NewRegistry()
	registry.Register(NewHelpCommand(registry))

	completionCmd := NewCompletionCommand(registry)

	var output strings.Builder
	var stderr strings.Builder

	// Test that no args defaults to bash
	err := completionCmd.Execute([]string{}, &output, &stderr)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	outputStr := output.String()
	if !strings.Contains(outputStr, "_osm_completion") {
		t.Errorf("Expected bash completion output (default), but got:\n%s", outputStr)
	}
}

func TestCompletionCommandIncludesScriptCommands(t *testing.T) {
	registry := NewRegistry()
	registry.Register(NewHelpCommand(registry))

	scriptDir := t.TempDir()
	scriptName := "dummy-script"
	scriptPath := filepath.Join(scriptDir, scriptName)
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("failed to create script command: %v", err)
	}

	registry.AddScriptPath(scriptDir)

	shells := map[string]string{
		"bash":       "_osm_completion",
		"zsh":        "#compdef osm",
		"fish":       "complete -c osm",
		"powershell": "Register-ArgumentCompleter",
	}

	for shell, marker := range shells {
		shell := shell
		marker := marker
		t.Run(shell, func(t *testing.T) {
			completionCmd := NewCompletionCommand(registry)

			var output strings.Builder
			var stderr strings.Builder

			if err := completionCmd.Execute([]string{shell}, &output, &stderr); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			outputStr := output.String()
			if !strings.Contains(outputStr, marker) {
				to := outputStr
				if len(to) > 1024 {
					to = to[:1024]
				}
				t.Fatalf("expected %s completion output, got: %s", shell, to)
			}

			if !strings.Contains(outputStr, scriptName) {
				to := outputStr
				if len(to) > 1024 {
					to = to[:1024]
				}
				t.Fatalf("expected %s completion to include script command %q, got: %s", shell, scriptName, to)
			}
		})
	}
}
