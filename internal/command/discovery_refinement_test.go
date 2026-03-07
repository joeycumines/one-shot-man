package command

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/config"
)

// ---------------------------------------------------------------------------
// 1. Config key enforcement tests
// ---------------------------------------------------------------------------

func TestGoalDiscovery_MaxTraversalDepthRespected(t *testing.T) {
	// Changes working directory — not parallel.

	tmpDir := t.TempDir()

	// Build 5-level tree: tmpDir/l1/l2/l3/l4/l5
	deepDir := tmpDir
	for _, name := range []string{"l1", "l2", "l3", "l4", "l5"} {
		deepDir = filepath.Join(deepDir, name)
	}
	if err := os.MkdirAll(deepDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	// Goal at level 1 (reachable within depth 2 from l5).
	goalLevel1 := filepath.Join(tmpDir, "l1", "osm-goals")
	if err := os.Mkdir(goalLevel1, 0o755); err != nil {
		t.Fatalf("Mkdir level1 goals: %v", err)
	}

	// Goal at level 4 (NOT reachable within depth 2 from l5 — needs 4 levels up).
	// l5 → l4 (1) → l3 (2) → l2 (3) → l1 (4, goal here).
	// Wait — let me reconsider: from l5, going up 1 is l4, up 2 is l3.
	// So depth=2 means we check l5 and l4 only (indices 0 and 1 in the loop).
	// Goal at level 4 relative to root: tmpDir/l1/l2/l3/l4/osm-goals.
	// From l5, that is 1 level up — reachable at depth 2.
	// Goal at level 1: tmpDir/l1/osm-goals → from l5 that is 4 levels up, NOT reachable.
	// So let's put the unreachable goal at level 1 and the reachable goal at level 4.
	goalLevel4 := filepath.Join(tmpDir, "l1", "l2", "l3", "l4", "osm-goals")
	if err := os.Mkdir(goalLevel4, 0o755); err != nil {
		t.Fatalf("Mkdir level4 goals: %v", err)
	}

	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	defer os.Chdir(origWd)

	if err := os.Chdir(deepDir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}

	cfg := config.NewConfig()
	cfg.SetGlobalOption("goal.disable-standard-paths", "true")
	cfg.SetGlobalOption("goal.autodiscovery", "true")
	cfg.SetGlobalOption("goal.max-traversal-depth", "2")
	cfg.SetGlobalOption("goal.path-patterns", "osm-goals")

	discovery := NewGoalDiscovery(cfg)
	paths := discovery.DiscoverGoalPaths()

	// level4 goals (1 level up from l5) — SHOULD be found.
	foundLevel4 := false
	// level1 goals (4 levels up from l5) — should NOT be found.
	foundLevel1 := false

	for _, p := range paths {
		if pathsEqual(p, goalLevel4) {
			foundLevel4 = true
		}
		if pathsEqual(p, goalLevel1) {
			foundLevel1 = true
		}
	}

	if !foundLevel4 {
		t.Errorf("Expected to find goal at level 4 (%s) within depth 2, paths: %v", goalLevel4, paths)
	}
	if foundLevel1 {
		t.Errorf("Should NOT find goal at level 1 (%s) when depth is limited to 2, paths: %v", goalLevel1, paths)
	}
}

func TestScriptDiscovery_MaxTraversalDepthRespected(t *testing.T) {
	// Changes working directory — not parallel.

	tmpDir := t.TempDir()

	deepDir := tmpDir
	for _, name := range []string{"l1", "l2", "l3", "l4", "l5"} {
		deepDir = filepath.Join(deepDir, name)
	}
	if err := os.MkdirAll(deepDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	// Script dir at level 4 (1 up from l5) — reachable at depth 2.
	scriptsLevel4 := filepath.Join(tmpDir, "l1", "l2", "l3", "l4", "scripts")
	if err := os.Mkdir(scriptsLevel4, 0o755); err != nil {
		t.Fatalf("Mkdir level4 scripts: %v", err)
	}

	// Script dir at level 1 (4 up from l5) — NOT reachable at depth 2.
	scriptsLevel1 := filepath.Join(tmpDir, "l1", "scripts")
	if err := os.Mkdir(scriptsLevel1, 0o755); err != nil {
		t.Fatalf("Mkdir level1 scripts: %v", err)
	}

	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	defer os.Chdir(origWd)

	if err := os.Chdir(deepDir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}

	cfg := config.NewConfig()
	cfg.SetGlobalOption("script.disable-standard-paths", "true")
	cfg.SetGlobalOption("script.autodiscovery", "true")
	cfg.SetGlobalOption("script.max-traversal-depth", "2")
	cfg.SetGlobalOption("script.path-patterns", "scripts")

	discovery := NewScriptDiscovery(cfg)
	paths := discovery.DiscoverScriptPaths()

	foundLevel4 := false
	foundLevel1 := false
	for _, p := range paths {
		if pathsEqual(p, scriptsLevel4) {
			foundLevel4 = true
		}
		if pathsEqual(p, scriptsLevel1) {
			foundLevel1 = true
		}
	}

	if !foundLevel4 {
		t.Errorf("Expected to find scripts at level 4, paths: %v", paths)
	}
	if foundLevel1 {
		t.Errorf("Should NOT find scripts at level 1 within depth 2, paths: %v", paths)
	}
}

func TestGoalDiscovery_CustomPathPatternsOverride(t *testing.T) {
	// Changes working directory — not parallel.

	tmpDir := t.TempDir()

	defaultDir := filepath.Join(tmpDir, "osm-goals")
	customDir := filepath.Join(tmpDir, "my-goals")
	for _, d := range []string{defaultDir, customDir} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatalf("Mkdir %s: %v", d, err)
		}
	}

	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	defer os.Chdir(origWd)

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}

	cfg := config.NewConfig()
	cfg.SetGlobalOption("goal.disable-standard-paths", "true")
	cfg.SetGlobalOption("goal.autodiscovery", "true")
	cfg.SetGlobalOption("goal.path-patterns", "my-goals")

	discovery := NewGoalDiscovery(cfg)
	paths := discovery.DiscoverGoalPaths()

	foundCustom := false
	foundDefault := false
	for _, p := range paths {
		if pathsEqual(p, customDir) {
			foundCustom = true
		}
		if pathsEqual(p, defaultDir) {
			foundDefault = true
		}
	}

	if !foundCustom {
		t.Errorf("Expected custom pattern 'my-goals' to be discovered, paths: %v", paths)
	}
	if foundDefault {
		t.Errorf("Default pattern 'osm-goals' should NOT be found when overridden, paths: %v", paths)
	}
}

