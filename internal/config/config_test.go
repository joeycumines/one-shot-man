package config

import (
	"strings"
	"testing"
)

func TestConfigParsing(t *testing.T) {
	configContent := `# Global options
verbose true
color auto

[help]
pager less
format detailed

[version]
format short`

	config, err := LoadFromReader(strings.NewReader(configContent))
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Test global options
	if value, ok := config.GetGlobalOption("verbose"); !ok || value != "true" {
		t.Errorf("Expected verbose=true, got %s (exists: %v)", value, ok)
	}

	if value, ok := config.GetGlobalOption("color"); !ok || value != "auto" {
		t.Errorf("Expected color=auto, got %s (exists: %v)", value, ok)
	}

	// Test command-specific options
	if value, ok := config.GetCommandOption("help", "pager"); !ok || value != "less" {
		t.Errorf("Expected help.pager=less, got %s (exists: %v)", value, ok)
	}

	if value, ok := config.GetCommandOption("help", "format"); !ok || value != "detailed" {
		t.Errorf("Expected help.format=detailed, got %s (exists: %v)", value, ok)
	}

	// Test fallback to global options
	if value, ok := config.GetCommandOption("help", "verbose"); !ok || value != "true" {
		t.Errorf("Expected help.verbose=true (fallback), got %s (exists: %v)", value, ok)
	}

	// Test non-existent option
	if value, ok := config.GetCommandOption("nonexistent", "option"); ok {
		t.Errorf("Expected nonexistent option to not exist, but got %s", value)
	}
}

func TestEmptyConfig(t *testing.T) {
	config, err := LoadFromReader(strings.NewReader(""))
	if err != nil {
		t.Fatalf("Failed to load empty config: %v", err)
	}

	if len(config.Global) != 0 {
		t.Errorf("Expected empty global config, got %v", config.Global)
	}

	if len(config.Commands) != 0 {
		t.Errorf("Expected empty commands config, got %v", config.Commands)
	}
}

func TestConfigWithComments(t *testing.T) {
	configContent := `# This is a comment
verbose true
# Another comment
color auto
# Command section
[help]
# Command option comment
pager less`

	config, err := LoadFromReader(strings.NewReader(configContent))
	if err != nil {
		t.Fatalf("Failed to load config with comments: %v", err)
	}

	if value, ok := config.GetGlobalOption("verbose"); !ok || value != "true" {
		t.Errorf("Expected verbose=true, got %s (exists: %v)", value, ok)
	}

	if value, ok := config.GetCommandOption("help", "pager"); !ok || value != "less" {
		t.Errorf("Expected help.pager=less, got %s (exists: %v)", value, ok)
	}
}

func TestGetScriptConfig(t *testing.T) {
	config := NewConfig()
	// Set up test configuration
	config.SetGlobalOption("script.default.timeout", "30")
	config.SetGlobalOption("script.default.verbose", "true")
	config.SetCommandOption("prompt-flow", "script.template.style", "detailed")
	config.SetCommandOption("prompt-flow", "script.behavior.auto-save", "false")
	config.SetCommandOption("prompt-flow", "script.default.timeout", "60")  // Override global

	// Test script config extraction for prompt-flow
	scriptConfig := config.GetScriptConfig("prompt-flow", "script.")
	
	expectedKeys := []string{"default.timeout", "default.verbose", "template.style", "behavior.auto-save"}
	for _, key := range expectedKeys {
		if _, exists := scriptConfig[key]; !exists {
			t.Errorf("Expected script config key '%s' to exist", key)
		}
	}
	
	// Test values with proper precedence
	if scriptConfig["default.timeout"] != "60" {
		t.Errorf("Expected command-specific timeout override '60', got '%s'", scriptConfig["default.timeout"])
	}
	
	if scriptConfig["default.verbose"] != "true" {
		t.Errorf("Expected global fallback 'true', got '%s'", scriptConfig["default.verbose"])
	}
	
	if scriptConfig["template.style"] != "detailed" {
		t.Errorf("Expected command-specific 'detailed', got '%s'", scriptConfig["template.style"])
	}
}

func TestGetTemplateOverride(t *testing.T) {
	config := NewConfig()
	
	// Test with inline template content
	config.SetCommandOption("test-command", "template.content", "Custom template content")
	
	content, exists := config.GetTemplateOverride("test-command")
	if !exists {
		t.Error("Expected template override to exist for inline content")
	}
	if content != "Custom template content" {
		t.Errorf("Expected 'Custom template content', got '%s'", content)
	}
	
	// Test with non-existent command
	_, exists = config.GetTemplateOverride("non-existent")
	if exists {
		t.Error("Expected no template override for non-existent command")
	}
}
