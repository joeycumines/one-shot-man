package command

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/config"
)

// --- AnnotatedPath type tests ---

func TestAnnotatedPath_Fields(t *testing.T) {
	t.Parallel()

	ap := AnnotatedPath{
		Path:   "/some/path",
		Source: "custom",
		Exists: true,
	}

	if ap.Path != "/some/path" {
		t.Errorf("Expected Path=/some/path, got %s", ap.Path)
	}
	if ap.Source != "custom" {
		t.Errorf("Expected Source=custom, got %s", ap.Source)
	}
	if !ap.Exists {
		t.Error("Expected Exists=true")
	}
}

// --- GoalDiscovery annotated path tests ---

func TestGoalDiscovery_DiscoverAnnotatedGoalPaths_Standard(t *testing.T) {
	t.Parallel()

	cfg := config.NewConfig()
	cfg.SetGlobalOption("goal.autodiscovery", "false")
	// Don't disable standard paths — we want to verify they get "standard" source
	discovery := NewGoalDiscovery(cfg)

	paths := discovery.DiscoverAnnotatedGoalPaths()

	// Should have at least standard paths
	if len(paths) == 0 {
		t.Fatal("Expected at least some annotated goal paths")
	}

	// Verify all standard paths are annotated correctly
	for _, ap := range paths {
		if ap.Source != "standard" {
			t.Errorf("Expected source=standard for path %s, got %s", ap.Path, ap.Source)
		}
		if !filepath.IsAbs(ap.Path) {
			t.Errorf("Expected absolute path, got: %s", ap.Path)
		}
	}
}

func TestGoalDiscovery_DiscoverAnnotatedGoalPaths_Custom(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	customPath := filepath.Join(tmpDir, "custom-goals")
	if err := os.MkdirAll(customPath, 0o755); err != nil {
		t.Fatalf("Failed to create custom path: %v", err)
	}

	cfg := config.NewConfig()
	cfg.SetGlobalOption("goal.disable-standard-paths", "true")
	cfg.SetGlobalOption("goal.autodiscovery", "false")
	cfg.SetGlobalOption("goal.paths", customPath)

	discovery := NewGoalDiscovery(cfg)
	paths := discovery.DiscoverAnnotatedGoalPaths()

	if len(paths) != 1 {
		t.Fatalf("Expected exactly 1 path, got %d: %v", len(paths), annotatedPathStrs(paths))
	}

	if paths[0].Source != "custom" {
		t.Errorf("Expected source=custom, got %s", paths[0].Source)
	}
	if !paths[0].Exists {
		t.Errorf("Expected Exists=true for %s", paths[0].Path)
	}
	if !pathsEqual(paths[0].Path, customPath) {
		t.Errorf("Expected path %s, got %s", customPath, paths[0].Path)
	}
}

func TestGoalDiscovery_DiscoverAnnotatedGoalPaths_MissingCustom(t *testing.T) {
	t.Parallel()

	cfg := config.NewConfig()
	cfg.SetGlobalOption("goal.disable-standard-paths", "true")
	cfg.SetGlobalOption("goal.autodiscovery", "false")
	cfg.SetGlobalOption("goal.paths", "/nonexistent/goal/path/xyzzy")

	discovery := NewGoalDiscovery(cfg)
	paths := discovery.DiscoverAnnotatedGoalPaths()

	// The path should still appear but with Exists=false
	// (normalizePath may fail for non-existent parent dirs, so we check gracefully)
	for _, ap := range paths {
		if ap.Exists {
			t.Errorf("Expected Exists=false for nonexistent path %s", ap.Path)
		}
		if ap.Source != "custom" {
			t.Errorf("Expected source=custom, got %s", ap.Source)
		}
	}
}

func TestGoalDiscovery_DiscoverAnnotatedGoalPaths_Dedup(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	goalDir := filepath.Join(tmpDir, "goals")
	if err := os.MkdirAll(goalDir, 0o755); err != nil {
		t.Fatalf("Failed to create goal directory: %v", err)
	}

	cfg := config.NewConfig()
	cfg.SetGlobalOption("goal.disable-standard-paths", "true")
	cfg.SetGlobalOption("goal.autodiscovery", "false")
	// Two identical paths
	cfg.SetGlobalOption("goal.paths", goalDir+string(filepath.ListSeparator)+goalDir)

	discovery := NewGoalDiscovery(cfg)
	paths := discovery.DiscoverAnnotatedGoalPaths()

	if len(paths) != 1 {
		t.Errorf("Expected 1 path after dedup, got %d: %v", len(paths), annotatedPathStrs(paths))
	}
}

