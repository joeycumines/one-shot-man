package scripting

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/joeycumines/one-shot-man/internal/filepathutil"
	"golang.org/x/tools/txtar"
)

// canonicalizeUserPath converts a user-provided path to a canonical owner key.
// It performs tilde expansion, resolves to an absolute path, and normalizes
// relative to the ContextManager's base path. This ensures consistent handling
// of user inputs across AddPath and RemovePath.
//
// Note: RefreshPath has its own empty-path guard at the top of the function
// to prevent basePath-relative logic from converting "" to "." before
// canonicalization can reject it.
//
// It returns both the canonical owner key and the absolute path for efficiency,
// avoiding redundant path resolution in callers.
func (cm *ContextManager) canonicalizeUserPath(path string) (string, string, error) {
	// Guard against empty string inputs to prevent unintentional root owner mutation.
	// An empty string would resolve to the process CWD, which could cause
	// RemovePath("") to canonicalize to "." and wipe the tracked root owner
	// unintentionally.
	if path == "" {
		return "", "", fmt.Errorf("empty path is not valid")
	}

	// Expand tilde before converting to absolute path
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
	paths      map[string]*contextPath
	basePath   string
	mutex      sync.RWMutex
	ownerFiles map[string]map[string]struct{}
	fileOwners map[string]int
}

