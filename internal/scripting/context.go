package scripting

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"golang.org/x/tools/txtar"
)

// ContextManager handles tracking and managing file paths and content as context
// for building LLM prompts.
type ContextManager struct {
	paths    map[string]*ContextPath
	basePath string
	mutex    sync.RWMutex
}

// ContextPath represents a tracked file or directory with metadata.
type ContextPath struct {
	Path        string            `json:"path"`
	Type        string            `json:"type"` // "file" or "directory"
	Content     string            `json:"content,omitempty"`
	Metadata    map[string]string `json:"metadata"`
	Children    []string          `json:"children,omitempty"` // for directories
	LastUpdated int64             `json:"lastUpdated"`
}

// NewContextManager creates a new context manager.
func NewContextManager(basePath string) *ContextManager {
	return &ContextManager{
		paths:    make(map[string]*ContextPath),
		basePath: basePath,
	}
}

// AddPath adds a file or directory to the context.
func (cm *ContextManager) AddPath(path string) error {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	// Normalize path to be relative to base path if possible
	relPath, err := filepath.Rel(cm.basePath, absPath)
	if err != nil || strings.HasPrefix(relPath, "..") {
		// If we can't get a relative path, or it would escape the base path,
		// use the absolute path to preserve uniqueness and enable exact lookups
		relPath = absPath
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return fmt.Errorf("failed to stat path %s: %w", path, err)
	}

	contextPath := &ContextPath{
		Path:        relPath,
		Metadata:    make(map[string]string),
		LastUpdated: info.ModTime().Unix(),
	}

	if info.IsDir() {
		contextPath.Type = "directory"
		children, err := cm.scanDirectory(absPath)
		if err != nil {
			return fmt.Errorf("failed to scan directory %s: %w", path, err)
		}
		contextPath.Children = children
	} else {
		contextPath.Type = "file"
		content, err := os.ReadFile(absPath)
		if err != nil {
			return fmt.Errorf("failed to read file %s: %w", path, err)
		}
		contextPath.Content = string(content)
		contextPath.Metadata["size"] = fmt.Sprintf("%d", len(content))
		contextPath.Metadata["extension"] = filepath.Ext(relPath)
	}

	cm.paths[relPath] = contextPath
	return nil
}

// RemovePath removes a path from the context.
func (cm *ContextManager) RemovePath(path string) error {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	// Fast path: exact key match
	if _, ok := cm.paths[path]; ok {
		delete(cm.paths, path)
		return nil
	}

	// Try to normalize provided path to the relative key used by the manager
	// Accept either absolute or relative inputs
	var rel string
	if path != "" {
		abs := path
		if !filepath.IsAbs(abs) {
			// Resolve relative to basePath
			abs = filepath.Join(cm.basePath, path)
		}
		if a2, err := filepath.Abs(abs); err == nil {
			if r, err2 := filepath.Rel(cm.basePath, a2); err2 == nil {
				// If outside basePath, prefer the absolute path key we store
				if strings.HasPrefix(r, "..") {
					rel = a2
				} else {
					rel = r
				}
			}
		}
	}

	if rel != "" {
		if _, ok := cm.paths[rel]; ok {
			delete(cm.paths, rel)
			return nil
		}
	}

	// If still not found, attempt a last-resort match by suffix (handles cases
	// where basePath moved or user supplied slightly different relative form).
	// IMPORTANT: Detect ambiguity and return an error if multiple matches exist.
	// This ensures deterministic behavior and surfaces issues to the caller.
	if path != "" || rel != "" {
		needleA := filepath.ToSlash(path)
		needleB := filepath.ToSlash(rel)
		var candidates []string
		for k := range cm.paths {
			ks := filepath.ToSlash(k)
			// Match exact strings (either provided path or normalized rel),
			// or suffix in EITHER direction to account for absolute vs relative keys
			if strings.EqualFold(k, path) ||
				strings.EqualFold(k, rel) ||
				(needleA != "" && ks != "" && (strings.HasSuffix(needleA, ks) || strings.HasSuffix(ks, needleA))) ||
				(needleB != "" && ks != "" && (strings.HasSuffix(needleB, ks) || strings.HasSuffix(ks, needleB))) {
				candidates = append(candidates, k)
			}
		}
		if len(candidates) == 1 {
			delete(cm.paths, candidates[0])
			return nil
		}
		if len(candidates) > 1 {
			// Sort for stable, user-friendly output
			slices.Sort(candidates)
			return fmt.Errorf("ambiguous path: %q matches %d tracked paths: %s", path, len(candidates), strings.Join(candidates, ", "))
		}
	}

	// Not found is a no-op
	return fmt.Errorf("path not found: %s", path)
}

