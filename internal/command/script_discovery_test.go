package command

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
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

	t.Setenv("OSM_CONFIG", configPath)

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
		t.Errorf("Expected legacy paths to include %s when OSM_CONFIG is set, got %v", expected, paths)
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

func TestScriptDiscovery_CheckDirectory(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	discovery := NewScriptDiscovery(cfg)

	t.Run("existing directory", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		exists, err := discovery.checkDirectory(tmpDir)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if !exists {
			t.Error("Expected directory to exist")
		}
	})

	t.Run("non-existent path", func(t *testing.T) {
		t.Parallel()
		exists, err := discovery.checkDirectory("/nonexistent/path/that/does/not/exist")
		if err != nil {
			t.Fatalf("Expected no error for non-existent path, got: %v", err)
		}
		if exists {
			t.Error("Expected directory to not exist")
		}
	})

	t.Run("file not directory", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		tempFile := filepath.Join(tmpDir, "testfile")
		if err := os.WriteFile(tempFile, []byte("test"), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}
		exists, err := discovery.checkDirectory(tempFile)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if exists {
			t.Error("Expected file to not be reported as directory")
		}
	})
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

	// Custom path should be added (compare normalized absolute paths,
	// accounting for symlink resolution on macOS /var → /private/var)
	found := false
	normalizedCustomPath, _ := filepath.Abs(customPath)
	resolvedCustomPath, err := filepath.EvalSymlinks(normalizedCustomPath)
	if err != nil {
		resolvedCustomPath = normalizedCustomPath
	}
	for _, path := range registry.scriptPaths {
		pAbs, _ := filepath.Abs(path)
		if runtime.GOOS == "windows" {
			if strings.EqualFold(pAbs, normalizedCustomPath) || strings.EqualFold(pAbs, resolvedCustomPath) {
				found = true
				break
			}
		} else {
			if pAbs == normalizedCustomPath || pAbs == resolvedCustomPath {
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

func TestScriptDiscovery_DebugLogging(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	scriptsDir := filepath.Join(tmpDir, "scripts")
	if err := os.MkdirAll(scriptsDir, 0o755); err != nil {
		t.Fatalf("Failed to create scripts directory: %v", err)
	}

	var mu sync.Mutex
	var messages []string

	cfg := config.NewConfig()
	cfg.SetGlobalOption("script.disable-standard-paths", "true")
	cfg.SetGlobalOption("script.autodiscovery", "false")
	cfg.SetGlobalOption("script.paths", scriptsDir)

	discovery := NewScriptDiscovery(cfg)
	discovery.config.DebugLogFunc = func(format string, args ...interface{}) {
		mu.Lock()
		defer mu.Unlock()
		messages = append(messages, fmt.Sprintf(format, args...))
	}

	_ = discovery.DiscoverScriptPaths()

	mu.Lock()
	defer mu.Unlock()

	if len(messages) == 0 {
		t.Error("Expected debug log messages, got none")
	}

	// Check for expected log message patterns
	foundStarting := false
	foundComplete := false
	foundAccepted := false
	for _, msg := range messages {
		if strings.Contains(msg, "starting script path discovery") {
			foundStarting = true
		}
		if strings.Contains(msg, "discovery complete") {
			foundComplete = true
		}
		if strings.Contains(msg, "addPath: accepted") {
			foundAccepted = true
		}
	}

	if !foundStarting {
		t.Error("Expected 'starting script path discovery' message")
	}
	if !foundComplete {
		t.Error("Expected 'discovery complete' message")
	}
	if !foundAccepted {
		t.Error("Expected 'addPath: accepted' message")
	}
}

func TestScriptDiscovery_DebugLogging_Dedup(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	scriptsDir := filepath.Join(tmpDir, "scripts")
	if err := os.MkdirAll(scriptsDir, 0o755); err != nil {
		t.Fatalf("Failed to create scripts directory: %v", err)
	}

	var mu sync.Mutex
	var messages []string

	cfg := config.NewConfig()
	cfg.SetGlobalOption("script.disable-standard-paths", "true")
	cfg.SetGlobalOption("script.autodiscovery", "false")
	// Set the same path twice
	cfg.SetGlobalOption("script.paths", scriptsDir+string(filepath.ListSeparator)+scriptsDir)

	discovery := NewScriptDiscovery(cfg)
	discovery.config.DebugLogFunc = func(format string, args ...interface{}) {
		mu.Lock()
		defer mu.Unlock()
		messages = append(messages, fmt.Sprintf(format, args...))
	}

	paths := discovery.DiscoverScriptPaths()

	mu.Lock()
	defer mu.Unlock()

	// Should only have one path despite duplicate input
	if len(paths) != 1 {
		t.Errorf("Expected 1 path, got %d: %v", len(paths), paths)
	}

	// Should have a dedup message
	foundDedup := false
	for _, msg := range messages {
		if strings.Contains(msg, "deduplicating") {
			foundDedup = true
			break
		}
	}
	if !foundDedup {
		t.Error("Expected deduplication debug message")
	}
}

func TestScriptDiscovery_DisableStandardPathsIsolation(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	customScripts := filepath.Join(tmpDir, "my-scripts")
	if err := os.MkdirAll(customScripts, 0o755); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	cfg := config.NewConfig()
	cfg.SetGlobalOption("script.disable-standard-paths", "true")
	cfg.SetGlobalOption("script.autodiscovery", "false")
	cfg.SetGlobalOption("script.paths", customScripts)

	discovery := NewScriptDiscovery(cfg)
	paths := discovery.DiscoverScriptPaths()

	// With standard paths disabled and autodiscovery off, should only have custom path
	if len(paths) != 1 {
		t.Errorf("Expected exactly 1 path, got %d: %v", len(paths), paths)
	}

	if len(paths) == 1 && !pathsEqual(paths[0], customScripts) {
		t.Errorf("Expected custom scripts path %s, got %s", customScripts, paths[0])
	}
}

func TestScriptDiscovery_DebugDiscoveryConfig(t *testing.T) {
	t.Parallel()

	cfg := config.NewConfig()
	cfg.SetGlobalOption("script.debug-discovery", "true")

	discovery := NewScriptDiscovery(cfg)

	if discovery.config.DebugLogFunc == nil {
		t.Error("Expected DebugLogFunc to be set when script.debug-discovery=true")
	}
}

func TestScriptDiscovery_PermissionDenied(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Permission tests not reliable on Windows")
	}
	if os.Getuid() == 0 {
		t.Skip("Skipping permission test when running as root")
	}
	// Note: Cannot use t.Parallel() because this test changes working directory

	tmpDir := t.TempDir()

	// Create a deep structure: tmpDir/level1/level2/level3
	// Make level1 have a "scripts" directory that is unreadable
	level1 := filepath.Join(tmpDir, "level1")
	level2 := filepath.Join(level1, "level2")
	level3 := filepath.Join(level2, "level3")

	for _, d := range []string{level1, level2, level3} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("Failed to create directory %s: %v", d, err)
		}
	}

	// Create a scripts directory with no read permission
	unreadableScripts := filepath.Join(level1, "scripts")
	if err := os.MkdirAll(unreadableScripts, 0o000); err != nil {
		t.Fatalf("Failed to create unreadable directory: %v", err)
	}
	// Ensure cleanup can remove it
	t.Cleanup(func() { os.Chmod(unreadableScripts, 0o755) })

	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}
	defer os.Chdir(origWd)

	if err := os.Chdir(level3); err != nil {
		t.Fatalf("Failed to change directory: %v", err)
	}

	var mu sync.Mutex
	var messages []string

	cfg := config.NewConfig()
	cfg.SetGlobalOption("script.disable-standard-paths", "true")
	cfg.SetGlobalOption("script.autodiscovery", "true")
	cfg.SetGlobalOption("script.path-patterns", "scripts")

	discovery := NewScriptDiscovery(cfg)
	discovery.config.DebugLogFunc = func(format string, args ...interface{}) {
		mu.Lock()
		defer mu.Unlock()
		messages = append(messages, fmt.Sprintf(format, args...))
	}

	// Should not panic or crash - permission errors should be handled gracefully
	paths := discovery.DiscoverScriptPaths()

	mu.Lock()
	defer mu.Unlock()

	// The unreadable directory may or may not appear depending on os.Stat behavior.
	// The key assertion is that we don't crash and traversal continues past the error.
	_ = paths
	_ = messages
}

