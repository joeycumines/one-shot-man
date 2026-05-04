package scripting

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/joeycumines/one-shot-man/internal/filepathutil"
	"golang.org/x/tools/txtar"
)

// isTildeOwnerLabel reports whether the given path is a tilde form requiring expansion
// before path resolution. It intentionally accepts both forward-slash and backslash
// forms, bypassing filepathutil.IsTildeExpansionPath's host-awareness, to ensure that
// persisted session state originating from different operating systems (e.g., Windows)
// can be correctly rehydrated on any platform.
func isTildeOwnerLabel(path string) bool {
	return path == "~" ||
		strings.HasPrefix(path, "~/") ||
		(len(path) >= 2 && path[0] == '~' && path[1] == '\\')
}

// expandTildeOwnerLabel expands a tilde owner label to an absolute path.
// Backslash-tilde forms are normalized to forward slashes beforehand to guarantee
// that filepathutil.ExpandTilde successfully expands them across all host platforms.
func expandTildeOwnerLabel(path string) (string, error) {
	normalized := path
	if len(path) >= 2 && path[0] == '~' && path[1] == '\\' {
		// Normalize backslashes to ensure filepathutil.ExpandTilde handles
		// Windows-style tilde paths correctly on all platforms.
		normalized = "~/" + strings.ReplaceAll(path[2:], "\\", "/")
	}
	return filepathutil.ExpandTilde(normalized)
}

// canonicalizeUserPath converts a user-provided path to a canonical owner key.
// It performs tilde expansion, resolves to an absolute path, and normalizes
// relative to the ContextManager's base path to ensure consistent handling of
// user inputs across state mutation methods.
//
// It returns both the canonical owner key and the absolute path for efficiency,
// avoiding redundant path resolution in callers.
func (cm *ContextManager) canonicalizeUserPath(path string) (string, string, error) {
	// Reject empty strings to prevent resolving to the process CWD, which
	// could cause unintentional mutations to the root owner.
	if path == "" {
		return "", "", fmt.Errorf("empty path is not valid")
	}

	// Use host-specific filepathutil.ExpandTilde to ensure POSIX systems treat
	// "~\foo" as a literal path rather than "$HOME/foo". This maintains semantics
	// consistent with internal/builtin/os. Cross-platform expansion is reserved
	// strictly for rehydration workflows.
	expanded, err := filepathutil.ExpandTilde(path)
	if err != nil {
		return "", "", err
	}

	absPath, err := filepath.Abs(expanded)
	if err != nil {
		return "", "", fmt.Errorf("failed to get absolute path: %w", err)
	}

	owner := cm.normalizeOwnerPath(absPath)
	return owner, absPath, nil
}

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
	Path       string            `json:"path"`
	Type       string            `json:"type"` // "file" or "directory"
	Content    string            `json:"content,omitempty"`
	Metadata   map[string]string `json:"metadata"`
	Children   []string          `json:"children,omitempty"` // for directories
	UpdateTime time.Time         `json:"updateTime"`
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

// normalizeOwnerPath standardizes an absolute path relative to the ContextManager's
// base path, ensuring consistent lookup keys regardless of traversal anomalies.
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