func TestGoalDiscovery_EnvVarOverride(t *testing.T) {
	// Sets env vars — NOT parallel.

	tmpDir := t.TempDir()
	goalDir := filepath.Join(tmpDir, "osm-goals")
	if err := os.Mkdir(goalDir, 0o755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}

	t.Setenv("OSM_DISABLE_GOAL_AUTODISCOVERY", "true")

	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	defer os.Chdir(origWd)

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}

	cfg := config.NewConfig()
	cfg.SetGlobalOption("goal.disable-standard-paths", "true")
	cfg.SetGlobalOption("goal.autodiscovery", "true") // would enable, but env overrides

	discovery := NewGoalDiscovery(cfg)

	if discovery.config.EnableAutodiscovery {
		t.Fatal("Expected autodiscovery to be disabled by OSM_DISABLE_GOAL_AUTODISCOVERY")
	}

	paths := discovery.DiscoverGoalPaths()

	for _, p := range paths {
		if pathsEqual(p, goalDir) {
			t.Errorf("Autodiscovery should be disabled — should not find %s, paths: %v", goalDir, paths)
		}
	}
}

// ---------------------------------------------------------------------------
// 2. Symlink traversal edge cases
// ---------------------------------------------------------------------------

func TestGoalDiscovery_SymlinkToParentCreatesUpwardCycle(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Symlink tests not reliable on Windows")
	}
	// Does NOT need chdir — we call traverseForGoalDirs directly.
	t.Parallel()

	tmpDir := t.TempDir()

	// Structure: tmpDir/real/sub/
	realDir := filepath.Join(tmpDir, "real")
	subDir := filepath.Join(realDir, "sub")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	// Symlink: tmpDir/real/sub/link -> tmpDir/real/
	linkPath := filepath.Join(subDir, "link")
	if err := os.Symlink(realDir, linkPath); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	// Use the UNRESOLVED symlink path as the start directory.
	// On macOS, os.Getwd() resolves symlinks, so chdir won't preserve the
	// unresolved path. We bypass that by calling traverseForGoalDirs directly.
	//
	// Trace: startDir = .../real/sub/link/sub
	//   i=0: EvalSymlinks(.../real/sub/link/sub) = .../real/sub → visited
	//        parent = Dir(.../real/sub/link/sub) = .../real/sub/link
	//   i=1: EvalSymlinks(.../real/sub/link) = .../real → visited
	//        parent = Dir(.../real/sub/link) = .../real/sub
	//   i=2: EvalSymlinks(.../real/sub) = .../real/sub → CYCLE DETECTED
	startDir := filepath.Join(linkPath, "sub")

	var mu sync.Mutex
	var messages []string

	cfg := config.NewConfig()
	cfg.SetGlobalOption("goal.disable-standard-paths", "true")
	cfg.SetGlobalOption("goal.autodiscovery", "true")
	cfg.SetGlobalOption("goal.path-patterns", "osm-goals")
	cfg.SetGlobalOption("goal.max-traversal-depth", "20")

	discovery := NewGoalDiscovery(cfg)
	discovery.config.DebugLogFunc = func(format string, args ...any) {
		mu.Lock()
		defer mu.Unlock()
		messages = append(messages, fmt.Sprintf(format, args...))
	}

	// Call traverseForGoalDirs directly with the unresolved path.
	// Must not infinite-loop.
	paths := discovery.traverseForGoalDirs(startDir)
	_ = paths

	mu.Lock()
	defer mu.Unlock()

	foundCycle := false
	for _, msg := range messages {
		if strings.Contains(msg, "symlink cycle detected") {
			foundCycle = true
			break
		}
	}
	if !foundCycle {
		t.Error("Expected 'symlink cycle detected' in debug log, got messages:")
		for _, m := range messages {
			t.Logf("  %s", m)
		}
	}
}

