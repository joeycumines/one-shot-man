package scripting

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/require"
)

// validateModulePaths checks each configured module path at startup.
// Invalid paths (nonexistent, not a directory, empty) are logged as warnings
// and excluded. Only valid, resolved paths are returned.
func validateModulePaths(paths []string, logger *slog.Logger) []string {
	valid := make([]string, 0, len(paths))
	for _, p := range paths {
		if p == "" {
			logger.Warn("ignoring empty module path")
			continue
		}
		info, err := os.Stat(p)
		if err != nil {
			logger.Warn("ignoring invalid module path",
				slog.String("path", p),
				slog.String("error", err.Error()))
			continue
		}
		if !info.IsDir() {
			logger.Warn("ignoring module path: not a directory",
				slog.String("path", p))
			continue
		}
		// Resolve to absolute + evaluate symlinks for consistent comparison.
		resolved, err := filepath.Abs(p)
		if err != nil {
			logger.Warn("ignoring module path: cannot resolve absolute path",
				slog.String("path", p),
				slog.String("error", err.Error()))
			continue
		}
		resolved, err = filepath.EvalSymlinks(resolved)
		if err != nil {
			logger.Warn("ignoring module path: cannot resolve symlinks",
				slog.String("path", p),
				slog.String("error", err.Error()))
			continue
		}
		valid = append(valid, resolved)
	}
	return valid
}

// circularDependencyTracker tracks module loading to detect require() cycles.
// It is safe for concurrent access, though goja's event loop serialises JS execution.
type circularDependencyTracker struct {
	mu    sync.Mutex
	stack []string // current require chain
}

// enter records entry into a module. If the module is already in the stack,
// it returns the cycle path (e.g. "a.js → b.js → a.js") and true.
// Otherwise it pushes the module and returns ("", false).
func (t *circularDependencyTracker) enter(module string) (string, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, m := range t.stack {
		if m == module {
			// Build cycle description: existing chain + the repeated module.
			cycle := make([]string, 0, len(t.stack)+1)
			found := false
			for _, s := range t.stack {
				if s == module {
					found = true
				}
				if found {
					cycle = append(cycle, s)
				}
			}
			cycle = append(cycle, module)
			return strings.Join(cycle, " → "), true
		}
	}
	t.stack = append(t.stack, module)
	return "", false
}

// leave removes the most-recently entered module from the stack.
func (t *circularDependencyTracker) leave() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.stack) > 0 {
		t.stack = t.stack[:len(t.stack)-1]
	}
}

// newHardenedSourceLoader builds a require.SourceLoader that wraps the base
// shebangStrippingLoader with:
//   - Symlink escape security: after reading a file that's under a global module
//     dir, checks that the resolved (post-symlink) path stays within allowed dirs.
//   - Better error reporting: includes search paths in "not found" errors.
//
// allowedDirs must contain pre-resolved (absolute, symlink-evaluated) paths.
// searchPathsDisplay is the original (pre-resolved) list shown in error messages.
func newHardenedSourceLoader(
	allowedDirs []string,
	searchPathsDisplay []string,
) require.SourceLoader {
	return func(filename string) ([]byte, error) {
		// Security check for symlink escapes: goja appends .js/.json extensions
		// AFTER the PathResolver runs, so symlinks within global module dirs can
		// escape detection. We check here at the actual file-read level.
		if len(allowedDirs) > 0 {
			absFilename, absErr := filepath.Abs(filename)
			if absErr == nil && isContainedInDir(absFilename, allowedDirs) {
				// The pre-symlink path is under an allowed dir.
				// Check that the post-symlink path also stays there.
				resolved, resolveErr := filepath.EvalSymlinks(absFilename)
				if resolveErr == nil && !isContainedInDir(resolved, allowedDirs) {
					return nil, fmt.Errorf("path traversal blocked: %q resolves via symlink outside allowed module paths %v",
						filename, searchPathsDisplay)
				}
			}
		}

		// Delegate to the base loader (shebang stripping + default file read).
		data, err := shebangStrippingLoader(filename)
		if err != nil {
			// Enhance "not found" errors with search path context.
			if errors.Is(err, require.ModuleFileDoesNotExistError) || os.IsNotExist(err) {
				if len(searchPathsDisplay) > 0 {
					return nil, fmt.Errorf("module %q not found (searched: %s): %w",
						filename, strings.Join(searchPathsDisplay, ", "), require.ModuleFileDoesNotExistError)
				}
				return nil, err
			}
			return nil, err
		}
		return data, nil
	}
}