// GetPath returns information about a tracked path.
func (cm *ContextManager) GetPath(path string) (*ContextPath, bool) {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()

	contextPath, exists := cm.paths[path]
	return contextPath, exists
}

// ListPaths returns all tracked paths.
func (cm *ContextManager) ListPaths() []string {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()

	paths := make([]string, 0, len(cm.paths))
	for path := range cm.paths {
		paths = append(paths, path)
	}
	return paths
}

// ToTxtar converts the context to txtar format.
func (cm *ContextManager) ToTxtar() *txtar.Archive {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()

	archive := &txtar.Archive{
		Comment: []byte("Context archive generated by one-shot-man"),
	}

	// Build a list of file paths and group by basename to detect collisions.
	type entry struct {
		key     string
		path    string
		content string
	}
	var files []entry
	baseGroups := make(map[string][]entry)
	for k, cp := range cm.paths {
		if cp.Type != "file" {
			continue
		}
		e := entry{key: k, path: cp.Path, content: cp.Content}
		files = append(files, e)
		base := filepath.Base(cp.Path)
		baseGroups[base] = append(baseGroups[base], e)
	}

	// Helper: compute minimal unique suffixes for a set of paths that share the same basename.
	// Returns map[key]exportName, where exportName uses OS separators; we'll normalize to '/'.
	computeUniqueSuffixes := func(group []entry) map[string]string {
		out := make(map[string]string, len(group))
		if len(group) == 1 {
			out[group[0].key] = filepath.Base(group[0].path)
			return out
		}
		// Pre-split into path components (clean first).
		type comps struct {
			key   string
			parts []string
		}
		arr := make([]comps, 0, len(group))
		maxDepth := 0
		sep := string(filepath.Separator)
		for _, e := range group {
			clean := filepath.Clean(e.path)
			// Split by OS separator into components
			// Guard against volume names or leading separators by using strings.Split after trimming trailing sep
			// filepath.Clean guarantees no trailing separator (except root), which is fine.
			parts := strings.Split(clean, sep)
			// Handle cases like Windows volume "C:" which may appear as part of first component; keep as-is.
			arr = append(arr, comps{key: e.key, parts: parts})
			if n := len(parts); n > maxDepth {
				maxDepth = n
			}
		}
		// Increase depth from 1 (basename) until all suffixes are unique or we exhaust.
		depth := 1
		for depth <= maxDepth {
			counts := make(map[string]int, len(arr))
			suffixes := make(map[string]string, len(arr))
			for _, c := range arr {
				n := len(c.parts)
				if depth > n {
					// If depth exceeds, use full path
					depth = n
				}
				start := n - depth
				if start < 0 {
					start = 0
				}
				suf := strings.Join(c.parts[start:], sep)
				suffixes[c.key] = suf
				counts[suf]++
			}
			// Check uniqueness
			unique := true
			for _, cnt := range counts {
				if cnt > 1 {
					unique = false
					break
				}
			}
			if unique {
				for k, suf := range suffixes {
					out[k] = suf
				}
				return out
			}
			depth++
		}
		// Fallback: use full cleaned paths
		for _, c := range arr {
			out[c.key] = strings.Join(c.parts, sep)
		}
		return out
	}

	// Determine export names per entry key.
	exportNames := make(map[string]string, len(files))
	for _, group := range baseGroups {
		names := computeUniqueSuffixes(group)
		for k, v := range names {
			// Normalize separators to '/' for portability and stable txtar display
			exportNames[k] = filepath.ToSlash(v)
		}
	}

	// Emit files in a stable order (sorted by export name)
	type outFile struct {
		name string
		data []byte
	}
	var outs []outFile
	for _, e := range files {
		if name, ok := exportNames[e.key]; ok {
			outs = append(outs, outFile{name: name, data: []byte(e.content)})
		}
	}
	slices.SortFunc(outs, func(a, b outFile) int {
		if a.name < b.name {
			return -1
		} else if a.name > b.name {
			return 1
		} else {
			return 0
		}
	})
	for _, of := range outs {
		archive.Files = append(archive.Files, txtar.File{Name: of.name, Data: of.data})
	}

	return archive
}

