package scripting

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Test ToTxtar naming preserves uniqueness with minimal suffixes
func TestContextManager_ToTxtar_UniqueNaming(t *testing.T) {
	dir := t.TempDir()
	// Create files: a/handlers.go, b/handlers.go, c/utils.go
	mustWrite := func(p, s string) {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	f1 := filepath.Join(dir, "a", "handlers.go")
	f2 := filepath.Join(dir, "b", "handlers.go")
	f3 := filepath.Join(dir, "c", "utils.go")
	mustWrite(f1, "package a\n")
	mustWrite(f2, "package b\n")
	mustWrite(f3, "package c\n")

	cm := NewContextManager(dir)
	if err := cm.AddPath(f1); err != nil {
		t.Fatal(err)
	}
	if err := cm.AddPath(f2); err != nil {
		t.Fatal(err)
	}
	if err := cm.AddPath(f3); err != nil {
		t.Fatal(err)
	}

	txt := cm.GetTxtarString()

	// Expect utils.go appears as basename (no collision)
	if !strings.Contains(txt, "-- utils.go --") {
		t.Fatalf("expected basename for unique file: %s", txt)
	}
	// Expect handlers.go entries include directory suffixes ensuring uniqueness
	if !strings.Contains(txt, "-- a/handlers.go --") || !strings.Contains(txt, "-- b/handlers.go --") {
		t.Fatalf("expected directory-qualified names for collisions, got: %s", txt)
	}
}
