package command

import (
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/config"
)

func TestScriptDiscovery_DefaultBehavior(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	discovery := NewScriptDiscovery(cfg)

	if discovery.config.EnableAutodiscovery {
		t.Error("Expected autodiscovery to be disabled by default")
	}

	if discovery.config.MaxTraversalDepth != 10 {
		t.Errorf("Expected default max traversal depth of 10, got %d", discovery.config.MaxTraversalDepth)
	}

	if len(discovery.config.ScriptPathPatterns) != 1 || discovery.config.ScriptPathPatterns[0] != "scripts" {
		t.Errorf("Expected default script path pattern ['scripts'], got %v", discovery.config.ScriptPathPatterns)
	}
}

func TestScriptDiscovery_ConfigurationLoading(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cfg.SetGlobalOption("script.autodiscovery", "true")
	cfg.SetGlobalOption("script.git-traversal", "true")
	cfg.SetGlobalOption("script.max-traversal-depth", "5")
	// Use comma separator for cross-platform compatibility
	cfg.SetGlobalOption("script.paths", "~/custom-scripts,/opt/scripts")
	cfg.SetGlobalOption("script.path-patterns", "scripts,bin,commands")

	discovery := NewScriptDiscovery(cfg)

	if !discovery.config.EnableAutodiscovery {
		t.Error("Expected autodiscovery to be enabled")
	}

	if !discovery.config.EnableGitTraversal {
		t.Error("Expected git traversal to be enabled")
	}

	if discovery.config.MaxTraversalDepth != 5 {
		t.Errorf("Expected max traversal depth of 5, got %d", discovery.config.MaxTraversalDepth)
	}

	if len(discovery.config.CustomPaths) != 2 {
		t.Errorf("Expected 2 custom paths, got %d: %v", len(discovery.config.CustomPaths), discovery.config.CustomPaths)
	}

	if len(discovery.config.ScriptPathPatterns) != 3 {
		t.Errorf("Expected 3 script path patterns, got %d: %v", len(discovery.config.ScriptPathPatterns), discovery.config.ScriptPathPatterns)
	}
}

func TestScriptDiscovery_LegacyPaths(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	discovery := NewScriptDiscovery(cfg)

	paths := discovery.getLegacyPaths()

	// Should have at least one 'scripts' path (cwd may vary under `go test`)
	found := false
	cwd, _ := os.Getwd()
	expectedPath := filepath.Join(cwd, "scripts")

	for _, path := range paths {
		if path == expectedPath {
			found = true
			break
		}
	}

	// Accept other legitimate 'scripts' locations when running via `go test` (temp build dirs etc.)
	if !found {
		for _, path := range paths {
			if filepath.Base(path) == "scripts" {
				found = true
				break
			}
		}
	}

	if !found {
		t.Errorf("Expected to find current directory scripts path %s in legacy paths %v", expectedPath, paths)
	}
}

func TestScriptDiscovery_LegacyPathsRespectsConfigEnv(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "custom", "config")
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		t.Fatalf("Failed to create config directory: %v", err)
	}

	t.Setenv("ONESHOTMAN_CONFIG", configPath)

	cfg := config.NewConfig()
	discovery := NewScriptDiscovery(cfg)

	paths := discovery.getLegacyPaths()
	expected := filepath.Join(filepath.Dir(configPath), "scripts")

	found := false
	for _, path := range paths {
		if path == expected {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("Expected legacy paths to include %s when ONESHOTMAN_CONFIG is set, got %v", expected, paths)
	}
}

func TestScriptDiscovery_PathExpansion(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	discovery := NewScriptDiscovery(cfg)

	// Test tilde expansion
	tildeResult := discovery.expandPath("~/test")
	if homeDir, err := os.UserHomeDir(); err == nil {
		expected := filepath.Join(homeDir, "test")
		if tildeResult != expected {
			t.Errorf("Expected tilde expansion to result in %s, got %s", expected, tildeResult)
		}
	}

	// Test environment variable expansion
	os.Setenv("TEST_PATH", "/custom/path")
	envResult := discovery.expandPath("$TEST_PATH/scripts")
	expected := "/custom/path/scripts"
	if envResult != expected {
		t.Errorf("Expected env expansion to result in %s, got %s", expected, envResult)
	}
}

func TestScriptDiscovery_GitRepositoryDetection(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	discovery := NewScriptDiscovery(cfg)

	repoDir := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatalf("failed to create repo dir: %v", err)
	}

	cmd := exec.Command("git", "init", "--quiet")
	cmd.Dir = repoDir
	if err := cmd.Run(); err != nil {
		t.Skipf("git init failed (%v), skipping git detection test", err)
	}

	if !discovery.isGitRepository(repoDir) {
		t.Fatalf("expected %s to be detected as git repository", repoDir)
	}

	nonRepoDir := t.TempDir()
	if discovery.isGitRepository(nonRepoDir) {
		t.Errorf("expected %s to not be detected as git repository", nonRepoDir)
	}
}

