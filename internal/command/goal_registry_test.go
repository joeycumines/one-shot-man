package command

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/config"
)

func TestDynamicGoalRegistry_List(t *testing.T) {
	// Create a minimal built-in goal
	builtIn := []Goal{
		{
			Name:        "builtin-goal",
			Description: "Built-in goal",
			Category:    "test",
			Script:      "test-script",
			FileName:    "builtin.js",
		},
	}

	// Create a mock discovery that returns no paths
	cfg := config.NewConfig()
	discovery := NewGoalDiscovery(cfg)

	registry := NewDynamicGoalRegistry(builtIn, discovery)

	goals := registry.List()

	// Should have at least the built-in goal
	if len(goals) == 0 {
		t.Fatal("Expected at least one goal, got none")
	}

	found := false
	for _, name := range goals {
		if name == "builtin-goal" {
			found = true
			break
		}
	}

	if !found {
		t.Error("Expected to find 'builtin-goal' in list")
	}
}

func TestDynamicGoalRegistry_Get(t *testing.T) {
	builtIn := []Goal{
		{
			Name:        "test-goal",
			Description: "Test goal",
			Category:    "test",
			Script:      "test-script",
			FileName:    "test.js",
		},
	}

	cfg := config.NewConfig()
	discovery := NewGoalDiscovery(cfg)
	registry := NewDynamicGoalRegistry(builtIn, discovery)

	goal, err := registry.Get("test-goal")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if goal.Name != "test-goal" {
		t.Errorf("Expected Name='test-goal', got '%s'", goal.Name)
	}
}

func TestDynamicGoalRegistry_GetNonExistent(t *testing.T) {
	cfg := config.NewConfig()
	discovery := NewGoalDiscovery(cfg)
	registry := NewDynamicGoalRegistry([]Goal{}, discovery)

	_, err := registry.Get("nonexistent")
	if err == nil {
		t.Fatal("Expected error for non-existent goal, got nil")
	}
}

func TestDynamicGoalRegistry_UserOverridesBuiltIn(t *testing.T) {
	// Create temp directory for user goal
	tmpDir := t.TempDir()

	// Create a user goal that overrides a built-in
	userGoalFile := filepath.Join(tmpDir, "override-goal.json")
	userGoalJSON := `{
		"Name": "override-goal",
		"Description": "User version",
		"Category": "user"
	}`

	err := os.WriteFile(userGoalFile, []byte(userGoalJSON), 0644)
	if err != nil {
		t.Fatalf("Failed to write user goal: %v", err)
	}

	// Create built-in goal with same name
	builtIn := []Goal{
		{
			Name:        "override-goal",
			Description: "Built-in version",
			Category:    "builtin",
			Script:      "builtin-script",
			FileName:    "builtin.js",
		},
	}

	// Configure discovery to look in tmpDir
	cfg := config.NewConfig()
	cfg.SetGlobalOption("goal.paths", tmpDir)
	cfg.SetGlobalOption("goal.autodiscovery", "false")

	discovery := NewGoalDiscovery(cfg)
	registry := NewDynamicGoalRegistry(builtIn, discovery)

	// Get the goal
	goal, err := registry.Get("override-goal")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	// Should be the user version
	if goal.Description != "User version" {
		t.Errorf("Expected user version to override, got Description='%s'", goal.Description)
	}

	if goal.Category != "user" {
		t.Errorf("Expected Category='user', got '%s'", goal.Category)
	}
}

func TestDynamicGoalRegistry_GetAllGoals(t *testing.T) {
	builtIn := []Goal{
		{
			Name:        "goal1",
			Description: "First goal",
			Category:    "test",
			Script:      "script1",
			FileName:    "goal1.js",
		},
		{
			Name:        "goal2",
			Description: "Second goal",
			Category:    "test",
			Script:      "script2",
			FileName:    "goal2.js",
		},
	}

	cfg := config.NewConfig()
	discovery := NewGoalDiscovery(cfg)
	registry := NewDynamicGoalRegistry(builtIn, discovery)

	goals := registry.GetAllGoals()

	if len(goals) < 2 {
		t.Fatalf("Expected at least 2 goals, got %d", len(goals))
	}

	// Verify goals are sorted by name
	for i := 0; i < len(goals)-1; i++ {
		if goals[i].Name > goals[i+1].Name {
			t.Errorf("Goals not sorted: '%s' > '%s'", goals[i].Name, goals[i+1].Name)
		}
	}
}

func TestDynamicGoalRegistry_Reload(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := config.NewConfig()
	cfg.SetGlobalOption("goal.paths", tmpDir)
	cfg.SetGlobalOption("goal.autodiscovery", "false")

	discovery := NewGoalDiscovery(cfg)
	registry := NewDynamicGoalRegistry([]Goal{}, discovery)

	// Initially no custom goals
	initialCount := len(registry.List())

	// Add a new goal file
	goalFile := filepath.Join(tmpDir, "new-goal.json")
	goalJSON := `{
		"Name": "new-goal",
		"Description": "Newly added",
		"Category": "new"
	}`

	err := os.WriteFile(goalFile, []byte(goalJSON), 0644)
	if err != nil {
		t.Fatalf("Failed to write goal file: %v", err)
	}

	// Reload
	err = registry.Reload()
	if err != nil {
		t.Fatalf("Reload failed: %v", err)
	}

	// Should now have the new goal
	newCount := len(registry.List())
	if newCount != initialCount+1 {
		t.Errorf("Expected %d goals after reload, got %d", initialCount+1, newCount)
	}

	// Verify we can get the new goal
	goal, err := registry.Get("new-goal")
	if err != nil {
		t.Fatalf("Failed to get new goal after reload: %v", err)
	}

	if goal.Description != "Newly added" {
		t.Errorf("Expected Description='Newly added', got '%s'", goal.Description)
	}
}
