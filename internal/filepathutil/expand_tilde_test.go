package filepathutil

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestIsTildeExpansionPath tests that IsTildeExpansionPath correctly
// identifies actual tilde expansion forms vs literal paths starting with "~".
func TestIsTildeExpansionPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		// Actual tilde expansion forms (should return true)
		{"bare tilde", "~", true},
		{"tilde with forward slash", "~/", true},
		{"tilde with path", "~/Documents", true},
		{"tilde with nested path", "~/src/project", true},
		{"tilde with trailing slash", "~/src/", true},

		// Literal paths starting with "~" (should return false on all platforms)
		{"literal tilde cache", "~cache", false},
		{"literal tilde tmp", "~tmp", false},
		{"literal tilde foo", "~foo", false},
		{"literal tilde with slash but not second char", "~cache/", false},
		{"literal tilde bar baz", "~bar/baz", false},

		// Paths that don't start with "~"
		{"absolute path", "/usr/local/bin", false},
		{"relative path", "src/file.txt", false},
		{"current directory", ".", false},
		{"parent directory", "..", false},
		{"empty string", "", false},
		{"regular file", "file.txt", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := IsTildeExpansionPath(tc.path)
			if result != tc.expected {
				t.Errorf("IsTildeExpansionPath(%q) = %v, want %v", tc.path, result, tc.expected)
			}
		})
	}
}

// TestIsTildeExpansionPathWindows tests Windows-specific tilde expansion forms.
func TestIsTildeExpansionPathWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("skipping Windows-specific test")
	}

	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{"tilde with backslash", "~\\", true},
		{"tilde with Windows path", "~\\Documents\\file.txt", true},
		{"tilde with nested Windows path", "~\\src\\project", true},
		{"tilde with trailing backslash", "~\\src\\", true},

		{"literal tilde cache", "~cache", false},
		{"literal tilde tmp", "~tmp", false},
		{"literal tilde with backslash but not second char", "~cache\\", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := IsTildeExpansionPath(tc.path)
			if result != tc.expected {
				t.Errorf("IsTildeExpansionPath(%q) = %v, want %v", tc.path, result, tc.expected)
			}
		})
	}
}

// TestIsTildeExpansionPathPOSIX tests that backslash after tilde is NOT
// treated as a tilde expansion form on POSIX systems.
func TestIsTildeExpansionPathPOSIX(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping POSIX-specific test on Windows")
	}

	tests := []string{
		"~\\",
		"~\\Documents\\file.txt",
		"~\\cache",
	}

	for _, path := range tests {
		t.Run(path, func(t *testing.T) {
			result := IsTildeExpansionPath(path)
			if result {
				t.Errorf("IsTildeExpansionPath(%q) = true on POSIX, want false", path)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ExpandTilde tests — deterministic via direct userHomeDir override
// ---------------------------------------------------------------------------

// setupFakeHome overrides userHomeDir for deterministic testing. Returns the
// fake home path. Original is restored via t.Cleanup.
func setupFakeHome(t *testing.T) string {
	t.Helper()
	fakeHome := t.TempDir()
	orig := userHomeDir
	userHomeDir = func() (string, error) { return fakeHome, nil }
	t.Cleanup(func() { userHomeDir = orig })

	// Verify the override works
	home, err := userHomeDir()
	if err != nil {
		t.Fatalf("overridden userHomeDir failed: %v", err)
	}
	if home != fakeHome {
		t.Fatalf("overridden userHomeDir returned %q, want %q", home, fakeHome)
	}
	return fakeHome
}

// setupBrokenHome overrides userHomeDir to always fail. Original is restored
// via t.Cleanup.
func setupBrokenHome(t *testing.T) {
	t.Helper()
	orig := userHomeDir
	userHomeDir = func() (string, error) {
		return "", fmt.Errorf("home directory unavailable: $HOME is not set")
	}
	t.Cleanup(func() { userHomeDir = orig })
}

// setupRelativeHome overrides userHomeDir to return a relative (non-absolute)
// path. Original is restored via t.Cleanup.
func setupRelativeHome(t *testing.T) string {
	t.Helper()
	relativeHome := "relative-home"
	orig := userHomeDir
	userHomeDir = func() (string, error) { return relativeHome, nil }
	t.Cleanup(func() { userHomeDir = orig })
	return relativeHome
}

// TestExpandTildeBareTilde tests that bare "~" expands to the home directory
// resolved by the injected resolver.
func TestExpandTildeBareTilde(t *testing.T) {
	fakeHome := setupFakeHome(t)

	result, err := ExpandTilde("~")
	if err != nil {
		t.Fatalf("ExpandTilde(\"~\") returned error: %v", err)
	}
	if result != fakeHome {
		t.Errorf("ExpandTilde(\"~\") = %q, want %q", result, fakeHome)
	}
}

// TestExpandTildeUnix tests Unix-style tilde expansion using the injected resolver.
func TestExpandTildeUnix(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-specific test")
	}

	fakeHome := setupFakeHome(t)

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"tilde path unix", "~/.claude/agents/Takumi.md", filepath.Join(fakeHome, ".claude", "agents", "Takumi.md")},
		{"tilde path with subdirs", "~/foo/bar/baz.txt", filepath.Join(fakeHome, "foo", "bar", "baz.txt")},
		{"tilde path root", "~/", filepath.Join(fakeHome, "")},
		{"bare tilde", "~", fakeHome},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := ExpandTilde(tc.input)
			if err != nil {
				t.Fatalf("ExpandTilde(%q) returned error: %v", tc.input, err)
			}
			if result != tc.expected {
				t.Errorf("ExpandTilde(%q) = %q, want %q", tc.input, result, tc.expected)
			}
		})
	}
}

