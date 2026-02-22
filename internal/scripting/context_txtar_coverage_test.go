package scripting

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// ==========================================================================
// ToTxtar — tracked directories metadata in comment
// ==========================================================================

func TestToTxtar_TrackedDirectories(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Create a real file so ToTxtar can read it.
	fileDir := filepath.Join(dir, "src")
	if err := os.MkdirAll(fileDir, 0755); err != nil {
		t.Fatal(err)
	}
	filePath := filepath.Join(fileDir, "main.go")
	if err := os.WriteFile(filePath, []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cm, err := NewContextManager(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Add a regular file.
	if err := cm.AddPath(filePath); err != nil {
		t.Fatal(err)
	}

	// Manually add a tracked directory to exercise the directory branch.
	relDir := "tracked-dir"
	cm.paths[relDir] = &contextPath{
		Path:       relDir,
		Type:       "directory",
		Metadata:   map[string]string{},
		UpdateTime: time.Now(),
	}

	archive := cm.ToTxtar()
	comment := string(archive.Comment)

	// Comment must contain "tracked directories:" mentioning our dir.
	if !strings.Contains(comment, "tracked directories:") {
		t.Errorf("expected 'tracked directories:' in comment, got:\n%s", comment)
	}
	if !strings.Contains(comment, "tracked-dir/") {
		t.Errorf("expected 'tracked-dir/' in comment, got:\n%s", comment)
	}

	// We should still have the file in the archive.
	if len(archive.Files) != 1 {
		t.Fatalf("expected 1 file in archive, got %d", len(archive.Files))
	}
}

// ==========================================================================
// ToTxtar — file that cannot be read (silently skipped)
// ==========================================================================

func TestToTxtar_UnreadableFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	cm, err := NewContextManager(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Add a path that points to a non-existent file.
	cm.paths["ghost.go"] = &contextPath{
		Path:       "ghost.go",
		Type:       "file",
		Metadata:   map[string]string{},
		UpdateTime: time.Now(),
	}

	archive := cm.ToTxtar()

	// The unreadable file should be silently skipped.
	if len(archive.Files) != 0 {
		t.Errorf("expected 0 files (ghost skipped), got %d", len(archive.Files))
	}
}

// ==========================================================================
// ToTxtar — non-file, non-directory type (silently skipped)
// ==========================================================================

func TestToTxtar_UnknownType(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	cm, err := NewContextManager(dir)
	if err != nil {
		t.Fatal(err)
	}

	cm.paths["symlink-thing"] = &contextPath{
		Path:       "symlink-thing",
		Type:       "symlink", // neither "file" nor "directory"
		Metadata:   map[string]string{},
		UpdateTime: time.Now(),
	}

	archive := cm.ToTxtar()

	if len(archive.Files) != 0 {
		t.Errorf("expected 0 files for unknown type, got %d", len(archive.Files))
	}
}

// ==========================================================================
// ToTxtar — absolute path single file (no collision)
// ==========================================================================

func TestToTxtar_AbsolutePath_SingleFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Create a file at an absolute path.
	absFile := filepath.Join(dir, "absolute.go")
	if err := os.WriteFile(absFile, []byte("package abs\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cm, err := NewContextManager(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Add with absolute path in the context path struct.
	cm.paths[absFile] = &contextPath{
		Path:       absFile,
		Type:       "file",
		Metadata:   map[string]string{},
		UpdateTime: time.Now(),
	}

	archive := cm.ToTxtar()

	if len(archive.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(archive.Files))
	}
	// Single absolute path with no collision should use basename only.
	if archive.Files[0].Name != "absolute.go" {
		t.Errorf("expected basename 'absolute.go', got %q", archive.Files[0].Name)
	}
}

// ==========================================================================
// ToTxtar — absolute path collision (suffix expansion)
// ==========================================================================

func TestToTxtar_AbsolutePath_Collision(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Create two files with the same basename in different directories.
	dir1 := filepath.Join(dir, "pkg1")
	dir2 := filepath.Join(dir, "pkg2")
	if err := os.MkdirAll(dir1, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir2, 0755); err != nil {
		t.Fatal(err)
	}
	file1 := filepath.Join(dir1, "handler.go")
	file2 := filepath.Join(dir2, "handler.go")
	if err := os.WriteFile(file1, []byte("pkg1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file2, []byte("pkg2\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cm, err := NewContextManager(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Add as absolute paths.
	cm.paths[file1] = &contextPath{
		Path:       file1,
		Type:       "file",
		Metadata:   map[string]string{},
		UpdateTime: time.Now(),
	}
	cm.paths[file2] = &contextPath{
		Path:       file2,
		Type:       "file",
		Metadata:   map[string]string{},
		UpdateTime: time.Now(),
	}

	archive := cm.ToTxtar()

	if len(archive.Files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(archive.Files))
	}

	// Both should be in the archive with disambiguated names.
	names := map[string]bool{}
	for _, f := range archive.Files {
		names[f.Name] = true
	}

	// They should be unique.
	if len(names) != 2 {
		t.Errorf("expected 2 unique names, got %d: %v", len(names), names)
	}

	// The names should contain directory parts for disambiguation.
	for name := range names {
		if name == "handler.go" {
			t.Errorf("expected disambiguated name, got bare 'handler.go'")
		}
	}
}

// ==========================================================================
// ToTxtar — empty context (no files, no directories)
// ==========================================================================

func TestToTxtar_EmptyContext(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	cm, err := NewContextManager(dir)
	if err != nil {
		t.Fatal(err)
	}

	archive := cm.ToTxtar()

	if len(archive.Files) != 0 {
		t.Errorf("expected 0 files, got %d", len(archive.Files))
	}
	comment := string(archive.Comment)
	if !strings.Contains(comment, "context root:") {
		t.Errorf("comment should contain 'context root:', got:\n%s", comment)
	}
}

// ==========================================================================
// isRelativeOrAbsolutePath edge cases
// ==========================================================================

func TestIsRelativeOrAbsolutePath(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  bool
	}{
		// Relative paths
		{".", true},
		{"..", true},
		{"./foo", true},
		{"../bar", true},
		{"./", true},
		{"../", true},

		// Bare module names — not paths
		{"express", false},
		{"my-module", false},
		{"@scope/pkg", false},
		{"lodash", false},

		// Edge cases
		{".hidden", false}, // starts with "." but is not "./" or "."
		{"..foo", false},   // starts with ".." but is not "../" or ".."
	}

	// Absolute paths depend on the platform.
	if runtime.GOOS == "windows" {
		tests = append(tests,
			struct {
				input string
				want  bool
			}{`C:\Windows`, true},
			struct {
				input string
				want  bool
			}{`C:\`, true},
		)
	} else {
		tests = append(tests,
			struct {
				input string
				want  bool
			}{"/usr/bin", true},
			struct {
				input string
				want  bool
			}{"/", true},
		)
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			got := isRelativeOrAbsolutePath(tc.input)
			if got != tc.want {
				t.Errorf("isRelativeOrAbsolutePath(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}
