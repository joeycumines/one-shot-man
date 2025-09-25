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

	cm, err := NewContextManager(dir)
	if err != nil {
		t.Fatalf("NewContextManager failed: %v", err)
	}
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

	// Expect utils.go appears with its directory structure (preserving meaningful path)
	if !strings.Contains(txt, "-- c/utils.go --") {
		t.Fatalf("expected meaningful path for file: %s", txt)
	}
	// Expect handlers.go entries include directory suffixes ensuring uniqueness
	if !strings.Contains(txt, "-- a/handlers.go --") || !strings.Contains(txt, "-- b/handlers.go --") {
		t.Fatalf("expected directory-qualified names for collisions, got: %s", txt)
	}
}

// Test ToTxtar disambiguation that requires multiple directory levels
func TestContextManager_ToTxtar_MultiLevelDisambiguation(t *testing.T) {
	dir := t.TempDir()
	mustWrite := func(p, s string) {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Create files: a/d/file.go, b/d/file.go -> need two-level suffixes
	f1 := filepath.Join(dir, "a", "d", "file.go")
	f2 := filepath.Join(dir, "b", "d", "file.go")
	// Also add: a/e/file.go -> should only need one-level (e/file.go)
	f3 := filepath.Join(dir, "a", "e", "file.go")
	mustWrite(f1, "package a_d\n")
	mustWrite(f2, "package b_d\n")
	mustWrite(f3, "package a_e\n")

	cm, err := NewContextManager(dir)
	if err != nil {
		t.Fatalf("NewContextManager failed: %v", err)
	}
	for _, p := range []string{f1, f2, f3} {
		if err := cm.AddPath(p); err != nil {
			t.Fatal(err)
		}
	}

	txt := cm.GetTxtarString()

	// Expect the two colliding files to be disambiguated with parent directory
	if !strings.Contains(txt, "-- a/d/file.go --") || !strings.Contains(txt, "-- b/d/file.go --") {
		t.Fatalf("expected two-level unique suffixes for colliding files, got: %s", txt)
	}
	// With global depth across the basename group, sibling will also include its parent dir
	if !strings.Contains(txt, "-- a/e/file.go --") {
		t.Fatalf("expected group-wide depth disambiguation for sibling, got: %s", txt)
	}
}

// Test a deeper chain requiring three-level disambiguation
func TestContextManager_ToTxtar_DeepDisambiguation(t *testing.T) {
	dir := t.TempDir()
	mustWrite := func(p, s string) {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Files: a/x/y/z.go, b/x/y/z.go, c/x/w/z.go
	// The first two require three components (a/x/y/z.go and b/x/y/z.go)
	// The third should be disambiguated with x/w/z.go (two components) since basename collides with the others but parent differs earlier.
	f1 := filepath.Join(dir, "a", "x", "y", "z.go")
	f2 := filepath.Join(dir, "b", "x", "y", "z.go")
	f3 := filepath.Join(dir, "c", "x", "w", "z.go")
	mustWrite(f1, "package axy\n")
	mustWrite(f2, "package bxy\n")
	mustWrite(f3, "package cxw\n")

	cm, err := NewContextManager(dir)
	if err != nil {
		t.Fatalf("NewContextManager failed: %v", err)
	}
	for _, p := range []string{f1, f2, f3} {
		if err := cm.AddPath(p); err != nil {
			t.Fatal(err)
		}
	}

	txt := cm.GetTxtarString()

	// Check expected disambiguations
	if !strings.Contains(txt, "-- a/x/y/z.go --") || !strings.Contains(txt, "-- b/x/y/z.go --") {
		t.Fatalf("expected deep disambiguation for a/b, got: %s", txt)
	}
	if !strings.Contains(txt, "-- c/x/w/z.go --") {
		t.Fatalf("expected group-wide depth disambiguation for c path, got: %s", txt)
	}
}
