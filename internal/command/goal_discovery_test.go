package command

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/config"
)

func TestGoalDiscovery_DiscoverGoalPaths(t *testing.T) {
	t.Parallel()

	t.Run("default configuration", func(t *testing.T) {
		t.Parallel()
		cfg := config.NewConfig()
		discovery := NewGoalDiscovery(cfg)

		paths := discovery.DiscoverGoalPaths()

		// Should have at least standard paths
		if len(paths) == 0 {
			t.Error("Expected at least some goal paths to be discovered")
		}

		// Verify no duplicate paths
		seen := make(map[string]bool)
		for _, path := range paths {
			if seen[path] {
				t.Errorf("Duplicate path found: %s", path)
			}
			seen[path] = true
		}

		// Verify all paths are absolute
		for _, path := range paths {
			if !filepath.IsAbs(path) {
				t.Errorf("Expected absolute path, got: %s", path)
			}
		}
	})

	t.Run("with custom paths", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		customPath := filepath.Join(tmpDir, "custom-goals")
		if err := os.MkdirAll(customPath, 0o755); err != nil {
			t.Fatalf("Failed to create custom path: %v", err)
		}

		cfg := config.NewConfig()
		cfg.SetGlobalOption("goal.paths", customPath)

		discovery := NewGoalDiscovery(cfg)
		paths := discovery.DiscoverGoalPaths()

		// Verify custom path is included
		found := false
		for _, path := range paths {
			if pathsEqual(path, customPath) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Custom path %s not found in discovered paths: %v", customPath, paths)
		}
	})

	t.Run("with autodiscovery disabled", func(t *testing.T) {
		t.Parallel()
		cfg := config.NewConfig()
		cfg.SetGlobalOption("goal.autodiscovery", "false")

		discovery := NewGoalDiscovery(cfg)
		paths := discovery.DiscoverGoalPaths()

		// Should still have standard paths even with autodiscovery disabled
		if len(paths) == 0 {
			t.Error("Expected standard paths even with autodiscovery disabled")
		}
	})

	t.Run("with autodiscovery enabled and goal directory present", func(t *testing.T) {
		// Note: Cannot use t.Parallel() because this test changes working directory

		// Create a temporary directory structure
		tmpDir := t.TempDir()
		goalDir := filepath.Join(tmpDir, "osm-goals")
		if err := os.MkdirAll(goalDir, 0o755); err != nil {
			t.Fatalf("Failed to create goal directory: %v", err)
		}

		// Change to the temp directory for this test
		origWd, err := os.Getwd()
		if err != nil {
			t.Fatalf("Failed to get working directory: %v", err)
		}
		defer os.Chdir(origWd)

		if err := os.Chdir(tmpDir); err != nil {
			t.Fatalf("Failed to change directory: %v", err)
		}

		cfg := config.NewConfig()
		cfg.SetGlobalOption("goal.autodiscovery", "true")

		discovery := NewGoalDiscovery(cfg)
		paths := discovery.DiscoverGoalPaths()

		// Should discover the osm-goals directory
		found := false
		for _, path := range paths {
			// Handle macOS /var vs /private/var symlink normalization
			if pathsEqual(path, goalDir) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected to discover goal directory %s, found paths: %v", goalDir, paths)
		}
	})

	t.Run("with custom path patterns", func(t *testing.T) {
		// Note: Cannot use t.Parallel() because this test changes working directory

		tmpDir := t.TempDir()
		customPattern := "my-custom-goals"
		goalDir := filepath.Join(tmpDir, customPattern)
		if err := os.MkdirAll(goalDir, 0o755); err != nil {
			t.Fatalf("Failed to create goal directory: %v", err)
		}

		origWd, err := os.Getwd()
		if err != nil {
			t.Fatalf("Failed to get working directory: %v", err)
		}
		defer os.Chdir(origWd)

		if err := os.Chdir(tmpDir); err != nil {
			t.Fatalf("Failed to change directory: %v", err)
		}

		cfg := config.NewConfig()
		cfg.SetGlobalOption("goal.autodiscovery", "true")
		cfg.SetGlobalOption("goal.path-patterns", customPattern)

		discovery := NewGoalDiscovery(cfg)
		paths := discovery.DiscoverGoalPaths()

		found := false
		for _, path := range paths {
			if pathsEqual(path, goalDir) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected to discover custom pattern directory %s, found paths: %v", goalDir, paths)
		}
	})

	t.Run("with max traversal depth", func(t *testing.T) {
		// Note: Cannot use t.Parallel() because this test changes working directory

		// Create a deep directory structure
		tmpDir := t.TempDir()
		deepPath := tmpDir
		for i := 0; i < 15; i++ {
			deepPath = filepath.Join(deepPath, "level")
		}
		if err := os.MkdirAll(deepPath, 0o755); err != nil {
			t.Fatalf("Failed to create deep directory: %v", err)
		}

		// Create a goal directory at the root
		goalDir := filepath.Join(tmpDir, "osm-goals")
		if err := os.MkdirAll(goalDir, 0o755); err != nil {
			t.Fatalf("Failed to create goal directory: %v", err)
		}

		origWd, err := os.Getwd()
		if err != nil {
			t.Fatalf("Failed to get working directory: %v", err)
		}
		defer os.Chdir(origWd)

		if err := os.Chdir(deepPath); err != nil {
			t.Fatalf("Failed to change directory: %v", err)
		}

		// With very limited depth, shouldn't find the goal directory
		cfg := config.NewConfig()
		cfg.SetGlobalOption("goal.autodiscovery", "true")
		cfg.SetGlobalOption("goal.max-traversal-depth", "5")

		discovery := NewGoalDiscovery(cfg)
		paths := discovery.DiscoverGoalPaths()

		found := false
		for _, path := range paths {
			if path == goalDir {
				found = true
				break
			}
		}
		// Should NOT find it because it's beyond the traversal depth
		if found {
			t.Errorf("Should not have discovered goal directory %s beyond max traversal depth", goalDir)
		}
	})
}

