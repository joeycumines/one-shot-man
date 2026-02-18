package command

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestGoalDiscovery_IsolatedFromWorkingDir verifies that with autodiscovery disabled,
// no goals leak from the host filesystem regardless of the working directory.
func TestGoalDiscovery_IsolatedFromWorkingDir(t *testing.T) {
	t.Parallel()

	cfg := newIsolatedGoalConfig()
	discovery := NewGoalDiscovery(cfg)
	registry := NewDynamicGoalRegistry(nil, discovery)

	goals := registry.List()
	if len(goals) != 0 {
		t.Errorf("expected zero goals with isolated config, got %d: %v", len(goals), goals)
	}
}

// TestGoalDiscovery_CustomPathCrossPlatform verifies that custom goal paths work
// correctly with the platform's path separator and normalization rules.
func TestGoalDiscovery_CustomPathCrossPlatform(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create a goal file in a subdirectory with a known structure
	goalDir := filepath.Join(tmpDir, "my-goals")
	if err := os.MkdirAll(goalDir, 0755); err != nil {
		t.Fatalf("failed to create goal dir: %v", err)
	}

	goalJSON := `{"name": "cross-platform-test", "description": "Tests cross-platform path handling", "category": "test"}`
	if err := os.WriteFile(filepath.Join(goalDir, "cross-platform-test.json"), []byte(goalJSON), 0644); err != nil {
		t.Fatalf("failed to write goal file: %v", err)
	}

	cfg := newIsolatedGoalConfig()
	cfg.SetGlobalOption("goal.paths", goalDir)

	discovery := NewGoalDiscovery(cfg)
	registry := NewDynamicGoalRegistry(nil, discovery)

	// Must find exactly the goal we created
	goals := registry.List()
	if len(goals) != 1 {
		t.Fatalf("expected exactly 1 goal, got %d: %v", len(goals), goals)
	}
	if goals[0] != "cross-platform-test" {
		t.Errorf("expected goal name 'cross-platform-test', got %q", goals[0])
	}

	// Verify the goal can be retrieved
	goal, err := registry.Get("cross-platform-test")
	if err != nil {
		t.Fatalf("failed to get goal: %v", err)
	}
	if goal.Description != "Tests cross-platform path handling" {
		t.Errorf("wrong description: %q", goal.Description)
	}
}

// TestGoalDiscovery_PathNormalizationCrossPlatform tests that paths with
// mixed separators, trailing separators, and `.` components are handled uniformly.
func TestGoalDiscovery_PathNormalizationCrossPlatform(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	goalDir := filepath.Join(tmpDir, "goals")
	if err := os.MkdirAll(goalDir, 0755); err != nil {
		t.Fatalf("failed to create goal dir: %v", err)
	}

	goalJSON := `{"name": "norm-test", "description": "Tests normalization", "category": "test"}`
	if err := os.WriteFile(filepath.Join(goalDir, "norm-test.json"), []byte(goalJSON), 0644); err != nil {
		t.Fatalf("failed to write goal file: %v", err)
	}

	// Construct a messy path with redundant components
	messyPath := filepath.Join(tmpDir, ".", "goals") + string(filepath.Separator)
	if runtime.GOOS != "windows" {
		// On Unix, also test with //
		messyPath = tmpDir + "/./goals/"
	}

	cfg := newIsolatedGoalConfig()
	cfg.SetGlobalOption("goal.paths", messyPath)

	discovery := NewGoalDiscovery(cfg)
	paths := discovery.DiscoverGoalPaths()

	if len(paths) == 0 {
		t.Fatal("expected at least one path after normalization")
	}

	// The path should be clean (no trailing separator, no ./ components)
	for _, p := range paths {
		if strings.HasSuffix(p, string(filepath.Separator)) {
			t.Errorf("path should not have trailing separator: %q", p)
		}
		if strings.Contains(p, string(filepath.Separator)+"."+string(filepath.Separator)) {
			t.Errorf("path should not contain /./ component: %q", p)
		}
	}
}
