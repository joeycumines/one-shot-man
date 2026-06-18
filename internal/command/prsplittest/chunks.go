package prsplittest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sync"
)

type manifestChunk struct {
	ID      string   `json:"id"`
	File    string   `json:"file"`
	Exports []string `json:"exports"`
}

type manifest struct {
	Version string          `json:"version"`
	Chunks  []manifestChunk `json:"chunks"`
}

var (
	chunkOnce    sync.Once
	chunkSources map[string]string // chunk ID → JS source
	chunkNames   []string          // manifest-ordered chunk IDs
	chunkErr     error
)

// CommandDir returns the absolute path to internal/command/ by navigating
// from this source file's location (internal/command/prsplittest/).
func CommandDir() string {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		panic("prsplittest: runtime.Caller failed")
	}
	return filepath.Dir(filepath.Dir(filename))
}

// discoverChunks reads the manifest from internal/command/ and loads chunk
// sources from disk. Results are cached for the lifetime of the test binary.
func discoverChunks() (map[string]string, []string, error) {
	chunkOnce.Do(func() {
		dir := CommandDir()
		manifestPath := filepath.Join(dir, "pr_split_manifest.json")
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			chunkErr = err
			return
		}

		var m manifest
		if err := json.Unmarshal(data, &m); err != nil {
			chunkErr = err
			return
		}
		if len(m.Chunks) == 0 {
			chunkErr = &os.PathError{Op: "manifest", Path: manifestPath, Err: os.ErrNotExist}
			return
		}

		chunkSources = make(map[string]string, len(m.Chunks))
		chunkNames = make([]string, len(m.Chunks))

		for i, chunk := range m.Chunks {
			chunkNames[i] = chunk.ID
			path := filepath.Join(dir, chunk.File)
			content, err := os.ReadFile(path)
			if err != nil {
				chunkErr = err
				return
			}
			chunkSources[chunk.ID] = string(content)
		}
	})
	return chunkSources, chunkNames, chunkErr
}

// AllChunkNames returns all chunk IDs in manifest order.
func AllChunkNames() []string {
	_, names, err := discoverChunks()
	if err != nil {
		panic("prsplittest: chunk discovery failed: " + err.Error())
	}
	cp := make([]string, len(names))
	copy(cp, names)
	return cp
}

// ChunkNamesThrough returns chunk IDs from the beginning through (and
// including) the last chunk whose ID starts with the given prefix.
//
// Example: ChunkNamesThrough("12") returns IDs from "00_core" through
// "12_exports" (inclusive). ChunkNamesThrough("10") includes all of
// 10a_pipeline_config, 10b_pipeline_send, 10c_pipeline_resolve,
// 10d_pipeline_orchestrator.
func ChunkNamesThrough(prefix string) []string {
	_, names, err := discoverChunks()
	if err != nil {
		panic("prsplittest: chunk discovery failed: " + err.Error())
	}
	lastIdx := -1
	for i, name := range names {
		if len(name) >= len(prefix) && name[:len(prefix)] == prefix {
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

// ChunkNamesAfter returns chunk IDs that come after the last chunk matching
// the given prefix. Used to get TUI chunks (13+) after loading core chunks.
//
// Example: ChunkNamesAfter("12") returns IDs from "13_tui" onwards.
func ChunkNamesAfter(prefix string) []string {
	_, names, err := discoverChunks()
	if err != nil {
		panic("prsplittest: chunk discovery failed: " + err.Error())
	}
	lastIdx := -1
	for i, name := range names {
		if len(name) >= len(prefix) && name[:len(prefix)] == prefix {
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

// ChunkSource returns the JS source code for a specific chunk ID.
// Panics if chunk discovery fails or the chunk ID is unknown.
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