// findOwnerFromUserPath attempts to resolve a user-provided path to a tracked
// owner key using a multi-step resolution strategy to ensure paths like "root/",
// "./root", or "~/project" consistently resolve to their tracked owner.
//
// The caller MUST hold at least a read lock on cm.mutex when calling this method.
//
// It returns (owner, true, nil) on success, ("", false, nil) on a cache miss,
// or an error if canonicalization fails (e.g., corrupted $HOME).
func (cm *ContextManager) findOwnerFromUserPath(path string) (string, bool, error) {
	// Prevent empty strings from matching a tracked owner, avoiding
	// accidental root owner removals.
	if path == "" {
		return "", false, nil
	}

	// Step 1: Try raw path as exact owner match
	if _, tracked := cm.ownerFiles[path]; tracked {
		return path, true, nil
	}

	// Step 2: basePath-relative normalization for relative, non-tilde paths.
	// We bypass this for literal tilde expansion forms to prevent conflict, but
	// literal paths starting with "~" (e.g., "~cache") must normalize relative to basePath.
	if !filepath.IsAbs(path) && !filepathutil.IsTildeExpansionPath(path) {
		absPath := filepath.Join(cm.basePath, path)
		if cleaned, err := filepath.Abs(absPath); err == nil {
			if relativeOwner := cm.normalizeOwnerPath(cleaned); relativeOwner != path {
				if _, tracked := cm.ownerFiles[relativeOwner]; tracked {
					return relativeOwner, true, nil
				}
			}
		}
	}

	// Step 2.5: Host-specific canonicalization.
	// AddPath user-input semantics take priority over cross-platform rehydration.
	// On POSIX, this treats "~\foo" as a literal path matching canonicalizeUserPath.
	hostNormalized, _, canonErr := cm.canonicalizeUserPath(path)
	if canonErr == nil {
		if _, tracked := cm.ownerFiles[hostNormalized]; tracked {
			return hostNormalized, true, nil
		}
	}

	// Step 2.7: Normalized forward-slash probe for POSIX backslash labels.
	// Maps original backslash labels from AddRelativePath (e.g., "~\docs\notes.txt")
	// back to their forward-slash stored owner keys to prevent Step 3 from misinterpreting
	// them as $HOME-relative. Restricted to tilde-prefixes to avoid false positives.
	if runtime.GOOS != "windows" && strings.HasPrefix(path, `~\`) {
		normalizedRel := filepath.Clean(strings.ReplaceAll(path, "\\", "/"))
		normalizedAbs := filepath.Join(cm.basePath, normalizedRel)
		normalizedOwner := cm.normalizeOwnerPath(normalizedAbs)
		if _, tracked := cm.ownerFiles[normalizedOwner]; tracked {
			return normalizedOwner, true, nil
		}
	}

	// Step 3: Cross-platform tilde fallback.
	// Uses expandTildeOwnerLabel so rehydration labels accurately resolve on all hosts.
	// Errors bubble up to distinguish system failures from benign cache misses.
	expanded, err := expandTildeOwnerLabel(path)
	if err != nil {
		return "", false, fmt.Errorf("findOwnerFromUserPath(%q): %w", path, err)
	}
	absPath, err := filepath.Abs(expanded)
	if err != nil {
		return "", false, fmt.Errorf("findOwnerFromUserPath(%q): %w", path, err)
	}
	normalized := cm.normalizeOwnerPath(absPath)
	if _, tracked := cm.ownerFiles[normalized]; tracked {
		return normalized, true, nil
	}

	return "", false, nil
}

// absolutePathFromOwner converts a tracked owner key back to a canonical absolute path.
func (cm *ContextManager) absolutePathFromOwner(owner string) (string, error) {
	if owner == "." {
		return cm.basePath, nil
	}
	if filepath.IsAbs(owner) {
		// Guarantee a canonical absolute path for absolute inputs.
		return filepath.Abs(owner)
	}
	return filepath.Abs(filepath.Join(cm.basePath, owner))
}

// AddPath adds a file or directory to the context.
//
// Historically AddPath resolved relative inputs against the process CWD
// (matching typical CLI/shell expectations). That behavior is preserved so
// callers which supply user/CLI paths continue to get the expected result.
// Additionally, ~ is expanded to the user's home directory.
//
// BREAKING CHANGE: Empty strings are now explicitly rejected and return an error.
// Previously, AddPath("") would resolve to the current working directory, but this
// behavior was unsafe and could cause unintended root context mutation.
func (cm *ContextManager) AddPath(path string) error {
	// Perform I/O-intensive path resolution (tilde expansion, Abs, Lstat) before
	// acquiring the lock to prevent syscalls from causing lock contention.
	owner, absPath, err := cm.canonicalizeUserPath(path)
	if err != nil {
		return err
	}

	info, err := os.Lstat(absPath)
	if err != nil {
		return fmt.Errorf("failed to stat path %s: %w", path, err)
	}

	// Minimize lock contention by restricting the critical section to state mutation and file I/O.
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	return cm.addPathWithOwnerLocked(absPath, owner, info)
}

// AddRelativePath resolves and registers an owner-style path relative to the
// ContextManager base path. It is primarily used for internal session rehydration
// where stored owner labels must resolve against the configured base, not the process CWD.
//
// It returns a stable label suitable for persisting as a TUI display label.
func (cm *ContextManager) AddRelativePath(ownerPath string) (string, error) {
	// Reject empty strings to prevent silent and unintentional root context
	// mutation during session rehydration.
	if ownerPath == "" {
		return "", fmt.Errorf("empty path is not valid for AddRelativePath")
	}

	verifiedPath := ownerPath
	posixForwardSlashLiteralProbeUsed := false

	if isTildeOwnerLabel(ownerPath) {
		if runtime.GOOS != "windows" {
			if strings.HasPrefix(ownerPath, `~\`) {
				// Step 1: Exact literal probe. Preserves literal backslash directory names.
				literalPath := filepath.Join(cm.basePath, ownerPath)
				if _, err := os.Lstat(literalPath); err == nil {
					verifiedPath = literalPath
				} else if !os.IsNotExist(err) {
					return "", fmt.Errorf("failed to stat literal path %s: %w", literalPath, err)
				} else {
					// Step 2: Normalized probe. Facilitates Windows-to-POSIX rehydration
					// where the directory structure uses forward slashes.
					normalizedRelative := strings.ReplaceAll(ownerPath, "\\", "/")
					normalizedPath := filepath.Join(cm.basePath, normalizedRelative)
					if _, err := os.Lstat(normalizedPath); err == nil {
						verifiedPath = normalizedPath
						// Maintain the original backslash label so it doesn't collide
						// with genuine tilde expansion labels.
					} else if !os.IsNotExist(err) {
						return "", fmt.Errorf("failed to stat normalized path %s: %w", normalizedPath, err)
					} else {
						// Step 3: Cross-platform tilde expansion fallback for Windows-originated
						// rehydration labels targeting $HOME.
						expanded, expandErr := expandTildeOwnerLabel(ownerPath)
						if expandErr != nil {
							return "", fmt.Errorf("failed to expand tilde in owner path %s: %w", ownerPath, expandErr)
						}
						verifiedPath = expanded
					}
				}
			} else {
				// Probe the literal basePath-relative location first so persisted
				// owner keys rehydrate consistently.
				literalPath := filepath.Join(cm.basePath, ownerPath)
				if _, err := os.Lstat(literalPath); err == nil {
					verifiedPath = literalPath
					posixForwardSlashLiteralProbeUsed = true
				} else if !os.IsNotExist(err) {
					return "", fmt.Errorf("failed to stat literal path %s: %w", literalPath, err)
				} else {
					expanded, expandErr := expandTildeOwnerLabel(ownerPath)
					if expandErr != nil {
						return "", fmt.Errorf("failed to expand tilde in owner path %s: %w", ownerPath, expandErr)
					}
					verifiedPath = expanded
				}
			}
		} else {
			expanded, expandErr := expandTildeOwnerLabel(ownerPath)
			if expandErr != nil {
				return "", fmt.Errorf("failed to expand tilde in owner path %s: %w", ownerPath, expandErr)
			}
			verifiedPath = expanded
		}
	}

	// Perform I/O-intensive absolute path resolution before acquiring the lock.
	absPath, err := cm.absolutePathFromOwner(verifiedPath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve path %s: %w", ownerPath, err)
	}

	// Tolerate relative owner labels resolving outside the base path to support
	// rehydrating portable sessions that were moved across hosts.
	if !filepath.IsAbs(verifiedPath) {
		if _, rerr := filepath.Rel(cm.basePath, absPath); rerr != nil {
			return "", fmt.Errorf("failed to compute relative path: %w", rerr)
		}
	}

	// Prevent adding dead entries during rehydration by verifying existence.
	info, err := os.Lstat(absPath)
	if err != nil {
		return "", fmt.Errorf("failed to stat path %s: %w", ownerPath, err)
	}

	// Normalize the owner path before acquiring the lock to maintain symmetry
	// with AddPath and minimize the critical section.
	owner := cm.normalizeOwnerPath(absPath)

	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	if err := cm.addPathWithOwnerLocked(absPath, owner, info); err != nil {
		return "", err
	}

	if isTildeOwnerLabel(ownerPath) {
		if runtime.GOOS != "windows" && posixForwardSlashLiteralProbeUsed {
			return owner, nil
		}
		return ownerPath, nil
	}
	return owner, nil
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
	cp.UpdateTime = info.ModTime()

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
		Path:       owner,
		Type:       "directory",
		Metadata:   make(map[string]string),
		Children:   children,
		UpdateTime: info.ModTime(),
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
// Tilde expansion is supported, so paths like "~/myfile.txt" work correctly.
//
// Empty strings are explicitly rejected and return an error to maintain
// API symmetry with AddPath, AddRelativePath, and RefreshPath.
//
// If the caller supplies a basename-only value (no separators), RemovePath
// attempts to match tracked paths by basename. If multiple matches exist,
// this returns an error; if a single unique match exists, it performs the
// appropriate removal.
func (cm *ContextManager) RemovePath(path string) error {
	if path == "" {
		return fmt.Errorf("empty path is not valid")
	}

	// Acquire the write lock for the entire operation to guarantee atomicity.
	// Lookup and removal must be contiguous to prevent TOCTOU race conditions.
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	owner, found, findErr := cm.findOwnerFromUserPath(path)
	if findErr != nil {
		return fmt.Errorf("removePath: %w", findErr)
	}
	if found {
		cm.removeOwnerLocked(owner)
		return nil
	}

	// Resolve targeted removals for bare basenames. Multiple matches are treated as
	// ambiguous to prevent unintended deletions.
	base := filepath.Base(path)
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
			if cm.removeOwnerLocked(matchKey) {
				return nil
			}

			if cp, ok := cm.paths[matchKey]; ok {
				if cp.Type == "directory" {
					// Handle defensive removals where the owner is implicitly a directory.
					cm.removeOwnerLocked(matchKey)
					return nil
				}

				delete(cm.paths, matchKey)

				for owner, set := range cm.ownerFiles {
					if _, present := set[matchKey]; present {
						delete(set, matchKey)
						if len(set) == 0 {
							delete(cm.ownerFiles, owner)
						}

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

				delete(cm.fileOwners, matchKey)

				return nil
			}
		}
	}

	// Idempotent success if the path is already absent.
	return nil
}

// GetPath returns information about a tracked path using a multi-step resolution:
//  1. Direct cm.paths[path] lookup (fast path)
//  2. basePath-relative normalization
//  3. Full canonicalization (host-specific tilde + CWD)
//  4. Fallback to owner resolution (handles tilde-form labels from AddRelativePath)
func (cm *ContextManager) GetPath(path string) (*ContextPath, bool) {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()

	// Step 1: Direct lookup fast path.
	if cp, exists := cm.paths[path]; exists {
		return cp, exists
	}

	// Step 2: basePath-relative normalization for non-tilde relative paths
	// (e.g., resolving "root/sub/child.txt" registered via directory tracking).
	if path != "" && !filepath.IsAbs(path) && !filepathutil.IsTildeExpansionPath(path) {
		absPath := filepath.Join(cm.basePath, path)
		if cleaned, err := filepath.Abs(absPath); err == nil {
			if relative := cm.normalizeOwnerPath(cleaned); relative != path {
				if cp, exists := cm.paths[relative]; exists {
					return cp, exists
				}
			}
		}
	}

	// Step 3: Full canonicalization against cm.paths to handle CWD-aware
	// resolutions (e.g., "./note.txt" or "../sibling/file.txt").
	if path != "" {
		normalized, _, err := cm.canonicalizeUserPath(path)
		if err == nil && normalized != path {
			if cp, exists := cm.paths[normalized]; exists {
				return cp, exists
			}
		}
	}

	// Step 3.5: Legacy POSIX backslash-label probe. Matches the normalized
	// forward-slash owner key to ensure child files round-trip correctly.
	if runtime.GOOS != "windows" && strings.HasPrefix(path, `~\`) {
		normalizedRel := filepath.Clean(strings.ReplaceAll(path, "\\", "/"))
		normalizedAbs := filepath.Join(cm.basePath, normalizedRel)
		normalized := cm.normalizeOwnerPath(normalizedAbs)
		if cp, exists := cm.paths[normalized]; exists {
			return cp, exists
		}
	}

	// Step 4: Fallback to owner resolution to handle tilde-form labels
	// where the internal key is the normalized absolute path.
	owner, found, err := cm.findOwnerFromUserPath(path)
	if err != nil || !found {
		return nil, false
	}
	cp, exists := cm.paths[owner]
	return cp, exists
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

// computePathLCA computes the lowest common ancestor directory prefix shared
// by all given paths, providing structural context. Returns "" if paths diverge immediately.
func computePathLCA(paths []string) string {
	if len(paths) == 0 {
		return ""
	}

	sep := string(filepath.Separator)

	var allDirs [][]string
	for _, p := range paths {
		dir := filepath.Dir(filepath.Clean(p))
		if dir == "." {
			continue
		}
		allDirs = append(allDirs, strings.Split(dir, sep))
	}

	if len(allDirs) == 0 {
		return ""
	}

	prefix := make([]string, len(allDirs[0]))
	copy(prefix, allDirs[0])
	for _, parts := range allDirs[1:] {
		n := len(prefix)
		if len(parts) < n {
			n = len(parts)
		}
		for i := 0; i < n; i++ {
			if prefix[i] != parts[i] {
				n = i
				break
			}
		}
		prefix = prefix[:n]
	}

	if len(prefix) == 0 {
		return ""
	}
	return strings.Join(prefix, sep)
}

// ToTxtar converts the context to txtar format.
func (cm *ContextManager) ToTxtar() *txtar.Archive {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()

	archive := &txtar.Archive{}

	type entry struct {
		key     string
		path    string
		content string
	}
	var files []entry
	// Group by basename to detect naming collisions.
	baseGroups := make(map[string][]entry)

	var trackedDirs []string

	for k, cp := range cm.paths {
		if cp.Type == "directory" {
			trackedDirs = append(trackedDirs, k)
			continue
		}
		if cp.Type != "file" {
			continue
		}
		absPath := cp.Path
		if !filepath.IsAbs(absPath) {
			absPath = filepath.Join(cm.basePath, cp.Path)
		}
		// Read the latest content from disk; silently skip on error to tolerate deleted files.
		data, err := os.ReadFile(absPath)
		if err != nil {
			continue
		}
		e := entry{key: k, path: cp.Path, content: string(data)}
		files = append(files, e)
		base := filepath.Base(cp.Path)
		baseGroups[base] = append(baseGroups[base], e)
	}

	// Compute LCA of all relative file paths to establish structural context.
	var relativePaths []string
	for _, e := range files {
		if !filepath.IsAbs(e.path) {
			relativePaths = append(relativePaths, e.path)
		}
	}
	lca := computePathLCA(relativePaths)

	// Embed context metadata in the txtar comment to help LLMs understand file origins
	// and tracked directory structures.
	var comment strings.Builder
	comment.WriteString("context root: " + filepath.ToSlash(cm.basePath) + "\n")
	if lca != "" {
		comment.WriteString("common path: " + filepath.ToSlash(lca) + "\n")
	}
	if len(trackedDirs) > 0 {
		slices.Sort(trackedDirs)
		dirList := make([]string, len(trackedDirs))
		for i, d := range trackedDirs {
			dirList[i] = filepath.ToSlash(d) + "/"
		}
		comment.WriteString("tracked directories: " + strings.Join(dirList, ", ") + "\n")
	}
	archive.Comment = []byte(comment.String())

	// computeUniqueSuffixes generates export names. It prioritizes full relative paths
	// for all-relative collision groups to preserve directory structure and prevent
	// false proximity impressions. Absolute paths fall back to suffix expansion.
	computeUniqueSuffixes := func(group []entry) map[string]string {
		out := make(map[string]string, len(group))
		if len(group) == 1 {
			path := group[0].path
			if filepath.IsAbs(path) {
				out[group[0].key] = filepath.Base(path)
			} else {
				out[group[0].key] = path
			}
			return out
		}

		allRelative := true
		for _, e := range group {
			if filepath.IsAbs(e.path) {
				allRelative = false
				break
			}
		}

		if allRelative {
			for _, e := range group {
				out[e.key] = filepath.Clean(e.path)
			}
			return out
		}

		type comps struct {
			key   string
			parts []string
		}
		arr := make([]comps, 0, len(group))
		maxDepth := 0
		sep := string(filepath.Separator)
		for _, e := range group {
			clean := filepath.Clean(e.path)
			parts := strings.Split(clean, sep)
			arr = append(arr, comps{key: e.key, parts: parts})
			if n := len(parts); n > maxDepth {
				maxDepth = n
			}
		}
		depth := 1
		for depth <= maxDepth {
			counts := make(map[string]int, len(arr))
			suffixes := make(map[string]string, len(arr))
			for _, c := range arr {
				n := len(c.parts)
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
		for _, c := range arr {
			out[c.key] = strings.Join(c.parts, sep)
		}
		return out
	}

	exportNames := make(map[string]string, len(files))
	for _, group := range baseGroups {
		names := computeUniqueSuffixes(group)
		for k, v := range names {
			// Normalize separators for portability and stable txtar display.
			exportNames[k] = filepath.ToSlash(v)
		}
	}

	// Emit files in a stable, alphabetically sorted order.
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
//
// The provided path is matched against tracked owner keys. If the raw value
// is not an exact match (e.g. the caller passes "src/" but the canonical
// owner is "src"), RefreshPath normalizes the input via the same logic used
// by AddPath to recover the canonical key. Tilde expansion is supported.
//
// Empty strings are explicitly rejected and return an error to prevent
// unintended root context mutation.
func (cm *ContextManager) RefreshPath(path string) error {
	// Prevent empty strings from resolving to "." and unintentionally mutating the root owner.
	if path == "" {
		return fmt.Errorf("empty path is not valid")
	}

	// Execute I/O-intensive path resolution operations outside the lock.
	cm.mutex.RLock()
	owner, found, findErr := cm.findOwnerFromUserPath(path)
	cm.mutex.RUnlock()

	if findErr != nil {
		return fmt.Errorf("refreshPath: %w", findErr)
	}

	if !found {
		return fmt.Errorf("path %s is not a tracked owner", path)
	}

	absPath, err := cm.absolutePathFromOwner(owner)
	if err != nil {
		return fmt.Errorf("failed to resolve path %s: %w", path, err)
	}

	info, err := os.Lstat(absPath)
	if err != nil {
		return fmt.Errorf("failed to stat path %s: %w", path, err)
	}

	// Minimize the write lock critical section.
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	// Re-verify tracked state to prevent TOCTOU resurrection if another
	// goroutine removed the owner between releasing the read lock and acquiring the write lock.
	if _, tracked := cm.ownerFiles[owner]; !tracked {
		return fmt.Errorf("path %s is not a tracked owner", path)
	}

	return cm.addPathWithOwnerLocked(absPath, owner, info)
}

// GetStats returns statistics about the context.
func (cm *ContextManager) GetStats() map[string]any {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()

	var files, directories, totalSize int

	for _, contextPath := range cm.paths {
		if contextPath.Type == "file" {
			files++
			if sizeStr, ok := contextPath.Metadata["size"]; ok {
				var size int
				fmt.Sscanf(sizeStr, "%d", &size)
				totalSize += size
			}
		} else {
			directories++
		}
	}

	return map[string]any{
		"totalPaths":  len(cm.paths),
		"files":       files,
		"directories": directories,
		"totalSize":   totalSize,
	}
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
