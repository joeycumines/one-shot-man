package command

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/config"
	"github.com/joeycumines/one-shot-man/internal/scripting"
	"github.com/joeycumines/one-shot-man/internal/testutil"
)

// TestCodeReviewCommandEdgeCases tests edge cases for the code-review command
func TestCodeReviewCommandEdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("EmptyTargetList", func(t *testing.T) {
		cfg := config.NewConfig()
		cmd := NewCodeReviewCommand(cfg)

		var stdout, stderr bytes.Buffer

		cmd.testMode = true
		cmd.interactive = false
		cmd.store = "memory"
		cmd.session = t.Name()

		// Execute with empty args - should still work
		err := cmd.Execute([]string{}, &stdout, &stderr)
		if err != nil {
			t.Fatalf("Expected no error with empty args, got: %v", err)
		}

		output := stdout.String()
		if !contains(output, "Type 'help' for commands") {
			t.Errorf("Expected help message in output, got: %s", output)
		}
	})

	t.Run("NonExistentTargetFile", func(t *testing.T) {
		cfg := config.NewConfig()
		cmd := NewCodeReviewCommand(cfg)

		var stdout, stderr bytes.Buffer

		cmd.testMode = true
		cmd.interactive = false
		cmd.store = "memory"
		cmd.session = t.Name()

		// Execute with non-existent file as argument
		err := cmd.Execute([]string{"/path/to/nonexistent/file.go"}, &stdout, &stderr)
		if err != nil {
			// This is expected - script should handle missing files gracefully
			t.Logf("Got expected error for non-existent file: %v", err)
		} else {
			// If no error, check that script still ran
			output := stdout.String()
			if !contains(output, "Type 'help' for commands") {
				t.Errorf("Expected help message even with non-existent file, got: %s", output)
			}
		}
	})

	t.Run("NonExistentTargetDirectory", func(t *testing.T) {
		cfg := config.NewConfig()
		cmd := NewCodeReviewCommand(cfg)

		var stdout, stderr bytes.Buffer

		cmd.testMode = true
		cmd.interactive = false
		cmd.store = "memory"
		cmd.session = t.Name()

		// Execute with non-existent directory as argument
		err := cmd.Execute([]string{"/path/to/nonexistent/directory"}, &stdout, &stderr)
		if err != nil {
			t.Logf("Got error for non-existent directory: %v", err)
		} else {
			output := stdout.String()
			if !contains(output, "Type 'help' for commands") {
				t.Errorf("Expected help message even with non-existent directory, got: %s", output)
			}
		}
	})

	t.Run("DirectoryInsteadOfFile", func(t *testing.T) {
		cfg := config.NewConfig()
		cmd := NewCodeReviewCommand(cfg)

		var stdout, stderr bytes.Buffer

		cmd.testMode = true
		cmd.interactive = false
		cmd.store = "memory"
		cmd.session = t.Name()

		// Create a temporary directory
		tmpDir := t.TempDir()

		// Execute with directory path as argument
		err := cmd.Execute([]string{tmpDir}, &stdout, &stderr)
		if err != nil {
			t.Logf("Got error when passing directory: %v", err)
		} else {
			output := stdout.String()
			if !contains(output, "Type 'help' for commands") {
				t.Errorf("Expected help message when passing directory, got: %s", output)
			}
		}
	})

	t.Run("ExtremelyLongPath", func(t *testing.T) {
		cfg := config.NewConfig()
		cmd := NewCodeReviewCommand(cfg)

		var stdout, stderr bytes.Buffer

		cmd.testMode = true
		cmd.interactive = false
		cmd.store = "memory"
		cmd.session = t.Name()

		// Create an extremely long path (but within reasonable limits)
		longPath := strings.Repeat("a", 200) + "/file.go"

		err := cmd.Execute([]string{longPath}, &stdout, &stderr)
		if err != nil {
			t.Logf("Got error for long path: %v", err)
		} else {
			output := stdout.String()
			if !contains(output, "Type 'help' for commands") {
				t.Errorf("Expected help message with long path, got: %s", output)
			}
		}
	})

	t.Run("PathWithSpecialCharacters", func(t *testing.T) {
		cfg := config.NewConfig()
		cmd := NewCodeReviewCommand(cfg)

		var stdout, stderr bytes.Buffer

		cmd.testMode = true
		cmd.interactive = false
		cmd.store = "memory"
		cmd.session = t.Name()

		// Path with special characters
		specialPath := "/path/with spaces and-underscores/file.go"

		err := cmd.Execute([]string{specialPath}, &stdout, &stderr)
		if err != nil {
			t.Logf("Got error for path with special chars: %v", err)
		} else {
			output := stdout.String()
			if !contains(output, "Type 'help' for commands") {
				t.Errorf("Expected help message with special char path, got: %s", output)
			}
		}
	})

	t.Run("PermissionDeniedFile", func(t *testing.T) {
		cfg := config.NewConfig()
		cmd := NewCodeReviewCommand(cfg)

		var stdout, stderr bytes.Buffer

		cmd.testMode = true
		cmd.interactive = false
		cmd.store = "memory"
		cmd.session = t.Name()

		// Create a file with no read permissions
		tmpDir := t.TempDir()
		deniedFile := filepath.Join(tmpDir, "denied.go")
		if err := os.WriteFile(deniedFile, []byte("package test"), 0000); err != nil {
			t.Skipf("Cannot set file permissions on this platform: %v", err)
		}

		err := cmd.Execute([]string{deniedFile}, &stdout, &stderr)
		if err != nil {
			t.Logf("Got error for permission denied file: %v", err)
		}
		// The script should handle this gracefully or report an error
	})
}

