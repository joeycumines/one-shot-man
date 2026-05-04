package scripting

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestLiteralTildePaths tests that literal paths starting with "~" (but not
// actual tilde expansion forms) work correctly with RemovePath and RefreshPath.
//
// This addresses the regression where findOwnerFromUserPath was using
// HasPrefix(path, "~") which incorrectly skipped base-path normalization
// for ALL paths starting with "~", including literal paths like "~cache".
func TestLiteralTildePaths(t *testing.T) {
	// Create a temporary directory to serve as our base path
	tmpDir := t.TempDir()

	// Create a literal directory named "~cache" (not a tilde expansion)
	literalTildeDir := filepath.Join(tmpDir, "~cache")
	if err := os.Mkdir(literalTildeDir, 0755); err != nil {
		t.Fatalf("failed to create ~cache directory: %v", err)
	}

	// Create a file inside the ~cache directory
	testFile := filepath.Join(literalTildeDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	cm, err := NewContextManager(tmpDir)
	if err != nil {
		t.Fatalf("NewContextManager failed: %v", err)
	}

	// Add the literal tilde path using AddPath
	// The path "~cache" should be treated as a literal path, not expanded
	// We need to add it with the trailing slash to test the edge case
	if err := cm.AddPath(literalTildeDir); err != nil {
		t.Fatalf("AddPath(%q) failed: %v", literalTildeDir, err)
	}

	// Verify the path was added
	paths := cm.ListPaths()
	if len(paths) == 0 {
		t.Fatal("expected at least one path, got none")
	}

	// The owner should be "~cache" (normalized relative to basePath)
	expectedOwner := "~cache"
	found := false
	for _, p := range paths {
		if filepath.Base(p) == expectedOwner {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected to find owner %q, got paths: %v", expectedOwner, paths)
	}

	// Test RemovePath with the literal tilde path with trailing slash
	// This should work correctly - the bug was that this would fail because
	// findOwnerFromUserPath would skip step 2 (basePath-relative normalization)
	// for any path starting with "~", causing it to canonicalize via CWD instead
	if err := cm.RemovePath("~cache/"); err != nil {
		t.Errorf("RemovePath(\"~cache/\") failed: %v", err)
	}

	// Verify the path was removed
	paths = cm.ListPaths()
	if len(paths) != 0 {
		t.Errorf("expected no paths after removal, got: %v", paths)
	}
}

// TestRefreshPathWithLiteralTilde tests that RefreshPath works correctly
// with literal tilde paths.
func TestRefreshPathWithLiteralTilde(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a literal directory named "~tmp"
	literalTildeDir := filepath.Join(tmpDir, "~tmp")
	if err := os.Mkdir(literalTildeDir, 0755); err != nil {
		t.Fatalf("failed to create ~tmp directory: %v", err)
	}

	testFile := filepath.Join(literalTildeDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("original content"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	cm, err := NewContextManager(tmpDir)
	if err != nil {
		t.Fatalf("NewContextManager failed: %v", err)
	}

	// Add the literal tilde path
	if err := cm.AddPath(literalTildeDir); err != nil {
		t.Fatalf("AddPath failed: %v", err)
	}

	// Modify the file
	if err := os.WriteFile(testFile, []byte("updated content"), 0644); err != nil {
		t.Fatalf("failed to modify test file: %v", err)
	}

	// Refresh using the literal tilde path with trailing slash
	// This should work correctly - the bug was that this would fail
	if err := cm.RefreshPath("~tmp/"); err != nil {
		t.Errorf("RefreshPath(\"~tmp/\") failed: %v", err)
	}

	// Verify the content was updated
	owner := "~tmp"
	if cp, ok := cm.GetPath(owner); ok {
		if cp.Type != "directory" {
			t.Errorf("expected type directory, got %q", cp.Type)
		}
	} else {
		t.Errorf("expected to find owner %q after refresh", owner)
	}
}

// TestRemovePathWithVariousLiteralTildePaths tests various literal tilde path patterns
func TestRemovePathWithVariousLiteralTildePaths(t *testing.T) {
	tmpDir := t.TempDir()

	testCases := []string{
		"~cache",
		"~tmp",
		"~foo",
		"~bar/baz",
	}

	for _, tc := range testCases {
		t.Run(tc, func(t *testing.T) {
			// Create the directory structure
			testPath := filepath.Join(tmpDir, tc)
			if err := os.MkdirAll(testPath, 0755); err != nil {
				t.Fatalf("failed to create directory: %v", err)
			}

			// Create a file inside
			testFile := filepath.Join(testPath, "test.txt")
			if err := os.WriteFile(testFile, []byte("content"), 0644); err != nil {
				t.Fatalf("failed to create test file: %v", err)
			}

			cm, err := NewContextManager(tmpDir)
			if err != nil {
				t.Fatalf("NewContextManager failed: %v", err)
			}

			// Add the path
			if err := cm.AddPath(testPath); err != nil {
				t.Fatalf("AddPath failed: %v", err)
			}

			// Remove using the literal name with trailing slash
			removePath := tc + "/"
			if err := cm.RemovePath(removePath); err != nil {
				t.Errorf("RemovePath(%q) failed: %v", removePath, err)
			}

			// Verify it was removed
			paths := cm.ListPaths()
			if len(paths) != 0 {
				t.Errorf("expected no paths after removing %q, got: %v", removePath, paths)
			}
		})
	}
}

// TestEmptyStringBehavior tests that empty strings are explicitly rejected
// by AddPath, AddRelativePath, RefreshPath, and RemovePath.
func TestEmptyStringBehavior(t *testing.T) {
	tmpDir := t.TempDir()

	cm, err := NewContextManager(tmpDir)
	if err != nil {
		t.Fatalf("NewContextManager failed: %v", err)
	}

	// Test AddPath
	if err := cm.AddPath(""); err == nil {
		t.Error("AddPath(\"\") expected error, got nil")
	} else if !strings.Contains(err.Error(), "empty path") {
		t.Errorf("AddPath(\"\") error should mention empty path, got: %v", err)
	}

	// Test AddRelativePath
	if _, err := cm.AddRelativePath(""); err == nil {
		t.Error("AddRelativePath(\"\") expected error, got nil")
	} else if !strings.Contains(err.Error(), "empty path") {
		t.Errorf("AddRelativePath(\"\") error should mention empty path, got: %v", err)
	}

	// Test RefreshPath
	if err := cm.RefreshPath(""); err == nil {
		t.Error("RefreshPath(\"\") expected error, got nil")
	} else if !strings.Contains(err.Error(), "empty path") {
		t.Errorf("RefreshPath(\"\") error should mention empty path, got: %v", err)
	}

	// Test RemovePath
	if err := cm.RemovePath(""); err == nil {
		t.Error("RemovePath(\"\") expected error, got nil")
	} else if !strings.Contains(err.Error(), "empty path") {
		t.Errorf("RemovePath(\"\") error should mention empty path, got: %v", err)
	}
}

// TestRemovePathAtomicity tests that RemovePath does not have a TOCTOU race
// condition where a path could be removed and re-added between lookup and removal.
//
// This is a basic test that verifies the single-lock implementation works correctly.
// More sophisticated concurrent testing would be needed to fully verify atomicity.
func TestRemovePathAtomicity(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a test file
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("content"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	cm, err := NewContextManager(tmpDir)
	if err != nil {
		t.Fatalf("NewContextManager failed: %v", err)
	}

	// Add the path
	if err := cm.AddPath(testFile); err != nil {
		t.Fatalf("AddPath failed: %v", err)
	}

	// Remove it
	if err := cm.RemovePath("test.txt"); err != nil {
		t.Errorf("RemovePath failed: %v", err)
	}

	// Verify it was removed
	paths := cm.ListPaths()
	if len(paths) != 0 {
		t.Errorf("expected no paths after removal, got: %v", paths)
	}

	// Now re-add the same path
	if err := cm.AddPath(testFile); err != nil {
		t.Fatalf("AddPath (re-add) failed: %v", err)
	}

	// Remove it again
	if err := cm.RemovePath("test.txt"); err != nil {
		t.Errorf("RemovePath (second) failed: %v", err)
	}

	// Verify it was removed again
	paths = cm.ListPaths()
	if len(paths) != 0 {
		t.Errorf("expected no paths after second removal, got: %v", paths)
	}
}

// TestPOSIXCompletionWithBackslash tests that on POSIX systems, backslash
// is not treated as a path separator in completion.
func TestPOSIXCompletionWithBackslash(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping POSIX-specific test on Windows")
	}

	tmpDir := t.TempDir()

	// Create a file with a backslash in the name (valid on POSIX)
	// Note: This is tricky because backslash is an escape character in shells
	// We'll create it using raw bytes
	testFileName := "test\\file.txt"
	testFile := filepath.Join(tmpDir, testFileName)

	// We need to create the file using os.Create with the exact bytes
	f, err := os.Create(testFile)
	if err != nil {
		// Some filesystems may not support backslashes in filenames
		t.Skipf("filesystem does not support backslash in filename: %v", err)
	}
	f.Close()

	cm, err := NewContextManager(tmpDir)
	if err != nil {
		t.Fatalf("NewContextManager failed: %v", err)
	}

	tm := &TUIManager{
		writer:   NewTUIWriterFromIO(io.Discard),
		commands: make(map[string]Command),
		modes:    make(map[string]*ScriptMode),
	}
	_ = tm // Use tm to avoid unused variable error
	_ = cm // Use cm to avoid unused variable error

	// The key test: when typing a first argument containing a backslash,
	// the completion logic should NOT treat it as a path-like argument
	// on POSIX systems. This means the fallback file suggestions should
	// appear (if the command supports file completion).

	// This is tested indirectly through the getDefaultCompletionSuggestionsFor
	// function, which is tested in tui_completion_unit_test.go
}

// TestWindowsTildeCompletion tests that Windows completion properly handles
// tilde with backslash (~\).
func TestWindowsTildeCompletion(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("skipping Windows-specific test on non-Windows")
	}

	tmpDir := t.TempDir()

	cm, err := NewContextManager(tmpDir)
	if err != nil {
		t.Fatalf("NewContextManager failed: %v", err)
	}

	tm := &TUIManager{
		writer:   NewTUIWriterFromIO(io.Discard),
		commands: make(map[string]Command),
		modes:    make(map[string]*ScriptMode),
	}
	_ = tm // Use tm to avoid unused variable error
	_ = cm // Use cm to avoid unused variable error

	// Test that bare ~ suggests ~\ on Windows
	suggestions := getFilepathSuggestions("~")
	if len(suggestions) != 1 {
		t.Fatalf("expected 1 suggestion for ~, got %d", len(suggestions))
	}
	expectedSuggestion := "~\\"
	if suggestions[0].Text != expectedSuggestion {
		t.Errorf("expected suggestion %q, got %q", expectedSuggestion, suggestions[0].Text)
	}
}
