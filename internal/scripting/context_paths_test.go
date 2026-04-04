package scripting

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestContextPathsInTxtarHeaders tests that file paths are meaningfully preserved
// in txtar file headers as requested in the issue.
func TestContextPathsInTxtarHeaders(t *testing.T) {
	t.Run("RelativePathsPreserveStructure", func(t *testing.T) {
		dir := t.TempDir()

		mustWrite := func(p, content string) {
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
		}

		// Create files with meaningful directory structure
		f1 := filepath.Join(dir, "src", "main.go")
		f2 := filepath.Join(dir, "docs", "README.md")
		f3 := filepath.Join(dir, "config", "app.yaml")
		mustWrite(f1, "package main\n")
		mustWrite(f2, "# README\n")
		mustWrite(f3, "app: test\n")

		cm, err := NewContextManager(dir)
		if err != nil {
			t.Fatalf("NewContextManager failed: %v", err)
		}
		for _, f := range []string{f1, f2, f3} {
			if err := cm.AddPath(f); err != nil {
				t.Fatal(err)
			}
		}

		txtarString := cm.GetTxtarString()

		// Files should preserve their meaningful relative paths in txtar headers
		if !strings.Contains(txtarString, "-- src/main.go --") {
			t.Errorf("Expected 'src/main.go' in txtar headers, got: %s", txtarString)
		}
		if !strings.Contains(txtarString, "-- docs/README.md --") {
			t.Errorf("Expected 'docs/README.md' in txtar headers, got: %s", txtarString)
		}
		if !strings.Contains(txtarString, "-- config/app.yaml --") {
			t.Errorf("Expected 'config/app.yaml' in txtar headers, got: %s", txtarString)
		}
	})

	t.Run("AbsolutePathsUseBasename", func(t *testing.T) {
		baseDir := t.TempDir()
		externalDir := t.TempDir()

		mustWrite := func(p, content string) {
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
		}

		// Create files both inside and outside base directory
		internalFile := filepath.Join(baseDir, "internal.go")
		externalFile := filepath.Join(externalDir, "external.go")

		mustWrite(internalFile, "package internal\n")
		mustWrite(externalFile, "package external\n")

		cm, err := NewContextManager(baseDir)
		if err != nil {
			t.Fatalf("NewContextManager failed: %v", err)
		}

		// Add both files
		if err := cm.AddPath(internalFile); err != nil {
			t.Fatal(err)
		}
		if err := cm.AddPath(externalFile); err != nil {
			t.Fatal(err)
		}

		txtarString := cm.GetTxtarString()

		// Internal file should preserve relative path structure if meaningful
		if !strings.Contains(txtarString, "-- internal.go --") {
			t.Errorf("Expected 'internal.go' in txtar headers, got: %s", txtarString)
		}

		// External file should use basename to avoid verbose absolute paths
		if !strings.Contains(txtarString, "-- external.go --") {
			t.Errorf("Expected 'external.go' in txtar headers, got: %s", txtarString)
		}

		// Should not contain the full absolute path
		if strings.Contains(txtarString, externalDir) {
			t.Errorf("Should not contain absolute path %s in txtar: %s", externalDir, txtarString)
		}
	})

	t.Run("CollisionsStillDisambiguated", func(t *testing.T) {
		dir := t.TempDir()
		mustWrite := func(p, content string) {
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
		}

		// Create files with basename collisions
		f1 := filepath.Join(dir, "a", "handlers.go")
		f2 := filepath.Join(dir, "b", "handlers.go")
		mustWrite(f1, "package a\n")
		mustWrite(f2, "package b\n")

		cm, err := NewContextManager(dir)
		if err != nil {
			t.Fatalf("NewContextManager failed: %v", err)
		}
		for _, f := range []string{f1, f2} {
			if err := cm.AddPath(f); err != nil {
				t.Fatal(err)
			}
		}

		txtarString := cm.GetTxtarString()

		// Colliding files should still be disambiguated with directory suffixes
		if !strings.Contains(txtarString, "-- a/handlers.go --") || !strings.Contains(txtarString, "-- b/handlers.go --") {
			t.Errorf("Expected disambiguated names for colliding files, got: %s", txtarString)
		}
	})

	t.Run("DirectoryRecursivelyIncludesFilesAndSymlinks", func(t *testing.T) {
		base := t.TempDir()
		root := filepath.Join(base, "root")
		if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
			t.Fatal(err)
		}

		mustWrite := func(p, content string) {
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
		}

		fileA := filepath.Join(root, "file_a.txt")
		fileB := filepath.Join(root, "sub", "file_b.txt")
		target := filepath.Join(base, "target.txt")
		link := filepath.Join(root, "link.txt")

		mustWrite(fileA, "a")
		mustWrite(fileB, "b")
		mustWrite(target, "c")

		if err := os.Symlink(target, link); err != nil {
			if runtime.GOOS == "windows" {
				t.Skipf("symlink creation not supported: %v", err)
			}
			t.Fatalf("failed to create symlink: %v", err)
		}

		cm, err := NewContextManager(base)
		if err != nil {
			t.Fatalf("NewContextManager failed: %v", err)
		}
		if err := cm.AddPath(root); err != nil {
			t.Fatalf("AddPath(root) failed: %v", err)
		}

		txtarString := cm.GetTxtarString()
		for _, want := range []string{"-- root/file_a.txt --", "-- root/sub/file_b.txt --", "-- root/link.txt --"} {
			if !strings.Contains(txtarString, want) {
				t.Fatalf("expected %q in txtar output, got: %s", want, txtarString)
			}
		}
		if strings.Contains(txtarString, "-- root --") {
			t.Fatalf("directory headers should not be exported directly: %s", txtarString)
		}

		paths := cm.ListPaths()
		if !containsString(paths, "root") {
			t.Fatalf("expected directory entry 'root' to be tracked, got: %v", paths)
		}
		if !containsString(paths, strings.ReplaceAll("root/file_a.txt", "/", string(filepath.Separator))) ||
			!containsString(paths, strings.ReplaceAll("root/sub/file_b.txt", "/", string(filepath.Separator))) ||
			!containsString(paths, strings.ReplaceAll("root/link.txt", "/", string(filepath.Separator))) {
			t.Fatalf("expected directory files to be tracked individually, got: %v", paths)
		}
	})

	t.Run("RemovingDirectoryRetainsStandaloneFiles", func(t *testing.T) {
		base := t.TempDir()
		root := filepath.Join(base, "root")
		if err := os.MkdirAll(root, 0o755); err != nil {
			t.Fatal(err)
		}

		filePath := filepath.Join(root, "keep.txt")
		if err := os.WriteFile(filePath, []byte("hello"), 0o644); err != nil {
			t.Fatal(err)
		}

		cm, err := NewContextManager(base)
		if err != nil {
			t.Fatalf("NewContextManager failed: %v", err)
		}
		if err := cm.AddPath(filePath); err != nil {
			t.Fatalf("AddPath(file) failed: %v", err)
		}
		if err := cm.AddPath(root); err != nil {
			t.Fatalf("AddPath(root) failed: %v", err)
		}

		if err := cm.RemovePath("root"); err != nil {
			t.Fatalf("RemovePath(root) failed: %v", err)
		}

		if containsString(cm.ListPaths(), "root") {
			t.Fatalf("expected directory entry 'root' to be removed")
		}

		txtarString := cm.GetTxtarString()
		if !strings.Contains(txtarString, "-- root/keep.txt --") {
			t.Fatalf("expected standalone file to remain tracked, got: %s", txtarString)
		}
	})
}