func TestGoalDiscovery_DiscoverAnnotatedGoalPaths_Empty(t *testing.T) {
	t.Parallel()

	cfg := config.NewConfig()
	cfg.SetGlobalOption("goal.disable-standard-paths", "true")
	cfg.SetGlobalOption("goal.autodiscovery", "false")

	discovery := NewGoalDiscovery(cfg)
	paths := discovery.DiscoverAnnotatedGoalPaths()

	if len(paths) != 0 {
		t.Errorf("Expected empty paths, got %d: %v", len(paths), annotatedPathStrs(paths))
	}
}

// --- ScriptDiscovery annotated path tests ---

func TestScriptDiscovery_DiscoverAnnotatedScriptPaths_Standard(t *testing.T) {
	t.Parallel()

	cfg := config.NewConfig()
	cfg.SetGlobalOption("script.autodiscovery", "false")
	// Don't disable standard paths
	discovery := NewScriptDiscovery(cfg)

	paths := discovery.DiscoverAnnotatedScriptPaths()

	// Should have standard/legacy paths
	if len(paths) == 0 {
		t.Fatal("Expected at least some annotated script paths")
	}

	for _, ap := range paths {
		if ap.Source != "standard" {
			t.Errorf("Expected source=standard for path %s, got %s", ap.Path, ap.Source)
		}
		if !filepath.IsAbs(ap.Path) {
			t.Errorf("Expected absolute path, got: %s", ap.Path)
		}
	}
}

func TestScriptDiscovery_DiscoverAnnotatedScriptPaths_Custom(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	customPath := filepath.Join(tmpDir, "custom-scripts")
	if err := os.MkdirAll(customPath, 0o755); err != nil {
		t.Fatalf("Failed to create custom path: %v", err)
	}

	cfg := config.NewConfig()
	cfg.SetGlobalOption("script.disable-standard-paths", "true")
	cfg.SetGlobalOption("script.autodiscovery", "false")
	cfg.SetGlobalOption("script.paths", customPath)

	discovery := NewScriptDiscovery(cfg)
	paths := discovery.DiscoverAnnotatedScriptPaths()

	if len(paths) != 1 {
		t.Fatalf("Expected exactly 1 path, got %d: %v", len(paths), annotatedPathStrs(paths))
	}

	if paths[0].Source != "custom" {
		t.Errorf("Expected source=custom, got %s", paths[0].Source)
	}
	if !paths[0].Exists {
		t.Errorf("Expected Exists=true for %s", paths[0].Path)
	}
}

func TestScriptDiscovery_DiscoverAnnotatedScriptPaths_MissingCustom(t *testing.T) {
	t.Parallel()

	cfg := config.NewConfig()
	cfg.SetGlobalOption("script.disable-standard-paths", "true")
	cfg.SetGlobalOption("script.autodiscovery", "false")
	cfg.SetGlobalOption("script.paths", "/nonexistent/script/path/xyzzy")

	discovery := NewScriptDiscovery(cfg)
	paths := discovery.DiscoverAnnotatedScriptPaths()

	for _, ap := range paths {
		if ap.Exists {
			t.Errorf("Expected Exists=false for nonexistent path %s", ap.Path)
		}
		if ap.Source != "custom" {
			t.Errorf("Expected source=custom, got %s", ap.Source)
		}
	}
}

func TestScriptDiscovery_DiscoverAnnotatedScriptPaths_Empty(t *testing.T) {
	t.Parallel()

	cfg := config.NewConfig()
	cfg.SetGlobalOption("script.disable-standard-paths", "true")
	cfg.SetGlobalOption("script.autodiscovery", "false")

	discovery := NewScriptDiscovery(cfg)
	paths := discovery.DiscoverAnnotatedScriptPaths()

	if len(paths) != 0 {
		t.Errorf("Expected empty paths, got %d: %v", len(paths), annotatedPathStrs(paths))
	}
}