// TestGoalCommandEdgeCases tests edge cases for the goal command
func TestGoalCommandEdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("NonExistentGoalFile", func(t *testing.T) {
		cfg := config.NewConfig()
		discovery := NewGoalDiscovery(cfg)
		registry := NewDynamicGoalRegistry(GetBuiltInGoals(), discovery)

		_, err := registry.Get("totally-fake-goal-name")
		if err == nil {
			t.Fatal("Expected error for non-existent goal")
		}
		if !contains(err.Error(), "not found") {
			t.Errorf("Expected 'not found' in error message, got: %s", err.Error())
		}
	})

	t.Run("GoalFileIsDirectory", func(t *testing.T) {
		cfg := config.NewConfig()
		discovery := NewGoalDiscovery(cfg)
		_ = NewDynamicGoalRegistry(GetBuiltInGoals(), discovery)

		// Create a directory where a goal file would be
		tmpDir := t.TempDir()
		dirAsGoal := filepath.Join(tmpDir, "fake-goal")
		if err := os.Mkdir(dirAsGoal, 0755); err != nil {
			t.Fatalf("Failed to create directory: %v", err)
		}

		// The discovery should skip directories
		paths := discovery.DiscoverGoalPaths()
		_ = paths // This is just to verify no panic
	})

	t.Run("GoalFileContainingInvalidJSON", func(t *testing.T) {
		cfg := config.NewConfig()

		// Create a goal discovery with invalid JSON goal file
		tmpDir := t.TempDir()
		cfg.SetGlobalOption("goal.paths", tmpDir)
		cfg.SetGlobalOption("goal.autodiscovery", "false")

		// Write invalid JSON
		invalidGoalFile := filepath.Join(tmpDir, "invalid-goal.json")
		invalidJSON := `{ "name": "invalid-goal", invalid json content }`
		if err := os.WriteFile(invalidGoalFile, []byte(invalidJSON), 0644); err != nil {
			t.Fatalf("Failed to write invalid goal file: %v", err)
		}

		discovery := NewGoalDiscovery(cfg)
		registry := NewDynamicGoalRegistry(GetBuiltInGoals(), discovery)

		// Should not find the invalid goal (or should handle gracefully)
		_, err := registry.Get("invalid-goal")
		if err == nil {
			t.Log("Invalid goal was not loaded (expected behavior)")
		} else {
			t.Logf("Got error loading invalid goal (expected): %v", err)
		}

		// Built-in goals should still work
		_, err = registry.Get("comment-stripper")
		if err != nil {
			t.Errorf("Built-in goal should still be accessible: %v", err)
		}
	})

	t.Run("EmptyGoalArray", func(t *testing.T) {
		cfg := config.NewConfig()
		discovery := NewGoalDiscovery(cfg)

		// Create registry with no built-in goals
		registry := NewDynamicGoalRegistry([]Goal{}, discovery)

		goals := registry.List()
		if len(goals) != 0 {
			t.Logf("Registry has %d goals (may have discovered goals)", len(goals))
		}

		// Should return empty list without error
		_, err := registry.Get("nonexistent")
		if err == nil {
			t.Error("Expected error for non-existent goal")
		}
	})

	t.Run("GoalContainingInvalidTemplateSyntax", func(t *testing.T) {
		cfg := config.NewConfig()

		// Create a goal with invalid template syntax
		tmpDir := t.TempDir()
		cfg.SetGlobalOption("goal.paths", tmpDir)
		cfg.SetGlobalOption("goal.autodiscovery", "false")

		discovery := NewGoalDiscovery(cfg)
		invalidTemplateGoal := filepath.Join(tmpDir, "bad-template.json")
		goalJSON := `{
			"name": "bad-template",
			"description": "Goal with bad template",
			"category": "test",
			"promptTemplate": "{{.undefinedVariable | invalidFilter}}"
		}`
		if err := os.WriteFile(invalidTemplateGoal, []byte(goalJSON), 0644); err != nil {
			t.Fatalf("Failed to write goal file: %v", err)
		}

		registry := NewDynamicGoalRegistry(GetBuiltInGoals(), discovery)

		// Goal might load but template processing will fail at runtime
		goal, err := registry.Get("bad-template")
		if err != nil {
			t.Logf("Goal failed to load due to invalid template: %v", err)
		} else if goal != nil {
			// The goal loaded - the template error will occur when rendering
			t.Log("Goal loaded successfully, template error will occur at runtime")
		}
	})

	t.Run("GoalCommandWithNoArgsAndNoInteractive", func(t *testing.T) {
		cfg := config.NewConfig()
		discovery := NewGoalDiscovery(cfg)
		registry := NewDynamicGoalRegistry(GetBuiltInGoals(), discovery)

		cmd := &GoalCommand{
			BaseCommand: NewBaseCommand("goal", "Test goal command", "goal [options]"),
			config:      cfg,
			registry:    registry,
			interactive: false,
			list:        false,
		}

		var stdout bytes.Buffer
		var stderr bytes.Buffer

		// Execute with no args and no interactive mode
		err := cmd.Execute([]string{}, &stdout, &stderr)
		if err != nil {
			t.Logf("Got error with no args: %v", err)
		}

		output := stdout.String()
		// Should list goals when no args provided
		if !contains(output, "Available Goals") && !contains(output, "No goals available") {
			t.Errorf("Expected goal listing, got: %s", output)
		}
	})

	t.Run("GoalCommandInvalidFlagCombination", func(t *testing.T) {
		cfg := config.NewConfig()
		discovery := NewGoalDiscovery(cfg)
		registry := NewDynamicGoalRegistry(GetBuiltInGoals(), discovery)

		cmd := &GoalCommand{
			BaseCommand: NewBaseCommand("goal", "Test goal command", "goal [options]"),
			config:      cfg,
			registry:    registry,
			list:        true,
			category:    "nonexistent-category",
		}

		var stdout bytes.Buffer
		var stderr bytes.Buffer

		// Execute with list flag and non-existent category
		err := cmd.Execute([]string{}, &stdout, &stderr)
		if err != nil {
			t.Logf("Got error for non-existent category: %v", err)
		}

		output := stdout.String()
		if !contains(output, "No goals found") && !contains(output, "category") {
			t.Logf("Output for non-existent category: %s", output)
		}
	})
}

