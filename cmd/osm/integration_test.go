package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/config"
)

// TestConfigIntegration tests the end-to-end configuration functionality
func TestConfigIntegration(t *testing.T) {
	// Create a temporary config file
	tmpDir, err := os.MkdirTemp("", "osm-config-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	configFile := filepath.Join(tmpDir, "config")
	configContent := `# Test configuration
verbose true
prompt.color.input blue

[prompt-flow]
script.ui.title Test Prompt Flow
script.ui.banner Test banner message
template.content Test template: {{goal}} and {{context_txtar}}

[code-review]
script.ui.title Test Code Review
script.ui.banner Test review banner
`

	err = os.WriteFile(configFile, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Load the configuration
	cfg, err := config.LoadFromPath(configFile)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Test global options
	if value, exists := cfg.GetGlobalOption("verbose"); !exists || value != "true" {
		t.Errorf("Expected verbose=true, got %s (exists: %v)", value, exists)
	}

	// Test script configuration
	promptFlowConfig := cfg.GetScriptConfig("prompt-flow", "script.")
	if promptFlowConfig["ui.title"] != "Test Prompt Flow" {
		t.Errorf("Expected 'Test Prompt Flow', got '%s'", promptFlowConfig["ui.title"])
	}

	if promptFlowConfig["ui.banner"] != "Test banner message" {
		t.Errorf("Expected 'Test banner message', got '%s'", promptFlowConfig["ui.banner"])
	}

	// Test template override
	template, exists := cfg.GetTemplateOverride("prompt-flow")
	if !exists {
		t.Error("Expected template override to exist")
	}
	if !strings.Contains(template, "Test template") {
		t.Errorf("Expected template to contain 'Test template', got: %s", template)
	}

	// Test code-review configuration
	codeReviewConfig := cfg.GetScriptConfig("code-review", "script.")
	if codeReviewConfig["ui.title"] != "Test Code Review" {
		t.Errorf("Expected 'Test Code Review', got '%s'", codeReviewConfig["ui.title"])
	}
}