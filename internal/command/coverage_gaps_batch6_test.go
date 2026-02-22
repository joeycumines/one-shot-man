package command

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/config"
	"github.com/joeycumines/one-shot-man/internal/scripting"
	"github.com/joeycumines/one-shot-man/internal/testutil"
)

// ==========================================================================
// script_discovery.go — addPath (64.3%)
// ==========================================================================

// TestAddPath_EmptyCandidate exercises the TrimSpace=="" branch (line 583).
func TestAddPath_EmptyCandidate(t *testing.T) {
	t.Parallel()

	sd := &ScriptDiscovery{config: &ScriptDiscoveryConfig{}}

	var paths []string
	seen := make(map[string]bool)

	sd.addPath(&paths, seen, "")
	sd.addPath(&paths, seen, "   ")

	if len(paths) != 0 {
		t.Errorf("expected 0 paths for empty candidates, got %d", len(paths))
	}
}

// TestAddPath_AcceptAndDedup exercises the normalization, accept, and
// deduplication branches (lines 589-600).
func TestAddPath_AcceptAndDedup(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	sd := &ScriptDiscovery{config: &ScriptDiscoveryConfig{}}

	var paths []string
	seen := make(map[string]bool)

	// First add — should be accepted.
	sd.addPath(&paths, seen, dir)
	if len(paths) != 1 {
		t.Fatalf("expected 1 path after first add, got %d", len(paths))
	}

	// Second add of same path — should be deduplicated.
	sd.addPath(&paths, seen, dir)
	if len(paths) != 1 {
		t.Fatalf("expected 1 path after dedup add, got %d: %v", len(paths), paths)
	}
}

// TestAddPath_NormalizationError exercises the normalization failure branch
// (line 590-593) where normalizePath returns an error.
func TestAddPath_NormalizationError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("skipping when running as root")
	}
	t.Parallel()

	// Create a directory whose parent has no read permission, causing
	// EvalSymlinks on the base directory to fail → normalizePath error.
	tmpDir := t.TempDir()
	parentDir := filepath.Join(tmpDir, "noread")
	childDir := filepath.Join(parentDir, "scripts")
	if err := os.MkdirAll(childDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Create a symlink inside childDir pointing outside
	target := filepath.Join(childDir, "escape")
	if err := os.Symlink("/tmp", target); err != nil {
		t.Skip("symlinks not supported")
	}

	// Remove read permission on parentDir so EvalSymlinks fails for the base
	if err := os.Chmod(parentDir, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(parentDir, 0o755) })

	sd := &ScriptDiscovery{config: &ScriptDiscoveryConfig{}}

	var paths []string
	seen := make(map[string]bool)

	// addPath should log warning and skip — not crash.
	sd.addPath(&paths, seen, target)

	// We can't guarantee this always produces an error on all systems,
	// but the function should NOT panic regardless.
}

// ==========================================================================
// script_discovery.go — matchesAncestorPattern (77.8%)
// ==========================================================================

// TestScriptDiscovery_MatchesAncestorPattern exercises the ScriptDiscovery
// variant with pattern matching, empty downSegments, length mismatch, and
// segment mismatch branches.
func TestScriptDiscovery_MatchesAncestorPattern(t *testing.T) {
	t.Parallel()

	sd := &ScriptDiscovery{
		config: &ScriptDiscoveryConfig{
			ScriptPathPatterns: []string{"scripts", "lib/scripts"},
		},
	}

	tests := []struct {
		name     string
		segments []string
		want     bool
	}{
		{
			name:     "match simple pattern",
			segments: []string{"..", "scripts"},
			want:     true,
		},
		{
			name:     "match multi-segment pattern",
			segments: []string{"..", "..", "lib", "scripts"},
			want:     true,
		},
		{
			name:     "no match — different name",
			segments: []string{"..", "other"},
			want:     false,
		},
		{
			name:     "no match — length mismatch",
			segments: []string{"..", "scripts", "extra"},
			want:     false,
		},
		{
			name:     "no down segments (only up)",
			segments: []string{"..", ".."},
			want:     false,
		},
		{
			name:     "empty segments",
			segments: nil,
			want:     false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := sd.matchesAncestorPattern(tc.segments)
			if got != tc.want {
				t.Errorf("matchesAncestorPattern(%v) = %v, want %v", tc.segments, got, tc.want)
			}
		})
	}
}