func TestGoalDiscovery_computePathScore(t *testing.T) {
	t.Parallel()

	cfg := config.NewConfig()
	discovery := NewGoalDiscovery(cfg)

	// Set up test paths
	tmpDir := t.TempDir()
	cwd := filepath.Join(tmpDir, "project", "src")
	configDir := filepath.Join(tmpDir, "config")
	execDir := filepath.Join(tmpDir, "bin")

	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatalf("Failed to create cwd: %v", err)
	}
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("Failed to create configDir: %v", err)
	}
	if err := os.MkdirAll(execDir, 0o755); err != nil {
		t.Fatalf("Failed to create execDir: %v", err)
	}

	tests := []struct {
		name          string
		path          string
		expectedClass int
		description   string
	}{
		{
			name:          "current directory",
			path:          cwd,
			expectedClass: 0,
			description:   "CWD itself should be class 0",
		},
		{
			name:          "subdirectory of CWD",
			path:          filepath.Join(cwd, "goals"),
			expectedClass: 0,
			description:   "Child of CWD should be class 0",
		},
		{
			name:          "parent directory pattern",
			path:          filepath.Join(tmpDir, "project", "osm-goals"),
			expectedClass: 1,
			description:   "Parent directory with pattern should be class 1",
		},
		{
			name:          "config directory",
			path:          filepath.Join(configDir, "goals"),
			expectedClass: 2,
			description:   "Under config dir should be class 2",
		},
		{
			name:          "exec directory",
			path:          filepath.Join(execDir, "goals"),
			expectedClass: 3,
			description:   "Under exec dir should be class 3",
		},
		{
			name:          "unrelated directory",
			path:          filepath.Join(tmpDir, "other", "goals"),
			expectedClass: 4,
			description:   "Unrelated path should be class 4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if err := os.MkdirAll(tt.path, 0o755); err != nil {
				t.Fatalf("Failed to create test path: %v", err)
			}

			score := discovery.computePathScore(tt.path, cwd, configDir, execDir)

			if score.class != tt.expectedClass {
				t.Errorf("%s: expected class %d, got %d (distance=%d, depth=%d)",
					tt.description, tt.expectedClass, score.class, score.distance, score.depth)
			}
		})
	}
}

