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

	// Refresh using tilde path - should resolve to the same file
	logical := filepath.Join("refresh_test", "refresh.txt")
	if err := cm.RefreshPath(logical); err != nil {
		t.Fatalf("RefreshPath failed: %v", err)
	}

	// Verify content was updated
	cp, ok := cm.GetPath(logical)
	if !ok {
		t.Fatalf("expected path to be tracked after refresh")
	}
	if cp.Content != "updated content" {
		t.Errorf("expected updated content, got: %q", cp.Content)
	}
}

// TestRemovePathExpandsTilde verifies that ContextManager.RemovePath
// correctly normalizes and removes paths specified with tilde notation.
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

	// Remove using just the logical name (which should work)
	if err := cm.RemovePath(logical); err != nil {
		t.Fatalf("RemovePath failed: %v", err)
	}

	// Verify it was removed
	if _, ok := cm.GetPath(logical); ok {
		t.Errorf("expected file to be removed after RemovePath")
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