// TestExpandTildeWindows tests Windows-style tilde expansion using the injected resolver.
func TestExpandTildeWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-only test")
	}

	fakeHome := setupFakeHome(t)

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"tilde path windows backslash", "~\\Documents\\file.txt", filepath.Join(fakeHome, "Documents", "file.txt")},
		{"tilde path windows with subdirs", "~\\foo\\bar\\baz.txt", filepath.Join(fakeHome, "foo", "bar", "baz.txt")},
		{"tilde path root backslash", "~\\", filepath.Join(fakeHome, "")},
		{"bare tilde", "~", fakeHome},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := ExpandTilde(tc.input)
			if err != nil {
				t.Fatalf("ExpandTilde(%q) returned error: %v", tc.input, err)
			}
			if result != tc.expected {
				t.Errorf("ExpandTilde(%q) = %q, want %q", tc.input, result, tc.expected)
			}
		})
	}
}

// TestExpandTilde_JoinRegressionGuard is a targeted regression test proving
// that ExpandTilde("~/foo") produces /home/user/foo — NOT /foo.
//
// This guards against a critical path-math bug: Go's filepath.Join replaces
// prior elements when a later element begins with a separator. If the
// remainder after tilde extraction is joined naively, the home directory is
// lost:
//
//	filepath.Join("/home/user", "/foo") == "/foo"  // BUG — home discarded!
//
// The current implementation avoids this by using filepath.Clean on the
// concatenation of home, separator, and path[2:] (which skips both "~" and
// the separator, yielding "foo" from "~/foo"). filepath.Clean then
// normalizes the result. Because path[2:] strips the separator, repeated
// separators in the original input (e.g., "~//foo") are handled correctly —
// filepath.Clean collapses them.
func TestExpandTilde_JoinRegressionGuard(t *testing.T) {
	fakeHome := setupFakeHome(t)

	tests := []struct {
		input  string
		suffix string // expected suffix after the home directory
	}{
		{"~/foo", "foo"},
		{"~/bar/baz", filepath.Join("bar", "baz")},
		{"~/.hidden", ".hidden"},
		{"~/nested/deep/path.txt", filepath.Join("nested", "deep", "path.txt")},
		{"~//foo", "foo"},                    // double-slash: filepath.Clean collapses
		{"~///bar", "bar"},                   // triple-slash: filepath.Clean collapses
		{"~//a//b", filepath.Join("a", "b")}, // multiple double-slashes
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			result, err := ExpandTilde(tc.input)
			if err != nil {
				t.Fatalf("ExpandTilde(%q) error: %v", tc.input, err)
			}

			expected := filepath.Join(fakeHome, tc.suffix)
			if result != expected {
				t.Fatalf("ExpandTilde(%q) = %q, want %q — home directory was discarded!", tc.input, result, expected)
			}

			// Verify result is actually under the home directory
			if !strings.HasPrefix(result, fakeHome) {
				t.Fatalf("ExpandTilde(%q) = %q — result is NOT under home directory %q!", tc.input, result, fakeHome)
			}
		})
	}
}