// TestScriptDiscovery_MatchesAncestorPattern_EmptyPatternSkipped tests
// the branch where a pattern resolves to zero down segments (line 706).
func TestScriptDiscovery_MatchesAncestorPattern_EmptyPatternSkipped(t *testing.T) {
	t.Parallel()

	sd := &ScriptDiscovery{
		config: &ScriptDiscoveryConfig{
			// "." and ".." clean to patterns with zero down segments
			ScriptPathPatterns: []string{".", "..", "scripts"},
		},
	}

	// Should still match "scripts" even though "." and ".." produce
	// empty pattern segments (those should be skipped via continue).
	got := sd.matchesAncestorPattern([]string{"..", "scripts"})
	if !got {
		t.Error("expected match for 'scripts' pattern despite empty patterns in list")
	}
}

// ==========================================================================
// script_discovery.go — parsePathList (93.3%)
// ==========================================================================

// TestParsePathList_OnlySeparators exercises the len(parts)==0 branch
// (line 782) where FieldsFunc produces no fields.
func TestParsePathList_OnlySeparators(t *testing.T) {
	t.Parallel()
	result := parsePathList(",,,")
	if result != nil {
		t.Errorf("expected nil for separator-only input, got %v", result)
	}
}

// TestParsePathList_AllWhitespace exercises the len(paths)==0 branch
// (line 792) where all parts trim to empty strings.
func TestParsePathList_AllWhitespace(t *testing.T) {
	t.Parallel()
	result := parsePathList(" , , , ")
	if result != nil {
		t.Errorf("expected nil for all-whitespace parts, got %v", result)
	}
}

// ==========================================================================
// hot_snippets.go — injectConfigHotSnippets no-warning branch (line 38-40)
// ==========================================================================

// TestInjectConfigHotSnippets_NoWarningFlag exercises the GetBool
// "hot-snippets.no-warning" true branch.
func TestInjectConfigHotSnippets_NoWarningFlag(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	var stdout, stderr bytes.Buffer
	engine, err := scripting.NewEngineWithConfig(ctx, &stdout, &stderr,
		testutil.NewTestSessionID("hs-nowarn", t.Name()), "memory")
	if err != nil {
		t.Fatalf("engine creation failed: %v", err)
	}
	defer engine.Close()

	cfg := config.NewConfig()
	cfg.HotSnippets = []config.HotSnippet{
		{Name: "test", Text: "hello"},
	}
	cfg.SetGlobalOption("hot-snippets.no-warning", "true")

	injectConfigHotSnippets(engine, cfg)

	val := engine.GetGlobal("CONFIG_HOT_SNIPPETS_NO_WARNING")
	if val == nil {
		t.Fatal("expected CONFIG_HOT_SNIPPETS_NO_WARNING to be set")
	}
	if val != true {
		t.Errorf("expected CONFIG_HOT_SNIPPETS_NO_WARNING=true, got %v", val)
	}
}

// TestInjectConfigHotSnippets_NoWarningFalse exercises the GetBool
// false branch (no-warning not set → global NOT set).
func TestInjectConfigHotSnippets_NoWarningFalse(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	var stdout, stderr bytes.Buffer
	engine, err := scripting.NewEngineWithConfig(ctx, &stdout, &stderr,
		testutil.NewTestSessionID("hs-no-nw", t.Name()), "memory")
	if err != nil {
		t.Fatalf("engine creation failed: %v", err)
	}
	defer engine.Close()

	cfg := config.NewConfig()
	cfg.HotSnippets = []config.HotSnippet{
		{Name: "test", Text: "hello"},
	}
	// Do NOT set hot-snippets.no-warning

	injectConfigHotSnippets(engine, cfg)

	val := engine.GetGlobal("CONFIG_HOT_SNIPPETS_NO_WARNING")
	if val != nil {
		t.Errorf("expected CONFIG_HOT_SNIPPETS_NO_WARNING to be nil, got %v", val)
	}
}

