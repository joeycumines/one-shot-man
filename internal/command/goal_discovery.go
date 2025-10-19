package command

import (
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/joeycumines/one-shot-man/internal/config"
)

// GoalDiscoveryConfig holds configuration for goal path discovery
type GoalDiscoveryConfig struct {
	// EnableAutodiscovery enables advanced autodiscovery features (default: true for goals)
	EnableAutodiscovery bool

	// CustomPaths are user-defined goal paths
	CustomPaths []string

	// MaxTraversalDepth limits how many directories to traverse upward (default: 10)
	MaxTraversalDepth int

	// GoalPathPatterns are patterns for goal directories (default: ["osm-goals", "goals"])
	GoalPathPatterns []string

	// DisableStandardPaths disables standard goal paths like ~/.one-shot-man/goals,
	// $exe/goals, and ./osm-goals. Useful for tests to ensure determinism.
	DisableStandardPaths bool
}

// GoalDiscovery manages goal path discovery with configurable rules
type GoalDiscovery struct {
	config *GoalDiscoveryConfig
}

// NewGoalDiscovery creates a new goal discovery instance
func NewGoalDiscovery(cfg *config.Config) *GoalDiscovery {
	discoveryConfig := &GoalDiscoveryConfig{
		EnableAutodiscovery: true, // On by default for goals
		MaxTraversalDepth:   10,
		GoalPathPatterns:    []string{"osm-goals", "goals"},
	}

	// Load configuration options

	if val, exists := cfg.GetGlobalOption("goal.autodiscovery"); exists {
		result, _ := strconv.ParseBool(val)
		discoveryConfig.EnableAutodiscovery = result
	}

	if val, exists := cfg.GetGlobalOption("goal.disable-standard-paths"); exists {
		if parsed, err := strconv.ParseBool(val); err == nil {
			discoveryConfig.DisableStandardPaths = parsed
		}
	}

	if val, exists := cfg.GetGlobalOption("goal.max-traversal-depth"); exists {
		if depth := parsePositiveInt(val, 10, 100); depth > 0 {
			discoveryConfig.MaxTraversalDepth = depth
		}
	}

	if val, exists := cfg.GetGlobalOption("goal.paths"); exists {
		if paths := parsePathList(val); len(paths) > 0 {
			discoveryConfig.CustomPaths = paths
		}
	}

	if val, exists := cfg.GetGlobalOption("goal.path-patterns"); exists {
		if patterns := parsePathList(val); len(patterns) > 0 {
			discoveryConfig.GoalPathPatterns = patterns
		}
	}

	// Environment overrides (primarily for tests/CI)
	if v, _ := strconv.ParseBool(os.Getenv("OSM_DISABLE_GOAL_AUTODISCOVERY")); v {
		discoveryConfig.EnableAutodiscovery = false
	}

	return &GoalDiscovery{config: discoveryConfig}
}