func TestScriptDiscovery_SymlinkCycleInTraversal(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Symlink tests not reliable on Windows")
	}
	// Does NOT need chdir — we call traverseForScriptDirs directly.
	t.Parallel()

	tmpDir := t.TempDir()

	realDir := filepath.Join(tmpDir, "real")
	subDir := filepath.Join(realDir, "sub")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	linkPath := filepath.Join(subDir, "link")
	if err := os.Symlink(realDir, linkPath); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	// Use unresolved symlink path to trigger cycle detection.
	startDir := filepath.Join(linkPath, "sub")

	var mu sync.Mutex
	var messages []string

	cfg := config.NewConfig()
	cfg.SetGlobalOption("script.disable-standard-paths", "true")
	cfg.SetGlobalOption("script.autodiscovery", "true")
	cfg.SetGlobalOption("script.path-patterns", "scripts")
	cfg.SetGlobalOption("script.max-traversal-depth", "20")

	discovery := NewScriptDiscovery(cfg)
	discovery.config.DebugLogFunc = func(format string, args ...any) {
		mu.Lock()
		defer mu.Unlock()
		messages = append(messages, fmt.Sprintf(format, args...))
	}

	// Call traverseForScriptDirs directly with the unresolved path.
	paths := discovery.traverseForScriptDirs(startDir)
	_ = paths

	mu.Lock()
	defer mu.Unlock()

	foundCycle := false
	for _, msg := range messages {
		if strings.Contains(msg, "symlink cycle detected") {
			foundCycle = true
			break
		}
	}
	if !foundCycle {
		t.Error("Expected 'symlink cycle detected' in debug log")
		for _, m := range messages {
			t.Logf("  %s", m)
		}
	}
}