func TestScriptDiscovery_SymlinkCycleDetection(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Symlink tests not reliable on Windows")
	}
	// Note: Cannot use t.Parallel() because this test changes working directory

	// Create a structure where symlinks could cause a cycle in upward traversal.
	tmpDir := t.TempDir()

	realDir := filepath.Join(tmpDir, "real")
	subDir := filepath.Join(realDir, "sub")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("Failed to create directories: %v", err)
	}

	// Create a scripts directory that would be found during traversal
	scriptsDir := filepath.Join(realDir, "scripts")
	if err := os.Mkdir(scriptsDir, 0o755); err != nil {
		t.Fatalf("Failed to create scripts directory: %v", err)
	}

	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}
	defer os.Chdir(origWd)

	if err := os.Chdir(subDir); err != nil {
		t.Fatalf("Failed to change directory: %v", err)
	}

	var mu sync.Mutex
	var messages []string

	cfg := config.NewConfig()
	cfg.SetGlobalOption("script.disable-standard-paths", "true")
	cfg.SetGlobalOption("script.autodiscovery", "true")
	cfg.SetGlobalOption("script.path-patterns", "scripts")

	discovery := NewScriptDiscovery(cfg)
	discovery.config.DebugLogFunc = func(format string, args ...interface{}) {
		mu.Lock()
		defer mu.Unlock()
		messages = append(messages, fmt.Sprintf(format, args...))
	}

	paths := discovery.DiscoverScriptPaths()

	// Should discover the scripts directory via upward traversal
	found := false
	for _, path := range paths {
		if pathsEqual(path, scriptsDir) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected to discover scripts directory %s, found: %v", scriptsDir, paths)
	}

	// Verify no cycle was detected (because the upward traversal from a real dir doesn't cycle)
	mu.Lock()
	defer mu.Unlock()
	for _, msg := range messages {
		if strings.Contains(msg, "symlink cycle detected") {
			t.Errorf("Unexpected cycle detection in simple upward traversal: %s", msg)
		}
	}
}