// DiscoverGoalPaths returns all goal paths based on configuration
func (gd *GoalDiscovery) DiscoverGoalPaths() []string {
	var paths []string
	seenPaths := make(map[string]bool)

	// Add standard paths
	standardPaths := gd.getStandardPaths()
	for _, path := range standardPaths {
		gd.addPath(&paths, seenPaths, path)
	}

	// Add custom paths from configuration
	for _, path := range gd.config.CustomPaths {
		expandedPath := gd.expandPath(path)
		gd.addPath(&paths, seenPaths, expandedPath)
	}

	// Add autodiscovered paths if enabled
	if gd.config.EnableAutodiscovery {
		autoPaths := gd.autodiscoverPaths()
		for _, path := range autoPaths {
			gd.addPath(&paths, seenPaths, path)
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
		pi := gd.computePathScore(paths[i], cwd, configDir, execDir)
		pj := gd.computePathScore(paths[j], cwd, configDir, execDir)

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

// getStandardPaths returns the standard goal discovery paths
func (gd *GoalDiscovery) getStandardPaths() []string {
	var paths []string

	// Allow disabling standard goal paths via config/env (useful for tests)
	if gd.config.DisableStandardPaths {
		return paths
	}

	// 1. ~/.one-shot-man/goals/ (user goals)
	if configPath, err := config.GetConfigPath(); err == nil {
		configDir := filepath.Dir(configPath)
		paths = append(paths, filepath.Join(configDir, "goals"))
	}

	// 2. goals/ directory relative to the executable
	if execPath, err := os.Executable(); err == nil {
		execDir := filepath.Dir(execPath)
		paths = append(paths, filepath.Join(execDir, "goals"))
	}

	// 3. ./osm-goals/ (current directory goals - primary pattern)
	if cwd, err := os.Getwd(); err == nil {
		paths = append(paths, filepath.Join(cwd, "osm-goals"))
	}

	return paths
}

// autodiscoverPaths discovers goal paths using advanced rules
func (gd *GoalDiscovery) autodiscoverPaths() []string {
	var paths []string

	// Start from current working directory
	cwd, err := os.Getwd()
	if err != nil {
		return paths
	}

	// Look for goal directories in current path and parent directories
	paths = append(paths, gd.traverseForGoalDirs(cwd)...)

	return paths
}

// traverseForGoalDirs traverses up from the given directory looking for goal directories
func (gd *GoalDiscovery) traverseForGoalDirs(startDir string) []string {
	var paths []string

	dir := startDir
	for i := 0; i < gd.config.MaxTraversalDepth; i++ {
		// Check for goal directories using configured patterns
		for _, pattern := range gd.config.GoalPathPatterns {
			goalPath := filepath.Join(dir, pattern)
			if gd.directoryExists(goalPath) {
				paths = append(paths, goalPath)
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

// directoryExists checks if a directory exists
func (gd *GoalDiscovery) directoryExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// expandPath expands tilde and environment variables in paths
func (gd *GoalDiscovery) expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		if homeDir, err := os.UserHomeDir(); err == nil {
			return filepath.Join(homeDir, path[2:])
		}
	}
	return os.ExpandEnv(path)
}

func (gd *GoalDiscovery) normalizePath(path string) (string, error) {
	cleaned := filepath.Clean(path)
	absPath, err := filepath.Abs(cleaned)
	if err != nil {
		return "", err
	}

	// Resolve symlinks to ensure semantic deduplication
	// (a directory and a symlink to it should be treated as the same path)
	resolvedPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		// If symlink resolution fails (e.g., broken symlink), fall back to absolute path
		return absPath, nil
	}

	// SECURITY: Validate that the resolved path is within the canonical parent directory.
	// Canonicalize the parent directory (resolve symlinks) and ensure the resolved
	// target path has the parent directory as a strict prefix.
	baseDir := filepath.Dir(absPath)
	resolvedBaseDir, err := filepath.EvalSymlinks(baseDir)
	if err != nil {
		// Fail closed if the base directory can't be validated.
		return "", fmt.Errorf("failed to resolve base directory symlinks for %q: %w", baseDir, err)
	}

	// Clean up for stable comparisons.
	rp := filepath.Clean(resolvedPath)
	pb := filepath.Clean(resolvedBaseDir)

	// Ensure pb ends with a path separator for strict prefix checking.
	// This prevents false positives like /foo/bar matching /foo/barista.
	sep := string(filepath.Separator)
	pbWithSep := pb
	if !strings.HasSuffix(pbWithSep, sep) {
		pbWithSep += sep
	}

	// Require the target to be a strict descendant of the parent directory.
	if !strings.HasPrefix(rp, pbWithSep) {
		return "", fmt.Errorf("symlink validation failed for %q: resolved path %q escapes parent %q", path, rp, pb)
	}

	return rp, nil
}

func (gd *GoalDiscovery) addPath(paths *[]string, seenPaths map[string]bool, candidate string) {
	if strings.TrimSpace(candidate) == "" {
		return
	}

	normalized, err := gd.normalizePath(candidate)
	if err != nil {
		log.Printf("warning: skipping goal path %q: %v", candidate, err)
		return
	}

	if !seenPaths[normalized] {
		*paths = append(*paths, normalized)
		seenPaths[normalized] = true
	}
}

type goalPathScore struct {
	class    int
	distance int
	depth    int
}

func (gd *GoalDiscovery) computePathScore(path, cwd, configDir, execDir string) goalPathScore {
	score := goalPathScore{
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
				return goalPathScore{class: 0, distance: 0, depth: 0}
			}

			if upCount == 0 {
				return goalPathScore{class: 0, distance: downCount, depth: downCount}
			}

			if gd.matchesAncestorPattern(segments) {
				return goalPathScore{class: 1, distance: upCount, depth: downCount}
			}
		}
	}

	if hasDirPrefix(path, configDir) {
		depth := pathDepthRelative(path, configDir)
		return goalPathScore{class: 2, distance: depth, depth: depth}
	}

	if hasDirPrefix(path, execDir) {
		depth := pathDepthRelative(path, execDir)
		return goalPathScore{class: 3, distance: depth, depth: depth}
	}

	return score
}

func (gd *GoalDiscovery) matchesAncestorPattern(segments []string) bool {
	downSegments := collectDownSegments(segments)
	if len(downSegments) == 0 {
		return false
	}

	for _, pattern := range gd.config.GoalPathPatterns {
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