func TestGoalDiscovery_SymlinkToGoalDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Symlink tests not reliable on Windows")
	}
	t.Parallel()

	tmpDir := t.TempDir()
	realGoalDir := filepath.Join(tmpDir, "real-goals")
	if err := os.Mkdir(realGoalDir, 0o755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}

	linkGoalDir := filepath.Join(tmpDir, "link-goals")
	if err := os.Symlink(realGoalDir, linkGoalDir); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	cfg := config.NewConfig()
	cfg.SetGlobalOption("goal.disable-standard-paths", "true")
	cfg.SetGlobalOption("goal.autodiscovery", "false")
	cfg.SetGlobalOption("goal.paths", realGoalDir+string(filepath.ListSeparator)+linkGoalDir)

	discovery := NewGoalDiscovery(cfg)
	paths := discovery.DiscoverGoalPaths()

	// Both should resolve to the same real path, so dedup to 1.
	count := 0
	resolvedReal, _ := filepath.EvalSymlinks(realGoalDir)
	for _, p := range paths {
		if pathsEqual(p, resolvedReal) {
			count++
		}
	}

	if count != 1 {
		t.Errorf("Expected exactly 1 deduplicated path, got %d in %v", count, paths)
	}
}

// ---------------------------------------------------------------------------
// 3. Permission error resilience
// ---------------------------------------------------------------------------

func TestGoalDiscovery_UnreadableDirectoryInTraversal(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Permission tests not reliable on Windows")
	}
	if os.Getuid() == 0 {
		t.Skip("Skipping permission test when running as root")
	}
	// Changes working directory — not parallel.

	tmpDir := t.TempDir()

	workDir := filepath.Join(tmpDir, "work")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	// Create an unreadable directory in the traversal path.
	unreadable := filepath.Join(tmpDir, "unreadable")
	if err := os.Mkdir(unreadable, 0o000); err != nil {
		t.Fatalf("Mkdir unreadable: %v", err)
	}
	t.Cleanup(func() { os.Chmod(unreadable, 0o755) })

	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	defer os.Chdir(origWd)

	if err := os.Chdir(workDir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}

	var mu sync.Mutex
	var messages []string

	cfg := config.NewConfig()
	cfg.SetGlobalOption("goal.disable-standard-paths", "true")
	cfg.SetGlobalOption("goal.autodiscovery", "true")
	cfg.SetGlobalOption("goal.path-patterns", "osm-goals")
	cfg.SetGlobalOption("goal.max-traversal-depth", "10")

	discovery := NewGoalDiscovery(cfg)
	discovery.config.DebugLogFunc = func(format string, args ...any) {
		mu.Lock()
		defer mu.Unlock()
		messages = append(messages, fmt.Sprintf(format, args...))
	}

	// Must not crash.
	_ = discovery.DiscoverGoalPaths()

	// We mainly assert that the function completed without panicking.
	// The traversal goes upward from workDir, not into unreadable,
	// so permission errors may or may not appear — both are acceptable.
}

func TestScriptDiscovery_UnreadableDirInCheckDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Permission tests not reliable on Windows")
	}
	if os.Getuid() == 0 {
		t.Skip("Skipping permission test when running as root")
	}
	t.Parallel()

	tmpDir := t.TempDir()
	unreadable := filepath.Join(tmpDir, "noperm")
	if err := os.Mkdir(unreadable, 0o000); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	t.Cleanup(func() { os.Chmod(unreadable, 0o755) })

	cfg := config.NewConfig()
	discovery := NewScriptDiscovery(cfg)

	// checkDirectory stat's the path. On most Unix systems, stat on a
	// directory with 0o000 succeeds (you need execute, not read, to stat
	// subdirectories, but stat on the directory itself works). However,
	// checking a *child* of the unreadable dir will fail.
	childPath := filepath.Join(unreadable, "scripts")
	exists, err := discovery.checkDirectory(childPath)

	if exists {
		t.Error("Expected exists=false for child of unreadable directory")
	}
	if err != nil && !errors.Is(err, os.ErrPermission) && !errors.Is(err, os.ErrNotExist) {
		// It's acceptable to get either a permission error or a not-exist
		// error, depending on OS behavior.
		t.Errorf("Expected permission or not-exist error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// 4. normalizePath edge cases
// ---------------------------------------------------------------------------

func TestNormalizePath_EmptyInput(t *testing.T) {
	t.Parallel()

	cfg := config.NewConfig()
	discovery := NewGoalDiscovery(cfg)

	result, err := discovery.normalizePath("")
	if err != nil {
		// An error is acceptable, but it must not panic.
		return
	}

	// If no error, should be an absolute path (equivalent to ".").
	if !filepath.IsAbs(result) {
		t.Errorf("Expected absolute path for empty input, got: %s", result)
	}
}

func TestNormalizePath_DotDotTraversal(t *testing.T) {
	t.Parallel()

	cfg := config.NewConfig()
	discovery := NewGoalDiscovery(cfg)

	// normalizePath uses filepath.Abs which resolves relative to CWD.
	// The result must be absolute and clean.
	result, err := discovery.normalizePath("../../../etc")
	if err != nil {
		// An error from the security check (escapes parent) is acceptable.
		return
	}

	if !filepath.IsAbs(result) {
		t.Errorf("Expected absolute path, got: %s", result)
	}
	if result != filepath.Clean(result) {
		t.Errorf("Expected clean path, got: %s (clean: %s)", result, filepath.Clean(result))
	}
}

func TestNormalizePath_BrokenSymlinkFallback(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Symlink tests not reliable on Windows")
	}
	t.Parallel()

	tmpDir := t.TempDir()

	// Create a target, create a symlink, then remove the target.
	target := filepath.Join(tmpDir, "target")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatalf("Mkdir target: %v", err)
	}
	link := filepath.Join(tmpDir, "broken-link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("Symlink: %v", err)
	}
	if err := os.Remove(target); err != nil {
		t.Fatalf("Remove target: %v", err)
	}

	cfg := config.NewConfig()
	discovery := NewGoalDiscovery(cfg)

	result, err := discovery.normalizePath(link)
	if err != nil {
		// Acceptable — the security check may reject the broken symlink.
		return
	}

	// If it succeeds, the fallback should return an absolute path.
	if !filepath.IsAbs(result) {
		t.Errorf("Expected absolute fallback path, got: %s", result)
	}
}