// contextPath represents a tracked file or directory with metadata.
type contextPath struct {
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
		paths:      make(map[string]*contextPath),
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

// findOwnerFromUserPath attempts to resolve a user-provided path to a tracked
// owner key. It performs a sophisticated 3-step resolution:
//
//  1. Try the raw path as an exact owner match
//  2. If not found and path is relative/non-tilde, try basePath-relative normalization
//  3. If still not found, try full canonicalization (tilde expansion + CWD resolution)
//
// This ensures that paths like "root/", "./root", or "~/project" all resolve to
// the same tracked owner regardless of which form was used to add the path.
//
// The caller MUST hold at least a read lock on cm.mutex when calling this method.
//
// Returns (owner, true, nil) if a tracked owner is found, ("", false, nil) if
// not found, or ("", false, err) if canonicalization itself fails (e.g.,
// corrupted $HOME preventing tilde expansion).
func (cm *ContextManager) findOwnerFromUserPath(path string) (string, bool, error) {
	// Guard against empty strings - they should never match a tracked owner.
	// This prevents RemovePath("") from accidentally removing the root owner.
	if path == "" {
		return "", false, nil
	}

	// Step 1: Try raw path as exact owner match
	if _, tracked := cm.ownerFiles[path]; tracked {
		return path, true, nil
	}

	// Step 2: Try basePath-relative normalization for relative, non-tilde paths.
	// This handles cases where the caller adds "root/" but the canonical owner
	// is "root". We need to resolve against basePath, not the process CWD.
	//
	// IMPORTANT: We only skip this step for actual tilde expansion forms (e.g., "~/", "~\"),
	// not for literal paths that happen to start with "~" (e.g., "~cache", "~tmp").
	// Literal tilde paths should get basePath-relative normalization just like any
	// other relative path.
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

	// Step 3: Fall back to CLI/CWD-style canonicalization (tilde expansion,
	// absolute path resolution, CWD-relative handling).
	// If canonicalizeUserPath fails (e.g., corrupted $HOME preventing tilde
	// expansion), we MUST bubble the error up — the caller (RemovePath,
	// RefreshPath) needs to know that the failure was a system error, not a
	// benign cache miss.
	normalized, _, err := cm.canonicalizeUserPath(path)
	if err != nil {
		return "", false, fmt.Errorf("findOwnerFromUserPath(%q): %w", path, err)
	}
	if _, tracked := cm.ownerFiles[normalized]; tracked {
		return normalized, true, nil
	}

	// No tracked owner found
	return "", false, nil
}

func (cm *ContextManager) absolutePathFromOwner(owner string) (string, error) {
	if owner == "." {
		return cm.basePath, nil
	}
	if filepath.IsAbs(owner) {
		// Ensure we return a canonical absolute path for absolute inputs.
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
	// Perform path resolution operations (tilde expansion, filepath.Abs, os.Lstat)
	// BEFORE acquiring the lock. These operations can be slow and involve syscalls,
	// so holding the lock during them would cause contention. However, the actual
	// file reading and directory walking (in addPathWithOwnerLocked) still happen
	// inside the lock - adding large files/directories will temporarily block
	// other operations.
	owner, absPath, err := cm.canonicalizeUserPath(path)
	if err != nil {
		return err
	}

	info, err := os.Lstat(absPath)
	if err != nil {
		return fmt.Errorf("failed to stat path %s: %w", path, err)
	}

	// Only acquire the lock for the map mutation and file reading
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	return cm.addPathWithOwnerLocked(absPath, owner, info)
}

// AddRelativePath resolves the provided owner-style path relative to the
// ContextManager base path and then registers it. This is intended for
// internal use (e.g. session rehydration) where stored owner labels must be
// resolved against the manager's configured base rather than the process CWD.
//
// It returns the canonical "owner" key that the ContextManager used to store
// the path (this is the normalized, relative form when possible). Returning
// the owner allows callers (e.g., TUI rehydration) to keep persisted labels
// in sync with the backend and avoid ghost entries.
func (cm *ContextManager) AddRelativePath(ownerPath string) (string, error) {
	// Guard against empty string inputs to prevent unintentional root owner mutation.
	// An empty string would resolve to the basePath, which could cause
	// silent and unintentional modification of the root context during
	// session rehydration.
	if ownerPath == "" {
		return "", fmt.Errorf("empty path is not valid for AddRelativePath")
	}

	// Accept both forward- and back-slash separators for owner labels so
	// that sessions created on Windows (or with Windows-style labels) can
	// be rehydrated on other hosts. Important: do NOT mutate the caller's
	// label (e.g. converting backslashes to separators) — the ContextManager
	// should operate on the exact label provided. Normalization is a caller
	// concern (TUI rehydration code performs a conditional normalization
	// fallback when appropriate).

	// For tilde-prefixed owner labels (e.g. "~/.claude/agents/Takumi.md"),
	// expand tilde to an absolute path for existence verification below,
	// but preserve the original tilde-form label as the stored owner.
	// Without expansion, filepath.IsAbs returns false for "~/" and the path
	// would be incorrectly resolved relative to basePath.
	verifiedPath := ownerPath
	if filepathutil.IsTildeExpansionPath(ownerPath) {
		expanded, err := filepathutil.ExpandTilde(ownerPath)
		if err != nil {
			return "", fmt.Errorf("failed to expand tilde in owner path %s: %w", ownerPath, err)
		}
		verifiedPath = expanded
	}

	// Perform I/O-intensive operations BEFORE acquiring the lock
	absPath, err := cm.absolutePathFromOwner(verifiedPath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve path %s: %w", ownerPath, err)
	}

	// Historically AddRelativePath rejected relative owner labels that
	// resolved outside the configured base path. ContextManager is not a
	// security sandbox — rejecting such labels breaks legitimate
	// rehydration scenarios where external absolute paths were previously
	// normalized into relative labels (e.g. sessions that were made portable
	// and later rehydrated on a different host). To avoid creating sessions
	// that cannot be reloaded, we do not reject relative inputs solely
	// because they resolve outside the base path. We still compute the
	// relative form to detect errors, but do not treat leading ".." as an
	// operational error during rehydration.
	if !filepath.IsAbs(verifiedPath) {
		if _, rerr := filepath.Rel(cm.basePath, absPath); rerr != nil {
			return "", fmt.Errorf("failed to compute relative path: %w", rerr)
		}
	}

	// Verify the target exists to avoid adding dead entries during
	// rehydration.
	info, err := os.Lstat(absPath)
	if err != nil {
		return "", fmt.Errorf("failed to stat path %s: %w", ownerPath, err)
	}

	// Normalize the owner path BEFORE acquiring the lock to maintain
	// symmetry with AddPath, which calls canonicalizeUserPath (which
	// calls normalizeOwnerPath) outside the lock. This keeps the critical
	// section as small as possible.
	owner := cm.normalizeOwnerPath(absPath)

	// Only acquire the lock for the minimal critical section
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	if err := cm.addPathWithOwnerLocked(absPath, owner, info); err != nil {
		return "", err
	}
	// For tilde-prefixed inputs, return the original tilde form so that
	// TUI state labels are preserved (e.g. "~/.claude/agents/Takumi.md"
	// stays as-is rather than being replaced with the expanded absolute
	// path). The internal ContextManager maps use the normalized absolute
	// owner key; findOwnerFromUserPath (used by RemovePath, RefreshPath)
	// expands tildes during lookup, so the tilde label remains functional.
	if filepathutil.IsTildeExpansionPath(ownerPath) {
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
		cp = &contextPath{
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

	cm.paths[owner] = &contextPath{
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
	// Guard against empty string inputs to maintain API symmetry with
	// AddPath, AddRelativePath, and RefreshPath. All of these methods
	// explicitly reject empty strings, and RemovePath should do the same.
	if path == "" {
		return fmt.Errorf("empty path is not valid")
	}

	// Acquire write lock for the entire operation to ensure atomicity.
	// The lookup and removal must happen as one unit to prevent race
	// conditions where a path is removed and re-added between lookup and removal.
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	// Try to find the owner under the write lock
	owner, found, findErr := cm.findOwnerFromUserPath(path)
	if findErr != nil {
		return fmt.Errorf("removePath: %w", findErr)
	}
	if found {
		cm.removeOwnerLocked(owner)
		return nil
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
func (cm *ContextManager) GetPath(path string) (*contextPath, bool) {
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

// computePathLCA computes the lowest common ancestor directory prefix
// shared by all given paths. Returns "" if there is no common directory
// component (e.g. all files are at root level, or paths diverge immediately).
func computePathLCA(paths []string) string {
	if len(paths) == 0 {
		return ""
	}

	sep := string(filepath.Separator)

	// Split each path into directory components (exclude the filename).
	var allDirs [][]string
	for _, p := range paths {
		dir := filepath.Dir(filepath.Clean(p))
		if dir == "." {
			continue // no directory component
		}
		allDirs = append(allDirs, strings.Split(dir, sep))
	}

	if len(allDirs) == 0 {
		return ""
	}

	// Find the longest common prefix across all directory component slices.
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

	// Build a list of file paths and group by basename to detect collisions.
	type entry struct {
		key     string
		path    string
		content string
	}
	var files []entry
	baseGroups := make(map[string][]entry)

	// Collect tracked directory names for metadata.
	var trackedDirs []string

	for k, cp := range cm.paths {
		if cp.Type == "directory" {
			trackedDirs = append(trackedDirs, k)
			continue
		}
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

	// Compute LCA of all relative file paths to provide structural context.
	var relativePaths []string
	for _, e := range files {
		if !filepath.IsAbs(e.path) {
			relativePaths = append(relativePaths, e.path)
		}
	}
	lca := computePathLCA(relativePaths)

	// Build txtar comment with context metadata. This helps LLMs and humans
	// understand where the files originate and which directories are tracked.
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

	// Helper: compute export names for a set of paths that share the same basename.
	// For relative paths in collision groups, full paths are always used to preserve
	// directory structure and avoid false proximity impressions. For absolute paths,
	// suffix expansion is used to keep names manageable.
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

		// Check if all paths in the collision group are relative.
		allRelative := true
		for _, e := range group {
			if filepath.IsAbs(e.path) {
				allRelative = false
				break
			}
		}

		// For all-relative collision groups, always use the full relative path.
		// Full relative paths are inherently unique (same map key) and preserve
		// the complete directory structure, avoiding the false proximity that
		// occurs when minimal unique suffixes strip common prefixes.
		if allRelative {
			for _, e := range group {
				out[e.key] = filepath.Clean(e.path)
			}
			return out
		}

		// For mixed or all-absolute groups: use suffix expansion algorithm.
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
		// Increase depth from 1 (basename) until all suffixes are unique or we exhaust.
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
	cm.paths = make(map[string]*contextPath)
	cm.ownerFiles = make(map[string]map[string]struct{})
	cm.fileOwners = make(map[string]int)

	for _, file := range archive.Files {
		contextPath := &contextPath{
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
	// Hard guard against empty strings at the very top, before any path
	// resolution logic runs. This prevents "" from resolving to "." and
	// potentially mutating the root owner unintentionally.
	if path == "" {
		return fmt.Errorf("empty path is not valid")
	}

	// Find the tracked owner before acquiring the lock. This involves path
	// resolution operations (filepath.Abs, etc.) that we don't want to do
	// while holding the lock.
	cm.mutex.RLock()
	owner, found, findErr := cm.findOwnerFromUserPath(path)
	cm.mutex.RUnlock()

	if findErr != nil {
		return fmt.Errorf("refreshPath: %w", findErr)
	}

	if !found {
		return fmt.Errorf("path %s is not a tracked owner", path)
	}

	// Resolve absolute path and stat outside the lock
	absPath, err := cm.absolutePathFromOwner(owner)
	if err != nil {
		return fmt.Errorf("failed to resolve path %s: %w", path, err)
	}

	info, err := os.Lstat(absPath)
	if err != nil {
		return fmt.Errorf("failed to stat path %s: %w", path, err)
	}

	// Only acquire the write lock for the minimal critical section
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	// Re-verify tracked state to prevent TOCTOU resurrection.
	// Between releasing the read lock (above) and acquiring this write lock,
	// another goroutine could have removed the owner. If so, error out.
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

	cm.paths = make(map[string]*contextPath)
	cm.ownerFiles = make(map[string]map[string]struct{})
	cm.fileOwners = make(map[string]int)
}
