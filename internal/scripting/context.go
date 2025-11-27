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
	paths      map[string]*ContextPath
	basePath   string
	mutex      sync.RWMutex
	ownerFiles map[string]map[string]struct{}
	fileOwners map[string]int
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
func NewContextManager(basePath string) (*ContextManager, error) {
	absBase, err := filepath.Abs(basePath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve absolute base path for %q: %w", basePath, err)
	}

	return &ContextManager{
		paths:      make(map[string]*ContextPath),
		basePath:   absBase,
		ownerFiles: make(map[string]map[string]struct{}),
		fileOwners: make(map[string]int),
	}, nil
}

func (cm *ContextManager) normalizeOwnerPath(absPath string) string {
	relPath, err := filepath.Rel(cm.basePath, absPath)
	if err != nil {
		return absPath
	}

	relPath = filepath.Clean(relPath)
	if relPath == "." {
		return "."
	}

	if strings.HasPrefix(relPath, ".."+string(filepath.Separator)) || relPath == ".." {
		return absPath
	}

	return relPath
}

func (cm *ContextManager) absolutePathFromOwner(owner string) (string, error) {
	if owner == "." {
		return cm.basePath, nil
	}
	if filepath.IsAbs(owner) {
		return owner, nil
	}
	return filepath.Abs(filepath.Join(cm.basePath, owner))
}

// AddPath adds a file or directory to the context.
func (cm *ContextManager) AddPath(path string) error {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	owner := cm.normalizeOwnerPath(absPath)

	info, err := os.Lstat(absPath)
	if err != nil {
		return fmt.Errorf("failed to stat path %s: %w", path, err)
	}

	return cm.addPathWithOwnerLocked(absPath, owner, info)
}

func (cm *ContextManager) addPathWithOwnerLocked(absPath, owner string, info fs.FileInfo) error {
	if info.Mode()&os.ModeSymlink != 0 {
		targetInfo, err := os.Stat(absPath)
		if err != nil {
			return fmt.Errorf("failed to resolve symlink %s: %w", absPath, err)
		}
		info = targetInfo
	}

	cm.removeOwnerLocked(owner)

	if info.IsDir() {
		return cm.addDirectoryLocked(absPath, owner, info)
	}

	if info.Mode().IsRegular() {
		return cm.addFileLocked(absPath, owner, owner, info)
	}

	return fmt.Errorf("unsupported path type: %s", absPath)
}

func (cm *ContextManager) addFileLocked(absPath, logicalPath, owner string, info fs.FileInfo) error {
	data, err := os.ReadFile(absPath)
	if err != nil {
		return fmt.Errorf("failed to read file %s: %w", absPath, err)
	}

	cp, exists := cm.paths[logicalPath]
	if !exists || cp.Type != "file" {
		cp = &ContextPath{
			Path:     logicalPath,
			Type:     "file",
			Metadata: make(map[string]string),
		}
	} else if cp.Metadata == nil {
		cp.Metadata = make(map[string]string)
	}

	cp.Content = string(data)
	cp.Metadata["size"] = fmt.Sprintf("%d", len(data))
	cp.Metadata["extension"] = filepath.Ext(logicalPath)
	cp.LastUpdated = info.ModTime().Unix()

	cm.paths[logicalPath] = cp

	ownerSet, ok := cm.ownerFiles[owner]
	if !ok {
		ownerSet = make(map[string]struct{})
		cm.ownerFiles[owner] = ownerSet
	}

	if _, present := ownerSet[logicalPath]; !present {
		ownerSet[logicalPath] = struct{}{}
		cm.fileOwners[logicalPath]++
	}

	return nil
}

func (cm *ContextManager) addDirectoryLocked(absPath, owner string, info fs.FileInfo) error {
	ownerSet := make(map[string]struct{})
	cm.ownerFiles[owner] = ownerSet

	visited := make(map[string]struct{})
	var children []string
	if err := cm.walkDirectory(absPath, owner, owner, ownerSet, &children, visited); err != nil {
		delete(cm.ownerFiles, owner)
		return fmt.Errorf("failed to scan directory %s: %w", absPath, err)
	}

	cm.paths[owner] = &ContextPath{
		Path:        owner,
		Type:        "directory",
		Metadata:    make(map[string]string),
		Children:    children,
		LastUpdated: info.ModTime().Unix(),
	}

	return nil
}