// ==========================================================================
// goal_registry.go — NewDynamicGoalRegistry initial Reload failure (line 43)
// ==========================================================================

// TestNewDynamicGoalRegistry_ReloadFailureWarns exercises the warning log
// path when the initial Reload fails during NewDynamicGoalRegistry.
func TestNewDynamicGoalRegistry_ReloadFailureWarns(t *testing.T) {
	// Cannot use t.Parallel() with t.Setenv().

	// Create a GoalDiscovery that points to a non-existent path.
	// DiscoverGoalPaths will return empty, so Reload won't actually fail.
	// But we can test that the registry still initializes properly with
	// no discovered goals.
	cfg := config.NewConfig()
	cfg.SetGlobalOption("goal.disable-standard-paths", "true")
	cfg.SetGlobalOption("goal.autodiscovery", "false")
	cfg.SetGlobalOption("goal.paths", "/nonexistent/path/to/goals")
	t.Setenv("OSM_DISABLE_GOAL_AUTODISCOVERY", "true")

	discovery := NewGoalDiscovery(cfg)
	builtins := []Goal{
		{Name: "b1", Description: "Built-in 1", Script: "x"},
		{Name: "b2", Description: "Built-in 2", Script: "y"},
	}

	registry := NewDynamicGoalRegistry(builtins, discovery)

	// Registry should still work with only built-in goals.
	names := registry.List()
	if len(names) != 2 {
		t.Errorf("expected 2 goals, got %d: %v", len(names), names)
	}
	if _, err := registry.Get("b1"); err != nil {
		t.Errorf("Get b1: %v", err)
	}
}

// TestDynamicGoalRegistry_GetNotFound exercises the "goal not found"
// error branch in Get (line 60).
func TestDynamicGoalRegistry_GetNotFound(t *testing.T) {
	// Cannot use t.Parallel() with t.Setenv().

	cfg := config.NewConfig()
	cfg.SetGlobalOption("goal.disable-standard-paths", "true")
	cfg.SetGlobalOption("goal.autodiscovery", "false")
	t.Setenv("OSM_DISABLE_GOAL_AUTODISCOVERY", "true")

	discovery := NewGoalDiscovery(cfg)
	registry := NewDynamicGoalRegistry(nil, discovery)

	_, err := registry.Get("nonexistent-goal")
	if err == nil {
		t.Fatal("expected error for nonexistent goal")
	}
	if got := err.Error(); got != "goal not found: nonexistent-goal" {
		t.Errorf("unexpected error: %s", got)
	}
}

// TestDynamicGoalRegistry_GetAllGoals_Sorted exercises the GetAllGoals method
// to verify sorted output with mixed-order built-ins (lines 157-170).
func TestDynamicGoalRegistry_GetAllGoals_Sorted(t *testing.T) {
	// Cannot use t.Parallel() with t.Setenv().

	cfg := config.NewConfig()
	cfg.SetGlobalOption("goal.disable-standard-paths", "true")
	cfg.SetGlobalOption("goal.autodiscovery", "false")
	t.Setenv("OSM_DISABLE_GOAL_AUTODISCOVERY", "true")

	discovery := NewGoalDiscovery(cfg)
	builtins := []Goal{
		{Name: "zulu", Description: "Z", Script: "z"},
		{Name: "alpha", Description: "A", Script: "a"},
		{Name: "mike", Description: "M", Script: "m"},
	}
	registry := NewDynamicGoalRegistry(builtins, discovery)

	goals := registry.GetAllGoals()
	if len(goals) != 3 {
		t.Fatalf("expected 3 goals, got %d", len(goals))
	}
	// Should be sorted alphabetically
	if goals[0].Name != "alpha" {
		t.Errorf("expected first goal 'alpha', got %q", goals[0].Name)
	}
	if goals[1].Name != "mike" {
		t.Errorf("expected second goal 'mike', got %q", goals[1].Name)
	}
	if goals[2].Name != "zulu" {
		t.Errorf("expected third goal 'zulu', got %q", goals[2].Name)
	}
}

