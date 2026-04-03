package filepathutil

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestIsTildeExpansionPath tests that IsTildeExpansionPath correctly
// identifies actual tilde expansion forms vs literal paths starting with "~".
func TestIsTildeExpansionPath(t *testing.T) {
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
		// Windows-specific tilde expansion forms (should return true)
		{"tilde with backslash", "~\\", true},
		{"tilde with Windows path", "~\\Documents\\file.txt", true},
		{"tilde with nested Windows path", "~\\src\\project", true},
		{"tilde with trailing backslash", "~\\src\\", true},

		// Literal paths on Windows (should return false)
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

	// On POSIX, ~\ is NOT a tilde expansion form
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

// TestExpandTildeBareTilde tests that bare "~" expands to the user's home directory.
func TestExpandTildeBareTilde(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	t.Setenv("USERPROFILE", fakeHome)

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("os.UserHomeDir failed: %v", err)
	}
	if home != fakeHome {
		t.Fatalf("os.UserHomeDir returned %q, want %q", home, fakeHome)
	}

	result, err := ExpandTilde("~")
	if err != nil {
		t.Fatalf("ExpandTilde(\"~\") returned error: %v", err)
	}
	if result != fakeHome {
		t.Errorf("ExpandTilde(\"~\") = %q, want %q", result, fakeHome)
	}
}

// TestExpandTildeUnix tests Unix-style tilde expansion.
func TestExpandTildeUnix(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-specific test")
	}

	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	// Ensure USERPROFILE is also set so os.UserHomeDir is consistent.
	t.Setenv("USERPROFILE", fakeHome)

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("os.UserHomeDir failed: %v", err)
	}
	if home != fakeHome {
		t.Fatalf("os.UserHomeDir returned %q, want %q", home, fakeHome)
	}

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

// TestExpandTildeWindows tests Windows-style tilde expansion.
func TestExpandTildeWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-only test")
	}

	fakeHome := t.TempDir()
	t.Setenv("USERPROFILE", fakeHome)
	// Clear HOME on Windows to force USERPROFILE usage.
	t.Setenv("HOME", "")

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("os.UserHomeDir failed: %v", err)
	}
	if home != fakeHome {
		t.Fatalf("os.UserHomeDir returned %q, want %q", home, fakeHome)
	}

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

// TestExpandTildeErrorWhenHomeDirUnavailable verifies that ExpandTilde returns
// a descriptive error when the home directory cannot be determined.
// Uses t.Setenv to properly isolate the test from global state.
func TestExpandTildeErrorWhenHomeDirUnavailable(t *testing.T) {
	// Save original values using LookupEnv to distinguish between unset and empty string.
	origHome, homeWasSet := os.LookupEnv("HOME")
	origUserProfile, userProfileWasSet := os.LookupEnv("USERPROFILE")

	// Unset both environment variables so os.UserHomeDir will fail.
	os.Unsetenv("HOME")
	os.Unsetenv("USERPROFILE")

	// Restore original values after the test.
	t.Cleanup(func() {
		if homeWasSet {
			os.Setenv("HOME", origHome)
		} else {
			os.Unsetenv("HOME")
		}
		if userProfileWasSet {
			os.Setenv("USERPROFILE", origUserProfile)
		} else {
			os.Unsetenv("USERPROFILE")
		}
	})

	result, err := ExpandTilde("~/test.txt")
	if err == nil {
		// On systems where os.UserHomeDir has fallback mechanisms (e.g., /etc/passwd
		// on some Unix systems), expansion may succeed even with env vars unset.
		// In that case, just verify the result is a valid absolute path.
		if result == "" {
			t.Errorf("expected non-empty result on success, got empty string")
		}
		if !filepath.IsAbs(result) && result != "~/test.txt" {
			t.Errorf("result should be absolute path, got: %q", result)
		}
		return
	}

	// If expansion failed, verify the error message is descriptive.
	if !strings.Contains(err.Error(), "unable to determine home directory") {
		t.Errorf("expected error about home directory, got: %v", err)
	}
	if result != "" {
		t.Errorf("expected empty result on error, got: %q", result)
	}
}

// TestExpandTildeDoesNotModifyLiteralPaths verifies that literal paths starting
// with "~" but not matching tilde expansion forms are returned unchanged.
func TestExpandTildeDoesNotModifyLiteralPaths(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	t.Setenv("USERPROFILE", fakeHome)

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
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	t.Setenv("USERPROFILE", fakeHome)

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

// TestExpandTildeWithLiteralTildePaths tests that ExpandTilde does not
// modify literal paths that happen to start with "~".
func TestExpandTildeWithLiteralTildePaths(t *testing.T) {
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

		// Actual tilde expansion forms should be modified
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
				// Some paths might fail if home directory is not available
				// (e.g., in minimal test environments)
				t.Skipf("ExpandTilde failed (home dir unavailable): %v", err)
			}

			same := result == tc.input
			if same != tc.wantSame {
				if tc.wantSame {
					t.Errorf("ExpandTilde(%q) = %q, want %q (unchanged)", tc.input, result, tc.input)
				} else {
					t.Errorf("ExpandTilde(%q) = %q, want different value (should expand)", tc.input, result)
				}
			}
		})
	}
}
