package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetTemplateOverrideFromFile(t *testing.T) {
	// Create temporary file
	tmpDir := os.TempDir()
	templateFile := filepath.Join(tmpDir, "test-template.md")
	templateContent := "Test template content with {{goal}} and {{context_txtar}}"
	
	err := os.WriteFile(templateFile, []byte(templateContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test template file: %v", err)
	}
	defer os.Remove(templateFile)
	
	config := NewConfig()
	config.SetCommandOption("test-command", "template.file", templateFile)
	
	content, exists := config.GetTemplateOverride("test-command")
	if !exists {
		t.Error("Expected template override to exist for file path")
	}
	if content != templateContent {
		t.Errorf("Expected '%s', got '%s'", templateContent, content)
	}
}

func TestGetTemplateOverridePrecedence(t *testing.T) {
	// Create temporary file
	tmpDir := os.TempDir()
	templateFile := filepath.Join(tmpDir, "test-template.md")
	fileContent := "File template content"
	inlineContent := "Inline template content"
	
	err := os.WriteFile(templateFile, []byte(fileContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test template file: %v", err)
	}
	defer os.Remove(templateFile)
	
	config := NewConfig()
	// Set both file and inline content - file should take precedence
	config.SetCommandOption("test-command", "template.file", templateFile)
	config.SetCommandOption("test-command", "template.content", inlineContent)
	
	content, exists := config.GetTemplateOverride("test-command")
	if !exists {
		t.Error("Expected template override to exist")
	}
	if content != fileContent {
		t.Errorf("Expected file content to take precedence: '%s', got '%s'", fileContent, content)
	}
}