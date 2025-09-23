package scripting

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDebugPathBehavior - debug test to understand the path issue
func TestDebugPathBehavior(t *testing.T) {
	// Create a temporary directory structure to test with
	dir := t.TempDir()

	// Create some test files
	mustWrite := func(p, content string) {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	f1 := filepath.Join(dir, "src", "main.go")
	f2 := filepath.Join(dir, "docs", "README.md")
	mustWrite(f1, "package main\n\nfunc main() {}\n")
	mustWrite(f2, "# Project README\n")

	// Test with different ways of adding paths
	t.Logf("=== Testing path addition behavior ===")
	t.Logf("Base directory: %s", dir)
	t.Logf("File 1 absolute: %s", f1)
	t.Logf("File 2 absolute: %s", f2)

	cm := NewContextManager(dir)

	// Add files using absolute paths
	t.Logf("\n--- Adding files using absolute paths ---")
	if err := cm.AddPath(f1); err != nil {
		t.Fatal(err)
	}
	if err := cm.AddPath(f2); err != nil {
		t.Fatal(err)
	}

	// Show what paths are tracked
	t.Logf("\nTracked paths:")
	for _, path := range cm.ListPaths() {
		t.Logf("  %s", path)
	}

	// Generate txtar and show the file headers
	txtarString := cm.GetTxtarString()
	t.Logf("\nTxtar file headers:")
	lines := strings.Split(txtarString, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "-- ") && strings.HasSuffix(line, " --") {
			t.Logf("  %s", line)
		}
	}

	t.Logf("\nFull txtar output:")
	t.Logf("%s", txtarString)
}