func TestAddRelativePath_PreservesLiteralBackslash(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX-only test; windows does not allow backslash in filenames")
	}

	base := t.TempDir()

	// Create a file whose name contains a literal backslash character.
	// On POSIX systems '\\' is a valid byte in a filename and must not
	// be treated as a path separator by the ContextManager.
	filename := "foo\\bar.txt"
	full := filepath.Join(base, filename)

	if err := os.WriteFile(full, []byte("ok"), 0o644); err != nil {
		t.Fatalf("failed to write file with backslash in name: %v", err)
	}

	cm, err := NewContextManager(base)
	if err != nil {
		t.Fatalf("NewContextManager failed: %v", err)
	}

	owner, err := cm.AddRelativePath(filename)
	if err != nil {
		t.Fatalf("AddRelativePath failed to register literal-backslash filename: %v", err)
	}

	if _, ok := cm.GetPath(owner); !ok {
		t.Fatalf("expected path to be tracked for owner %q (original label %q)", owner, filename)
	}
}

// Test that AddRelativePath accepts relative owner labels that resolve
// outside of the configured base path. Historically such labels were
// rejected (treated as directory traversal), which prevented sessions that
// normalized absolute external paths into relative forms from being
// rehydrated correctly.
func TestAddRelativePath_AllowsRelativeOutsideBase(t *testing.T) {
	base := t.TempDir()
	external := t.TempDir()

	// Create a file in the external directory.
	extFile := filepath.Join(external, "external.txt")
	if err := os.WriteFile(extFile, []byte("ok"), 0o644); err != nil {
		t.Fatalf("failed to write external file: %v", err)
	}

	// Compute a relative path from base that points to the external file.
	rel, err := filepath.Rel(base, extFile)
	if err != nil {
		t.Fatalf("failed to compute relative path: %v", err)
	}

	cm, err := NewContextManager(base)
	if err != nil {
		t.Fatalf("NewContextManager failed: %v", err)
	}

	// Previously this would return an error. Now it should register the
	// external file successfully (relevant for session rehydration).
	owner, err := cm.AddRelativePath(rel)
	if err != nil {
		t.Fatalf("AddRelativePath(%q) unexpectedly failed: %v", rel, err)
	}

	if _, ok := cm.GetPath(owner); !ok {
		t.Fatalf("expected path to be tracked for owner %q (original label %q)", owner, rel)
	}
}