func TestGoalDiscovery_computePathScore_Ordering(t *testing.T) {
	t.Parallel()

	cfg := config.NewConfig()
	discovery := NewGoalDiscovery(cfg)

	tmpDir := t.TempDir()
	cwd := filepath.Join(tmpDir, "project", "src", "module")
	configDir := filepath.Join(tmpDir, "config")
	execDir := filepath.Join(tmpDir, "bin")

	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatalf("Failed to create cwd: %v", err)
	}

	// Create test paths
	cwdGoals := filepath.Join(cwd, "osm-goals")
	parentGoals := filepath.Join(tmpDir, "project", "src", "osm-goals")
	grandparentGoals := filepath.Join(tmpDir, "project", "osm-goals")
	configGoals := filepath.Join(configDir, "goals")
	execGoals := filepath.Join(execDir, "goals")

	paths := []string{
		execGoals,
		configGoals,
		grandparentGoals,
		parentGoals,
		cwdGoals,
	}

	for _, p := range paths {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatalf("Failed to create path: %v", err)
		}
	}

	// Compute scores
	scores := make(map[string]goalPathScore)
	for _, path := range paths {
		scores[path] = discovery.computePathScore(path, cwd, configDir, execDir)
	}

	// Verify ordering: CWD descendant should beat parent pattern
	if scores[cwdGoals].class > scores[parentGoals].class {
		t.Errorf("CWD descendant should have lower class than parent pattern, got cwd.class=%d parent.class=%d",
			scores[cwdGoals].class, scores[parentGoals].class)
	}
	if scores[cwdGoals].class == scores[parentGoals].class &&
		scores[cwdGoals].distance > scores[parentGoals].distance {
		t.Errorf("CWD descendant should have lower distance than parent pattern within same class")
	}

	// Verify ordering: closer parent should beat farther parent
	if scores[parentGoals].class != scores[grandparentGoals].class {
		t.Errorf("Parent patterns should have same class, got parent.class=%d grandparent.class=%d",
			scores[parentGoals].class, scores[grandparentGoals].class)
	}
	if scores[parentGoals].distance > scores[grandparentGoals].distance {
		t.Errorf("Closer parent should have SMALLER distance than farther parent, got parent.distance=%d grandparent.distance=%d",
			scores[parentGoals].distance, scores[grandparentGoals].distance)
	}

	// Verify ordering: parent pattern beats config
	if scores[parentGoals].class >= scores[configGoals].class {
		t.Errorf("Parent pattern should have lower class than config directory, got parent.class=%d config.class=%d",
			scores[parentGoals].class, scores[configGoals].class)
	}

	// Verify ordering: config beats exec
	if scores[configGoals].class >= scores[execGoals].class {
		t.Errorf("Config directory should have lower class than exec directory, got config.class=%d exec.class=%d",
			scores[configGoals].class, scores[execGoals].class)
	}
}

func TestGoalDiscovery_PathExpansion(t *testing.T) {
	t.Parallel()

	cfg := config.NewConfig()
	discovery := NewGoalDiscovery(cfg)

	t.Run("tilde expansion", func(t *testing.T) {
		t.Parallel()
		path := "~/test/path"
		expanded := discovery.expandPath(path)

		homeDir, _ := os.UserHomeDir()
		expected := filepath.Join(homeDir, "test", "path")

		if expanded != expected {
			t.Errorf("Expected %s, got %s", expected, expanded)
		}
	})

	t.Run("environment variable expansion", func(t *testing.T) {
		t.Parallel()

		// Set a test environment variable
		testVar := "TEST_GOAL_PATH_VAR"
		testValue := "/test/value"
		os.Setenv(testVar, testValue)
		defer os.Unsetenv(testVar)

		path := "$" + testVar + "/goals"
		expanded := discovery.expandPath(path)

		expected := testValue + "/goals"

		if expanded != expected {
			t.Errorf("Expected %s, got %s", expected, expanded)
		}
	})

	t.Run("no expansion needed", func(t *testing.T) {
		t.Parallel()
		path := "/absolute/path/to/goals"
		expanded := discovery.expandPath(path)

		if expanded != path {
			t.Errorf("Expected %s, got %s", path, expanded)
		}
	})
}

