package command

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/joeycumines/one-shot-man/internal/config"
)

// ScriptDiscoveryConfig holds configuration for script path discovery
type ScriptDiscoveryConfig struct {
	// EnableAutodiscovery enables advanced autodiscovery features (default: false)
	EnableAutodiscovery bool

	// CustomPaths are user-defined script paths
	CustomPaths []string

	// EnableGitTraversal enables traversing up directory tree to find git repositories
	EnableGitTraversal bool

	// MaxTraversalDepth limits how many directories to traverse upward (default: 10)
	MaxTraversalDepth int

	// ScriptPathPatterns are glob-like patterns for script directories (default: ["scripts"])
	ScriptPathPatterns []string
}

// ScriptDiscovery manages script path discovery with configurable rules
type ScriptDiscovery struct {
	config *ScriptDiscoveryConfig
}

// NewScriptDiscovery creates a new script discovery instance
func NewScriptDiscovery(cfg *config.Config) *ScriptDiscovery {
	discoveryConfig := &ScriptDiscoveryConfig{
		EnableAutodiscovery: false, // Off by default
		MaxTraversalDepth:   10,
		ScriptPathPatterns:  []string{"scripts"},
	}

	// Load configuration options
	if val, exists := cfg.GetGlobalOption("script.autodiscovery"); exists {
		discoveryConfig.EnableAutodiscovery = strings.ToLower(val) == "true"
	}

	if val, exists := cfg.GetGlobalOption("script.git-traversal"); exists {
		discoveryConfig.EnableGitTraversal = strings.ToLower(val) == "true"
	}

	if val, exists := cfg.GetGlobalOption("script.max-traversal-depth"); exists {
		if depth := parsePositiveInt(val, 10, 100); depth > 0 {
			discoveryConfig.MaxTraversalDepth = depth
		}
	}

	if val, exists := cfg.GetGlobalOption("script.paths"); exists {
		if paths := parsePathList(val); len(paths) > 0 {
			discoveryConfig.CustomPaths = paths
		}
	}

	if val, exists := cfg.GetGlobalOption("script.path-patterns"); exists {
		if patterns := parsePathList(val); len(patterns) > 0 {
			discoveryConfig.ScriptPathPatterns = patterns
		}
	}

	return &ScriptDiscovery{config: discoveryConfig}
}

// DiscoverScriptPaths returns all script paths based on configuration
func (sd *ScriptDiscovery) DiscoverScriptPaths() []string {
	var paths []string
	seenPaths := make(map[string]bool)

	// Add legacy hardcoded paths for backward compatibility
	legacyPaths := sd.getLegacyPaths()
	for _, path := range legacyPaths {
		if !seenPaths[path] {
			paths = append(paths, path)
			seenPaths[path] = true
		}
	}

	// Add custom paths from configuration
	for _, path := range sd.config.CustomPaths {
		expandedPath := sd.expandPath(path)
		if !seenPaths[expandedPath] {
			paths = append(paths, expandedPath)
			seenPaths[expandedPath] = true
		}
	}

	// Add autodiscovered paths if enabled
	if sd.config.EnableAutodiscovery {
		autoPaths := sd.autodiscoverPaths()
		for _, path := range autoPaths {
			if !seenPaths[path] {
				paths = append(paths, path)
				seenPaths[path] = true
			}
		}
	}

	// Sort by priority: closer to CWD first, then user paths, then system paths
	sort.Slice(paths, func(i, j int) bool {
		return sd.getPathPriority(paths[i]) < sd.getPathPriority(paths[j])
	})

	return paths
}

// getLegacyPaths returns the original hardcoded paths for backward compatibility
func (sd *ScriptDiscovery) getLegacyPaths() []string {
	var paths []string

	// 1. scripts/ directory relative to the executable
	if execPath, err := os.Executable(); err == nil {
		execDir := filepath.Dir(execPath)
		paths = append(paths, filepath.Join(execDir, "scripts"))
	}

	// 2. ~/.one-shot-man/scripts/ (user scripts)
	if configPath, err := config.GetConfigPath(); err == nil {
		configDir := filepath.Dir(configPath)
		paths = append(paths, filepath.Join(configDir, "scripts"))
	}

	// 3. ./scripts/ (current directory scripts)
	if cwd, err := os.Getwd(); err == nil {
		paths = append(paths, filepath.Join(cwd, "scripts"))
	}

	return paths
}