func TestScriptDiscovery_DirectoryExists(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	discovery := NewScriptDiscovery(cfg)

	// Test existing directory
	tmpDir := t.TempDir()
	if !discovery.directoryExists(tmpDir) {
		t.Errorf("Expected %s to exist", tmpDir)
	}

	// Test non-existing directory
	nonExistentDir := filepath.Join(tmpDir, "nonexistent")
	if discovery.directoryExists(nonExistentDir) {
		t.Errorf("Expected %s to not exist", nonExistentDir)
	}

	// Test file (not directory)
	tempFile := filepath.Join(tmpDir, "testfile")
	if err := os.WriteFile(tempFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	if discovery.directoryExists(tempFile) {
		t.Errorf("Expected %s to not be detected as directory (it's a file)", tempFile)
	}
}

func TestScriptDiscovery_PathPriority(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	discovery := NewScriptDiscovery(cfg)

	baseDir := t.TempDir()
	cwd := filepath.Join(baseDir, "workspace", "project")
	configDir := filepath.Join(baseDir, "config")
	execDir := filepath.Join(baseDir, "bin")

	for _, dir := range []string{cwd, configDir, execDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("failed to create test directory %q: %v", dir, err)
		}
	}

	type pathExpectation struct {
		name     string
		path     string
		expected pathScore
	}

	cases := []pathExpectation{
		{
			name:     "cwd root",
			path:     filepath.Join(cwd, "scripts"),
			expected: pathScore{class: 0, distance: 1, depth: 1},
		},
		{
			name:     "cwd nested",
			path:     filepath.Join(cwd, "nested", "scripts"),
			expected: pathScore{class: 0, distance: 2, depth: 2},
		},
		{
			name:     "cwd exact",
			path:     cwd,
			expected: pathScore{class: 0, distance: 0, depth: 0},
		},
		{
			name:     "ancestor immediate",
			path:     filepath.Join(filepath.Dir(cwd), "scripts"),
			expected: pathScore{class: 1, distance: 1, depth: 1},
		},
		{
			name:     "ancestor deep",
			path:     filepath.Join(baseDir, "scripts"),
			expected: pathScore{class: 1, distance: 2, depth: 1},
		},
		{
			name:     "config",
			path:     filepath.Join(configDir, "scripts"),
			expected: pathScore{class: 2, distance: 1, depth: 1},
		},
		{
			name:     "exec",
			path:     filepath.Join(execDir, "scripts"),
			expected: pathScore{class: 3, distance: 1, depth: 1},
		},
		{
			name:     "other",
			path:     filepath.Join(baseDir, "other", "scripts"),
			expected: pathScore{class: 4, distance: math.MaxInt, depth: math.MaxInt},
		},
	}

	scores := make(map[string]pathScore)
	paths := make(map[string]string)
	for _, tc := range cases {
		score := discovery.computePathScore(tc.path, cwd, configDir, execDir)
		if score != tc.expected {
			t.Errorf("%s: expected score %+v, got %+v", tc.name, tc.expected, score)
		}
		scores[tc.name] = score
		paths[tc.name] = tc.path
	}

	less := func(aName, bName string) bool {
		aPath := paths[aName]
		bPath := paths[bName]

		aScore := scores[aName]
		bScore := scores[bName]

		if aScore.class != bScore.class {
			return aScore.class < bScore.class
		}
		if aScore.distance != bScore.distance {
			return aScore.distance < bScore.distance
		}
		if aScore.depth != bScore.depth {
			return aScore.depth < bScore.depth
		}
		return aPath < bPath
	}

	requireLess := func(higher, lower string) {
		if !less(higher, lower) {
			t.Fatalf("expected %s to outrank %s", higher, lower)
		}
	}

	requireLess("cwd exact", "cwd nested")
	requireLess("cwd root", "ancestor immediate")
	requireLess("ancestor immediate", "ancestor deep")
	requireLess("ancestor deep", "config")
	requireLess("config", "exec")
	requireLess("cwd root", "other")
}