func TestGoalDiscovery_DirectoryTraversal(t *testing.T) {
	// Note: Cannot use t.Parallel() because this test changes working directory

	tmpDir := t.TempDir()

	// Create directory structure:
	// tmpDir/
	//   level1/
	//     osm-goals/
	//     level2/
	//       goals/
	//       level3/
	level1 := filepath.Join(tmpDir, "level1")
	level2 := filepath.Join(level1, "level2")
	level3 := filepath.Join(level2, "level3")

	goals1 := filepath.Join(level1, "osm-goals")
	goals2 := filepath.Join(level2, "goals")

	for _, dir := range []string{level1, level2, level3, goals1, goals2} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("Failed to create directory: %v", err)
		}
	}

	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}
	defer os.Chdir(origWd)

	if err := os.Chdir(level3); err != nil {
		t.Fatalf("Failed to change directory: %v", err)
	}

	cfg := config.NewConfig()
	cfg.SetGlobalOption("goal.autodiscovery", "true")
	cfg.SetGlobalOption("goal.path-patterns", "osm-goals,goals")

	discovery := NewGoalDiscovery(cfg)
	paths := discovery.DiscoverGoalPaths()

	// Should find at least one of the goal directories when traversing upward from level3
	// Note: The exact directories found depend on the traversal depth and current working directory
	foundGoals1 := false
	foundGoals2 := false

	for _, path := range paths {
		if pathsEqual(path, goals1) {
			foundGoals1 = true
		}
		if pathsEqual(path, goals2) {
			foundGoals2 = true
		}
	}

	if !foundGoals1 && !foundGoals2 {
		t.Errorf("Expected to find at least one goal directory, found paths: %v", paths)
		return
	}

	// If both were found, verify ordering: closer directory should come first
	if foundGoals1 && foundGoals2 {
		idx1, idx2 := -1, -1
		for i, path := range paths {
			if pathsEqual(path, goals1) {
				idx1 = i
			}
			if pathsEqual(path, goals2) {
				idx2 = i
			}
		}

		// goals2 is closer to level3 (in level2) than goals1 (in level1)
		// So goals2 should come first (LOWER index, i.e., idx2 < idx1)
		if idx2 > idx1 {
			t.Errorf("Closer goal directory should be prioritized: goals2 at index %d should come BEFORE goals1 at index %d", idx2, idx1)
		}
	}
}

func TestGoalDiscovery_DuplicatePathElimination(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	goalDir := filepath.Join(tmpDir, "goals")
	if err := os.MkdirAll(goalDir, 0o755); err != nil {
		t.Fatalf("Failed to create goal directory: %v", err)
	}

	// Create symbolic link to the same directory
	linkPath := filepath.Join(tmpDir, "goals-link")
	if err := os.Symlink(goalDir, linkPath); err != nil {
		// Skip test on platforms that don't support symlinks
		t.Skip("Skipping symlink test: platform doesn't support symlinks")
	}

	cfg := config.NewConfig()
	// Add both the real path and the symlink
	cfg.SetGlobalOption("goal.paths", goalDir+string(filepath.ListSeparator)+linkPath)

	discovery := NewGoalDiscovery(cfg)
	paths := discovery.DiscoverGoalPaths()

	// Normalize both paths for comparison
	normalizedGoalDir, err := filepath.Abs(goalDir)
	if err != nil {
		t.Fatalf("Failed to get absolute path: %v", err)
	}
	normalizedLinkPath, err := filepath.Abs(linkPath)
	if err != nil {
		t.Fatalf("Failed to get absolute path: %v", err)
	}

	// Count how many of our test paths appear in the discovered paths
	// With symlink resolution enabled, both should resolve to the same path
	count := 0
	for _, path := range paths {
		if pathsEqual(path, normalizedGoalDir) || pathsEqual(path, normalizedLinkPath) {
			count++
		}
	}

	// Should appear exactly once due to symlink resolution in normalizePath
	if count != 1 {
		t.Errorf("Expected exactly one occurrence after symlink deduplication, got %d in paths: %v", count, paths)
	}
}

func TestGoalDiscovery_EmptyPathHandling(t *testing.T) {
	t.Parallel()

	cfg := config.NewConfig()
	// Set paths with empty strings and whitespace
	cfg.SetGlobalOption("goal.paths", "  "+string(filepath.ListSeparator)+string(filepath.ListSeparator)+"  ")

	discovery := NewGoalDiscovery(cfg)
	paths := discovery.DiscoverGoalPaths()

	// Should not include empty paths
	for _, path := range paths {
		if path == "" {
			t.Error("Empty path should not be included in discovered paths")
		}
	}
}

func TestGoalDiscovery_CrossPlatformPaths(t *testing.T) {
	t.Parallel()

	cfg := config.NewConfig()
	discovery := NewGoalDiscovery(cfg)

	paths := discovery.DiscoverGoalPaths()

	// Verify all paths use platform-appropriate separators
	for _, path := range paths {
		if filepath.Separator == '/' {
			// Unix-like systems should not have backslashes
			if containsRune(path, '\\') {
				t.Errorf("Path contains backslash on Unix system: %s", path)
			}
		}
		// All paths should be clean (no redundant separators)
		cleaned := filepath.Clean(path)
		if cleaned != path {
			t.Errorf("Path is not clean: %s (cleaned: %s)", path, cleaned)
		}
	}
}