// TestPromptFlowCommandEdgeCases tests edge cases for the prompt-flow command
func TestPromptFlowCommandEdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("MissingContext", func(t *testing.T) {
		cfg := config.NewConfig()
		cmd := NewPromptFlowCommand(cfg)

		var stdout, stderr bytes.Buffer

		cmd.testMode = true
		cmd.interactive = false
		cmd.store = "memory"
		cmd.session = t.Name()

		err := cmd.Execute([]string{}, &stdout, &stderr)
		if err != nil {
			t.Fatalf("Expected no error even with missing context, got: %v", err)
		}

		output := stdout.String()
		if !contains(output, "Type 'help' for commands") {
			t.Errorf("Expected help message, got: %s", output)
		}
	})

	t.Run("VeryLongPromptText", func(t *testing.T) {
		cfg := config.NewConfig()
		cmd := NewPromptFlowCommand(cfg)

		var stdout, stderr bytes.Buffer

		cmd.testMode = true
		cmd.interactive = false
		cmd.store = "memory"
		cmd.session = t.Name()

		// Create a very long prompt text
		longPrompt := strings.Repeat("This is a test prompt. ", 1000)

		err := cmd.Execute([]string{longPrompt}, &stdout, &stderr)
		if err != nil {
			t.Logf("Got error with long prompt: %v", err)
		} else {
			output := stdout.String()
			if !contains(output, "Type 'help' for commands") {
				t.Errorf("Expected help message with long prompt, got: %s", output)
			}
		}
	})

	t.Run("InvalidPromptTemplateSyntax", func(t *testing.T) {
		cfg := config.NewConfig()
		cmd := NewPromptFlowCommand(cfg)

		var stdout, stderr bytes.Buffer

		cmd.testMode = true
		cmd.interactive = false
		cmd.store = "memory"
		cmd.session = t.Name()

		// Execute with an invalid template syntax argument
		invalidTemplate := "{{.undefined | filterThatDoesNotExist}}"

		err := cmd.Execute([]string{invalidTemplate}, &stdout, &stderr)
		if err != nil {
			t.Logf("Got error for invalid template: %v", err)
		} else {
			output := stdout.String()
			if !contains(output, "Type 'help' for commands") {
				t.Errorf("Expected help message even with invalid template, got: %s", output)
			}
		}
	})

	t.Run("UnicodeContentInPrompt", func(t *testing.T) {
		cfg := config.NewConfig()
		cmd := NewPromptFlowCommand(cfg)

		var stdout, stderr bytes.Buffer

		cmd.testMode = true
		cmd.interactive = false
		cmd.store = "memory"
		cmd.session = t.Name()

		// Unicode content
		unicodePrompt := "测试中文 🎉 Ελληνικά español français"

		err := cmd.Execute([]string{unicodePrompt}, &stdout, &stderr)
		if err != nil {
			t.Fatalf("Expected no error with unicode content, got: %v", err)
		}

		output := stdout.String()
		if !contains(output, "Type 'help' for commands") {
			t.Errorf("Expected help message with unicode, got: %s", output)
		}
	})

	t.Run("PromptFlowWithArgsOnlyNoInteractive", func(t *testing.T) {
		cfg := config.NewConfig()
		cmd := NewPromptFlowCommand(cfg)

		var stdout, stderr bytes.Buffer

		cmd.testMode = true
		cmd.interactive = false
		cmd.store = "memory"
		cmd.session = t.Name()

		// Execute with various arguments but no interactive mode
		args := []string{"--test", "--session", "test-session", "arg1", "arg2"}
		err := cmd.Execute(args, &stdout, &stderr)
		if err != nil {
			t.Fatalf("Expected no error with args only, got: %v", err)
		}

		output := stdout.String()
		if !contains(output, "Type 'help' for commands") {
			t.Errorf("Expected help message, got: %s", output)
		}
	})
}

