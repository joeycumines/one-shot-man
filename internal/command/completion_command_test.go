package command

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/config"
)

// Helper function to create a test goal registry
func newTestGoalRegistry() GoalRegistry {
	cfg := config.NewConfig()
	// Avoid external goals leaking into tests
	cfg.SetGlobalOption("goal.disable-standard-paths", "true")
	// Disable autodiscovery to prevent filesystem goals (e.g. goals/orchestrate-pr-split.json)
	// from leaking into tests when run from the project root
	cfg.SetGlobalOption("goal.autodiscovery", "false")
	discovery := NewGoalDiscovery(cfg)
	return NewDynamicGoalRegistry(GetBuiltInGoals(), discovery)
}

func TestCompletionCommand(t *testing.T) {
	t.Parallel()
	// Create a test registry with some commands
	cfg := config.NewConfig()
	registry := NewRegistryWithConfig(cfg)
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
			t.Parallel()
			var output strings.Builder
			var stderr strings.Builder

			goalRegistry := newTestGoalRegistry()
			completionCmd := NewCompletionCommand(registry, goalRegistry)
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

func TestCompletionCommandZshShell(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	registry := NewRegistryWithConfig(cfg)
	registry.Register(NewHelpCommand(registry))

	goalRegistry := newTestGoalRegistry()
	completionCmd := NewCompletionCommand(registry, goalRegistry)

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

func TestCompletionCommandDefaultBash(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	registry := NewRegistryWithConfig(cfg)
	registry.Register(NewHelpCommand(registry))

	goalRegistry := newTestGoalRegistry()
	completionCmd := NewCompletionCommand(registry, goalRegistry)

	var output strings.Builder
	var stderr strings.Builder

	// Test that no args defaults to bash
	err := completionCmd.Execute([]string{}, &output, &stderr)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	outputStr := output.String()
	if !strings.Contains(outputStr, "_osm_completion") {
		t.Errorf("Expected default bash completion output")
	}
}

func TestCompletionCommandIncludesScriptCommands(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	registry := NewRegistryWithConfig(cfg)
	registry.Register(NewHelpCommand(registry))

	scriptDir := t.TempDir()
	var scriptName string
	if runtime.GOOS == "windows" {
		scriptName = "dummy-script.bat"
	} else {
		scriptName = "dummy-script.sh"
	}
	scriptPath := filepath.Join(scriptDir, scriptName)
	if runtime.GOOS == "windows" {
		if err := os.WriteFile(scriptPath, []byte("@echo off\nexit /b 0\n"), 0644); err != nil {
			t.Fatalf("failed to create script command: %v", err)
		}
	} else {
		if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatalf("failed to create script command: %v", err)
		}
		// Ensure execute bit regardless of umask (no-op on Windows).
		if err := os.Chmod(scriptPath, 0o755); err != nil {
			t.Fatalf("failed to chmod script command: %v", err)
		}
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
			t.Parallel()
			goalRegistry := newTestGoalRegistry()
			completionCmd := NewCompletionCommand(registry, goalRegistry)

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

func TestCompletionCommandGoalSubcommand(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	registry := NewRegistryWithConfig(cfg)
	registry.Register(NewHelpCommand(registry))
	goalRegistry := newTestGoalRegistry()
	registry.Register(NewGoalCommand(cfg, goalRegistry))

	goalNames := []string{
		"adaptive-editor",
		"bug-buster",
		"cite-sources",
		"code-explainer",
		"code-optimizer",
		"comment-stripper",
		"commit-message",
		"data-to-json",
		"doc-generator",
		"implementation-plan",
		"meeting-notes",
		"morale-improver",
		"pii-scrubber",
		"prose-polisher",
		"report-analyzer",
		"review-classifier",
		"sql-generator",
		"test-generator",
		"which-one-is-better",
	}

	tests := []struct {
		name         string
		shell        string
		expectedText []string
	}{
		{
			name:  "bash goal completion",
			shell: "bash",
			expectedText: append([]string{
				"goal)",
				// Verify exact format: sorted, space-delimited list in compgen -W
				"COMPREPLY=($(compgen -W \"paths adaptive-editor bug-buster cite-sources code-explainer code-optimizer comment-stripper commit-message data-to-json doc-generator implementation-plan meeting-notes morale-improver pii-scrubber prose-polisher report-analyzer review-classifier sql-generator test-generator which-one-is-better\"",
			}, goalNames...),
		},
		{
			name:  "zsh goal completion",
			shell: "zsh",
			expectedText: append([]string{
				"goal)",
				"_values 'goal-name'",
			}, goalNames...),
		},
		{
			name:  "fish goal completion",
			shell: "fish",
			expectedText: append([]string{
				"__fish_seen_subcommand_from goal",
			}, goalNames...),
		},
		{
			name:  "powershell goal completion",
			shell: "powershell",
			expectedText: append([]string{
				"$goals = @(",
				"$command -eq 'goal'",
			}, goalNames...),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			goalRegistry := newTestGoalRegistry()
			completionCmd := NewCompletionCommand(registry, goalRegistry)

			var output strings.Builder
			var stderr strings.Builder

			err := completionCmd.Execute([]string{tt.shell}, &output, &stderr)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			outputStr := output.String()

			for _, expected := range tt.expectedText {
				if !strings.Contains(outputStr, expected) {
					to := outputStr
					if len(to) > 2048 {
						to = to[:2048]
					}
					t.Errorf("Expected %s completion to contain %q, got:\n%s", tt.shell, expected, to)
				}
			}
		})
	}
}

func TestCompletionCommandSessionSubcommand(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	registry := NewRegistryWithConfig(cfg)
	// Ensure help and session commands are registered so completions include them
	registry.Register(NewHelpCommand(registry))
	registry.Register(NewSessionCommand(cfg))

	goalRegistry := newTestGoalRegistry()

	completionCmd := NewCompletionCommand(registry, goalRegistry)

	tests := []struct {
		name        string
		shell       string
		expectedTxt []string
	}{
		{name: "bash session", shell: "bash", expectedTxt: []string{"session)", "COMPREPLY=($(compgen -W \"list clean purge delete info path id\""}},
		{name: "zsh session", shell: "zsh", expectedTxt: []string{"session)", "_values 'session-subcommand' 'list' 'clean' 'purge' 'delete' 'info' 'path' 'id'"}},
		{name: "fish session", shell: "fish", expectedTxt: []string{"__fish_seen_subcommand_from session", "list clean purge delete info path id"}},
		{name: "powershell session", shell: "powershell", expectedTxt: []string{"$subs = @('list','clean','purge','delete','info','path','id')", "session'"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var output strings.Builder
			var stderr strings.Builder

			if err := completionCmd.Execute([]string{tt.shell}, &output, &stderr); err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			out := output.String()
			for _, expected := range tt.expectedTxt {
				if !strings.Contains(out, expected) {
					t.Errorf("Expected %s completion to contain %q, got:\n%s", tt.shell, expected, out)
				}
			}
		})
	}
}

func TestCompletionCommandGoalDescriptions(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	registry := NewRegistryWithConfig(cfg)
	goalRegistry := newTestGoalRegistry()
	registry.Register(NewGoalCommand(cfg, goalRegistry))

	completionCmd := NewCompletionCommand(registry, goalRegistry)

	var output strings.Builder
	var stderr strings.Builder

	// Fish shell includes descriptions in completions
	err := completionCmd.Execute([]string{"fish"}, &output, &stderr)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	outputStr := output.String()

	expectedDescriptions := map[string]string{
		"comment-stripper": "Remove useless comments",
		"doc-generator":    "Generate comprehensive documentation",
		"test-generator":   "Generate comprehensive test suites",
		"commit-message":   "Generate Kubernetes-style commit messages",
	}

	for goalName, description := range expectedDescriptions {
		if !strings.Contains(outputStr, goalName) {
			t.Errorf("Expected fish completion to contain goal name %q", goalName)
		}
		if !strings.Contains(outputStr, description) {
			t.Errorf("Expected fish completion to contain description snippet %q for goal %q", description, goalName)
		}
	}
}

func TestCompletionCommandSyncSubcommand(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	registry := NewRegistryWithConfig(cfg)
	registry.Register(NewHelpCommand(registry))

	goalRegistry := newTestGoalRegistry()
	completionCmd := NewCompletionCommand(registry, goalRegistry)

	tests := []struct {
		name        string
		shell       string
		expectedTxt []string
	}{
		{name: "bash sync", shell: "bash", expectedTxt: []string{"sync)", `COMPREPLY=($(compgen -W "save list load init push pull config-push config-pull"`}},
		{name: "zsh sync", shell: "zsh", expectedTxt: []string{"sync)", "_values 'sync-subcommand' 'save' 'list' 'load' 'init' 'push' 'pull' 'config-push' 'config-pull'"}},
		{name: "fish sync", shell: "fish", expectedTxt: []string{"__fish_seen_subcommand_from sync", "save list load init push pull config-push config-pull"}},
		{name: "powershell sync", shell: "powershell", expectedTxt: []string{"$subs = @('save','list','load','init','push','pull','config-push','config-pull')", "sync'"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var output strings.Builder
			var stderr strings.Builder

			if err := completionCmd.Execute([]string{tt.shell}, &output, &stderr); err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			out := output.String()
			for _, expected := range tt.expectedTxt {
				if !strings.Contains(out, expected) {
					t.Errorf("Expected %s completion to contain %q, got:\n%s", tt.shell, expected, out)
				}
			}
		})
	}
}

func TestCompletionCommandConfigSubcommand(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	registry := NewRegistryWithConfig(cfg)
	registry.Register(NewHelpCommand(registry))

	goalRegistry := newTestGoalRegistry()
	completionCmd := NewCompletionCommand(registry, goalRegistry)

	// Representative config keys that must appear in completions
	configKeys := []string{"verbose", "color", "debug", "session.id", "log.level"}

	tests := []struct {
		name        string
		shell       string
		expectedTxt []string
	}{
		{name: "bash config", shell: "bash", expectedTxt: append([]string{"config)", "validate", "schema", "list", "diff", "reset", "schema)", "--json"}, configKeys...)},
		{name: "zsh config", shell: "zsh", expectedTxt: append([]string{"config)", "_values 'config-subcommand' 'validate' 'schema' 'list' 'diff' 'reset'", "schema)", "'--json'"}, configKeys...)},
		{name: "fish config", shell: "fish", expectedTxt: append([]string{"__fish_seen_subcommand_from config", "validate", "schema", "list", "diff", "reset", "__fish_seen_subcommand_from schema", "--json"}, configKeys...)},
		{name: "powershell config", shell: "powershell", expectedTxt: append([]string{"config'", "'validate'", "'schema'", "'list'", "'diff'", "'reset'", "'--json'"}, configKeys...)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var output strings.Builder
			var stderr strings.Builder

			if err := completionCmd.Execute([]string{tt.shell}, &output, &stderr); err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			out := output.String()
			for _, expected := range tt.expectedTxt {
				if !strings.Contains(out, expected) {
					t.Errorf("Expected %s completion to contain %q, got:\n%s", tt.shell, expected, out)
				}
			}
		})
	}
}

func TestCompletionCommandLogSubcommand(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	registry := NewRegistryWithConfig(cfg)
	registry.Register(NewHelpCommand(registry))

	goalRegistry := newTestGoalRegistry()
	completionCmd := NewCompletionCommand(registry, goalRegistry)

	tests := []struct {
		name        string
		shell       string
		expectedTxt []string
	}{
		{name: "bash log", shell: "bash", expectedTxt: []string{"log)", `COMPREPLY=($(compgen -W "tail follow"`}},
		{name: "zsh log", shell: "zsh", expectedTxt: []string{"log)", "_values 'log-subcommand' 'tail' 'follow'"}},
		{name: "fish log", shell: "fish", expectedTxt: []string{"__fish_seen_subcommand_from log", "tail follow"}},
		{name: "powershell log", shell: "powershell", expectedTxt: []string{"$subs = @('tail','follow')", "log'"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var output strings.Builder
			var stderr strings.Builder

			if err := completionCmd.Execute([]string{tt.shell}, &output, &stderr); err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			out := output.String()
			for _, expected := range tt.expectedTxt {
				if !strings.Contains(out, expected) {
					t.Errorf("Expected %s completion to contain %q, got:\n%s", tt.shell, expected, out)
				}
			}
		})
	}
}

func TestCompletionCommandFishEscaping(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()

	// Create a custom goal with a single quote in the description
	testGoal := Goal{
		Name:        "test-escaping",
		Description: "Test the tool's quote handling",
		Category:    "testing",
	}

	// Create a custom goal registry with our test goal
	discovery := NewGoalDiscovery(cfg)
	builtInGoals := []Goal{testGoal}
	goalRegistry := NewDynamicGoalRegistry(builtInGoals, discovery)

	registry := NewRegistryWithConfig(cfg)
	registry.Register(NewGoalCommand(cfg, goalRegistry))

	completionCmd := NewCompletionCommand(registry, goalRegistry)

	var output strings.Builder
	var stderr strings.Builder

	// Generate Fish completion script
	err := completionCmd.Execute([]string{"fish"}, &output, &stderr)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	outputStr := output.String()

	// Verify the goal name is present
	if !strings.Contains(outputStr, "test-escaping") {
		t.Errorf("Expected fish completion to contain goal name 'test-escaping'")
	}

	// Verify the description is correctly escaped
	// Single quote in "tool's" should be escaped as '\''
	expectedEscaped := "Test the tool'\\''s quote handling"
	if !strings.Contains(outputStr, expectedEscaped) {
		t.Errorf("Expected fish completion to contain correctly escaped description %q, got output:\n%s",
			expectedEscaped, outputStr)
	}

	// Ensure the unescaped version is NOT present (which would break the shell script)
	unescapedBroken := "'Test the tool's quote handling'"
	if strings.Contains(outputStr, unescapedBroken) {
		t.Errorf("Fish completion contains unescaped single quote which would break shell syntax")
	}
}

func TestCompletionCommandPwshAlias(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	registry := NewRegistryWithConfig(cfg)
	registry.Register(NewHelpCommand(registry))

	goalRegistry := newTestGoalRegistry()
	completionCmd := NewCompletionCommand(registry, goalRegistry)

	var output strings.Builder
	var stderr strings.Builder

	err := completionCmd.Execute([]string{"pwsh"}, &output, &stderr)
	if err != nil {
		t.Fatalf("Unexpected error for 'pwsh' alias: %v", err)
	}

	if !strings.Contains(output.String(), "Register-ArgumentCompleter") {
		t.Error("Expected 'pwsh' alias to produce PowerShell completion output")
	}
}

func TestCompletionCommandHelpSubcommand(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	registry := NewRegistryWithConfig(cfg)
	registry.Register(NewHelpCommand(registry))
	registry.Register(NewVersionCommand("1.0.0"))

	goalRegistry := newTestGoalRegistry()
	completionCmd := NewCompletionCommand(registry, goalRegistry)

	shells := []string{"bash", "zsh", "fish", "powershell"}
	for _, shell := range shells {
		t.Run(shell, func(t *testing.T) {
			t.Parallel()
			var output strings.Builder
			var stderr strings.Builder

			if err := completionCmd.Execute([]string{shell}, &output, &stderr); err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			out := output.String()
			if !strings.Contains(out, "help") {
				t.Errorf("%s completion should contain 'help' for help subcommand completion", shell)
			}
			// The help completion block should reference registered command names
			// so that "osm help <TAB>" suggests them. We registered "version",
			// so it must appear in help-related completion output.
			if !strings.Contains(out, "version") {
				t.Errorf("%s completion should include registered commands like 'version' for help subcommand", shell)
			}
		})
	}
}
