package jscompliance

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGlobals_ContextManager exercises the `context` global (file/path context
// manager) — the core abstraction workflows use — against real temp files:
// addPath, listPaths, toTxtar (content surfaces), getFilesByExt, removePath.
func TestGlobals_ContextManager(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	dir := t.TempDir()
	// A .go file and a .md file, with distinct content.
	goFile := filepath.Join(dir, "main.go")
	mdFile := filepath.Join(dir, "README.md")
	if err := os.WriteFile(goFile, []byte("package main\n"), 0o600); err != nil {
		t.Fatalf("write go: %v", err)
	}
	if err := os.WriteFile(mdFile, []byte("# title\n"), 0o600); err != nil {
		t.Fatalf("write md: %v", err)
	}

	ctx := context.Background()
	engine, _, _ := newComplianceEngine(t, ctx)

	// addPath + listPaths
	for _, p := range []string{goFile, mdFile} {
		if _, err := evalJS(t, engine, `context.addPath(`+jsStringLit(p)+`)`, defaultEvalTimeout); err != nil {
			t.Fatalf("context.addPath(%s): %v", p, err)
		}
	}
	listed, err := evalJS(t, engine, `JSON.stringify(context.listPaths())`, defaultEvalTimeout)
	if err != nil {
		t.Fatalf("listPaths: %v", err)
	}
	if s, _ := listed.(string); !strings.Contains(s, "main.go") || !strings.Contains(s, "README.md") {
		t.Errorf("context.listPaths() = %s, want both files", s)
	}

	// toTxtar surfaces file contents
	txtar, err := evalJS(t, engine, `context.toTxtar()`, defaultEvalTimeout)
	if err != nil {
		t.Fatalf("toTxtar: %v", err)
	}
	if s, _ := txtar.(string); !strings.Contains(s, "package main") || !strings.Contains(s, "# title") {
		t.Errorf("context.toTxtar() missing file contents; got %q", s)
	}

	// getFilesByExt('.go') returns the go file
	goFiles, err := evalJS(t, engine, `JSON.stringify(context.getFilesByExt('.go'))`, defaultEvalTimeout)
	if err != nil {
		t.Fatalf("getFilesByExt: %v", err)
	}
	if s, _ := goFiles.(string); !strings.Contains(s, "main.go") {
		t.Errorf("context.getFilesByExt('.go') = %s, want main.go", s)
	}

	// removePath removes it from the tracked set
	if _, err := evalJS(t, engine, `context.removePath(`+jsStringLit(goFile)+`)`, defaultEvalTimeout); err != nil {
		t.Fatalf("removePath: %v", err)
	}
	listed2, _ := evalJS(t, engine, `JSON.stringify(context.listPaths())`, defaultEvalTimeout)
	if s, _ := listed2.(string); strings.Contains(s, "main.go") {
		t.Errorf("context.removePath did not remove main.go; listPaths=%s", s)
	}
}
