package scripting

import (
	"os"
	"path/filepath"
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

		cm := NewContextManager(dir)
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

		cm := NewContextManager(baseDir)

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

		cm := NewContextManager(dir)
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
}