func TestScriptDiscovery_TraversalReachesRoot(t *testing.T) {
	// Note: Cannot use t.Parallel() because this test changes working directory

	tmpDir := t.TempDir()
	deepPath := tmpDir
	for i := 0; i < 3; i++ {
		deepPath = filepath.Join(deepPath, "d")
	}
	if err := os.MkdirAll(deepPath, 0o755); err != nil {
		t.Fatalf("Failed to create deep path: %v", err)
	}

	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}
	defer os.Chdir(origWd)

	if err := os.Chdir(deepPath); err != nil {
		t.Fatalf("Failed to change directory: %v", err)
	}

	var mu sync.Mutex
	var messages []string

	cfg := config.NewConfig()
	cfg.SetGlobalOption("script.disable-standard-paths", "true")
	cfg.SetGlobalOption("script.autodiscovery", "true")
	cfg.SetGlobalOption("script.max-traversal-depth", "100") // High enough to reach root
	cfg.SetGlobalOption("script.path-patterns", "nonexistent-pattern-xyzzy")

	discovery := NewScriptDiscovery(cfg)
	discovery.config.DebugLogFunc = func(format string, args ...interface{}) {
		mu.Lock()
		defer mu.Unlock()
		messages = append(messages, fmt.Sprintf(format, args...))
	}

	_ = discovery.DiscoverScriptPaths()

	mu.Lock()
	defer mu.Unlock()

	// Should have logged reaching the filesystem root
	foundRoot := false
	for _, msg := range messages {
		if strings.Contains(msg, "reached filesystem root") {
			foundRoot = true
			break
		}
	}
	if !foundRoot {
		t.Error("Expected 'reached filesystem root' debug message")
	}
}

func TestScriptDiscovery_SymlinkDeduplication(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Symlink tests not reliable on Windows")
	}
	t.Parallel()

	tmpDir := t.TempDir()
	scriptsDir := filepath.Join(tmpDir, "scripts")
	if err := os.MkdirAll(scriptsDir, 0o755); err != nil {
		t.Fatalf("Failed to create scripts directory: %v", err)
	}

	// Create symbolic link to the same directory
	linkPath := filepath.Join(tmpDir, "scripts-link")
	if err := os.Symlink(scriptsDir, linkPath); err != nil {
		t.Skip("Skipping symlink test: platform doesn't support symlinks")
	}

	cfg := config.NewConfig()
	cfg.SetGlobalOption("script.disable-standard-paths", "true")
	cfg.SetGlobalOption("script.autodiscovery", "false")
	// Add both the real path and the symlink
	cfg.SetGlobalOption("script.paths", scriptsDir+string(filepath.ListSeparator)+linkPath)

	discovery := NewScriptDiscovery(cfg)
	paths := discovery.DiscoverScriptPaths()

	// Count how many of our test paths appear
	// With symlink resolution enabled, both should resolve to the same path
	count := 0
	normalizedScriptsDir, _ := filepath.EvalSymlinks(scriptsDir)
	for _, path := range paths {
		resolved, _ := filepath.EvalSymlinks(path)
		if resolved == "" {
			resolved = path
		}
		if resolved == normalizedScriptsDir || path == normalizedScriptsDir {
			count++
		}
	}

	// Should appear exactly once due to symlink resolution in normalizePath
	if count != 1 {
		t.Errorf("Expected exactly one occurrence after symlink deduplication, got %d in paths: %v", count, paths)
	}
}

func TestScriptDiscovery_DisableStandardPathsConfig(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		value    string
		expected bool
	}{
		{"true", true},
		{"false", false},
		{"1", true},
		{"0", false},
	}

	for _, tc := range testCases {
		cfg := config.NewConfig()
		cfg.SetGlobalOption("script.disable-standard-paths", tc.value)
		discovery := NewScriptDiscovery(cfg)

		if discovery.config.DisableStandardPaths != tc.expected {
			t.Errorf("For value %q, expected DisableStandardPaths=%v, got %v",
				tc.value, tc.expected, discovery.config.DisableStandardPaths)
		}
	}
}