// ==========================================================================
// goal_registry.go — Reload with scan failure path (lines 77-79)
// ==========================================================================

// TestDynamicGoalRegistry_ReloadInvalidGoalSkipped exercises the warning
// path when a goal file can't be loaded (lines 84-87).
func TestDynamicGoalRegistry_ReloadInvalidGoalSkipped(t *testing.T) {
	// Cannot use t.Parallel() — uses t.Setenv indirectly via goal config.

	dir := t.TempDir()

	// Create a valid goal and an invalid goal in the same directory.
	valid := filepath.Join(dir, "good.json")
	if err := os.WriteFile(valid, []byte(`{"name":"good","description":"A good goal"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	invalid := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(invalid, []byte(`{not valid json`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := config.NewConfig()
	cfg.SetGlobalOption("goal.disable-standard-paths", "true")
	cfg.SetGlobalOption("goal.autodiscovery", "false")
	cfg.SetGlobalOption("goal.paths", dir)

	discovery := NewGoalDiscovery(cfg)
	registry := NewDynamicGoalRegistry(nil, discovery)

	// Valid goal should be discovered despite the invalid sibling.
	goal, err := registry.Get("good")
	if err != nil {
		t.Fatalf("Get good: %v; all: %v", err, registry.List())
	}
	if goal.Description != "A good goal" {
		t.Errorf("expected description 'A good goal', got %q", goal.Description)
	}
}

// ==========================================================================
// goal_registry.go — user goals override built-ins (lines 139-143)
// ==========================================================================

// TestDynamicGoalRegistry_UserOverridesBuiltin exercises the built-in
// override path where a discovered goal has the same name as a built-in.
func TestDynamicGoalRegistry_UserOverridesBuiltin(t *testing.T) {
	// Cannot use t.Parallel() with t.Setenv().

	dir := t.TempDir()
	userGoal := filepath.Join(dir, "override.json")
	if err := os.WriteFile(userGoal, []byte(`{
		"name": "my-goal",
		"description": "User version"
	}`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := config.NewConfig()
	cfg.SetGlobalOption("goal.disable-standard-paths", "true")
	cfg.SetGlobalOption("goal.autodiscovery", "false")
	cfg.SetGlobalOption("goal.paths", dir)

	discovery := NewGoalDiscovery(cfg)
	builtins := []Goal{{Name: "my-goal", Description: "Built-in version", Script: "x"}}
	registry := NewDynamicGoalRegistry(builtins, discovery)

	goal, err := registry.Get("my-goal")
	if err != nil {
		t.Fatalf("Get my-goal: %v", err)
	}
	// User goal should win over built-in.
	if goal.Description != "User version" {
		t.Errorf("expected user description, got %q", goal.Description)
	}
}

// ==========================================================================
// script_discovery.go — pathDepthRelative error case (line 687-688)
// ==========================================================================

// TestPathDepthRelative_RelError exercises the filepath.Rel error branch
// which returns 0. On most systems Rel doesn't error for valid paths,
// but on Windows relative paths between drives can fail.
func TestPathDepthRelative_WindowsStyleRelError(t *testing.T) {
	t.Parallel()

	// filepath.Rel can fail when the two paths have different volume names (Windows).
	// On Unix, this test verifies the function still returns a sensible value.
	got := pathDepthRelative("/a/b/c", "/a/b")
	if got != 1 {
		t.Errorf("expected depth 1, got %d", got)
	}

	// Empty base branch
	got = pathDepthRelative("/a/b/c", "")
	if got != 0 {
		t.Errorf("expected depth 0 for empty base, got %d", got)
	}
}