// TestSuperDocumentCommandEdgeCases tests edge cases for the super-document command
func TestSuperDocumentCommandEdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("EmptyInput", func(t *testing.T) {
		ctx := context.Background()
		var stdout, stderr bytes.Buffer
		engine, err := scripting.NewEngineWithConfig(ctx, &stdout, &stderr, testutil.NewTestSessionID("super-document", t.Name()), "memory")
		if err != nil {
			t.Fatalf("NewEngineWithConfig failed: %v", err)
		}
		defer engine.Close()
		engine.SetTestMode(true)

		engine.SetGlobal("config", map[string]interface{}{"name": "super-document", "theme": map[string]interface{}{}})
		engine.SetGlobal("args", []string{})
		engine.SetGlobal("superDocumentTemplate", "dummy template")

		script := engine.LoadScriptFromString("super-document", superDocumentScript)
		if err := engine.ExecuteScript(script); err != nil {
			t.Fatalf("failed to execute super-document script: %v", err)
		}
	})

	t.Run("InputContainingOnlyWhitespace", func(t *testing.T) {
		ctx := context.Background()
		var stdout, stderr bytes.Buffer
		engine, err := scripting.NewEngineWithConfig(ctx, &stdout, &stderr, testutil.NewTestSessionID("super-document", t.Name()), "memory")
		if err != nil {
			t.Fatalf("NewEngineWithConfig failed: %v", err)
		}
		defer engine.Close()
		engine.SetTestMode(true)

		engine.SetGlobal("config", map[string]interface{}{"name": "super-document", "theme": map[string]interface{}{}})
		engine.SetGlobal("args", []string{"   \t\n  "})
		engine.SetGlobal("superDocumentTemplate", "dummy template")

		script := engine.LoadScriptFromString("super-document", superDocumentScript)
		if err := engine.ExecuteScript(script); err != nil {
			t.Fatalf("failed to execute super-document script: %v", err)
		}
	})

	t.Run("InputIsNonExistentFilePath", func(t *testing.T) {
		ctx := context.Background()
		var stdout, stderr bytes.Buffer
		engine, err := scripting.NewEngineWithConfig(ctx, &stdout, &stderr, testutil.NewTestSessionID("super-document", t.Name()), "memory")
		if err != nil {
			t.Fatalf("NewEngineWithConfig failed: %v", err)
		}
		defer engine.Close()
		engine.SetTestMode(true)

		engine.SetGlobal("config", map[string]interface{}{"name": "super-document", "theme": map[string]interface{}{}})
		engine.SetGlobal("args", []string{"/nonexistent/path/to/file.txt"})
		engine.SetGlobal("superDocumentTemplate", "dummy template")

		script := engine.LoadScriptFromString("super-document", superDocumentScript)
		if err := engine.ExecuteScript(script); err != nil {
			t.Fatalf("failed to execute super-document script: %v", err)
		}
	})

	t.Run("InputIsDirectory", func(t *testing.T) {
		ctx := context.Background()
		var stdout, stderr bytes.Buffer
		engine, err := scripting.NewEngineWithConfig(ctx, &stdout, &stderr, testutil.NewTestSessionID("super-document", t.Name()), "memory")
		if err != nil {
			t.Fatalf("NewEngineWithConfig failed: %v", err)
		}
		defer engine.Close()
		engine.SetTestMode(true)

		// Use temp directory as input
		tmpDir := t.TempDir()

		engine.SetGlobal("config", map[string]interface{}{"name": "super-document", "theme": map[string]interface{}{}})
		engine.SetGlobal("args", []string{tmpDir})
		engine.SetGlobal("superDocumentTemplate", "dummy template")

		script := engine.LoadScriptFromString("super-document", superDocumentScript)
		if err := engine.ExecuteScript(script); err != nil {
			t.Fatalf("failed to execute super-document script: %v", err)
		}
	})

	t.Run("ExtremelyLongInput", func(t *testing.T) {
		ctx := context.Background()
		var stdout, stderr bytes.Buffer
		engine, err := scripting.NewEngineWithConfig(ctx, &stdout, &stderr, testutil.NewTestSessionID("super-document", t.Name()), "memory")
		if err != nil {
			t.Fatalf("NewEngineWithConfig failed: %v", err)
		}
		defer engine.Close()
		engine.SetTestMode(true)

		// Create extremely long input
		longInput := strings.Repeat("x", 100000)

		engine.SetGlobal("config", map[string]interface{}{"name": "super-document", "theme": map[string]interface{}{}})
		engine.SetGlobal("args", []string{longInput})
		engine.SetGlobal("superDocumentTemplate", "dummy template")

		script := engine.LoadScriptFromString("super-document", superDocumentScript)
		if err := engine.ExecuteScript(script); err != nil {
			t.Fatalf("failed to execute super-document script: %v", err)
		}
	})

	t.Run("SuperDocumentCommandWithNilConfig", func(t *testing.T) {
		_ = NewSuperDocumentCommand(nil)

		var stdout, stderr bytes.Buffer

		ctx := context.Background()
		engine, err := scripting.NewEngineWithConfig(ctx, &stdout, &stderr, testutil.NewTestSessionID("super-document", t.Name()), "memory")
		if err != nil {
			t.Fatalf("NewEngineWithConfig failed: %v", err)
		}
		defer engine.Close()
		engine.SetTestMode(true)

		engine.SetGlobal("config", map[string]interface{}{"name": "super-document", "theme": map[string]interface{}{}})
		engine.SetGlobal("args", []string{})
		engine.SetGlobal("superDocumentTemplate", "dummy template")

		script := engine.LoadScriptFromString("super-document", superDocumentScript)
		if err := engine.ExecuteScript(script); err != nil {
			t.Fatalf("failed to execute super-document script with nil config: %v", err)
		}
	})

	t.Run("SuperDocumentCommandFlagsCombination", func(t *testing.T) {
		cfg := config.NewConfig()
		cmd := NewSuperDocumentCommand(cfg)

		fs := flag.NewFlagSet("test", flag.ContinueOnError)
		cmd.SetupFlags(fs)

		// Test that all flags are properly defined
		flags := []string{"interactive", "i", "shell", "test", "session", "store"}
		for _, flagName := range flags {
			if fs.Lookup(flagName) == nil {
				t.Errorf("Expected flag %s to be defined", flagName)
			}
		}
	})
}

