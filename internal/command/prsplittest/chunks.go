package prsplittest

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
)

// chunkCache caches the discovered chunk names and sources. Populated once
// per test binary via sync.Once.
var (
	chunkOnce    sync.Once
	chunkSources map[string]string // chunk name → JS source
	chunkNames   []string          // sorted chunk names
	chunkErr     error
)

// commandDir returns the absolute path to internal/command/ by navigating
// from this source file's location (internal/command/prsplittest/).
func commandDir() string {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		panic("prsplittest: runtime.Caller failed")
	}
	// filename is .../internal/command/prsplittest/chunks.go
	// parent is .../internal/command/prsplittest/
	// grandparent is .../internal/command/
	return filepath.Dir(filepath.Dir(filename))
}

// discoverChunks scans the internal/command/ directory for pr_split_*.js files,
// extracts chunk names (stripping "pr_split_" prefix and ".js" suffix), and
// reads their contents. Results are cached for the lifetime of the test binary.
//
// The chunk names are sorted lexicographically, which produces the correct
// load order because chunk files use zero-padded numeric prefixes:
// 00_core, 01_analysis, ..., 10a_pipeline_config, ..., 16f_tui_model.
func discoverChunks() (map[string]string, []string, error) {
	chunkOnce.Do(func() {
		dir := commandDir()
		pattern := filepath.Join(dir, "pr_split_*.js")
		matches, err := filepath.Glob(pattern)
		if err != nil {
			chunkErr = err
			return
		}
		if len(matches) == 0 {
			chunkErr = &os.PathError{Op: "glob", Path: pattern, Err: os.ErrNotExist}
			return
		}

		chunkSources = make(map[string]string, len(matches))
		for _, path := range matches {
			base := filepath.Base(path)
			// Extract chunk name: pr_split_00_core.js → 00_core
			name := strings.TrimPrefix(base, "pr_split_")
			name = strings.TrimSuffix(name, ".js")

			content, err := os.ReadFile(path)
			if err != nil {
				chunkErr = err
				return
			}
			chunkSources[name] = string(content)
		}

		// Build sorted name list.
		chunkNames = make([]string, 0, len(chunkSources))
		for name := range chunkSources {
			chunkNames = append(chunkNames, name)
		}
		sort.Strings(chunkNames)
	})
	return chunkSources, chunkNames, chunkErr
}

// AllChunkNames returns all discovered chunk names in lexicographic order.
// This is the prsplittest equivalent of iterating the prSplitChunks array.
func AllChunkNames() []string {
	_, names, err := discoverChunks()
	if err != nil {
		panic("prsplittest: chunk discovery failed: " + err.Error())
	}
	cp := make([]string, len(names))
	copy(cp, names)
	return cp
}

// ChunkNamesThrough returns chunk names from the beginning through (and
// including) the last chunk whose name starts with the given prefix. This
// replaces the allChunksThrough12, allChunksThrough11 patterns.
//
// Example: ChunkNamesThrough("12") returns names from "00_core" through
// "12_exports" (inclusive). ChunkNamesThrough("10") includes all of
// 10a_pipeline_config, 10b_pipeline_send, 10c_pipeline_resolve,
// 10d_pipeline_orchestrator.
func ChunkNamesThrough(prefix string) []string {
	_, names, err := discoverChunks()
	if err != nil {
		panic("prsplittest: chunk discovery failed: " + err.Error())
	}
	// Find the last index matching the prefix.
	lastIdx := -1
	for i, name := range names {
		if strings.HasPrefix(name, prefix) {
			lastIdx = i
		}
	}
	if lastIdx < 0 {
		return nil
	}
	result := make([]string, lastIdx+1)
	copy(result, names[:lastIdx+1])
	return result
}

// ChunkNamesAfter returns chunk names that come after the last chunk matching
// the given prefix. Used to get TUI chunks (13+) after loading core chunks.
//
// Example: ChunkNamesAfter("12") returns names from "13_tui" onwards.
func ChunkNamesAfter(prefix string) []string {
	_, names, err := discoverChunks()
	if err != nil {
		panic("prsplittest: chunk discovery failed: " + err.Error())
	}
	// Find the last index matching the prefix.
	lastIdx := -1
	for i, name := range names {
		if strings.HasPrefix(name, prefix) {
			lastIdx = i
		}
	}
	if lastIdx < 0 || lastIdx+1 >= len(names) {
		return nil
	}
	result := make([]string, len(names)-lastIdx-1)
	copy(result, names[lastIdx+1:])
	return result
}

// ChunkSource returns the JS source code for a specific chunk name.
// Panics if chunk discovery fails or the chunk name is unknown.
func ChunkSource(name string) string {
	sources, _, err := discoverChunks()
	if err != nil {
		panic("prsplittest: chunk discovery failed: " + err.Error())
	}
	src, ok := sources[name]
	if !ok {
		panic("prsplittest: unknown chunk name: " + name)
	}
	return src
}
