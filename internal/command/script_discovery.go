package command

import (
	"log"
	"math"
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
		sd.addPath(&paths, seenPaths, path)
	}

	// Add custom paths from configuration
	for _, path := range sd.config.CustomPaths {
		expandedPath := sd.expandPath(path)
		sd.addPath(&paths, seenPaths, expandedPath)
	}

	// Add autodiscovered paths if enabled
	if sd.config.EnableAutodiscovery {
		autoPaths := sd.autodiscoverPaths()
		for _, path := range autoPaths {
			sd.addPath(&paths, seenPaths, path)
		}
	}

	// Sort by priority: closer to CWD first, then user paths, then system paths
	cwd, _ := os.Getwd()

	var configDir string
	if configPath, err := config.GetConfigPath(); err == nil {
		configDir = filepath.Dir(configPath)
	}

	var execDir string
	if execPath, err := os.Executable(); err == nil {
		execDir = filepath.Dir(execPath)
	}

	sort.Slice(paths, func(i, j int) bool {
		pi := sd.computePathScore(paths[i], cwd, configDir, execDir)
		pj := sd.computePathScore(paths[j], cwd, configDir, execDir)

		if pi.class != pj.class {
			return pi.class < pj.class
		}

		if pi.distance != pj.distance {
			return pi.distance < pj.distance
		}

		if pi.depth != pj.depth {
			return pi.depth < pj.depth
		}

		return paths[i] < paths[j]
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

func (sd *ScriptDiscovery) normalizePath(path string) (string, error) {
	cleaned := filepath.Clean(path)
	return filepath.Abs(cleaned)
}

func (sd *ScriptDiscovery) addPath(paths *[]string, seenPaths map[string]bool, candidate string) {
	if strings.TrimSpace(candidate) == "" {
		return
	}

	normalized, err := sd.normalizePath(candidate)
	if err != nil {
		log.Printf("warning: skipping script path %q: %v", candidate, err)
		return
	}

	if !seenPaths[normalized] {
		*paths = append(*paths, normalized)
		seenPaths[normalized] = true
	}
}

type pathScore struct {
	class    int
	distance int
	depth    int
}

func (sd *ScriptDiscovery) computePathScore(path, cwd, configDir, execDir string) pathScore {
	score := pathScore{
		class:    4,
		distance: math.MaxInt,
		depth:    math.MaxInt,
	}

	if cwd != "" {
		if rel, err := filepath.Rel(cwd, path); err == nil {
			rel = filepath.Clean(rel)

			segments := splitPathSegments(rel)
			upCount, downCount := countRelSegments(segments)

			if rel == "." {
				return pathScore{class: 0, distance: 0, depth: 0}
			}

			if upCount == 0 {
				return pathScore{class: 0, distance: downCount, depth: downCount}
			}

			if sd.matchesAncestorPattern(segments) {
				return pathScore{class: 1, distance: upCount, depth: downCount}
			}
		}
	}

	if hasDirPrefix(path, configDir) {
		depth := pathDepthRelative(path, configDir)
		return pathScore{class: 2, distance: depth, depth: depth}
	}

	if hasDirPrefix(path, execDir) {
		depth := pathDepthRelative(path, execDir)
		return pathScore{class: 3, distance: depth, depth: depth}
	}

	return score
}

func splitPathSegments(rel string) []string {
	if rel == "" {
		return nil
	}
	return strings.Split(rel, string(os.PathSeparator))
}

func countRelSegments(segments []string) (upCount, downCount int) {
	for _, segment := range segments {
		switch segment {
		case "", ".":
			continue
		case "..":
			upCount++
		default:
			downCount++
		}
	}
	return
}

func hasDirPrefix(path, dir string) bool {
	if dir == "" {
		return false
	}
	if path == dir {
		return true
	}
	separator := string(os.PathSeparator)
	dir = strings.TrimSuffix(dir, separator)
	return strings.HasPrefix(path, dir+separator)
}

func pathDepthRelative(path, base string) int {
	if base == "" {
		return 0
	}
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return 0
	}
	rel = filepath.Clean(rel)
	_, down := countRelSegments(splitPathSegments(rel))
	return down
}

func (sd *ScriptDiscovery) matchesAncestorPattern(segments []string) bool {
	downSegments := collectDownSegments(segments)
	if len(downSegments) == 0 {
		return false
	}

	for _, pattern := range sd.config.ScriptPathPatterns {
		pattern = filepath.Clean(pattern)
		patternSegments := collectDownSegments(splitPathSegments(pattern))
		if len(patternSegments) == 0 {
			continue
		}
		if len(patternSegments) != len(downSegments) {
			continue
		}
		match := true
		for i := range downSegments {
			if patternSegments[i] != downSegments[i] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}

	return false
}

func collectDownSegments(segments []string) []string {
	if len(segments) == 0 {
		return nil
	}
	var down []string
	for _, segment := range segments {
		switch segment {
		case "", ".":
			continue
		case "..":
			continue
		default:
			down = append(down, segment)
		}
	}
	return down
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