func TestNormalizePath_SecurityValidation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Symlink tests not reliable on Windows")
	}
	t.Parallel()

	tmpDir := t.TempDir()

	// Create: tmpDir/Goals/evil -> /tmp (or some path outside parent).
	goalsDir := filepath.Join(tmpDir, "Goals")
	if err := os.Mkdir(goalsDir, 0o755); err != nil {
		t.Fatalf("Mkdir Goals: %v", err)
	}

	evilLink := filepath.Join(goalsDir, "evil")
	// Point to a well-known directory that is NOT under goalsDir.
	if err := os.Symlink("/tmp", evilLink); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	cfg := config.NewConfig()
	discovery := NewGoalDiscovery(cfg)

	_, err := discovery.normalizePath(evilLink)
	if err == nil {
		t.Fatal("Expected error for symlink escaping parent directory")
	}
	if !strings.Contains(err.Error(), "escapes parent") {
		t.Errorf("Expected 'escapes parent' in error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// 5. parsePositiveInt edge cases
// ---------------------------------------------------------------------------

func TestParsePositiveInt_EdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    string
		def      int
		max      int
		expected int
	}{
		{"", 10, 100, 10},
		{"0", 10, 100, 10},
		{"-1", 10, 100, 10},
		{"101", 10, 100, 10},
		{"50", 10, 100, 50},
		{"1", 10, 100, 1},
		{"abc", 10, 100, 10},
		{"  42  ", 10, 100, 42},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("input=%q,def=%d,max=%d", tt.input, tt.def, tt.max), func(t *testing.T) {
			t.Parallel()
			got := parsePositiveInt(tt.input, tt.def, tt.max)
			if got != tt.expected {
				t.Errorf("parsePositiveInt(%q, %d, %d) = %d, want %d",
					tt.input, tt.def, tt.max, got, tt.expected)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 6. expandPath edge cases
// ---------------------------------------------------------------------------

func TestExpandPath_HomeTilde(t *testing.T) {
	t.Parallel()

	cfg := config.NewConfig()
	discovery := NewGoalDiscovery(cfg)

	result := discovery.expandPath("~/foo")

	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("Cannot determine home dir: %v", err)
	}

	expected := filepath.Join(homeDir, "foo")
	if result != expected {
		t.Errorf("expandPath(~/foo) = %s, want %s", result, expected)
	}
}

func TestExpandPath_EnvVar(t *testing.T) {
	// Sets env vars — NOT parallel.

	t.Setenv("MY_PATH", "/opt/custom")

	cfg := config.NewConfig()
	discovery := NewGoalDiscovery(cfg)

	result := discovery.expandPath("$MY_PATH/scripts")
	expected := "/opt/custom/scripts"
	if result != expected {
		t.Errorf("expandPath($MY_PATH/scripts) = %s, want %s", result, expected)
	}
}

func TestExpandPath_NoExpansion(t *testing.T) {
	t.Parallel()

	cfg := config.NewConfig()
	discovery := NewGoalDiscovery(cfg)

	path := "/absolute/path"
	result := discovery.expandPath(path)
	if result != path {
		t.Errorf("expandPath(%s) = %s, want unchanged", path, result)
	}
}

// ---------------------------------------------------------------------------
// 7. Debug logging completeness
// ---------------------------------------------------------------------------

func TestGoalDiscovery_DebugLogCoverage(t *testing.T) {
	// Changes working directory — not parallel.

	tmpDir := t.TempDir()

	// Standard custom path.
	customGoals := filepath.Join(tmpDir, "custom-goals")
	if err := os.Mkdir(customGoals, 0o755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}

	// Autodiscovery target.
	autoGoals := filepath.Join(tmpDir, "osm-goals")
	if err := os.Mkdir(autoGoals, 0o755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}

	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	defer os.Chdir(origWd)

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}

	var mu sync.Mutex
	var messages []string

	cfg := config.NewConfig()
	cfg.SetGlobalOption("goal.autodiscovery", "true")
	cfg.SetGlobalOption("goal.paths", customGoals)
	cfg.SetGlobalOption("goal.path-patterns", "osm-goals")
	// Keep standard paths enabled to exercise that code path.

	discovery := NewGoalDiscovery(cfg)
	discovery.config.DebugLogFunc = func(format string, args ...any) {
		mu.Lock()
		defer mu.Unlock()
		messages = append(messages, fmt.Sprintf(format, args...))
	}

	_ = discovery.DiscoverGoalPaths()

	mu.Lock()
	defer mu.Unlock()

	expected := []struct {
		substr string
		desc   string
	}{
		{"starting goal path discovery", "discovery start"},
		{"standard path", "standard path log"},
		{"custom path", "custom path log"},
		{"traversal", "traversal log"},
		{"autodiscovered path", "autodiscovered path log"},
		{"discovery complete", "discovery complete"},
	}

	for _, e := range expected {
		found := false
		for _, msg := range messages {
			if strings.Contains(msg, e.substr) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected debug log containing %q (%s), messages:", e.substr, e.desc)
			for _, m := range messages {
				t.Logf("  %s", m)
			}
		}
	}
}

func TestScriptDiscovery_DebugLogCoverage(t *testing.T) {
	// Changes working directory — not parallel.

	tmpDir := t.TempDir()

	customScripts := filepath.Join(tmpDir, "custom-scripts")
	if err := os.Mkdir(customScripts, 0o755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}

	autoScripts := filepath.Join(tmpDir, "scripts")
	if err := os.Mkdir(autoScripts, 0o755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}

	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	defer os.Chdir(origWd)

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}

	var mu sync.Mutex
	var messages []string

	cfg := config.NewConfig()
	cfg.SetGlobalOption("script.autodiscovery", "true")
	cfg.SetGlobalOption("script.paths", customScripts)
	cfg.SetGlobalOption("script.path-patterns", "scripts")

	discovery := NewScriptDiscovery(cfg)
	discovery.config.DebugLogFunc = func(format string, args ...any) {
		mu.Lock()
		defer mu.Unlock()
		messages = append(messages, fmt.Sprintf(format, args...))
	}

	_ = discovery.DiscoverScriptPaths()

	mu.Lock()
	defer mu.Unlock()

	expected := []struct {
		substr string
		desc   string
	}{
		{"starting script path discovery", "discovery start"},
		{"standard path", "standard path log"},
		{"custom path", "custom path log"},
		{"traversal", "traversal log"},
		{"discovery complete", "discovery complete"},
	}

	for _, e := range expected {
		found := false
		for _, msg := range messages {
			if strings.Contains(msg, e.substr) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected debug log containing %q (%s), messages:", e.substr, e.desc)
			for _, m := range messages {
				t.Logf("  %s", m)
			}
		}
	}
}