// FromTxtar loads context from a txtar archive.
func (cm *ContextManager) FromTxtar(archive *txtar.Archive) error {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	// Clear existing context
	cm.paths = make(map[string]*ContextPath)

	for _, file := range archive.Files {
		contextPath := &ContextPath{
			Path:     file.Name,
			Type:     "file",
			Content:  string(file.Data),
			Metadata: make(map[string]string),
		}
		contextPath.Metadata["size"] = fmt.Sprintf("%d", len(file.Data))
		contextPath.Metadata["extension"] = filepath.Ext(file.Name)

		cm.paths[file.Name] = contextPath
	}

	return nil
}

// GetTxtarString returns the context as a txtar-formatted string.
func (cm *ContextManager) GetTxtarString() string {
	archive := cm.ToTxtar()
	return string(txtar.Format(archive))
}

// LoadFromTxtarString loads context from a txtar-formatted string.
func (cm *ContextManager) LoadFromTxtarString(data string) error {
	archive := txtar.Parse([]byte(data))
	return cm.FromTxtar(archive)
}

// RefreshPath updates the content of a tracked path.
func (cm *ContextManager) RefreshPath(path string) error {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	contextPath, exists := cm.paths[path]
	if !exists {
		return fmt.Errorf("path %s is not tracked", path)
	}

	absPath := filepath.Join(cm.basePath, path)
	info, err := os.Stat(absPath)
	if err != nil {
		return fmt.Errorf("failed to stat path %s: %w", path, err)
	}

	contextPath.LastUpdated = info.ModTime().Unix()

	if contextPath.Type == "file" {
		content, err := os.ReadFile(absPath)
		if err != nil {
			return fmt.Errorf("failed to read file %s: %w", path, err)
		}
		contextPath.Content = string(content)
		contextPath.Metadata["size"] = fmt.Sprintf("%d", len(content))
	} else if contextPath.Type == "directory" {
		children, err := cm.scanDirectory(absPath)
		if err != nil {
			return fmt.Errorf("failed to scan directory %s: %w", path, err)
		}
		contextPath.Children = children
	}

	return nil
}

// scanDirectory scans a directory and returns relative paths of its contents.
func (cm *ContextManager) scanDirectory(dirPath string) ([]string, error) {
	var children []string

	err := filepath.WalkDir(dirPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if path == dirPath {
			return nil // Skip the directory itself
		}

		relPath, err := filepath.Rel(cm.basePath, path)
		if err != nil {
			relPath = path
		}

		children = append(children, relPath)
		return nil
	})

	return children, err
}

// GetStats returns statistics about the context.
func (cm *ContextManager) GetStats() map[string]interface{} {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()

	stats := map[string]interface{}{
		"totalPaths":  len(cm.paths),
		"files":       0,
		"directories": 0,
		"totalSize":   0,
	}

	for _, contextPath := range cm.paths {
		if contextPath.Type == "file" {
			stats["files"] = stats["files"].(int) + 1
			if sizeStr, ok := contextPath.Metadata["size"]; ok {
				var size int
				fmt.Sscanf(sizeStr, "%d", &size)
				stats["totalSize"] = stats["totalSize"].(int) + size
			}
		} else {
			stats["directories"] = stats["directories"].(int) + 1
		}
	}

	return stats
}

// FilterPaths returns paths matching the given pattern.
func (cm *ContextManager) FilterPaths(pattern string) ([]string, error) {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()

	var matches []string
	for path := range cm.paths {
		matched, err := filepath.Match(pattern, path)
		if err != nil {
			return nil, fmt.Errorf("invalid pattern %s: %w", pattern, err)
		}
		if matched {
			matches = append(matches, path)
		}
	}

	return matches, nil
}

// GetFilesByExtension returns all files with the given extension.
func (cm *ContextManager) GetFilesByExtension(ext string) []string {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()

	var files []string
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}

	for path, contextPath := range cm.paths {
		if contextPath.Type == "file" && strings.HasSuffix(path, ext) {
			files = append(files, path)
		}
	}

	return files
}