func TestAddPathHandlesAbsolutePaths(t *testing.T) {
	base := t.TempDir()

	// Create file inside base
	filePath := filepath.Join(base, "sub", "file.txt")
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		t.Fatalf("failed to make dirs: %v", err)
	}
	if err := os.WriteFile(filePath, []byte("hi"), 0o644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	cm, err := NewContextManager(base)
	if err != nil {
		t.Fatalf("NewContextManager failed: %v", err)
	}

	// Add absolute path; should be normalized to a relative owner key
	if err := cm.AddPath(filePath); err != nil {
		t.Fatalf("AddPath failed: %v", err)
	}

	paths := cm.ListPaths()
	expected := filepath.Join("sub", "file.txt")
	found := false
	for _, p := range paths {
		if p == expected {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected %q to be tracked, got paths: %v", expected, paths)
	}
}

func TestContextManagerRemoveOwnership(t *testing.T) {
	t.Run("FileRemovalKeepsRemainingOwners", func(t *testing.T) {
		base := t.TempDir()
		root := filepath.Join(base, "root")
		if err := os.MkdirAll(root, 0o755); err != nil {
			t.Fatalf("failed to create root: %v", err)
		}

		filePath := filepath.Join(root, "shared.txt")
		if err := os.WriteFile(filePath, []byte("content"), 0o644); err != nil {
			t.Fatalf("failed to write file: %v", err)
		}

		cm, err := NewContextManager(base)
		if err != nil {
			t.Fatalf("NewContextManager failed: %v", err)
		}
		if err := cm.AddPath(filePath); err != nil {
			t.Fatalf("AddPath(file) failed: %v", err)
		}
		if err := cm.AddPath(root); err != nil {
			t.Fatalf("AddPath(directory) failed: %v", err)
		}

		logical := filepath.Join("root", "shared.txt")
		if err := cm.RemovePath(logical); err != nil {
			t.Fatalf("RemovePath(file) failed: %v", err)
		}

		if _, ok := cm.GetPath(logical); !ok {
			t.Fatalf("expected %q to remain tracked after removing only one owner", logical)
		}

		paths := cm.ListPaths()
		if !containsString(paths, "root") || !containsString(paths, logical) {
			t.Fatalf("expected directory and file to remain tracked, got: %v", paths)
		}

		if err := cm.RemovePath("root"); err != nil {
			t.Fatalf("RemovePath(directory) failed: %v", err)
		}
		if _, ok := cm.GetPath(logical); ok {
			t.Fatalf("expected %q to be removed after all owners detached", logical)
		}
	})

	t.Run("BasenameRemovalAndAmbiguity", func(t *testing.T) {
		base := t.TempDir()
		root := filepath.Join(base, "root")
		if err := os.MkdirAll(filepath.Join(root, "nested"), 0o755); err != nil {
			t.Fatalf("failed to create nested directory: %v", err)
		}

		filePath := filepath.Join(root, "nested", "log.txt")
		if err := os.WriteFile(filePath, []byte("log"), 0o644); err != nil {
			t.Fatalf("failed to write file: %v", err)
		}

		cm, err := NewContextManager(base)
		if err != nil {
			t.Fatalf("NewContextManager failed: %v", err)
		}
		if err := cm.AddPath(root); err != nil {
			t.Fatalf("AddPath(directory) failed: %v", err)
		}

		// With only a single tracked entry matching the basename,
		// removing by basename should succeed and untrack that file.
		if err := cm.RemovePath("log.txt"); err != nil {
			t.Fatalf("expected RemovePath to succeed for unique basename, got: %v", err)
		}

		logical := filepath.Join("root", "nested", "log.txt")
		if _, ok := cm.GetPath(logical); ok {
			t.Fatalf("expected %q to be removed after basename removal", logical)
		}

		// Now exercise the ambiguous case: create a fresh manager with two
		// distinct tracked files that share the same basename so the basename
		// is ambiguous.
		filePath2 := filepath.Join(root, "other", "log.txt")
		if err := os.MkdirAll(filepath.Dir(filePath2), 0o755); err != nil {
			t.Fatalf("failed to create nested dir: %v", err)
		}
		if err := os.WriteFile(filePath2, []byte("log2"), 0o644); err != nil {
			t.Fatalf("failed to write second log: %v", err)
		}

		// Rebuild manager with both files to make the basename ambiguous.
		cm2, err := NewContextManager(base)
		if err != nil {
			t.Fatalf("NewContextManager failed: %v", err)
		}
		if err := cm2.AddPath(filePath); err != nil {
			t.Fatalf("AddPath(file1) failed: %v", err)
		}
		if err := cm2.AddPath(filePath2); err != nil {
			t.Fatalf("AddPath(file2) failed: %v", err)
		}

		// Now basename "log.txt" is ambiguous and should be rejected.
		err = cm2.RemovePath("log.txt")
		if err == nil {
			t.Fatalf("expected RemovePath to fail for ambiguous basename match")
		}
		if got := err.Error(); !strings.Contains(got, "ambiguous path") {
			t.Fatalf("expected 'ambiguous path' error, got: %v", got)
		}
	})

	t.Run("FullPathDoesNotTriggerAmbiguity", func(t *testing.T) {
		// Create two tracked files that share a basename but live under different
		// logical prefixes. Then attempt to remove using a non-matching full path
		// which happens to have the same basename: this should return 'path not found'
		// rather than an ambiguous error because the caller provided a full path.
		base2 := t.TempDir()
		if err := os.MkdirAll(filepath.Join(base2, "a"), 0o755); err != nil {
			t.Fatalf("failed to create dir a: %v", err)
		}
		if err := os.MkdirAll(filepath.Join(base2, "c"), 0o755); err != nil {
			t.Fatalf("failed to create dir c: %v", err)
		}

		f1 := filepath.Join(base2, "a", "b.txt")
		f2 := filepath.Join(base2, "c", "b.txt")
		if err := os.WriteFile(f1, []byte("1"), 0o644); err != nil {
			t.Fatalf("failed to write f1: %v", err)
		}
		if err := os.WriteFile(f2, []byte("2"), 0o644); err != nil {
			t.Fatalf("failed to write f2: %v", err)
		}

		cm, err := NewContextManager(base2)
		if err != nil {
			t.Fatalf("NewContextManager failed: %v", err)
		}
		if err := cm.AddPath(f1); err != nil {
			t.Fatalf("AddPath(f1) failed: %v", err)
		}
		if err := cm.AddPath(f2); err != nil {
			t.Fatalf("AddPath(f2) failed: %v", err)
		}

		// Use a non-matching full path (different directory) but same basename.
		// Because the caller provided a full/path-like value, this should not
		// be treated as a basename-ambiguous operation.
		// Since RemovePath is now idempotent, this should succeed (return nil).
		nonMatching := filepath.Join("x", "b.txt")
		err = cm.RemovePath(nonMatching)
		if err != nil {
			t.Fatalf("expected RemovePath to succeed (idempotent) for non-tracked full path, got: %v", err)
		}
	})
}

func TestContextManagerRefreshPath(t *testing.T) {
	t.Run("FileOwnerUpdatesContent", func(t *testing.T) {
		base := t.TempDir()
		filePath := filepath.Join(base, "note.txt")
		if err := os.WriteFile(filePath, []byte("initial"), 0o644); err != nil {
			t.Fatalf("failed to write initial file: %v", err)
		}

		cm, err := NewContextManager(base)
		if err != nil {
			t.Fatalf("NewContextManager failed: %v", err)
		}
		if err := cm.AddPath(filePath); err != nil {
			t.Fatalf("AddPath(file) failed: %v", err)
		}

		if err := os.WriteFile(filePath, []byte("updated"), 0o644); err != nil {
			t.Fatalf("failed to update file: %v", err)
		}

		if err := cm.RefreshPath("note.txt"); err != nil {
			t.Fatalf("RefreshPath(note.txt) failed: %v", err)
		}

		cp, ok := cm.GetPath("note.txt")
		if !ok {
			t.Fatalf("expected note.txt to remain tracked after refresh")
		}
		if cp.Content != "updated" {
			t.Fatalf("expected refreshed content to be 'updated', got: %q", cp.Content)
		}
	})

	t.Run("DirectoryRefreshIncludesNewFiles", func(t *testing.T) {
		base := t.TempDir()
		root := filepath.Join(base, "root")
		if err := os.MkdirAll(root, 0o755); err != nil {
			t.Fatalf("failed to create root directory: %v", err)
		}

		existing := filepath.Join(root, "a.txt")
		if err := os.WriteFile(existing, []byte("a"), 0o644); err != nil {
			t.Fatalf("failed to write existing file: %v", err)
		}

		cm, err := NewContextManager(base)
		if err != nil {
			t.Fatalf("NewContextManager failed: %v", err)
		}
		if err := cm.AddPath(root); err != nil {
			t.Fatalf("AddPath(directory) failed: %v", err)
		}

		newFile := filepath.Join(root, "b.txt")
		if err := os.WriteFile(newFile, []byte("b"), 0o644); err != nil {
			t.Fatalf("failed to write new file: %v", err)
		}

		if err := cm.RefreshPath("root"); err != nil {
			t.Fatalf("RefreshPath(root) failed: %v", err)
		}

		logicalNew := filepath.Join("root", "b.txt")
		if _, ok := cm.GetPath(logicalNew); !ok {
			t.Fatalf("expected %q to be tracked after refresh", logicalNew)
		}
		rootEntry, ok := cm.GetPath("root")
		if !ok {
			t.Fatalf("expected directory owner to remain tracked")
		}
		if !containsString(rootEntry.Children, logicalNew) {
			t.Fatalf("expected children to include %q, got: %v", logicalNew, rootEntry.Children)
		}
	})

	t.Run("NonOwnerRefreshFails", func(t *testing.T) {
		base := t.TempDir()
		root := filepath.Join(base, "root")
		if err := os.MkdirAll(root, 0o755); err != nil {
			t.Fatalf("failed to create root directory: %v", err)
		}

		filePath := filepath.Join(root, "file.txt")
		if err := os.WriteFile(filePath, []byte("content"), 0o644); err != nil {
			t.Fatalf("failed to write file: %v", err)
		}

		cm, err := NewContextManager(base)
		if err != nil {
			t.Fatalf("NewContextManager failed: %v", err)
		}
		if err := cm.AddPath(root); err != nil {
			t.Fatalf("AddPath(directory) failed: %v", err)
		}

		err = cm.RefreshPath(filepath.Join("root", "file.txt"))
		if err == nil {
			t.Fatalf("expected refresh to fail for non-owner file")
		}
		if got := err.Error(); !strings.Contains(got, "not a tracked owner") {
			t.Fatalf("expected error mentioning tracked owner, got: %v", got)
		}
	})

	t.Run("DirectoryTrailingSlashNormalized", func(t *testing.T) {
		// When a directory is added via AddPath("root"), the canonical
		// owner key is "root". RefreshPath should accept "root/" (with
		// trailing separator) and normalize it to match.
		base := t.TempDir()
		root := filepath.Join(base, "root")
		if err := os.MkdirAll(root, 0o755); err != nil {
			t.Fatalf("failed to create root directory: %v", err)
		}
		if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("a"), 0o644); err != nil {
			t.Fatalf("failed to write initial file: %v", err)
		}

		cm, err := NewContextManager(base)
		if err != nil {
			t.Fatalf("NewContextManager failed: %v", err)
		}
		if err := cm.AddPath(root); err != nil {
			t.Fatalf("AddPath(directory) failed: %v", err)
		}

		// Add a new file on disk after the initial scan.
		if err := os.WriteFile(filepath.Join(root, "b.txt"), []byte("b"), 0o644); err != nil {
			t.Fatalf("failed to write new file: %v", err)
		}

		// Refresh with trailing separator — this is the variant the JS
		// layer passes when the user types "add root/".
		if err := cm.RefreshPath("root" + string(filepath.Separator)); err != nil {
			t.Fatalf("RefreshPath(root/) failed: %v", err)
		}

		logicalNew := filepath.Join("root", "b.txt")
		if _, ok := cm.GetPath(logicalNew); !ok {
			t.Fatalf("expected %q to be tracked after refresh with trailing slash", logicalNew)
		}
	})

	t.Run("DotSlashPrefixNormalized", func(t *testing.T) {
		// When AddPath is called with an absolute path, the owner key
		// is the relative form (e.g. "note.txt"). RefreshPath should
		// accept "./note.txt" and normalize it to "note.txt".
		base := t.TempDir()
		filePath := filepath.Join(base, "note.txt")
		if err := os.WriteFile(filePath, []byte("initial"), 0o644); err != nil {
			t.Fatalf("failed to write initial file: %v", err)
		}

		cm, err := NewContextManager(base)
		if err != nil {
			t.Fatalf("NewContextManager failed: %v", err)
		}
		if err := cm.AddPath(filePath); err != nil {
			t.Fatalf("AddPath(file) failed: %v", err)
		}

		// Update content on disk.
		if err := os.WriteFile(filePath, []byte("updated"), 0o644); err != nil {
			t.Fatalf("failed to update file: %v", err)
		}

		// Refresh with "./" prefix variant.
		if err := cm.RefreshPath("." + string(filepath.Separator) + "note.txt"); err != nil {
			t.Fatalf("RefreshPath(./note.txt) failed: %v", err)
		}

		cp, ok := cm.GetPath("note.txt")
		if !ok {
			t.Fatalf("expected note.txt to remain tracked after refresh")
		}
		if cp.Content != "updated" {
			t.Fatalf("expected refreshed content to be 'updated', got: %q", cp.Content)
		}
	})

	t.Run("UnknownPathStillFails", func(t *testing.T) {
		// Even with normalization, a genuinely untracked path must
		// still return an error.
		base := t.TempDir()
		cm, err := NewContextManager(base)
		if err != nil {
			t.Fatalf("NewContextManager failed: %v", err)
		}
		if err := cm.RefreshPath("does-not-exist"); err == nil {
			t.Fatalf("expected error for untracked path")
		}
	})
}

