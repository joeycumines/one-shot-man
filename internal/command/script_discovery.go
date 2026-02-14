package command

import (
	"fmt"
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

	// DisableStandardPaths disables standard script paths like ~/.one-shot-man/scripts,
	// $exe/scripts, and ./scripts. Useful for tests to ensure determinism.
	DisableStandardPaths bool

	// DebugLogFunc is called with debug messages during discovery.
	// If nil, debug logging is suppressed. Useful for troubleshooting
	// why specific script directories are or aren't discovered.
	DebugLogFunc func(format string, args ...interface{})
}

// ScriptDiscovery manages script path discovery with configurable rules
type ScriptDiscovery struct {
	config *ScriptDiscoveryConfig
}

// debugf logs a debug message if DebugLogFunc is configured.
func (sd *ScriptDiscovery) debugf(format string, args ...interface{}) {
	if sd.config.DebugLogFunc != nil {
		sd.config.DebugLogFunc(format, args...)
	}
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
		result, _ := strconv.ParseBool(val)
		discoveryConfig.EnableAutodiscovery = result
	}

	if val, exists := cfg.GetGlobalOption("script.git-traversal"); exists {
		result, _ := strconv.ParseBool(val)
		discoveryConfig.EnableGitTraversal = result
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

	if val, exists := cfg.GetGlobalOption("script.disable-standard-paths"); exists {
		if parsed, err := strconv.ParseBool(val); err == nil {
			discoveryConfig.DisableStandardPaths = parsed
		}
	}

	// Enable debug logging via config option
	if val, exists := cfg.GetGlobalOption("script.debug-discovery"); exists {
		if parsed, _ := strconv.ParseBool(val); parsed {
			discoveryConfig.DebugLogFunc = func(format string, args ...interface{}) {
				log.Printf("[script-discovery] "+format, args...)
			}
		}
	}

	return &ScriptDiscovery{config: discoveryConfig}
}