// autodiscoverPaths discovers script paths using advanced rules
func (sd *ScriptDiscovery) autodiscoverPaths() []string {
	var paths []string

	// Start from current working directory
	cwd, err := os.Getwd()
	if err != nil {
		return paths
	}

	// Traverse up directory tree looking for git repositories and script directories
	if sd.config.EnableGitTraversal {
		paths = append(paths, sd.traverseForGitRepos(cwd)...)
	}

	// Look for script directories in current path and parent directories
	paths = append(paths, sd.traverseForScriptDirs(cwd)...)

	return paths
}

// traverseForGitRepos traverses up from the given directory looking for git repositories
func (sd *ScriptDiscovery) traverseForGitRepos(startDir string) []string {
	var paths []string
	var gitRepos []string

	dir := startDir
	for i := 0; i < sd.config.MaxTraversalDepth; i++ {
		// Check if this directory is a git repository
		if sd.isGitRepository(dir) {
			gitRepos = append(gitRepos, dir)
		}

		// Move up one directory
		parent := filepath.Dir(dir)
		if parent == dir {
			break // Reached filesystem root
		}
		dir = parent
	}

	// Add script paths from git repositories (innermost first - highest priority)
	for _, repo := range gitRepos {
		for _, pattern := range sd.config.ScriptPathPatterns {
			scriptPath := filepath.Join(repo, pattern)
			if sd.directoryExists(scriptPath) {
				paths = append(paths, scriptPath)
			}
		}
	}

	return paths
}

// traverseForScriptDirs traverses up from the given directory looking for script directories
func (sd *ScriptDiscovery) traverseForScriptDirs(startDir string) []string {
	var paths []string

	dir := startDir
	for i := 0; i < sd.config.MaxTraversalDepth; i++ {
		// Check for script directories using configured patterns
		for _, pattern := range sd.config.ScriptPathPatterns {
			scriptPath := filepath.Join(dir, pattern)
			if sd.directoryExists(scriptPath) {
				paths = append(paths, scriptPath)
			}
		}

		// Move up one directory
		parent := filepath.Dir(dir)
		if parent == dir {
			break // Reached filesystem root
		}
		dir = parent
	}

	return paths
}

// isGitRepository checks if a directory is a git repository
func (sd *ScriptDiscovery) isGitRepository(dir string) bool {
	cmd := exec.Command("git", "rev-parse", "--git-dir")
	cmd.Dir = dir
	return cmd.Run() == nil
}

// directoryExists checks if a directory exists
func (sd *ScriptDiscovery) directoryExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// expandPath expands tilde and environment variables in paths
func (sd *ScriptDiscovery) expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		if homeDir, err := os.UserHomeDir(); err == nil {
			return filepath.Join(homeDir, path[2:])
		}
	}
	return os.ExpandEnv(path)
}

// getPathPriority returns priority value for sorting paths (lower = higher priority)
func (sd *ScriptDiscovery) getPathPriority(path string) int {
	cwd, _ := os.Getwd()

	// Highest priority: paths in current directory or subdirectories
	if strings.HasPrefix(path, cwd) {
		return 1
	}

	// Medium priority: user config paths
	if homeDir, err := os.UserHomeDir(); err == nil {
		configDir := filepath.Join(homeDir, ".one-shot-man")
		if strings.HasPrefix(path, configDir) {
			return 2
		}
	}

	// Lower priority: system/executable paths
	if execPath, err := os.Executable(); err == nil {
		execDir := filepath.Dir(execPath)
		if strings.HasPrefix(path, execDir) {
			return 3
		}
	}

	// Lowest priority: everything else
	return 4
}

// parsePositiveInt parses a string as a positive integer within the given range.
// Values outside the range or invalid inputs result in the default value being returned.
func parsePositiveInt(s string, defaultVal, max int) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return defaultVal
	}

	value, err := strconv.Atoi(s)
	if err != nil {
		log.Printf("warning: invalid positive integer %q: %v", s, err)
		return defaultVal
	}

	if value < 1 {
		return defaultVal
	}

	if max > 0 && value > max {
		return defaultVal
	}

	return value
}

// parsePathList parses a colon or comma-separated list of paths
func parsePathList(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}

	splitter := func(r rune) bool {
		return r == ',' || r == rune(filepath.ListSeparator)
	}

	parts := strings.FieldsFunc(s, splitter)
	if len(parts) == 0 {
		return nil
	}

	paths := parts[:0]
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			paths = append(paths, trimmed)
		}
	}

	if len(paths) == 0 {
		return nil
	}

	return paths
}
