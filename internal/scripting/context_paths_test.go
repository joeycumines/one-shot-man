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