func TestGoalDiscovery_matchesAncestorPattern(t *testing.T) {
	t.Parallel()

	cfg := config.NewConfig()
	cfg.SetGlobalOption("goal.path-patterns", "osm-goals,goals")
	discovery := NewGoalDiscovery(cfg)

	tests := []struct {
		name     string
		segments []string
		expected bool
	}{
		{
			name:     "matches osm-goals pattern",
			segments: []string{"..", "osm-goals"},
			expected: true,
		},
		{
			name:     "matches goals pattern",
			segments: []string{"..", "..", "goals"},
			expected: true,
		},
		{
			name:     "does not match different pattern",
			segments: []string{"..", "other-dir"},
			expected: false,
		},
		{
			name:     "no down segments",
			segments: []string{"..", ".."},
			expected: false,
		},
		{
			name:     "matches with multiple up segments",
			segments: []string{"..", "..", "..", "osm-goals"},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := discovery.matchesAncestorPattern(tt.segments)
			if result != tt.expected {
				t.Errorf("matchesAncestorPattern(%v) = %v, expected %v", tt.segments, result, tt.expected)
			}
		})
	}
}

func TestGoalDiscovery_StandardPaths(t *testing.T) {
	t.Parallel()

	cfg := config.NewConfig()
	discovery := NewGoalDiscovery(cfg)

	standardPaths := discovery.getStandardPaths()

	// Should return at least one standard path
	if len(standardPaths) == 0 {
		t.Error("Expected at least one standard path")
	}

	// All standard paths should be absolute
	for _, path := range standardPaths {
		if !filepath.IsAbs(path) {
			t.Errorf("Standard path should be absolute: %s", path)
		}
	}

	// Should include config directory goals if config path is available
	configPath, err := config.GetConfigPath()
	if err == nil {
		configDir := filepath.Dir(configPath)
		expectedPath := filepath.Join(configDir, "goals")

		found := false
		for _, path := range standardPaths {
			if path == expectedPath {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected standard paths to include config goals path: %s", expectedPath)
		}
	}
}

func TestGoalDiscovery_ConfigOptionParsing(t *testing.T) {
	t.Parallel()

	t.Run("autodiscovery option", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			value    string
			expected bool
		}{
			{"true", true},
			{"false", false},
			{"1", true},
			{"0", false},
			{"invalid", false}, // Default to false for invalid values
		}

		for _, tc := range testCases {
			cfg := config.NewConfig()
			cfg.SetGlobalOption("goal.autodiscovery", tc.value)
			discovery := NewGoalDiscovery(cfg)

			if discovery.config.EnableAutodiscovery != tc.expected {
				t.Errorf("For value %q, expected EnableAutodiscovery=%v, got %v",
					tc.value, tc.expected, discovery.config.EnableAutodiscovery)
			}
		}
	})

	t.Run("max-traversal-depth option", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			value    string
			expected int
		}{
			{"5", 5},
			{"20", 20},
			{"0", 10},       // Below minimum, use default
			{"150", 10},     // Above maximum, use default
			{"invalid", 10}, // Invalid value, use default
		}

		for _, tc := range testCases {
			cfg := config.NewConfig()
			cfg.SetGlobalOption("goal.max-traversal-depth", tc.value)
			discovery := NewGoalDiscovery(cfg)

			if discovery.config.MaxTraversalDepth != tc.expected {
				t.Errorf("For value %q, expected MaxTraversalDepth=%d, got %d",
					tc.value, tc.expected, discovery.config.MaxTraversalDepth)
			}
		}
	})

	t.Run("path-patterns option", func(t *testing.T) {
		t.Parallel()

		cfg := config.NewConfig()
		cfg.SetGlobalOption("goal.path-patterns", "custom1,custom2,custom3")
		discovery := NewGoalDiscovery(cfg)

		expected := []string{"custom1", "custom2", "custom3"}
		if !reflect.DeepEqual(discovery.config.GoalPathPatterns, expected) {
			t.Errorf("Expected path patterns %v, got %v", expected, discovery.config.GoalPathPatterns)
		}
	})
}

// Helper function to check if a string contains a specific rune
func containsRune(s string, r rune) bool {
	for _, c := range s {
		if c == r {
			return true
		}
	}
	return false
}

// Helper function for cross-platform path comparison
// Handles macOS /var vs /private/var symlink normalization
func pathsEqual(path1, path2 string) bool {
	// First try direct comparison
	if path1 == path2 {
		return true
	}

	// Try evaluating symlinks
	eval1, err1 := filepath.EvalSymlinks(path1)
	eval2, err2 := filepath.EvalSymlinks(path2)

	if err1 == nil && err2 == nil {
		return eval1 == eval2
	}

	// Fallback to cleaned path comparison
	return filepath.Clean(path1) == filepath.Clean(path2)
}
