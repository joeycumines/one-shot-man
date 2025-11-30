package command

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadGoalFromFile_ValidJSON(t *testing.T) {
	// Create a temporary goal file
	tmpDir := t.TempDir()
	goalFile := filepath.Join(tmpDir, "test-goal.json")

	goalJSON := `{
		"name": "test-goal",
		"description": "A test goal",
		"category": "testing",
		"usage": "Test usage",
		"tuiTitle": "Test Goal",
		"tuiPrompt": "(test) > ",
		"historyFile": ".test_history",
		"enableHistory": true,
		"stateVars": {
			"testKey": "testValue"
		},
		"promptInstructions": "Test instructions",
		"promptTemplate": "Test template",
		"contextHeader": "TEST",
		"bannerTemplate": "Test Banner",
		"usageTemplate": "Test help",
		"commands": [
			{
				"name": "test",
				"type": "custom",
				"description": "Test command"
			}
		]
	}`

	err := os.WriteFile(goalFile, []byte(goalJSON), 0644)
	if err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	// Load the goal
	goal, err := LoadGoalFromFile(goalFile)
	if err != nil {
		t.Fatalf("LoadGoalFromFile failed: %v", err)
	}

	// Verify fields
	if goal.Name != "test-goal" {
		t.Errorf("Expected Name='test-goal', got '%s'", goal.Name)
	}
	if goal.Description != "A test goal" {
		t.Errorf("Expected Description='A test goal', got '%s'", goal.Description)
	}
	if goal.Category != "testing" {
		t.Errorf("Expected Category='testing', got '%s'", goal.Category)
	}
	if goal.FileName != "test-goal.json" {
		t.Errorf("Expected FileName='test-goal.json', got '%s'", goal.FileName)
	}
	if goal.Script != goalScript {
		t.Errorf("Expected Script to be set to default goalScript")
	}
	if len(goal.Commands) != 1 {
		t.Errorf("Expected 1 command, got %d", len(goal.Commands))
	}
}

func TestLoadGoalFromFile_MissingName(t *testing.T) {
	tmpDir := t.TempDir()
	goalFile := filepath.Join(tmpDir, "invalid-goal.json")

	goalJSON := `{
		"description": "Missing name"
	}`

	err := os.WriteFile(goalFile, []byte(goalJSON), 0644)
	if err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	_, err = LoadGoalFromFile(goalFile)
	if err == nil {
		t.Fatal("Expected error for missing Name, got nil")
	}
}

func TestLoadGoalFromFile_InvalidName(t *testing.T) {
	tmpDir := t.TempDir()
	goalFile := filepath.Join(tmpDir, "invalid-name.json")

	goalJSON := `{
		"name": "invalid name with spaces",
		"description": "Test"
	}`

	err := os.WriteFile(goalFile, []byte(goalJSON), 0644)
	if err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	_, err = LoadGoalFromFile(goalFile)
	if err == nil {
		t.Fatal("Expected error for invalid Name, got nil")
	}
}

func TestLoadGoalFromFile_MissingDescription(t *testing.T) {
	tmpDir := t.TempDir()
	goalFile := filepath.Join(tmpDir, "no-desc.json")

	goalJSON := `{
		"name": "valid-name"
	}`

	err := os.WriteFile(goalFile, []byte(goalJSON), 0644)
	if err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	_, err = LoadGoalFromFile(goalFile)
	if err == nil {
		t.Fatal("Expected error for missing Description, got nil")
	}
}

func TestLoadGoalFromFile_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	goalFile := filepath.Join(tmpDir, "invalid.json")

	err := os.WriteFile(goalFile, []byte("{invalid json"), 0644)
	if err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	_, err = LoadGoalFromFile(goalFile)
	if err == nil {
		t.Fatal("Expected error for invalid JSON, got nil")
	}
}

func TestLoadGoalFromFile_NonExistent(t *testing.T) {
	_, err := LoadGoalFromFile("/nonexistent/file.json")
	if err == nil {
		t.Fatal("Expected error for non-existent file, got nil")
	}
}

func TestIsValidGoalName(t *testing.T) {
	tests := []struct {
		name  string
		valid bool
	}{
		{"valid-name", true},
		{"valid123", true},
		{"Valid-Name-123", true},
		{"a", true},
		{"invalid name", false},
		{"invalid_name", false},
		{"invalid.name", false},
		{"-invalid", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidGoalName(tt.name)
			if result != tt.valid {
				t.Errorf("isValidGoalName(%q) = %v, want %v", tt.name, result, tt.valid)
			}
		})
	}
}

func TestFindGoalFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create some test files
	files := []string{
		"goal1.json",
		"goal2.json",
		"notgoal.txt",
		"GOAL3.JSON", // Test case-insensitivity
	}

	for _, f := range files {
		err := os.WriteFile(filepath.Join(tmpDir, f), []byte("{}"), 0644)
		if err != nil {
			t.Fatalf("Failed to create test file %s: %v", f, err)
		}
	}

	// Create a subdirectory (should be ignored)
	subdir := filepath.Join(tmpDir, "subdir")
	err := os.Mkdir(subdir, 0755)
	if err != nil {
		t.Fatalf("Failed to create subdirectory: %v", err)
	}

	// Find goal files
	candidates, err := FindGoalFiles(tmpDir)
	if err != nil {
		t.Fatalf("FindGoalFiles failed: %v", err)
	}

	// Should find 3 JSON files (goal1, goal2, GOAL3)
	if len(candidates) != 3 {
		t.Errorf("Expected 3 candidates, got %d", len(candidates))
	}

	// Verify goal names
	expectedNames := map[string]bool{
		"goal1": false,
		"goal2": false,
		"GOAL3": false,
	}

	for _, candidate := range candidates {
		expectedNames[candidate.Name] = true
	}

	for name, found := range expectedNames {
		if !found {
			t.Errorf("Expected to find goal '%s', but didn't", name)
		}
	}
}

func TestFindGoalFiles_NonExistentDir(t *testing.T) {
	candidates, err := FindGoalFiles("/nonexistent/directory")
	if err != nil {
		t.Fatalf("Expected no error for non-existent directory, got: %v", err)
	}
	if len(candidates) != 0 {
		t.Errorf("Expected 0 candidates, got %d", len(candidates))
	}
}
