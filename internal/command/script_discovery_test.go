package command

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/config"
)

func TestScriptDiscovery_DefaultBehavior(t *testing.T) {
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
	cfg := config.NewConfig()
	cfg.SetGlobalOption("script.autodiscovery", "true")
	cfg.SetGlobalOption("script.git-traversal", "true")
	cfg.SetGlobalOption("script.max-traversal-depth", "5")
	cfg.SetGlobalOption("script.paths", "~/custom-scripts:/opt/scripts")
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
	cfg := config.NewConfig()
	discovery := NewScriptDiscovery(cfg)

	paths := discovery.getLegacyPaths()

	// Should have at least the current directory scripts path
	found := false
	cwd, _ := os.Getwd()
	expectedPath := filepath.Join(cwd, "scripts")
	
	for _, path := range paths {
		if path == expectedPath {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("Expected to find current directory scripts path %s in legacy paths %v", expectedPath, paths)
	}
}

func TestScriptDiscovery_PathExpansion(t *testing.T) {
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
	cfg := config.NewConfig()
	discovery := NewScriptDiscovery(cfg)

	// Test in current directory (should be a git repo for this project)
	cwd, _ := os.Getwd()
	
	// Navigate to project root (we might be in a subdirectory)
	projectRoot := cwd
	for {
		if discovery.isGitRepository(projectRoot) {
			break
		}
		parent := filepath.Dir(projectRoot)
		if parent == projectRoot {
			// Reached filesystem root without finding git repo
			t.Skip("Not running in a git repository, skipping git detection test")
			return
		}
		projectRoot = parent
	}

	if !discovery.isGitRepository(projectRoot) {
		t.Errorf("Expected %s to be detected as git repository", projectRoot)
	}

	// Test a definitely non-git directory
	tmpDir := t.TempDir()
	if discovery.isGitRepository(tmpDir) {
		t.Errorf("Expected %s to not be detected as git repository", tmpDir)
	}
}

func TestScriptDiscovery_DirectoryExists(t *testing.T) {
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
	cfg := config.NewConfig()
	discovery := NewScriptDiscovery(cfg)

	cwd, _ := os.Getwd()
	
	// Create test paths
	cwdPath := filepath.Join(cwd, "scripts")
	
	var userPath string
	if homeDir, err := os.UserHomeDir(); err == nil {
		userPath = filepath.Join(homeDir, ".one-shot-man", "scripts")
	}
	
	var execPath string
	if execDir, err := os.Executable(); err == nil {
		execPath = filepath.Join(filepath.Dir(execDir), "scripts")
	}
	
	otherPath := "/some/other/path/scripts"

	// Test priorities (lower number = higher priority)
	if userPath != "" {
		cwdPriority := discovery.getPathPriority(cwdPath)
		userPriority := discovery.getPathPriority(userPath)
		
		if cwdPriority >= userPriority {
			t.Errorf("Expected current directory path to have higher priority than user path, got cwd=%d user=%d", cwdPriority, userPriority)
		}
	}

	if execPath != "" {
		cwdPriority := discovery.getPathPriority(cwdPath)
		execPriority := discovery.getPathPriority(execPath)
		
		if cwdPriority >= execPriority {
			t.Errorf("Expected current directory path to have higher priority than exec path, got cwd=%d exec=%d", cwdPriority, execPriority)
		}
	}

	otherPriority := discovery.getPathPriority(otherPath)
	cwdPriority := discovery.getPathPriority(cwdPath)
	
	if cwdPriority >= otherPriority {
		t.Errorf("Expected current directory path to have higher priority than other path, got cwd=%d other=%d", cwdPriority, otherPriority)
	}
}

func TestScriptDiscovery_ParsePathList(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"", nil},
		{"  ", nil},
		{"single", []string{"single"}},
		{"path1:path2", []string{"path1", "path2"}},
		{"path1,path2", []string{"path1", "path2"}},
		{"path1:path2:path3", []string{"path1", "path2", "path3"}},
		{" path1 : path2 ", []string{"path1", "path2"}},
		{" path1 , path2 ", []string{"path1", "path2"}},
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
	tests := []struct {
		input    string
		def      int
		expected int
	}{
		{"", 10, 10},
		{"0", 10, 10},
		{"-1", 10, 10},
		{"5", 10, 5},
		{"123", 10, 123},
		{"abc", 10, 10},
		{"12abc", 10, 10},
	}

	for _, test := range tests {
		result := parsePositiveInt(test.input, test.def)
		if result != test.expected {
			t.Errorf("parsePositiveInt(%q, %d): expected %d, got %d", test.input, test.def, test.expected, result)
		}
	}
}

func TestNewRegistryWithConfig_Integration(t *testing.T) {
	// Test that the registry properly integrates with script discovery
	cfg := config.NewConfig()
	cfg.SetGlobalOption("script.paths", "/custom/path")

	registry := NewRegistryWithConfig(cfg)

	// Should have discovered at least legacy paths
	if len(registry.scriptPaths) == 0 {
		t.Error("Expected registry to have discovered some script paths")
	}

	// Custom path should be added
	found := false
	for _, path := range registry.scriptPaths {
		if path == "/custom/path" {
			found = true
			break
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
	
	// Create a test script
	testScript := filepath.Join(scriptsDir, "testscript")
	if err := os.WriteFile(testScript, []byte("#!/bin/bash\necho test"), 0755); err != nil {
		t.Fatalf("Failed to create test script: %v", err)
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
	for _, script := range scriptCommands {
		if script == "testscript" {
			found = true
			break
		}
	}
	
	if !found {
		t.Errorf("Expected to find testscript in discovered scripts, got: %v", scriptCommands)
	}
}