// DiscoverScriptPaths returns all script paths based on configuration
func (sd *ScriptDiscovery) DiscoverScriptPaths() []string {
	var paths []string
	seenPaths := make(map[string]bool)

	sd.debugf("starting script path discovery (autodiscovery=%v, standardPaths=%v, gitTraversal=%v, patterns=%v, maxDepth=%d)",
		sd.config.EnableAutodiscovery, !sd.config.DisableStandardPaths,
		sd.config.EnableGitTraversal, sd.config.ScriptPathPatterns, sd.config.MaxTraversalDepth)

	// Add legacy/standard paths for backward compatibility
	standardPaths := sd.getLegacyPaths()
	for _, path := range standardPaths {
		sd.debugf("adding standard path candidate: %s", path)
		sd.addPath(&paths, seenPaths, path)
	}

	// Add custom paths from configuration
	for _, path := range sd.config.CustomPaths {
		expandedPath := sd.expandPath(path)
		sd.debugf("adding custom path candidate: %s (expanded from %s)", expandedPath, path)
		sd.addPath(&paths, seenPaths, expandedPath)
	}

	// Add autodiscovered paths if enabled
	if sd.config.EnableAutodiscovery {
		autoPaths := sd.autodiscoverPaths()
		for _, path := range autoPaths {
			sd.debugf("adding autodiscovered path candidate: %s", path)
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

	// Pre-compute path scores to avoid O(n log n) filesystem operations
	type scoredPath struct {
		path  string
		score pathScore
	}
	scoredPaths := make([]scoredPath, len(paths))
	for i, path := range paths {
		scoredPaths[i] = scoredPath{
			path:  path,
			score: sd.computePathScore(path, cwd, configDir, execDir),
		}
	}

	// Sort using pre-computed scores
	sort.Slice(scoredPaths, func(i, j int) bool {
		pi := scoredPaths[i].score
		pj := scoredPaths[j].score

		if pi.class != pj.class {
			return pi.class < pj.class
		}

		if pi.distance != pj.distance {
			return pi.distance < pj.distance
		}

		if pi.depth != pj.depth {
			return pi.depth < pj.depth
		}

		return scoredPaths[i].path < scoredPaths[j].path
	})

	// Extract sorted paths
	for i, sp := range scoredPaths {
		paths[i] = sp.path
	}

	sd.debugf("discovery complete: %d paths found", len(paths))
	for i, p := range paths {
		sd.debugf("  [%d] %s", i, p)
	}

	return paths
}

// getLegacyPaths returns the original hardcoded paths for backward compatibility
func (sd *ScriptDiscovery) getLegacyPaths() []string {
	var paths []string

	// Allow disabling standard script paths via config (useful for tests)
	if sd.config.DisableStandardPaths {
		sd.debugf("standard paths disabled by configuration")
		return paths
	}

	// 1. scripts/ directory relative to the executable
	if execPath, err := os.Executable(); err == nil {
		execDir := filepath.Dir(execPath)
		p := filepath.Join(execDir, "scripts")
		paths = append(paths, p)
		sd.debugf("standard path [exec]: %s", p)
	} else {
		sd.debugf("standard path [exec]: skipped (executable path unavailable: %v)", err)
	}

	// 2. ~/.one-shot-man/scripts/ (user scripts)
	if configPath, err := config.GetConfigPath(); err == nil {
		configDir := filepath.Dir(configPath)
		p := filepath.Join(configDir, "scripts")
		paths = append(paths, p)
		sd.debugf("standard path [config]: %s", p)
	} else {
		sd.debugf("standard path [config]: skipped (config path unavailable: %v)", err)
	}

	// 3. ./scripts/ (current directory scripts)
	if cwd, err := os.Getwd(); err == nil {
		p := filepath.Join(cwd, "scripts")
		paths = append(paths, p)
		sd.debugf("standard path [cwd]: %s", p)
	} else {
		sd.debugf("standard path [cwd]: skipped (getwd failed: %v)", err)
	}

	return paths
}

// autodiscoverPaths discovers script paths using advanced rules
func (sd *ScriptDiscovery) autodiscoverPaths() []string {
	var paths []string

	// Start from current working directory
	cwd, err := os.Getwd()
	if err != nil {
		sd.debugf("autodiscover: skipped (getwd failed: %v)", err)
		return paths
	}

	sd.debugf("autodiscover: starting from %s", cwd)

	// Traverse up directory tree looking for git repositories and script directories
	if sd.config.EnableGitTraversal {
		paths = append(paths, sd.traverseForGitRepos(cwd)...)
	}

	// Look for script directories in current path and parent directories
	paths = append(paths, sd.traverseForScriptDirs(cwd)...)

	return paths
}

// traverseForGitRepos traverses up from the given directory looking for git repositories.
// It tracks resolved real paths to detect symlink cycles that could cause infinite traversal.
func (sd *ScriptDiscovery) traverseForGitRepos(startDir string) []string {
	var paths []string
	var gitRepos []string

	// Track resolved real paths to detect symlink cycles in the upward traversal.
	visitedReal := make(map[string]bool)

	dir := startDir
	for i := 0; i < sd.config.MaxTraversalDepth; i++ {
		// Resolve the real path for cycle detection
		realDir, err := filepath.EvalSymlinks(dir)
		if err != nil {
			if os.IsPermission(err) {
				log.Printf("warning: permission denied resolving symlinks for %q, stopping git traversal", dir)
				sd.debugf("git-traversal: permission denied at %s: %v", dir, err)
			} else {
				sd.debugf("git-traversal: symlink resolution failed at %s: %v", dir, err)
			}
			break
		}

		if visitedReal[realDir] {
			sd.debugf("git-traversal: symlink cycle detected at %s (real: %s), stopping", dir, realDir)
			break
		}
		visitedReal[realDir] = true

		// Check if this directory is a git repository
		if sd.isGitRepository(dir) {
			sd.debugf("git-traversal: found git repository at %s", dir)
			gitRepos = append(gitRepos, dir)
		}

		// Move up one directory
		parent := filepath.Dir(dir)
		if parent == dir {
			sd.debugf("git-traversal: reached filesystem root at %s", dir)
			break // Reached filesystem root
		}
		dir = parent
	}

	// Add script paths from git repositories (innermost first - highest priority)
	for _, repo := range gitRepos {
		for _, pattern := range sd.config.ScriptPathPatterns {
			scriptPath := filepath.Join(repo, pattern)
			exists, checkErr := sd.checkDirectory(scriptPath)
			if checkErr != nil {
				if os.IsPermission(checkErr) {
					log.Printf("warning: permission denied checking script directory %q", scriptPath)
					sd.debugf("git-traversal: permission denied for %s", scriptPath)
				} else {
					sd.debugf("git-traversal: error checking %s: %v", scriptPath, checkErr)
				}
				continue
			}
			if exists {
				sd.debugf("git-traversal: found script directory %s", scriptPath)
				paths = append(paths, scriptPath)
			}
		}
	}

	return paths
}

// traverseForScriptDirs traverses up from the given directory looking for script directories.
// It tracks resolved real paths to detect symlink cycles that could cause infinite traversal.
func (sd *ScriptDiscovery) traverseForScriptDirs(startDir string) []string {
	var paths []string

	// Track resolved real paths to detect symlink cycles in the upward traversal.
	visitedReal := make(map[string]bool)

	dir := startDir
	for i := 0; i < sd.config.MaxTraversalDepth; i++ {
		// Resolve the real path for cycle detection
		realDir, err := filepath.EvalSymlinks(dir)
		if err != nil {
			if os.IsPermission(err) {
				log.Printf("warning: permission denied resolving symlinks for %q, stopping upward traversal", dir)
				sd.debugf("traversal: permission denied at %s: %v", dir, err)
			} else {
				sd.debugf("traversal: symlink resolution failed at %s: %v", dir, err)
			}
			break
		}

		if visitedReal[realDir] {
			sd.debugf("traversal: symlink cycle detected at %s (real: %s), stopping", dir, realDir)
			break
		}
		visitedReal[realDir] = true

		// Check for script directories using configured patterns
		for _, pattern := range sd.config.ScriptPathPatterns {
			scriptPath := filepath.Join(dir, pattern)
			exists, checkErr := sd.checkDirectory(scriptPath)
			if checkErr != nil {
				if os.IsPermission(checkErr) {
					log.Printf("warning: permission denied checking script directory %q", scriptPath)
					sd.debugf("traversal: permission denied for %s", scriptPath)
				} else {
					sd.debugf("traversal: error checking %s: %v", scriptPath, checkErr)
				}
				continue
			}
			if exists {
				sd.debugf("traversal: found script directory %s", scriptPath)
				paths = append(paths, scriptPath)
			}
		}

		// Move up one directory
		parent := filepath.Dir(dir)
		if parent == dir {
			sd.debugf("traversal: reached filesystem root at %s", dir)
			break // Reached filesystem root
		}
		dir = parent
	}

	if len(paths) == 0 {
		sd.debugf("traversal: no script directories found in %d levels from %s", sd.config.MaxTraversalDepth, startDir)
	}

	return paths
}

// isGitRepository checks if a directory is a git repository
func (sd *ScriptDiscovery) isGitRepository(dir string) bool {
	cmd := exec.Command("git", "rev-parse", "--git-dir")
	cmd.Dir = dir
	return cmd.Run() == nil
}

// checkDirectory checks if a path is an existing directory.
// Returns (exists, error) to allow callers to distinguish permission errors
// from simple non-existence.
func (sd *ScriptDiscovery) checkDirectory(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		// Return the error (permission denied, I/O error, etc.) for the caller to handle
		return false, err
	}
	return info.IsDir(), nil
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
	absPath, err := filepath.Abs(cleaned)
	if err != nil {
		return "", err
	}

	// Resolve symlinks to ensure semantic deduplication
	// (a directory and a symlink to it should be treated as the same path)
	resolvedPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		// If symlink resolution fails (e.g., broken symlink, non-existent path),
		// fall back to absolute path
		return absPath, nil
	}

	// SECURITY: Validate that the resolved path is within the canonical parent directory.
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

func (sd *ScriptDiscovery) addPath(paths *[]string, seenPaths map[string]bool, candidate string) {
	if strings.TrimSpace(candidate) == "" {
		sd.debugf("addPath: skipping empty candidate")
		return
	}

	normalized, err := sd.normalizePath(candidate)
	if err != nil {
		log.Printf("warning: skipping script path %q: %v", candidate, err)
		sd.debugf("addPath: normalization failed for %q: %v", candidate, err)
		return
	}

	if seenPaths[normalized] {
		sd.debugf("addPath: deduplicating %s (normalized: %s)", candidate, normalized)
		return
	}

	*paths = append(*paths, normalized)
	seenPaths[normalized] = true
	sd.debugf("addPath: accepted %s (normalized: %s)", candidate, normalized)
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