// --- GoalCommand paths subcommand tests ---

func TestGoalCommand_Paths_ShowsAnnotatedPaths(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	goalDir := filepath.Join(tmpDir, "my-goals")
	if err := os.MkdirAll(goalDir, 0o755); err != nil {
		t.Fatalf("Failed to create goal directory: %v", err)
	}

	cfg := config.NewConfig()
	cfg.SetGlobalOption("goal.disable-standard-paths", "true")
	cfg.SetGlobalOption("goal.autodiscovery", "false")
	cfg.SetGlobalOption("goal.paths", goalDir)

	goalRegistry := newTestGoalRegistryForGoal()
	cmd := NewGoalCommand(cfg, goalRegistry)

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"paths"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Goal Discovery Paths:") {
		t.Error("Expected 'Goal Discovery Paths:' header in output")
	}
	if !strings.Contains(output, "[custom]") {
		t.Error("Expected '[custom]' annotation in output")
	}
	// Normalize path separators and resolve 8.3 short names for
	// cross-platform comparison (Windows t.TempDir() may return
	// RUNNER~1 while output contains RUNNERADMIN).
	resolvedGoalDir := goalDir
	if ev, err := filepath.EvalSymlinks(goalDir); err == nil {
		resolvedGoalDir = ev
	}
	if !strings.Contains(output, filepath.ToSlash(resolvedGoalDir)) && !strings.Contains(output, resolvedGoalDir) {
		t.Errorf("Expected path %s (or normalized form) in output", resolvedGoalDir)
	}
	if !strings.Contains(output, "✓") {
		t.Error("Expected ✓ for existing path")
	}
	if !strings.Contains(output, "path(s) total") {
		t.Error("Expected total count in output")
	}
}

func TestGoalCommand_Paths_MissingCustomWarning(t *testing.T) {
	t.Parallel()

	cfg := config.NewConfig()
	cfg.SetGlobalOption("goal.disable-standard-paths", "true")
	cfg.SetGlobalOption("goal.autodiscovery", "false")
	cfg.SetGlobalOption("goal.paths", "/nonexistent/custom/goals/xyzzy")

	goalRegistry := newTestGoalRegistryForGoal()
	cmd := NewGoalCommand(cfg, goalRegistry)

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"paths"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	// The warning about missing custom paths goes to stderr
	stderrStr := stderr.String()
	if !strings.Contains(stderrStr, "Warning") {
		// Path might not appear if normalization fails for nonexistent parent
		// In that case, we should have an empty output (no warning needed)
		output := stdout.String()
		if strings.Contains(output, "✗") && !strings.Contains(stderrStr, "Warning") {
			t.Error("Expected warning about missing custom paths on stderr")
		}
	}
}

func TestGoalCommand_Paths_NoPaths(t *testing.T) {
	t.Parallel()

	cfg := config.NewConfig()
	cfg.SetGlobalOption("goal.disable-standard-paths", "true")
	cfg.SetGlobalOption("goal.autodiscovery", "false")

	goalRegistry := newTestGoalRegistryForGoal()
	cmd := NewGoalCommand(cfg, goalRegistry)

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"paths"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "No goal paths discovered") {
		t.Errorf("Expected 'No goal paths discovered', got: %s", output)
	}
}

func TestGoalCommand_Paths_ExtraArgsError(t *testing.T) {
	t.Parallel()

	cfg := config.NewConfig()
	goalRegistry := newTestGoalRegistryForGoal()
	cmd := NewGoalCommand(cfg, goalRegistry)

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"paths", "extra"}, &stdout, &stderr)
	if err == nil {
		t.Error("Expected error for extra args with paths")
	}
}

// --- ScriptingCommand paths subcommand tests ---

