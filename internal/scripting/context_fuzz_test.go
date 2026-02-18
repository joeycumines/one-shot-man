package scripting

import (
	"path/filepath"
	"strings"
	"testing"
)

// FuzzComputePathLCA fuzzes computePathLCA to verify that the returned LCA
// directory prefix is a valid common ancestor of all input paths. The fuzz
// function takes three string parameters (simulating a variable-length path
// list) and builds the slice dynamically.
func FuzzComputePathLCA(f *testing.F) {
	// Seed corpus: tuples of (path1, path2, path3)
	seeds := []struct {
		a, b, c string
	}{
		// Empty inputs
		{"", "", ""},
		// Single meaningful path
		{"src/main.go", "", ""},
		// Two paths with common prefix
		{"src/a/foo.go", "src/a/bar.go", ""},
		// Two paths without common prefix
		{"src/foo.go", "lib/bar.go", ""},
		// Root-level files
		{"foo.go", "bar.go", ""},
		// Deep nesting
		{"a/b/c/d/e/f.go", "a/b/c/d/g/h.go", "a/b/c/x.go"},
		// Relative paths
		{"./src/main.go", "./src/lib.go", ""},
		// Three divergent paths
		{"alpha/one.go", "beta/two.go", "gamma/three.go"},
		// Paths with dots
		{"../outside/file.go", "../outside/other.go", ""},
		// Same directory
		{"pkg/util/a.go", "pkg/util/b.go", "pkg/util/c.go"},
		// Longer common prefix
		{"very/long/common/path/a.go", "very/long/common/path/b.go", "very/long/common/other.go"},
		// Absolute root-level paths (LCA is "/")
		{"/x.go", "/y.go", ""},
		// Single absolute path
		{"/z.go", "", ""},
		// Absolute paths with common prefix
		{"/usr/local/a.go", "/usr/local/b.go", ""},
	}
	for _, s := range seeds {
		f.Add(s.a, s.b, s.c)
	}

	f.Fuzz(func(t *testing.T, a, b, c string) {
		// Build slice from non-empty inputs
		var paths []string
		for _, p := range []string{a, b, c} {
			if p != "" {
				paths = append(paths, p)
			}
		}

		result := computePathLCA(paths)

		// Invariant 1: If input is empty, result is empty.
		if len(paths) == 0 && result != "" {
			t.Fatalf("empty input produced non-empty result %q", result)
		}

		// Invariant 2: Result doesn't end with a path separator (unless it IS
		// the root separator itself, e.g. "/" for absolute root-level paths).
		if result != "" {
			sep := string(filepath.Separator)
			if result != sep && strings.HasSuffix(result, sep) {
				t.Fatalf("result %q ends with path separator", result)
			}
		}

		// Invariant 3: If result is non-empty, every input path (after
		// filepath.Clean + filepath.Dir) has result as a directory prefix.
		if result != "" {
			sep := string(filepath.Separator)
			resultPrefix := result + sep

			for _, p := range paths {
				dir := filepath.Dir(filepath.Clean(p))
				if dir == "." {
					// This path has no directory component; computePathLCA
					// should have returned "" since not all paths share a
					// common directory. If result is non-empty, there must
					// be at least some paths with directory components and
					// this one was skipped by the algorithm.
					continue
				}
				dirWithSep := dir + sep
				// The result must be a prefix of or equal to dir.
				if dir != result && !strings.HasPrefix(dirWithSep, resultPrefix) {
					t.Fatalf("result %q is not a prefix of dir %q (from path %q)",
						result, dir, p)
				}
			}
		}
	})
}