// TestExpandTildeErrorWhenHomeDirUnavailable verifies that ExpandTilde returns
// a descriptive error when the home directory cannot be determined.
// Uses setupBrokenHome to directly override the unexported userHomeDir
// variable for deterministic behavior regardless of OS-level fallbacks
// (getpwuid on Unix, registry on Windows).
func TestExpandTildeErrorWhenHomeDirUnavailable(t *testing.T) {
	setupBrokenHome(t)

	result, err := ExpandTilde("~/test.txt")
	if err == nil {
		t.Fatalf("ExpandTilde(\"~/test.txt\") succeeded with result %q — expected error when home directory is unavailable", result)
	}

	// Verify the error message is descriptive
	errMsg := err.Error()
	if !strings.Contains(errMsg, "unable to determine home directory") {
		t.Errorf("expected error mentioning 'unable to determine home directory', got: %v", errMsg)
	}
	if !strings.Contains(errMsg, "home directory unavailable") {
		t.Errorf("expected wrapped error from resolver mentioning 'home directory unavailable', got: %v", errMsg)
	}
	if result != "" {
		t.Errorf("expected empty result on error, got: %q", result)
	}
}

// TestExpandTilde_BareTildeFailure verifies that bare "~" also fails
// deterministically when the home directory resolver fails.
func TestExpandTilde_BareTildeFailure(t *testing.T) {
	setupBrokenHome(t)

	result, err := ExpandTilde("~")
	if err == nil {
		t.Fatalf("ExpandTilde(\"~\") succeeded with result %q — expected error when home directory is unavailable", result)
	}
	if !strings.Contains(err.Error(), "unable to determine home directory") {
		t.Errorf("expected error about home directory, got: %v", err)
	}
}

// TestExpandTilde_RelativeHomeErrors verifies that ExpandTilde rejects a
// tilde expansion when the resolved home directory is not an absolute path.
// This is a safety guard: tilde-expanded paths are documented as inherently
// absolute, so a relative home directory would produce a relative result,
// violating the contract and potentially causing downstream bugs in callers
// that assume absolute paths (e.g., ContextManager's canonicalizeUserPath).
func TestExpandTilde_RelativeHomeErrors(t *testing.T) {
	relativeHome := setupRelativeHome(t)

	tests := []struct {
		name  string
		input string
	}{
		{"tilde with forward slash", "~/test.txt"},
		{"bare tilde", "~"},
		{"tilde with nested path", "~/foo/bar/baz.txt"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := ExpandTilde(tc.input)
			if err == nil {
				t.Fatalf("ExpandTilde(%q) succeeded with result %q — expected error when home directory is relative (%q)", tc.input, result, relativeHome)
			}
			if !strings.Contains(err.Error(), "not absolute") {
				t.Errorf("expected error mentioning 'not absolute', got: %v", err)
			}
			if result != "" {
				t.Errorf("expected empty result on error, got: %q", result)
			}
		})
	}
}

// TestExpandTilde_NonTildePathsIgnoreBrokenHome verifies that paths without
// tilde expansion forms are returned unchanged even when the home directory
// resolver is broken. This is correct behavior — no tilde, no resolution needed.
func TestExpandTilde_NonTildePathsIgnoreBrokenHome(t *testing.T) {
	setupBrokenHome(t)

	tests := []struct {
		name  string
		input string
	}{
		{"relative path", "foo/bar.txt"},
		{"absolute path", "/usr/local/bin"},
		{"empty string", ""},
		{"literal tilde cache", "~cache"},
		{"literal tilde bar/baz", "~bar/baz"},
		{"dot", "."},
		{"dotdot", ".."},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := ExpandTilde(tc.input)
			if err != nil {
				t.Fatalf("ExpandTilde(%q) should not fail for non-tilde paths, got: %v", tc.input, err)
			}
			if result != tc.input {
				t.Errorf("ExpandTilde(%q) = %q, want %q (unchanged)", tc.input, result, tc.input)
			}
		})
	}
}