func TestContextManagerSymlinkHandling(t *testing.T) {
	t.Run("DirectorySymlinkIncluded", func(t *testing.T) {
		base := t.TempDir()
		root := filepath.Join(base, "root")
		target := filepath.Join(base, "target")
		if err := os.MkdirAll(filepath.Join(target, "nested"), 0o755); err != nil {
			t.Fatalf("failed to create target directory: %v", err)
		}
		if err := os.MkdirAll(root, 0o755); err != nil {
			t.Fatalf("failed to create root directory: %v", err)
		}

		targetFile := filepath.Join(target, "nested", "file.txt")
		if err := os.WriteFile(targetFile, []byte("target"), 0o644); err != nil {
			t.Fatalf("failed to write target file: %v", err)
		}

		linkDir := filepath.Join(root, "linkdir")
		if err := os.Symlink(target, linkDir); err != nil {
			if runtime.GOOS == "windows" {
				t.Skipf("symlink creation not supported: %v", err)
			}
			t.Fatalf("failed to create directory symlink: %v", err)
		}

		cm, err := NewContextManager(base)
		if err != nil {
			t.Fatalf("NewContextManager failed: %v", err)
		}
		if err := cm.AddPath(root); err != nil {
			t.Fatalf("AddPath(root) failed: %v", err)
		}

		logical := filepath.Join("root", "linkdir", "nested", "file.txt")
		if _, ok := cm.GetPath(logical); !ok {
			t.Fatalf("expected %q to be tracked via directory symlink", logical)
		}
	})

	t.Run("SymlinkLoopIsSafe", func(t *testing.T) {
		base := t.TempDir()
		dirA := filepath.Join(base, "a")
		dirB := filepath.Join(base, "b")
		if err := os.MkdirAll(dirA, 0o755); err != nil {
			t.Fatalf("failed to create dirA: %v", err)
		}
		if err := os.MkdirAll(dirB, 0o755); err != nil {
			t.Fatalf("failed to create dirB: %v", err)
		}

		fileA := filepath.Join(dirA, "a.txt")
		fileB := filepath.Join(dirB, "b.txt")
		if err := os.WriteFile(fileA, []byte("a"), 0o644); err != nil {
			t.Fatalf("failed to write fileA: %v", err)
		}
		if err := os.WriteFile(fileB, []byte("b"), 0o644); err != nil {
			t.Fatalf("failed to write fileB: %v", err)
		}

		linkAB := filepath.Join(dirA, "to-b")
		if err := os.Symlink(dirB, linkAB); err != nil {
			if runtime.GOOS == "windows" {
				t.Skipf("symlink creation not supported: %v", err)
			}
			t.Fatalf("failed to create symlink to dirB: %v", err)
		}
		linkBA := filepath.Join(dirB, "to-a")
		if err := os.Symlink(dirA, linkBA); err != nil {
			if runtime.GOOS == "windows" {
				t.Skipf("symlink creation not supported: %v", err)
			}
			t.Fatalf("failed to create symlink to dirA: %v", err)
		}

		cm, err := NewContextManager(base)
		if err != nil {
			t.Fatalf("NewContextManager failed: %v", err)
		}
		if err := cm.AddPath(dirA); err != nil {
			t.Fatalf("AddPath(dirA) failed: %v", err)
		}

		paths := cm.ListPaths()
		logicalA := filepath.Join("a", "a.txt")
		logicalB := filepath.Join("a", "to-b", "b.txt")
		if !containsString(paths, logicalA) {
			t.Fatalf("expected to track %q", logicalA)
		}
		if !containsString(paths, logicalB) {
			t.Fatalf("expected to track %q via symlink", logicalB)
		}
		unexpected := filepath.Join("a", "to-b", "to-a", "a.txt")
		if containsString(paths, unexpected) {
			t.Fatalf("unexpected loop traversal detected for %q", unexpected)
		}
	})
}