// TestGoalCommandGoalLoadingEdgeCases tests additional goal loading edge cases
func TestGoalCommandGoalLoadingEdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("GoalWithCircularTemplateReferences", func(t *testing.T) {
		cfg := config.NewConfig()

		// Create a goal with self-referencing template that might cause issues
		tmpDir := t.TempDir()
		cfg.SetGlobalOption("goal.paths", tmpDir)
		cfg.SetGlobalOption("goal.autodiscovery", "false")

		circularGoal := filepath.Join(tmpDir, "circular.json")
		goalJSON := `{
			"name": "circular",
			"description": "Test goal",
			"category": "test",
			"promptTemplate": "{{.description}} - {{.name}}"
		}`
		if err := os.WriteFile(circularGoal, []byte(goalJSON), 0644); err != nil {
			t.Fatalf("Failed to write goal file: %v", err)
		}

		discovery := NewGoalDiscovery(cfg)
		registry := NewDynamicGoalRegistry(GetBuiltInGoals(), discovery)

		goal, err := registry.Get("circular")
		if err != nil {
			t.Logf("Goal failed to load: %v", err)
		} else if goal != nil {
			// Verify the template can be parsed (actual rendering happens at runtime)
			if goal.PromptTemplate != "{{.description}} - {{.name}}" {
				t.Errorf("Unexpected template: %s", goal.PromptTemplate)
			}
		}
	})

	t.Run("GoalWithEmptyRequiredFields", func(t *testing.T) {
		cfg := config.NewConfig()

		// Create a goal with minimal/empty fields
		tmpDir := t.TempDir()
		cfg.SetGlobalOption("goal.paths", tmpDir)
		cfg.SetGlobalOption("goal.autodiscovery", "false")

		minimalGoal := filepath.Join(tmpDir, "minimal.json")
		goalJSON := `{
			"name": "minimal",
			"description": "",
			"category": "",
			"promptInstructions": ""
		}`
		if err := os.WriteFile(minimalGoal, []byte(goalJSON), 0644); err != nil {
			t.Fatalf("Failed to write goal file: %v", err)
		}

		discovery := NewGoalDiscovery(cfg)
		registry := NewDynamicGoalRegistry(GetBuiltInGoals(), discovery)

		goal, err := registry.Get("minimal")
		if err != nil {
			t.Logf("Minimal goal failed to load: %v", err)
		} else if goal != nil {
			t.Logf("Minimal goal loaded: name=%s, description='%s'", goal.Name, goal.Description)
		}
	})

	t.Run("GoalWithDuplicateCommands", func(t *testing.T) {
		cfg := config.NewConfig()

		tmpDir := t.TempDir()
		cfg.SetGlobalOption("goal.paths", tmpDir)
		cfg.SetGlobalOption("goal.autodiscovery", "false")

		dupCmdGoal := filepath.Join(tmpDir, "dup-commands.json")
		goalJSON := `{
			"name": "dup-commands",
			"description": "Goal with duplicate commands",
			"category": "test",
			"commands": [
				{"name": "add", "type": "contextManager"},
				{"name": "add", "type": "contextManager"}
			]
		}`
		if err := os.WriteFile(dupCmdGoal, []byte(goalJSON), 0644); err != nil {
			t.Fatalf("Failed to write goal file: %v", err)
		}

		discovery := NewGoalDiscovery(cfg)
		registry := NewDynamicGoalRegistry(GetBuiltInGoals(), discovery)

		goal, err := registry.Get("dup-commands")
		if err != nil {
			t.Logf("Goal with duplicate commands failed to load: %v", err)
		} else if goal != nil {
			t.Logf("Goal loaded with %d commands", len(goal.Commands))
		}
	})

	t.Run("GoalWithHyphenInName", func(t *testing.T) {
		cfg := config.NewConfig()

		tmpDir := t.TempDir()
		cfg.SetGlobalOption("goal.paths", tmpDir)
		cfg.SetGlobalOption("goal.autodiscovery", "false")

		specialGoal := filepath.Join(tmpDir, "special-name.json")
		goalJSON := `{
			"name": "special-goal-test-123",
			"description": "Goal with hyphens in name",
			"category": "test"
		}`
		if err := os.WriteFile(specialGoal, []byte(goalJSON), 0644); err != nil {
			t.Fatalf("Failed to write goal file: %v", err)
		}

		discovery := NewGoalDiscovery(cfg)
		registry := NewDynamicGoalRegistry(GetBuiltInGoals(), discovery)

		goal, err := registry.Get("special-goal-test-123")
		if err != nil {
			t.Errorf("Goal with hyphens should load: %v", err)
		} else if goal != nil {
			if goal.Name != "special-goal-test-123" {
				t.Errorf("Expected name 'special-goal-test-123', got '%s'", goal.Name)
			}
		}
	})

	t.Run("GoalJSONMarshalUnmarshal", func(t *testing.T) {
		// Test that Goal struct can be marshaled and unmarshaled correctly
		original := Goal{
			Name:        "test-goal",
			Description: "A test goal",
			Category:    "testing",
			Usage:       "Test usage",
			StateVars: map[string]interface{}{
				"key1": "value1",
				"key2": float64(42),
			},
			PromptInstructions: "Test instructions",
			PromptTemplate:     "Template: {{.name}}",
			ContextHeader:      "CONTEXT",
		}

		// Marshal to JSON
		data, err := json.Marshal(original)
		if err != nil {
			t.Fatalf("Failed to marshal goal: %v", err)
		}

		// Unmarshal back
		var restored Goal
		if err := json.Unmarshal(data, &restored); err != nil {
			t.Fatalf("Failed to unmarshal goal: %v", err)
		}

		// Verify fields
		if restored.Name != original.Name {
			t.Errorf("Name mismatch: got '%s', expected '%s'", restored.Name, original.Name)
		}
		if restored.Description != original.Description {
			t.Errorf("Description mismatch: got '%s', expected '%s'", restored.Description, original.Description)
		}
		if restored.Category != original.Category {
			t.Errorf("Category mismatch: got '%s', expected '%s'", restored.Category, original.Category)
		}
		if restored.PromptTemplate != original.PromptTemplate {
			t.Errorf("Template mismatch: got '%s', expected '%s'", restored.PromptTemplate, original.PromptTemplate)
		}
	})
}

