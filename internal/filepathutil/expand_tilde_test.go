package filepathutil

import (
	"runtime"
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