func containsString(values []string, target string) bool {
	for _, v := range values {
		if v == target {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// tilde expansion integration tests for ContextManager
// ---------------------------------------------------------------------------

// TestAddPathExpandsTilde verifies that ContextManager.AddPath correctly
// expands tilde paths and tracks files with their expanded absolute paths.
// This is an integration test that verifies the end-to-end behavior of
// tilde expansion in the context of path management.
func TestAddPathExpandsTilde(t *testing.T) {
	// Set up a fake HOME directory for hermetic test execution
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	t.Setenv("USERPROFILE", fakeHome)

	// Verify that os.UserHomeDir returns our fake home
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("os.UserHomeDir failed: %v", err)
	}
	if home != fakeHome {
		t.Fatalf("os.UserHomeDir returned %q, want %q", home, fakeHome)
	}

	// Create a test file in the fake home directory
	testDir := filepath.Join(fakeHome, "testsub")
	if err := os.MkdirAll(testDir, 0o755); err != nil {
		t.Fatalf("failed to create fake home subdir: %v", err)
	}
	tildeFile := filepath.Join(testDir, "tilde_file.txt")
	if err := os.WriteFile(tildeFile, []byte("tilde content"), 0o644); err != nil {
		t.Fatalf("failed to write tilde test file: %v", err)
	}

	cm, err := NewContextManager(fakeHome)
	if err != nil {
		t.Fatalf("NewContextManager failed: %v", err)
	}

	// Add path using tilde notation
	// The ContextManager should expand this to the fake home directory
	if err := cm.AddPath("~/testsub/tilde_file.txt"); err != nil {
		t.Fatalf("AddPath with tilde path failed: %v", err)
	}

	// The file should be tracked with its relative logical path
	logical := filepath.Join("testsub", "tilde_file.txt")
	cp, ok := cm.GetPath(logical)
	if !ok {
		t.Fatalf("expected %q to be tracked after AddPath with tilde, paths: %v", logical, cm.ListPaths())
	}
	if cp.Content != "tilde content" {
		t.Errorf("expected content %q, got %q", "tilde content", cp.Content)
	}

	// Verify the logical path is stored (not absolute path - this is by design)
	// ContextManager stores relative paths for portability
	if cp.Path != logical {
		t.Errorf("expected logical path %q, got: %q", logical, cp.Path)
	}
}

// TestRefreshPathExpandsTilde verifies that ContextManager.RefreshPath
// correctly handles tilde paths and updates the content from the expanded path.
// Uses an actual ~/tilde path (not a bare relative path) as the test name implies.
func TestRefreshPathExpandsTilde(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	t.Setenv("USERPROFILE", fakeHome)

	// Create a test file
	testDir := filepath.Join(fakeHome, "refresh_test")
	if err := os.MkdirAll(testDir, 0o755); err != nil {
		t.Fatalf("failed to create test dir: %v", err)
	}
	testFile := filepath.Join(testDir, "refresh.txt")
	if err := os.WriteFile(testFile, []byte("initial content"), 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	cm, err := NewContextManager(fakeHome)
	if err != nil {
		t.Fatalf("NewContextManager failed: %v", err)
	}

	// Add the file with its absolute path first
	if err := cm.AddPath(testFile); err != nil {
		t.Fatalf("AddPath failed: %v", err)
	}

	// Update the file on disk
	if err := os.WriteFile(testFile, []byte("updated content"), 0o644); err != nil {
		t.Fatalf("failed to update test file: %v", err)
	}

	// Refresh using an ACTUAL tilde path — the test name says "ExpandsTilde"
	tildePath := "~/refresh_test/refresh.txt"
	if err := cm.RefreshPath(tildePath); err != nil {
		t.Fatalf("RefreshPath(%q) failed: %v", tildePath, err)
	}

	// Verify content was updated (logical path is relative to basePath)
	logical := filepath.Join("refresh_test", "refresh.txt")
	cp, ok := cm.GetPath(logical)
	if !ok {
		t.Fatalf("expected path %q to be tracked after refresh", logical)
	}
	if cp.Content != "updated content" {
		t.Errorf("expected updated content, got: %q", cp.Content)
	}
}

// TestRemovePathExpandsTilde verifies that ContextManager.RemovePath
// correctly normalizes and removes paths specified with tilde notation.
// Uses an actual ~/tilde path (not a bare basename) as the test name implies.
func TestRemovePathExpandsTilde(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	t.Setenv("USERPROFILE", fakeHome)

	// Create a test file
	testFile := filepath.Join(fakeHome, "remove_test.txt")
	if err := os.WriteFile(testFile, []byte("content"), 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	cm, err := NewContextManager(fakeHome)
	if err != nil {
		t.Fatalf("NewContextManager failed: %v", err)
	}

	// Add with absolute path
	if err := cm.AddPath(testFile); err != nil {
		t.Fatalf("AddPath failed: %v", err)
	}

	// Verify it was added
	logical := "remove_test.txt"
	if _, ok := cm.GetPath(logical); !ok {
		t.Fatalf("expected file to be tracked before removal")
	}

	// Remove using an ACTUAL tilde path — the test name says "ExpandsTilde"
	tildePath := "~/remove_test.txt"
	if err := cm.RemovePath(tildePath); err != nil {
		t.Fatalf("RemovePath(%q) failed: %v", tildePath, err)
	}

	// Verify it was removed
	if _, ok := cm.GetPath(logical); ok {
		t.Errorf("expected file to be removed after RemovePath(%q)", tildePath)
	}
}

// TestTildePathRoundTrip verifies the complete round-trip behavior:
// Add with tilde → retrieve → verify content matches.
func TestTildePathRoundTrip(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	t.Setenv("USERPROFILE", fakeHome)

	// Create a test file with nested directories
	nestedDir := filepath.Join(fakeHome, "level1", "level2", "level3")
	if err := os.MkdirAll(nestedDir, 0o755); err != nil {
		t.Fatalf("failed to create nested dirs: %v", err)
	}
	testFile := filepath.Join(nestedDir, "roundtrip.txt")
	expectedContent := "round-trip test content with unicode: ñ, é, 中文"
	if err := os.WriteFile(testFile, []byte(expectedContent), 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	cm, err := NewContextManager(fakeHome)
	if err != nil {
		t.Fatalf("NewContextManager failed: %v", err)
	}

	// Add using tilde path with nested directories
	if err := cm.AddPath("~/level1/level2/level3/roundtrip.txt"); err != nil {
		t.Fatalf("AddPath with nested tilde path failed: %v", err)
	}

	// Retrieve the file
	logical := filepath.Join("level1", "level2", "level3", "roundtrip.txt")
	cp, ok := cm.GetPath(logical)
	if !ok {
		t.Fatalf("expected file to be tracked, got paths: %v", cm.ListPaths())
	}

	// Verify round-trip: content should match exactly
	if cp.Content != expectedContent {
		t.Errorf("round-trip content mismatch: got %q, want %q", cp.Content, expectedContent)
	}

	// Verify the logical path is stored (ContextManager stores relative paths)
	if cp.Path != logical {
		t.Errorf("expected logical path %q, got: %q", logical, cp.Path)
	}

	// Verify txtar round-trip
	txtarString := cm.GetTxtarString()
	if !strings.Contains(txtarString, expectedContent) {
		t.Errorf("expected txtar to contain content, got: %s", txtarString)
	}
	if !strings.Contains(txtarString, "-- level1/level2/level3/roundtrip.txt --") {
		t.Errorf("expected txtar header to contain logical path, got: %s", txtarString)
	}
}

// TestGetPath_ResolvesTildeLabelFromAddRelativePath verifies that GetPath
// resolves tilde-form labels returned by AddRelativePath. This is a TDD
// regression test for a critical API disconnect: AddRelativePath("~/foo.txt")
// returns "~/foo.txt" as the TUI label, but GetPath("~/foo.txt") did a raw
// cm.paths["~/foo.txt"] lookup which always failed because the internal owner
// key is a normalized relative/absolute path — never the tilde form.
//
// A TUI calling GetPath("~/foo.txt") would silently fail. The fix is for
// GetPath to use findOwnerFromUserPath, which expands tildes during lookup.
func TestGetPath_ResolvesTildeLabelFromAddRelativePath(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	t.Setenv("USERPROFILE", fakeHome)

	// Create a file under the fake home
	testFile := filepath.Join(fakeHome, "tracked_file.txt")
	if err := os.WriteFile(testFile, []byte("hello from tilde"), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	cm, err := NewContextManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewContextManager: %v", err)
	}

	// AddRelativePath with a tilde label
	tildeLabel := "~/tracked_file.txt"
	returnedLabel, err := cm.AddRelativePath(tildeLabel)
	if err != nil {
		t.Fatalf("AddRelativePath(%q): %v", tildeLabel, err)
	}
	if returnedLabel != tildeLabel {
		t.Fatalf("AddRelativePath returned %q, want %q", returnedLabel, tildeLabel)
	}

	// THE BUG: GetPath(tildeLabel) does raw map lookup and fails.
	// After the fix, it should resolve through findOwnerFromUserPath.
	cp, ok := cm.GetPath(tildeLabel)
	if !ok {
		t.Fatalf("GetPath(%q) returned false — TUI cannot retrieve paths added via AddRelativePath tilde label", tildeLabel)
	}
	if cp.Content != "hello from tilde" {
		t.Errorf("content mismatch: got %q, want %q", cp.Content, "hello from tilde")
	}

	// Also verify GetPath returns false for genuinely unknown paths
	_, ok = cm.GetPath("~/completely_unknown.txt")
	if ok {
		t.Error("GetPath should return false for untracked paths")
	}
}

// TestGetPath_ResolvesTildeLabelNestedPath tests GetPath with a more deeply
// nested tilde label to ensure the resolution pipeline works for multi-level
// relative paths under the home directory.
func TestGetPath_ResolvesTildeLabelNestedPath(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	t.Setenv("USERPROFILE", fakeHome)

	nestedDir := filepath.Join(fakeHome, ".config", "osm")
	if err := os.MkdirAll(nestedDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	testFile := filepath.Join(nestedDir, "config.yaml")
	if err := os.WriteFile(testFile, []byte("key: value"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	cm, err := NewContextManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewContextManager: %v", err)
	}

	tildeLabel := "~/.config/osm/config.yaml"
	returnedLabel, err := cm.AddRelativePath(tildeLabel)
	if err != nil {
		t.Fatalf("AddRelativePath(%q): %v", tildeLabel, err)
	}
	if returnedLabel != tildeLabel {
		t.Fatalf("returned label %q, want %q", returnedLabel, tildeLabel)
	}

	cp, ok := cm.GetPath(tildeLabel)
	if !ok {
		t.Fatalf("GetPath(%q) returned false for nested tilde label", tildeLabel)
	}
	if cp.Content != "key: value" {
		t.Errorf("content: got %q, want %q", cp.Content, "key: value")
	}
}

// TestCrossPlatformTildeRehydrationRoundTrip verifies that backslash tilde
// labels (e.g., "~\docs\notes.txt") round-trip correctly through
// AddRelativePath → RemovePath → AddRelativePath → RefreshPath on POSIX.
//
// The findOwnerFromUserPath function (step 3) uses expandTildeOwnerLabel
// (cross-platform) so that Windows-style backslash tilde forms are correctly
// expanded on all hosts during RemovePath/RefreshPath. canonicalizeUserPath
// uses host-specific tilde expansion (filepathutil.ExpandTilde) which is
// correct for AddPath but too narrow for session rehydration flows. Without
// cross-platform expansion in findOwnerFromUserPath, RemovePath and RefreshPath
// with backslash tilde labels would fail on POSIX hosts.
func TestCrossPlatformTildeRehydrationRoundTrip(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX-only test; verifies backslash-tilde expansion on non-Windows hosts")
	}

	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	t.Setenv("USERPROFILE", fakeHome)

	// Create file at ~/docs/notes.txt
	docsDir := filepath.Join(fakeHome, "docs")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	testFile := filepath.Join(docsDir, "notes.txt")
	if err := os.WriteFile(testFile, []byte("original content"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	cm, err := NewContextManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewContextManager: %v", err)
	}

	// Step 1: Add using Windows-style backslash tilde label
	backslashLabel := `~\docs\notes.txt`
	returnedLabel, err := cm.AddRelativePath(backslashLabel)
	if err != nil {
		t.Fatalf("AddRelativePath(%q): %v", backslashLabel, err)
	}
	if returnedLabel != backslashLabel {
		t.Errorf("AddRelativePath returned %q, want %q (backslash form preserved)", returnedLabel, backslashLabel)
	}

	// Step 2: Verify it's tracked via GetPath with forward-slash tilde label
	// (findOwnerFromUserPath should resolve either form)
	cp, ok := cm.GetPath("~/docs/notes.txt")
	if !ok {
		t.Fatalf("GetPath(\"~/docs/notes.txt\") returned false after AddRelativePath with backslash label")
	}
	if cp.Content != "original content" {
		t.Errorf("content: got %q, want %q", cp.Content, "original content")
	}

	// Step 3: RemovePath with the same backslash label
	err = cm.RemovePath(backslashLabel)
	if err != nil {
		t.Fatalf("RemovePath(%q): %v — backslash tilde label must be removable on POSIX", backslashLabel, err)
	}
	_, ok = cm.GetPath("~/docs/notes.txt")
	if ok {
		t.Error("path should be removed after RemovePath")
	}

	// Step 4: Re-add and RefreshPath with backslash label
	_, err = cm.AddRelativePath(backslashLabel)
	if err != nil {
		t.Fatalf("re-AddRelativePath(%q): %v", backslashLabel, err)
	}

	// Modify file on disk
	if err := os.WriteFile(testFile, []byte("refreshed content"), 0o644); err != nil {
		t.Fatalf("write updated: %v", err)
	}

	err = cm.RefreshPath(backslashLabel)
	if err != nil {
		t.Fatalf("RefreshPath(%q): %v — backslash tilde label must be refreshable on POSIX", backslashLabel, err)
	}

	cp, ok = cm.GetPath("~/docs/notes.txt")
	if !ok {
		t.Fatalf("GetPath after refresh failed")
	}
	if cp.Content != "refreshed content" {
		t.Errorf("content after refresh: got %q, want %q", cp.Content, "refreshed content")
	}
}

// ---------------------------------------------------------------------------
// Regression tests for review issues (scratch/review.md)
// ---------------------------------------------------------------------------

// TestGetPath_TrackedChildUnderTrackedDirectory verifies that GetPath resolves
// child files under tracked directories. This is the core regression from Issue 1:
// GetPath was changed to use findOwnerFromUserPath (which only checks ownerFiles),
// but child files are stored in cm.paths, NOT cm.ownerFiles. After AddPath("root")
// where root is a directory, GetPath("root/sub/child.txt") must return the tracked
// child — not (nil, false).
func TestGetPath_TrackedChildUnderTrackedDirectory(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "root")
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(root, "sub", "child.txt")
	if err := os.WriteFile(child, []byte("child content"), 0o644); err != nil {
		t.Fatal(err)
	}

	cm, err := NewContextManager(base)
	if err != nil {
		t.Fatal(err)
	}
	if err := cm.AddPath(root); err != nil {
		t.Fatalf("AddPath(root): %v", err)
	}

	// THE REGRESSION: GetPath must resolve tracked child paths, not just owners.
	logical := filepath.Join("root", "sub", "child.txt")
	cp, ok := cm.GetPath(logical)
	if !ok {
		t.Fatalf("GetPath(%q) returned false for tracked child under directory; paths: %v", logical, cm.ListPaths())
	}
	if cp.Content != "child content" {
		t.Errorf("content: got %q, want %q", cp.Content, "child content")
	}
	if cp.Type != "file" {
		t.Errorf("type: got %q, want %q", cp.Type, "file")
	}

	// Also verify the directory owner itself is still retrievable
	dirCP, dirOK := cm.GetPath("root")
	if !dirOK {
		t.Fatalf("GetPath(\"root\") returned false for directory owner; paths: %v", cm.ListPaths())
	}
	if dirCP.Type != "directory" {
		t.Errorf("directory type: got %q, want %q", dirCP.Type, "directory")
	}
}

// TestGetPath_DeeplyNestedChildUnderDirectory verifies that GetPath resolves
// deeply nested child paths (e.g., root/a/b/c/deep.txt) after tracking a directory.
// Multi-level nesting stress test for the tracked-path resolver.
func TestGetPath_DeeplyNestedChildUnderDirectory(t *testing.T) {
	base := t.TempDir()
	nested := filepath.Join(base, "root", "a", "b", "c")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	deepFile := filepath.Join(nested, "deep.txt")
	if err := os.WriteFile(deepFile, []byte("deep content"), 0o644); err != nil {
		t.Fatal(err)
	}

	cm, err := NewContextManager(base)
	if err != nil {
		t.Fatal(err)
	}
	if err := cm.AddPath(filepath.Join(base, "root")); err != nil {
		t.Fatalf("AddPath(root): %v", err)
	}

	logical := filepath.Join("root", "a", "b", "c", "deep.txt")
	cp, ok := cm.GetPath(logical)
	if !ok {
		t.Fatalf("GetPath(%q) returned false; paths: %v", logical, cm.ListPaths())
	}
	if cp.Content != "deep content" {
		t.Errorf("content: got %q, want %q", cp.Content, "deep content")
	}
}

// TestCanonicalizeUserPath_PosixBackslashNotExpanded verifies that on POSIX,
// canonicalizeUserPath does NOT expand ~\foo to $HOME/foo. The tilde expansion
// must be host-specific (filepathutil.ExpandTilde), not cross-platform
// (expandTildeOwnerLabel). On POSIX, ~\foo is a literal relative path, not a
// tilde expansion form.
func TestCanonicalizeUserPath_PosixBackslashNotExpanded(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX-only test")
	}

	base := t.TempDir()
	cm, err := NewContextManager(base)
	if err != nil {
		t.Fatal(err)
	}

	// On POSIX, ~\foo should NOT be expanded as a tilde path.
	// It should be treated as a literal relative path.
	owner, absPath, err := cm.canonicalizeUserPath(`~\foo`)
	if err != nil {
		t.Fatalf("canonicalizeUserPath(`~\\foo`): %v", err)
	}

	// The absolute path must NOT be $HOME/foo (tilde expansion).
	// On POSIX, filepathutil.ExpandTilde does NOT treat ~\foo as a tilde
	// form, so it should resolve relative to CWD, not via tilde expansion.
	home, _ := os.UserHomeDir()
	expectedTildeExpansion := filepath.Join(home, "foo")
	if absPath == expectedTildeExpansion {
		t.Errorf("canonicalizeUserPath(`~\\foo`) resolved to %q via tilde expansion; should be CWD-relative literal on POSIX", absPath)
	}

	// The owner should NOT be "foo" (which would indicate tilde expansion
	// stripped the ~ prefix and resolved relative to basePath root).
	if owner == "foo" {
		t.Errorf("owner is %q, indicating ~\\foo was tilde-expanded; should preserve literal ~\\foo characters", owner)
	}
}

// TestAddPath_LiteralBackslashTildeOnPOSIX verifies that on POSIX, AddPath
// with a relative ~\ input (e.g., ~\docs/notes.txt from cwd) tracks it as a
// literal path, not as $HOME/... . This is the end-to-end test for Issue 2:
// AddPath uses canonicalizeUserPath which must use host-specific tilde
// expansion, so ~\foo on POSIX is treated as a literal relative path.
func TestAddPath_LiteralBackslashTildeOnPOSIX(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX-only test")
	}

	base := t.TempDir()
	cwd := t.TempDir()

	// Create a literal directory "~\docs" under cwd (not base).
	literalDir := cwd + string(filepath.Separator) + `~\docs`
	if err := os.MkdirAll(literalDir, 0o755); err != nil {
		t.Skipf("filesystem does not support backslash in directory name: %v", err)
	}
	testFile := filepath.Join(literalDir, "notes.txt")
	if err := os.WriteFile(testFile, []byte("literal notes"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Save and restore original CWD.
	origCWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(origCWD) }()

	// Change CWD to cwd so that AddPath resolves ~\docs relative to it.
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}

	cm, err := NewContextManager(base)
	if err != nil {
		t.Fatal(err)
	}

	// AddPath with the RELATIVE ~\docs/notes.txt input (not absolute).
	// On POSIX, canonicalizeUserPath uses host-specific ExpandTilde which
	// does NOT treat ~\ as tilde expansion, so it resolves as a literal
	// path relative to CWD.
	relInput := filepath.Join(`~\docs`, "notes.txt")
	if err := cm.AddPath(relInput); err != nil {
		t.Fatalf("AddPath(%q): %v", relInput, err)
	}

	// Verify the file is tracked under the literal-backslash path.
	cp, ok := cm.GetPath(relInput)
	if !ok {
		t.Fatalf("GetPath(%q) returned false after AddPath; paths: %v", relInput, cm.ListPaths())
	}
	if cp.Content != "literal notes" {
		t.Errorf("content: got %q, want %q", cp.Content, "literal notes")
	}

	// Verify the logical path contains the literal ~\docs prefix.
	paths := cm.ListPaths()
	var found bool
	for _, p := range paths {
		if filepath.Base(p) == "notes.txt" {
			found = true
			if !strings.Contains(p, `~\docs`) {
				t.Errorf("logical path %q should contain literal ~\\docs prefix", p)
			}
			break
		}
	}
	if !found {
		t.Fatalf("notes.txt not found in tracked paths: %v", paths)
	}
}

// TestGetPath_StillResolvesTildeLabels ensures the GetPath fix (multi-step
// resolution against cm.paths) doesn't break the existing tilde-label
// resolution that was added for AddRelativePath. After AddRelativePath("~/foo.txt"),
// GetPath("~/foo.txt") must still work via the owner fallback step.
func TestGetPath_StillResolvesTildeLabels(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	t.Setenv("USERPROFILE", fakeHome)

	testFile := filepath.Join(fakeHome, "tilde_test.txt")
	if err := os.WriteFile(testFile, []byte("tilde hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	cm, err := NewContextManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	// Add via AddRelativePath with tilde label (TUI rehydration scenario)
	label, err := cm.AddRelativePath("~/tilde_test.txt")
	if err != nil {
		t.Fatalf("AddRelativePath: %v", err)
	}
	if label != "~/tilde_test.txt" {
		t.Fatalf("label: got %q, want ~/tilde_test.txt", label)
	}

	// GetPath with tilde label must resolve through owner fallback
	cp, ok := cm.GetPath("~/tilde_test.txt")
	if !ok {
		t.Fatalf("GetPath(\"~/tilde_test.txt\") returned false; paths: %v", cm.ListPaths())
	}
	if cp.Content != "tilde hello" {
		t.Errorf("content: got %q, want %q", cp.Content, "tilde hello")
	}

	// Also verify GetPath returns false for genuinely unknown tilde paths
	_, unknownOK := cm.GetPath("~/nonexistent.txt")
	if unknownOK {
		t.Error("GetPath should return false for untracked tilde paths")
	}
}

// ---------------------------------------------------------------------------
// Regression tests for AddPath/RemovePath/RefreshPath symmetry with ~\ paths
// on POSIX, and AddRelativePath literal-first resolution.
// ---------------------------------------------------------------------------

// TestRemovePath_LiteralBackslashTildeRelativeOnPOSIX verifies that on POSIX,
// RemovePath with a relative ~\ input is symmetric with AddPath. When a file
// is added via AddPath with a relative ~\ path (e.g., ~\docs/notes.txt from
// cwd), RemovePath with the same input must remove the same tracked owner —
// not misinterpret ~\ as a home-relative path.
//
// This test MUST fail against current code because findOwnerFromUserPath
// step 3 uses expandTildeOwnerLabel which reinterprets ~\docs as $HOME/docs,
// producing a different owner key than what AddPath stored.
func TestRemovePath_LiteralBackslashTildeRelativeOnPOSIX(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX-only test")
	}

	base := t.TempDir()
	cwd := t.TempDir()

	// Create a literal directory "~\docs" under cwd (not base).
	literalDir := cwd + string(filepath.Separator) + `~\docs`
	if err := os.MkdirAll(literalDir, 0o755); err != nil {
		t.Skipf("filesystem does not support backslash in directory name: %v", err)
	}
	testFile := filepath.Join(literalDir, "notes.txt")
	if err := os.WriteFile(testFile, []byte("literal notes"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Save and restore original CWD.
	origCWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(origCWD) }()

	// Change CWD to cwd so that AddPath resolves ~\docs relative to it.
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}

	cm, err := NewContextManager(base)
	if err != nil {
		t.Fatal(err)
	}

	// AddPath with the relative ~\docs/notes.txt input.
	// On POSIX, canonicalizeUserPath uses host-specific ExpandTilde which
	// does NOT treat ~\ as tilde expansion, so it resolves as a literal
	// path relative to CWD.
	relInput := filepath.Join(`~\docs`, "notes.txt")
	if err := cm.AddPath(relInput); err != nil {
		t.Fatalf("AddPath(%q): %v", relInput, err)
	}

	// Verify the file is tracked.
	cp, ok := cm.GetPath(relInput)
	if !ok {
		t.Fatalf("GetPath(%q) returned false after AddPath; paths: %v", relInput, cm.ListPaths())
	}
	if cp.Content != "literal notes" {
		t.Errorf("content: got %q, want %q", cp.Content, "literal notes")
	}

	// NOW: RemovePath with the SAME relative input.
	// This MUST find the same owner and remove it.
	// BUG: findOwnerFromUserPath step 3 uses expandTildeOwnerLabel which
	// reinterprets ~\docs as $HOME/docs, finding no match (or worse,
	// matching a different owner if one exists).
	if err := cm.RemovePath(relInput); err != nil {
		t.Fatalf("RemovePath(%q): %v", relInput, err)
	}

	// Verify the file is no longer tracked.
	_, ok = cm.GetPath(relInput)
	if ok {
		t.Fatalf("GetPath(%q) still returns true after RemovePath — RemovePath targeted the wrong owner", relInput)
	}
}

// TestRefreshPath_LiteralBackslashTildeRelativeOnPOSIX verifies that on POSIX,
// RefreshPath with a relative ~\ input is symmetric with AddPath. Same setup
// as TestRemovePath_LiteralBackslashTildeRelativeOnPOSIX but tests RefreshPath.
//
// This test MUST fail against current code for the same reason:
// findOwnerFromUserPath step 3 misinterprets the literal ~\ path.
func TestRefreshPath_LiteralBackslashTildeRelativeOnPOSIX(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX-only test")
	}

	base := t.TempDir()
	cwd := t.TempDir()

	// Create a literal directory "~\docs" under cwd.
	literalDir := cwd + string(filepath.Separator) + `~\docs`
	if err := os.MkdirAll(literalDir, 0o755); err != nil {
		t.Skipf("filesystem does not support backslash in directory name: %v", err)
	}
	testFile := filepath.Join(literalDir, "notes.txt")
	if err := os.WriteFile(testFile, []byte("original content"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Save and restore original CWD.
	origCWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(origCWD) }()

	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}

	cm, err := NewContextManager(base)
	if err != nil {
		t.Fatal(err)
	}

	relInput := filepath.Join(`~\docs`, "notes.txt")
	if err := cm.AddPath(relInput); err != nil {
		t.Fatalf("AddPath(%q): %v", relInput, err)
	}

	// Modify the file on disk.
	if err := os.WriteFile(testFile, []byte("updated content"), 0o644); err != nil {
		t.Fatalf("write updated: %v", err)
	}

	// RefreshPath with the same relative input.
	// BUG: findOwnerFromUserPath step 3 misinterprets ~\docs as $HOME/docs.
	if err := cm.RefreshPath(relInput); err != nil {
		t.Fatalf("RefreshPath(%q): %v", relInput, err)
	}

	// Verify content was updated.
	cp, ok := cm.GetPath(relInput)
	if !ok {
		t.Fatalf("GetPath(%q) returned false after RefreshPath; paths: %v", relInput, cm.ListPaths())
	}
	if cp.Content != "updated content" {
		t.Errorf("content after refresh: got %q, want %q", cp.Content, "updated content")
	}
}

// TestRemovePath_LiteralVsHomeBackslashTildeNoCollision verifies that on POSIX,
// when both a literal ~\ entry (added via AddPath from cwd) and a home-backed
// ~\ entry (added via AddRelativePath with HOME set to a fake home) exist,
// RemovePath with the ~\ input targets the literal entry, NOT the home-backed one.
//
// This test MUST fail against current code because findOwnerFromUserPath
// step 3 may resolve to the home-backed owner instead of the literal one.
func TestRemovePath_LiteralVsHomeBackslashTildeNoCollision(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX-only test")
	}

	fakeHome := t.TempDir()
	base := t.TempDir()
	cwd := t.TempDir()

	// Create a file at <fakeHome>/docs/notes.txt (home-backed).
	homeDocsDir := filepath.Join(fakeHome, "docs")
	if err := os.MkdirAll(homeDocsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	homeFile := filepath.Join(homeDocsDir, "notes.txt")
	if err := os.WriteFile(homeFile, []byte("home-backed content"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create a literal directory "~\docs" under cwd.
	literalDir := cwd + string(filepath.Separator) + `~\docs`
	if err := os.MkdirAll(literalDir, 0o755); err != nil {
		t.Skipf("filesystem does not support backslash in directory name: %v", err)
	}
	literalFile := filepath.Join(literalDir, "notes.txt")
	if err := os.WriteFile(literalFile, []byte("literal content"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Save and restore original CWD.
	origCWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(origCWD) }()

	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}

	// Set HOME to fakeHome for the AddRelativePath call.
	t.Setenv("HOME", fakeHome)
	t.Setenv("USERPROFILE", fakeHome)

	cm, err := NewContextManager(base)
	if err != nil {
		t.Fatal(err)
	}

	// Step 1: AddPath with the relative ~\docs/notes.txt input.
	// On POSIX, this resolves to the literal path under cwd.
	relInput := filepath.Join(`~\docs`, "notes.txt")
	if err := cm.AddPath(relInput); err != nil {
		t.Fatalf("AddPath(%q): %v", relInput, err)
	}

	// Step 2: AddRelativePath with ~\docs\notes.txt.
	// This uses cross-platform tilde expansion, resolves to <fakeHome>/docs/notes.txt.
	backslashLabel := `~\docs\notes.txt`
	returnedLabel, err := cm.AddRelativePath(backslashLabel)
	if err != nil {
		t.Fatalf("AddRelativePath(%q): %v", backslashLabel, err)
	}
	if returnedLabel != backslashLabel {
		t.Errorf("AddRelativePath returned %q, want %q", returnedLabel, backslashLabel)
	}

	// Both should be tracked as separate entries.
	paths := cm.ListPaths()
	if len(paths) < 2 {
		t.Fatalf("expected at least 2 tracked paths, got %d: %v", len(paths), paths)
	}

	// Step 3: RemovePath with the ~\docs/notes.txt input (forward-slash form).
	// This MUST remove the literal one (the AddPath entry), NOT the home-backed one.
	if err := cm.RemovePath(relInput); err != nil {
		t.Fatalf("RemovePath(%q): %v", relInput, err)
	}

	// Step 4: Verify the home-backed entry is still tracked.
	homeLabel := "~/docs/notes.txt"
	cp, ok := cm.GetPath(homeLabel)
	if !ok {
		t.Fatalf("GetPath(%q) returned false — home-backed entry was incorrectly removed", homeLabel)
	}
	if cp.Content != "home-backed content" {
		t.Errorf("home-backed content: got %q, want %q", cp.Content, "home-backed content")
	}

	// Step 5: Verify the literal entry is no longer tracked.
	// Note: GetPath(relInput) may still return true because findOwnerFromUserPath
	// step 3 (cross-platform tilde fallback) resolves ~\docs/notes.txt to the
	// home-backed entry. Instead, verify the literal path is not in ListPaths.
	paths = cm.ListPaths()
	for _, p := range paths {
		if strings.Contains(p, `~\docs`) {
			t.Fatalf("literal path %q still in tracked paths after RemovePath: %v", p, paths)
		}
	}
}

// TestAddRelativePath_LiteralBackslashTildeOnPOSIX verifies that on POSIX,
// when a real file exists at <basePath>/~/docs/notes.txt (forward-slash,
// where "~" is a literal directory name), AddRelativePath with the
// Windows-style label "~\docs\notes.txt" tracks the literal file under
// basePath, NOT $HOME/docs/notes.txt.
//
// This tests the literal-first resolution: on POSIX, AddRelativePath first
// checks if the path exists literally under basePath (converting backslashes
// to forward slashes) before falling back to cross-platform tilde expansion.
//
// This test MUST fail against current code because AddRelativePath
// unconditionally expands ~\ via expandTildeOwnerLabel (cross-platform),
// which reinterprets ~\docs as $HOME/docs.
func TestAddRelativePath_LiteralBackslashTildeOnPOSIX(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX-only test")
	}

	base := t.TempDir()
	fakeHome := t.TempDir()

	// Create a literal directory "~" under base, with "docs/notes.txt" inside.
	// This simulates a project that has a directory literally named "~".
	// When the Windows-style label "~\docs\notes.txt" is converted to
	// forward slashes, it becomes "~/docs/notes.txt" which resolves to
	// <base>/~/docs/notes.txt.
	literalDir := filepath.Join(base, "~", "docs")
	if err := os.MkdirAll(literalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	literalFile := filepath.Join(literalDir, "notes.txt")
	if err := os.WriteFile(literalFile, []byte("literal notes content"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Set HOME to fakeHome (different from base).
	t.Setenv("HOME", fakeHome)
	t.Setenv("USERPROFILE", fakeHome)

	// Also create a file at <fakeHome>/docs/notes.txt to prove we're NOT
	// targeting the home-backed file.
	homeDocsDir := filepath.Join(fakeHome, "docs")
	if err := os.MkdirAll(homeDocsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	homeFile := filepath.Join(homeDocsDir, "notes.txt")
	if err := os.WriteFile(homeFile, []byte("home-backed content"), 0o644); err != nil {
		t.Fatal(err)
	}

	cm, err := NewContextManager(base)
	if err != nil {
		t.Fatal(err)
	}

	// AddRelativePath with the Windows-style backslash tilde label.
	// On POSIX, this should first check for the literal file at
	// <base>/~/docs/notes.txt (backslashes converted to forward slashes).
	// Since it exists, it should track the literal file.
	backslashLabel := `~\docs\notes.txt`
	returnedLabel, err := cm.AddRelativePath(backslashLabel)
	if err != nil {
		t.Fatalf("AddRelativePath(%q): %v", backslashLabel, err)
	}
	if returnedLabel != backslashLabel {
		t.Errorf("AddRelativePath returned %q, want %q", returnedLabel, backslashLabel)
	}

	// The file should be tracked. Verify it's the literal file, not the home-backed one.
	paths := cm.ListPaths()
	var foundLiteral bool
	for _, p := range paths {
		// The owner key should be ~/docs/notes.txt (relative to base)
		if p == filepath.Join("~", "docs", "notes.txt") {
			foundLiteral = true
			cp, ok := cm.GetPath(p)
			if !ok {
				t.Fatalf("GetPath(%q) returned false", p)
			}
			if cp.Content != "literal notes content" {
				t.Errorf("content: got %q, want %q (should be literal file, not home-backed)", cp.Content, "literal notes content")
			}
			break
		}
	}
	if !foundLiteral {
		t.Fatalf("no tracked path matches ~/docs/notes.txt; paths: %v — AddRelativePath targeted the wrong file (home-backed instead of literal)", paths)
	}
}