// TestCodeReviewCommandWithVariousFlags tests the code-review command with various flag combinations
func TestCodeReviewCommandWithVariousFlags(t *testing.T) {
	t.Parallel()

	t.Run("NonInteractiveWithSessionOverride", func(t *testing.T) {
		cfg := config.NewConfig()
		cmd := NewCodeReviewCommand(cfg)

		var stdout, stderr bytes.Buffer

		cmd.testMode = true
		cmd.interactive = false
		cmd.session = "custom-session-id"
		cmd.store = "memory"

		err := cmd.Execute([]string{}, &stdout, &stderr)
		if err != nil {
			t.Fatalf("Expected no error with session override, got: %v", err)
		}

		output := stdout.String()
		if !contains(output, "Type 'help' for commands") {
			t.Errorf("Expected help message, got: %s", output)
		}
	})

	t.Run("WithStoreBackendFS", func(t *testing.T) {
		cfg := config.NewConfig()
		cmd := NewCodeReviewCommand(cfg)

		var stdout, stderr bytes.Buffer

		cmd.testMode = true
		cmd.interactive = false
		cmd.session = t.Name()
		cmd.store = "fs"

		err := cmd.Execute([]string{}, &stdout, &stderr)
		if err != nil {
			t.Logf("Got error with fs store: %v", err)
		} else {
			output := stdout.String()
			if !contains(output, "Type 'help' for commands") {
				t.Errorf("Expected help message with fs store, got: %s", output)
			}
		}
	})

	t.Run("FlagParsingEdgeCases", func(t *testing.T) {
		cfg := config.NewConfig()
		_ = NewCodeReviewCommand(cfg)

		tests := []struct {
			name    string
			args    []string
			wantErr bool
		}{
			{"empty", []string{}, false},
			{"testFlag", []string{"--test"}, false},
			{"testFlagShort", []string{"-test"}, false},
			{"interactiveFalse", []string{"--interactive=false"}, false},
			{"sessionFlag", []string{"--session", "test"}, false},
			{"storeFlag", []string{"--store", "memory"}, false},
			{"multipleFlags", []string{"--test", "--session", "test", "--store", "memory"}, false},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				cmd := NewCodeReviewCommand(cfg)
				fs := flag.NewFlagSet("test", flag.ContinueOnError)
				cmd.SetupFlags(fs)

				err := fs.Parse(tt.args)
				if (err != nil) != tt.wantErr {
					t.Errorf("Parse(%v) error = %v, wantErr %v", tt.args, err, tt.wantErr)
				}
			})
		}
	})
}