// newHardenedPathResolver builds a require.PathResolver that wraps goja's
// DefaultPathResolver with path-traversal security. After the default resolver
// evaluates symlinks and joins paths, we verify that the resolved path stays
// within the configured module directories.
//
// allowedDirs must contain pre-resolved (absolute, symlink-evaluated) paths.
//
// Two checks are applied:
//
// Check 1 (Global folder containment): When the resolution base is an exact
// match for a configured global folder, the resolved path must stay within
// the allowed directories. This catches direct traversal via global folders.
//
// Check 2 (Bare module traversal): When a bare module name (not starting with
// ".", "..", or "/") contains ".." path components, the resolved path must stay
// within the resolution base directory. This catches traversal via the
// node_modules directory walk, where goja constructs base paths that don't
// match any configured global folder (e.g., scriptDir/node_modules).
//
// Relative requires (./foo, ../bar) are not subject to these checks because
// their base directory is the script's own directory, and "../" traversal
// from within a script is legitimate behavior.
func newHardenedPathResolver(
	allowedDirs []string,
) require.PathResolver {
	return func(base, modpath string) string {
		resolved := require.DefaultPathResolver(base, modpath)

		// Skip all checks when hardening is not configured.
		if len(allowedDirs) == 0 {
			return resolved
		}

		// Check 1: Global folder containment.
		// When resolving through a global folder (where base == a global folder),
		// the resolved path must stay within the configured allowed directories.
		if isExactDir(base, allowedDirs) {
			if !isContainedInDir(resolved, allowedDirs) {
				// Return a path that will never exist, causing module resolution
				// to fail with ModuleFileDoesNotExistError. We encode the reason
				// in the path so it shows up in error messages.
				return filepath.Join(base, ".osm-blocked-traversal")
			}
		}

		// Check 2: Bare module name with ".." traversal.
		// Bare module names (e.g., 'x/../../secret') are resolved through both
		// global folders and node_modules directory walks. Check 1 catches the
		// global folder case, but the node_modules walk uses dynamically
		// constructed base paths that never match allowedDirs. This check
		// catches the escape by verifying the resolved path stays within base.
		if !isRelativeOrAbsolutePath(modpath) && containsTraversalComponent(modpath) && filepath.IsAbs(base) {
			absBase := base
			if rb, err := filepath.EvalSymlinks(base); err == nil {
				absBase = rb
			}
			if !isContainedInDir(resolved, []string{absBase}) {
				return filepath.Join(base, ".osm-blocked-traversal")
			}
		}

		return resolved
	}
}

// installRequireCycleDetection wraps the existing require() function on the VM
// with circular dependency detection. goja_nodejs caches modules before execution,
// so cycles cannot be detected at the SourceLoader level — they must be detected
// at the require()-call level.
//
// When a cycle is detected, a warning is logged but the require proceeds normally
// (matching Node.js behavior of returning partial exports).
func installRequireCycleDetection(vm *goja.Runtime, logger *slog.Logger) {
	tracker := &circularDependencyTracker{}

	origRequire := vm.Get("require")
	origFn, ok := goja.AssertFunction(origRequire)
	if !ok {
		// require not set up — shouldn't happen after registry.Enable
		return
	}

	vm.Set("require", func(call goja.FunctionCall) goja.Value {
		modPath := call.Argument(0).String()

		if cyclePath, isCycle := tracker.enter(modPath); isCycle {
			logger.Warn("circular require detected",
				slog.String("cycle", cyclePath))
			// Don't block — Node.js allows circular requires (partial exports).
		} else {
			defer tracker.leave()
		}

		// Delegate to the original require function.
		// AssertFunction's callable catches *Exception panics and returns them
		// as errors; other panics propagate directly.
		ret, err := origFn(goja.Undefined(), call.Arguments...)
		if err != nil {
			// Re-throw as goja expects — panic with the error so the JS
			// runtime surfaces it properly.
			panic(vm.NewGoError(err))
		}
		return ret
	})
}

// isExactDir returns true if base exactly matches one of the directory paths.
func isExactDir(base string, dirs []string) bool {
	for _, dir := range dirs {
		if base == dir {
			return true
		}
	}
	return false
}

// isContainedInDir returns true if filePath is strictly contained within
// one of the allowed directories (no ../ escape).
// Both filePath and dirs must be absolute, symlink-resolved paths.
func isContainedInDir(filePath string, dirs []string) bool {
	for _, dir := range dirs {
		prefix := dir + string(filepath.Separator)
		if strings.HasPrefix(filePath, prefix) {
			return true
		}
	}
	return false
}

// isRelativeOrAbsolutePath returns true if modpath is a relative or absolute
// path (starts with ".", "..", "/", or is a Windows absolute path). Matches
// the classification used by goja_nodejs's require module to distinguish
// file/directory paths from bare module names.
func isRelativeOrAbsolutePath(modpath string) bool {
	if modpath == "." || modpath == ".." {
		return true
	}
	if strings.HasPrefix(modpath, "./") || strings.HasPrefix(modpath, "../") {
		return true
	}
	// On Windows, also recognize backslash variants.
	if filepath.Separator == '\\' {
		if strings.HasPrefix(modpath, `.\`) || strings.HasPrefix(modpath, `..\`) {
			return true
		}
	}
	return filepath.IsAbs(modpath)
}

// containsTraversalComponent returns true if modpath contains ".." as a path
// component (not just as a substring). For example:
//   - "x/../../secret" → true (.. is a path component)
//   - "module..name"   → false (.. is within a component, not a component)
//   - "../foo"         → true (.. is a path component)
func containsTraversalComponent(modpath string) bool {
	// Normalize to forward slashes for consistent parsing.
	normalized := filepath.ToSlash(modpath)
	for _, part := range strings.Split(normalized, "/") {
		if part == ".." {
			return true
		}
	}
	return false
}
