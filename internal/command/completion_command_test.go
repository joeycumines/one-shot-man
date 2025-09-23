package command

import (
	"strings"
	"testing"
)

func TestCompletionCommand(t *testing.T) {
	// Create a test registry with some commands
	registry := NewRegistry()
	registry.Register(NewHelpCommand(registry))
	registry.Register(NewVersionCommand("1.0.0"))
	
	completionCmd := NewCompletionCommand(registry)
	
	tests := []struct {
		name           string
		shell          string
		expectError    bool
		expectedOutput string
	}{
		{
			name:           "bash completion",
			shell:          "bash",
			expectError:    false,
			expectedOutput: "_osm_completion",
		},
		{
			name:           "zsh completion",
			shell:          "zsh",
			expectError:    false,
			expectedOutput: "#compdef osm",
		},
		{
			name:           "fish completion",
			shell:          "fish",
			expectError:    false,
			expectedOutput: "complete -c osm",
		},
		{
			name:           "powershell completion",
			shell:          "powershell",
			expectError:    false,
			expectedOutput: "Register-ArgumentCompleter",
		},
		{
			name:        "unsupported shell",
			shell:       "unsupported",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output strings.Builder
			var stderr strings.Builder
			
			// Set the shell flag
			completionCmd.shell = tt.shell
			
			err := completionCmd.Execute([]string{}, &output, &stderr)
			
			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error for shell %s, but got none", tt.shell)
				}
				return
			}
			
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}
			
			outputStr := output.String()
			if !strings.Contains(outputStr, tt.expectedOutput) {
				t.Errorf("Expected output to contain %q, but got:\n%s", tt.expectedOutput, outputStr)
			}
			
			// Verify that the completion includes the commands from the registry
			if !strings.Contains(outputStr, "help") {
				t.Errorf("Expected completion to include 'help' command")
			}
			if !strings.Contains(outputStr, "version") {
				t.Errorf("Expected completion to include 'version' command")
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