// TestSuperDocumentCommandWithVariousFlags tests the super-document command with various flag combinations
func TestSuperDocumentCommandWithVariousFlags(t *testing.T) {
	t.Parallel()

	t.Run("ShellModeFlag", func(t *testing.T) {
		ctx := context.Background()
		var stdout, stderr bytes.Buffer
		engine, err := scripting.NewEngineWithConfig(ctx, &stdout, &stderr, testutil.NewTestSessionID("super-document", t.Name()), "memory")
		if err != nil {
			t.Fatalf("NewEngineWithConfig failed: %v", err)
		}
		defer engine.Close()
		engine.SetTestMode(true)

		engine.SetGlobal("config", map[string]interface{}{"name": "super-document", "shellMode": true, "theme": map[string]interface{}{}})
		engine.SetGlobal("args", []string{"--shell"})
		engine.SetGlobal("superDocumentTemplate", "dummy template")

		script := engine.LoadScriptFromString("super-document", superDocumentScript)
		if err := engine.ExecuteScript(script); err != nil {
			t.Fatalf("failed to execute super-document script: %v", err)
		}
	})

	t.Run("InteractiveFalseWithArgs", func(t *testing.T) {
		ctx := context.Background()
		var stdout, stderr bytes.Buffer
		engine, err := scripting.NewEngineWithConfig(ctx, &stdout, &stderr, testutil.NewTestSessionID("super-document", t.Name()), "memory")
		if err != nil {
			t.Fatalf("NewEngineWithConfig failed: %v", err)
		}
		defer engine.Close()
		engine.SetTestMode(true)

		engine.SetGlobal("config", map[string]interface{}{"name": "super-document", "interactive": false, "theme": map[string]interface{}{}})
		engine.SetGlobal("args", []string{"--test"})
		engine.SetGlobal("superDocumentTemplate", "dummy template")

		script := engine.LoadScriptFromString("super-document", superDocumentScript)
		if err := engine.ExecuteScript(script); err != nil {
			t.Fatalf("failed to execute super-document script: %v", err)
		}
	})
}
