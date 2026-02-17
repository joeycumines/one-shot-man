package scripting

import (
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
			if err == require.ModuleFileDoesNotExistError || os.IsNotExist(err) {
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
// This check only applies to bare module names resolved through global folders.
// Relative requires (./foo, ../bar) are not subject to this check because
// their base directory is the script's own directory, not a global folder.
func newHardenedPathResolver(
	allowedDirs []string,
) require.PathResolver {
	return func(base, modpath string) string {
		resolved := require.DefaultPathResolver(base, modpath)

		// Only enforce containment when the base is one of the allowed dirs.
		// This means only bare module names resolved through global folders
		// (where base == a global folder) are restricted. Relative requires
		// from script directories pass through unchecked.
		if len(allowedDirs) > 0 && isExactDir(base, allowedDirs) {
			if !isContainedInDir(resolved, allowedDirs) {
				// Return a path that will never exist, causing module resolution
				// to fail with ModuleFileDoesNotExistError. We encode the reason
				// in the path so it shows up in error messages.
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