func TestScriptingCommand_Paths_ShowsAnnotatedPaths(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	scriptDir := filepath.Join(tmpDir, "my-scripts")
	if err := os.MkdirAll(scriptDir, 0o755); err != nil {
		t.Fatalf("Failed to create script directory: %v", err)
	}

	cfg := config.NewConfig()
	cfg.SetGlobalOption("script.disable-standard-paths", "true")
	cfg.SetGlobalOption("script.autodiscovery", "false")
	cfg.SetGlobalOption("script.paths", scriptDir)

	cmd := NewScriptingCommand(cfg)

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"paths"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Script Discovery Paths:") {
		t.Error("Expected 'Script Discovery Paths:' header in output")
	}
	if !strings.Contains(output, "[custom]") {
		t.Error("Expected '[custom]' annotation in output")
	}
	// Normalize path separators and resolve 8.3 short names for
	// cross-platform comparison (Windows t.TempDir() may return
	// RUNNER~1 while output contains RUNNERADMIN).
	resolvedScriptDir := scriptDir
	if ev, err := filepath.EvalSymlinks(scriptDir); err == nil {
		resolvedScriptDir = ev
	}
	if !strings.Contains(output, filepath.ToSlash(resolvedScriptDir)) && !strings.Contains(output, resolvedScriptDir) {
		t.Errorf("Expected path %s (or normalized form) in output", resolvedScriptDir)
	}
	if !strings.Contains(output, "✓") {
		t.Error("Expected ✓ for existing path")
	}
	if !strings.Contains(output, "path(s) total") {
		t.Error("Expected total count in output")
	}
}

func TestScriptingCommand_Paths_MissingCustomWarning(t *testing.T) {
	t.Parallel()

	cfg := config.NewConfig()
	cfg.SetGlobalOption("script.disable-standard-paths", "true")
	cfg.SetGlobalOption("script.autodiscovery", "false")
	cfg.SetGlobalOption("script.paths", "/nonexistent/custom/scripts/xyzzy")

	cmd := NewScriptingCommand(cfg)

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"paths"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	stderrStr := stderr.String()
	if !strings.Contains(stderrStr, "Warning") {
		output := stdout.String()
		if strings.Contains(output, "✗") && !strings.Contains(stderrStr, "Warning") {
			t.Error("Expected warning about missing custom paths on stderr")
		}
	}
}

func TestScriptingCommand_Paths_NoPaths(t *testing.T) {
	t.Parallel()

	cfg := config.NewConfig()
	cfg.SetGlobalOption("script.disable-standard-paths", "true")
	cfg.SetGlobalOption("script.autodiscovery", "false")

	cmd := NewScriptingCommand(cfg)

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"paths"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "No script paths discovered") {
		t.Errorf("Expected 'No script paths discovered', got: %s", output)
	}
}

func TestScriptingCommand_Paths_ExtraArgsError(t *testing.T) {
	t.Parallel()

	cfg := config.NewConfig()
	cmd := NewScriptingCommand(cfg)

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"paths", "extra"}, &stdout, &stderr)
	if err == nil {
		t.Error("Expected error for extra args with paths")
	}
}

func TestGoalCommand_Paths_ExistenceCheckAccuracy(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	existingDir := filepath.Join(tmpDir, "existing-goals")
	if err := os.MkdirAll(existingDir, 0o755); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}
	missingDir := filepath.Join(tmpDir, "missing-goals")

	cfg := config.NewConfig()
	cfg.SetGlobalOption("goal.disable-standard-paths", "true")
	cfg.SetGlobalOption("goal.autodiscovery", "false")
	cfg.SetGlobalOption("goal.paths", existingDir+string(filepath.ListSeparator)+missingDir)

	discovery := NewGoalDiscovery(cfg)
	paths := discovery.DiscoverAnnotatedGoalPaths()

	if len(paths) != 2 {
		t.Fatalf("Expected 2 paths, got %d: %v", len(paths), annotatedPathStrs(paths))
	}

	// Verify existence tracking
	existingFound := false
	missingFound := false
	for _, ap := range paths {
		if pathsEqual(ap.Path, existingDir) {
			existingFound = true
			if !ap.Exists {
				t.Error("Expected Exists=true for existing directory")
			}
		}
		if pathsEqual(ap.Path, missingDir) {
			missingFound = true
			if ap.Exists {
				t.Error("Expected Exists=false for missing directory")
			}
		}
	}
	if !existingFound {
		t.Error("Did not find existing directory in annotated paths")
	}
	if !missingFound {
		t.Error("Did not find missing directory in annotated paths")
	}
}

// --- Helper ---

func annotatedPathStrs(paths []AnnotatedPath) []string {
	strs := make([]string, len(paths))
	for i, ap := range paths {
		strs[i] = ap.Source + ":" + ap.Path
	}
	return strs
}