func (cm *ContextManager) walkDirectory(absRoot, logicalRoot, owner string, ownerSet map[string]struct{}, children *[]string, visited map[string]struct{}) error {
	canonical, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		canonical = absRoot
	}

	if _, ok := visited[canonical]; ok {
		return nil
	}
	visited[canonical] = struct{}{}

	entries, err := os.ReadDir(absRoot)
	if err != nil {
		return fmt.Errorf("failed to read directory %s: %w", absRoot, err)
	}

	for _, entry := range entries {
		absChild := filepath.Join(absRoot, entry.Name())
		logicalChild := filepath.Join(logicalRoot, entry.Name())

		info, err := os.Lstat(absChild)
		if err != nil {
			return fmt.Errorf("failed to stat path %s: %w", absChild, err)
		}

		if info.Mode()&os.ModeSymlink != 0 {
			targetInfo, err := os.Stat(absChild)
			if err != nil {
				return fmt.Errorf("failed to resolve symlink %s: %w", absChild, err)
			}
			if targetInfo.IsDir() {
				if err := cm.walkDirectory(absChild, logicalChild, owner, ownerSet, children, visited); err != nil {
					return err
				}
				continue
			}
			_, seen := ownerSet[logicalChild]
			if err := cm.addFileLocked(absChild, logicalChild, owner, targetInfo); err != nil {
				return err
			}
			if !seen {
				*children = append(*children, logicalChild)
			}
			continue
		}

		if info.IsDir() {
			if err := cm.walkDirectory(absChild, logicalChild, owner, ownerSet, children, visited); err != nil {
				return err
			}
			continue
		}

		if info.Mode().IsRegular() {
			_, seen := ownerSet[logicalChild]
			if err := cm.addFileLocked(absChild, logicalChild, owner, info); err != nil {
				return err
			}
			if !seen {
				*children = append(*children, logicalChild)
			}
			continue
		}
	}

	return nil
}

func (cm *ContextManager) removeOwnerLocked(owner string) bool {
	removed := false

	if files, ok := cm.ownerFiles[owner]; ok {
		for file := range files {
			if count := cm.fileOwners[file]; count <= 1 {
				delete(cm.fileOwners, file)
				delete(cm.paths, file)
			} else {
				cm.fileOwners[file] = count - 1
			}
		}
		delete(cm.ownerFiles, owner)
		removed = true
	}

	if cp, ok := cm.paths[owner]; ok && cp.Type == "directory" {
		delete(cm.paths, owner)
		removed = true
	}

	return removed
}