func TestScriptDiscovery_ParsePathList(t *testing.T) {
	t.Parallel()
	sep := string([]byte{filepath.ListSeparator})

	// Platform-agnostic tests using comma (works on all platforms)
	tests := []struct {
		input    string
		expected []string
	}{
		{"", nil},
		{"  ", nil},
		{"single", []string{"single"}},
		{"path1,path2", []string{"path1", "path2"}},
		{" path1 , path2 ", []string{"path1", "path2"}},
		{"path1,path2,path3", []string{"path1", "path2", "path3"}},
		// Tests using platform-specific list separator
		{"path1" + sep + "path2", []string{"path1", "path2"}},
		{"path1" + sep + "path2" + sep + "path3", []string{"path1", "path2", "path3"}},
		{" path1 " + sep + " path2 ", []string{"path1", "path2"}},
		{"path1" + sep + "path2,path3", []string{"path1", "path2", "path3"}},
	}

	for _, test := range tests {
		result := parsePathList(test.input)
		if len(result) != len(test.expected) {
			t.Errorf("parsePathList(%q): expected %d paths, got %d", test.input, len(test.expected), len(result))
			continue
		}

		for i, expected := range test.expected {
			if result[i] != expected {
				t.Errorf("parsePathList(%q): expected path[%d]=%q, got %q", test.input, i, expected, result[i])
			}
		}
	}
}

func TestScriptDiscovery_ParsePositiveInt(t *testing.T) {
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
		{"5", 10, 100, 5},
		{"123", 10, 100, 10},
		{"123", 10, 0, 123},
		{"abc", 10, 100, 10},
		{"12abc", 10, 100, 10},
	}

	for _, test := range tests {
		result := parsePositiveInt(test.input, test.def, test.max)
		if result != test.expected {
			t.Errorf("parsePositiveInt(%q, %d, %d): expected %d, got %d", test.input, test.def, test.max, test.expected, result)
		}
	}
}

func TestNewRegistryWithConfig_Integration(t *testing.T) {
	t.Parallel()
	// Test that the registry properly integrates with script discovery
	cfg := config.NewConfig()
	// Create a real custom path so discovery can find it on all platforms
	tmp := t.TempDir()
	customPath := filepath.Join(tmp, "custom-scripts")
	if err := os.MkdirAll(customPath, 0755); err != nil {
		t.Fatalf("failed to create custom scripts dir: %v", err)
	}
	cfg.SetGlobalOption("script.paths", customPath)

	registry := NewRegistryWithConfig(cfg)

	// Should have discovered at least legacy paths
	if len(registry.scriptPaths) == 0 {
		t.Error("Expected registry to have discovered some script paths")
	}

	// Custom path should be added (compare normalized absolute paths)
	found := false
	normalizedCustomPath, _ := filepath.Abs(customPath)
	for _, path := range registry.scriptPaths {
		pAbs, _ := filepath.Abs(path)
		if runtime.GOOS == "windows" {
			if strings.EqualFold(pAbs, normalizedCustomPath) {
				found = true
				break
			}
		} else {
			if pAbs == normalizedCustomPath {
				found = true
				break
			}
		}
	}
	if !found {
		t.Error("Expected custom path to be discovered and added to registry")
	}
}

func TestScriptDiscovery_AutodiscoveryIntegration(t *testing.T) {
	// Create a temporary directory structure for testing
	tmpDir := t.TempDir()

	// Create nested directory with scripts
	scriptsDir := filepath.Join(tmpDir, "project", "scripts")
	if err := os.MkdirAll(scriptsDir, 0755); err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}

	// Create a test script appropriate for the host platform. On Unix-like
	// systems use a shell script with execute bits; on Windows use a .bat
	// file (execute bits are not meaningful on Windows in the same way).
	var testScript string
	if runtime.GOOS == "windows" {
		testScript = filepath.Join(scriptsDir, "testscript.bat")
		if err := os.WriteFile(testScript, []byte("@echo off\necho test"), 0644); err != nil {
			t.Fatalf("Failed to create test script: %v", err)
		}
	} else {
		testScript = filepath.Join(scriptsDir, "testscript.sh")
		if err := os.WriteFile(testScript, []byte("#!/bin/bash\necho test"), 0755); err != nil {
			t.Fatalf("Failed to create test script: %v", err)
		}
		// Ensure execute bit regardless of umask (no-op on Windows).
		if err := os.Chmod(testScript, 0755); err != nil {
			t.Fatalf("Failed to chmod test script: %v", err)
		}
	}

	// Change to the project directory
	projectDir := filepath.Join(tmpDir, "project")
	originalDir, _ := os.Getwd()
	defer os.Chdir(originalDir)

	if err := os.Chdir(projectDir); err != nil {
		t.Fatalf("Failed to change directory: %v", err)
	}

	// Test with autodiscovery enabled
	cfg := config.NewConfig()
	cfg.SetGlobalOption("script.autodiscovery", "true")

	registry := NewRegistryWithConfig(cfg)

	// Should find the test script
	scriptCommands := registry.ListScript()
	found := false
	expectedName := filepath.Base(testScript)
	for _, script := range scriptCommands {
		if script == expectedName {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("Expected to find testscript in discovered scripts, got: %v", scriptCommands)
	}
}