// TestExpandTildeDoesNotModifyLiteralPaths verifies that literal paths starting
// with "~" but not matching tilde expansion forms are returned unchanged.
func TestExpandTildeDoesNotModifyLiteralPaths(t *testing.T) {
	fakeHome := setupFakeHome(t)
	_ = fakeHome

	tests := []struct {
		name  string
		input string
	}{
		{"literal tilde cache", "~cache"},
		{"literal tilde tmp", "~tmp"},
		{"literal tilde foo", "~foo"},
		{"literal tilde with slash but not second char", "~cache/"},
		{"literal tilde bar/baz", "~bar/baz"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := ExpandTilde(tc.input)
			if err != nil {
				t.Fatalf("ExpandTilde(%q) returned error: %v", tc.input, err)
			}
			if result != tc.input {
				t.Errorf("ExpandTilde(%q) = %q, want %q (unchanged)", tc.input, result, tc.input)
			}
		})
	}
}

// TestExpandTildeAbsolutePathsUnchanged verifies that absolute paths and
// non-tilde relative paths are returned unchanged.
func TestExpandTildeAbsolutePathsUnchanged(t *testing.T) {
	fakeHome := setupFakeHome(t)
	_ = fakeHome

	tests := []struct {
		name  string
		input string
	}{
		{"absolute path unix", "/usr/local/bin"},
		{"absolute path windows", "C:\\Users\\test"},
		{"relative path", "src/file.txt"},
		{"current dir", "."},
		{"parent dir", ".."},
		{"empty string", ""},
		{"regular file", "file.txt"},
		{"tilde with spaces not valid", "~ /foo"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := ExpandTilde(tc.input)
			if err != nil {
				t.Fatalf("ExpandTilde(%q) returned error: %v", tc.input, err)
			}
			if result != tc.input {
				t.Errorf("ExpandTilde(%q) = %q, want %q (unchanged)", tc.input, result, tc.input)
			}
		})
	}
}

// TestExpandTildeWithLiteralTildePaths tests that ExpandTilde correctly
// distinguishes between literal tilde paths (unchanged) and actual tilde
// expansion forms (expanded). Uses injected resolver for determinism.
// No t.Skip on error — all paths must resolve deterministically.
func TestExpandTildeWithLiteralTildePaths(t *testing.T) {
	fakeHome := setupFakeHome(t)

	tests := []struct {
		name     string
		input    string
		wantSame bool // true if we expect output to equal input
	}{
		// Literal tilde paths should not be modified
		{"literal tilde cache", "~cache", true},
		{"literal tilde tmp", "~tmp", true},
		{"literal tilde foo", "~foo", true},
		{"literal tilde with slash but not second char", "~cache/", true},

		// Actual tilde expansion forms should be modified (expanded)
		{"bare tilde", "~", false},
		{"tilde with forward slash", "~/", false},
		{"tilde with path", "~/Documents", false},

		// Non-tilde paths should not be modified
		{"regular path", "src/file.txt", true},
		{"absolute path", "/usr/bin", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := ExpandTilde(tc.input)
			if err != nil {
				t.Fatalf("ExpandTilde(%q) failed: %v", tc.input, err)
			}

			same := result == tc.input
			if same != tc.wantSame {
				if tc.wantSame {
					t.Errorf("ExpandTilde(%q) = %q, want %q (unchanged)", tc.input, result, tc.input)
				} else {
					t.Errorf("ExpandTilde(%q) = %q, want different value (should expand to path under %q)", tc.input, result, fakeHome)
				}
			}

			// For expansion forms, verify the result is actually under the home directory
			if !tc.wantSame && !strings.HasPrefix(result, fakeHome) {
				t.Errorf("ExpandTilde(%q) = %q — expanded path is NOT under home directory %q", tc.input, result, fakeHome)
			}
		})
	}
}