// RemovePath removes a path from the context.
func (cm *ContextManager) RemovePath(path string) error {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	if cm.removeOwnerLocked(path) {
		return nil
	}

	var rel string
	if path != "" {
		abs := path
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(cm.basePath, path)
		}
		if a2, err := filepath.Abs(abs); err == nil {
			rel = cm.normalizeOwnerPath(a2)
			if rel != "" && rel != path {
				if cm.removeOwnerLocked(rel) {
					return nil
				}
			}
			if a2 != path {
				if cm.removeOwnerLocked(a2) {
					return nil
				}
			}
		}
	}

	// If the caller supplied a basename-only value (no separators) attempt
	// to match tracked paths by basename. If multiple matches exist treat
	// this as ambiguous; if a single unique match exists perform the
	// appropriate removal logic for that tracked entry.
	base := filepath.Base(path)
	// Only treat suffix matching when the input appears to be a bare basename
	// (e.g., "foo.txt") and not a path containing separators.
	if path != "" && path == base {
		var matchKey string
		matches := 0
		for k := range cm.paths {
			if filepath.Base(k) == base {
				matches++
				if matches > 1 {
					return fmt.Errorf("ambiguous path: %s", path)
				}
				matchKey = k
			}
		}

		if matches == 1 {
			// First, if the matching key is itself an owner entry attempt
			// to remove it via the existing owner-removal logic.
			if cm.removeOwnerLocked(matchKey) {
				return nil
			}

			// Otherwise perform a targeted removal of the tracked path. This
			// removes the path from the primary paths map and cleans up any
			// owner bookkeeping that references it.
			if cp, ok := cm.paths[matchKey]; ok {
				if cp.Type == "directory" {
					// If it is a directory, removing the owner is the correct
					// semantics (shouldn't generally reach here as removeOwner
					// would have handled it above), but handle defensively.
					cm.removeOwnerLocked(matchKey)
					return nil
				}

				// For files: remove from paths, from any owner sets, and
				// update fileOwners counts and directory children lists.
				delete(cm.paths, matchKey)

				// Clean up ownerFiles and update directory children where
				// applicable.
				for owner, set := range cm.ownerFiles {
					if _, present := set[matchKey]; present {
						delete(set, matchKey)
						if len(set) == 0 {
							delete(cm.ownerFiles, owner)
						}

						// If the owner is a directory entry, try to remove the
						// child from its recorded Children slice.
						if ownerCP, ok := cm.paths[owner]; ok && ownerCP.Type == "directory" {
							var newChildren []string
							for _, child := range ownerCP.Children {
								if child != matchKey {
									newChildren = append(newChildren, child)
								}
							}
							ownerCP.Children = newChildren
						}
					}
				}

				// Remove any fileOwners bookkeeping for the removed path.
				delete(cm.fileOwners, matchKey)

				return nil
			}
		}
	}

	// If path is not found, we consider it successfully removed (idempotent).
	return nil
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

	archive := &txtar.Archive{}

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
		// Determine absolute path to read from disk.
		absPath := cp.Path
		if !filepath.IsAbs(absPath) {
			absPath = filepath.Join(cm.basePath, cp.Path)
		}
		// Read the latest content from disk; silently skip on error (e.g., file removed).
		data, err := os.ReadFile(absPath)
		if err != nil {
			continue
		}
		e := entry{key: k, path: cp.Path, content: string(data)}
		files = append(files, e)
		base := filepath.Base(cp.Path)
		baseGroups[base] = append(baseGroups[base], e)
	}

	// Helper: compute minimal unique suffixes for a set of paths that share the same basename.
	// Returns map[key]exportName, where exportName uses OS separators; we'll normalize to '/'.
	computeUniqueSuffixes := func(group []entry) map[string]string {
		out := make(map[string]string, len(group))
		if len(group) == 1 {
			// Use the full relative path to preserve meaningful directory structure
			// But for absolute paths outside the base, prefer just the basename if it's unique
			path := group[0].path
			if filepath.IsAbs(path) {
				// For absolute paths, prefer basename since the full absolute path
				// is often not meaningful in the txtar context
				out[group[0].key] = filepath.Base(path)
			} else {
				// For relative paths, preserve the full structure as it's meaningful
				out[group[0].key] = path
			}
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
				// Use an effective depth per path; never mutate the outer loop counter
				effectiveDepth := depth
				if effectiveDepth > n {
					effectiveDepth = n
				}
				start := n - effectiveDepth
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
	cm.ownerFiles = make(map[string]map[string]struct{})
	cm.fileOwners = make(map[string]int)

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
		cm.ownerFiles[file.Name] = map[string]struct{}{file.Name: {}}
		cm.fileOwners[file.Name] = 1
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

	if _, tracked := cm.ownerFiles[path]; !tracked {
		return fmt.Errorf("path %s is not a tracked owner", path)
	}

	absPath, err := cm.absolutePathFromOwner(path)
	if err != nil {
		return fmt.Errorf("failed to resolve path %s: %w", path, err)
	}

	info, err := os.Lstat(absPath)
	if err != nil {
		return fmt.Errorf("failed to stat path %s: %w", path, err)
	}

	return cm.addPathWithOwnerLocked(absPath, path, info)
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

// Clear removes all tracked paths from the context.
func (cm *ContextManager) Clear() {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	cm.paths = make(map[string]*ContextPath)
	cm.ownerFiles = make(map[string]map[string]struct{})
	cm.fileOwners = make(map[string]int)
}
