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

func TestNewConfig(t *testing.T) {
	config := NewConfig()
	if config == nil {
		t.Fatal("NewConfig returned nil")
	}
	if config.Global == nil {
		t.Error("NewConfig should initialize Global map")
	}
	if config.Commands == nil {
		t.Error("NewConfig should initialize Commands map")
	}
	if len(config.Global) != 0 {
		t.Error("NewConfig should create empty Global map")
	}
	if len(config.Commands) != 0 {
		t.Error("NewConfig should create empty Commands map")
	}
}

func TestSetGlobalOption(t *testing.T) {
	config := NewConfig()
	config.SetGlobalOption("test", "value")

	if value, exists := config.GetGlobalOption("test"); !exists || value != "value" {
		t.Errorf("Expected test=value, got %s (exists: %v)", value, exists)
	}
}

func TestSetCommandOption(t *testing.T) {
	config := NewConfig()
	config.SetCommandOption("testcmd", "option", "value")

	if value, exists := config.GetCommandOption("testcmd", "option"); !exists || value != "value" {
		t.Errorf("Expected testcmd.option=value, got %s (exists: %v)", value, exists)
	}

	// Test that it creates the command map if it doesn't exist
	if config.Commands["testcmd"] == nil {
		t.Error("Expected command map to be created")
	}
}

func TestConfigOptionWithSpaces(t *testing.T) {
	configContent := `description This is a description with spaces
[test]
multiword option value with multiple words`

	config, err := LoadFromReader(strings.NewReader(configContent))
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if value, ok := config.GetGlobalOption("description"); !ok || value != "This is a description with spaces" {
		t.Errorf("Expected full description, got %s (exists: %v)", value, ok)
	}

	if value, ok := config.GetCommandOption("test", "multiword"); !ok || value != "option value with multiple words" {
		t.Errorf("Expected full multiword value, got %s (exists: %v)", value, ok)
	}
}

func TestConfigWithEmptyValues(t *testing.T) {
	configContent := `empty
blank_line 

[section]
empty_in_section`

	config, err := LoadFromReader(strings.NewReader(configContent))
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if value, ok := config.GetGlobalOption("empty"); !ok || value != "" {
		t.Errorf("Expected empty='' got %s (exists: %v)", value, ok)
	}

	if value, ok := config.GetGlobalOption("blank_line"); !ok || value != "" {
		t.Errorf("Expected blank_line='' got %s (exists: %v)", value, ok)
	}

	if value, ok := config.GetCommandOption("section", "empty_in_section"); !ok || value != "" {
		t.Errorf("Expected section.empty_in_section='' got %s (exists: %v)", value, ok)
	}
}

func TestLoadFromPathNonexistent(t *testing.T) {
	config, err := LoadFromPath("/nonexistent/path/config.txt")
	if err != nil {
		t.Fatalf("Expected no error for nonexistent file, got: %v", err)
	}
	if config == nil {
		t.Error("Expected valid config for nonexistent file")
	}
	if len(config.Global) != 0 || len(config.Commands) != 0 {
		t.Error("Expected empty config for nonexistent file")
	}
}
