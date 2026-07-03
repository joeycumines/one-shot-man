package jscompliance

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSlow_Path_GlobValue pins osm:path.glob (async) with a VALUE assertion:
// globbing a temp dir resolves to {matches, error} with the matching files.
// (Previously only Promise-shape was checked — half-assed; now value-complete.)
func TestSlow_Path_GlobValue(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("x"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("x"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "c.md"), []byte("x"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	ctx := context.Background()
	engine, _, _ := newComplianceEngine(t, ctx)
	v, err := evalJS(t, engine, `await require('osm:path').glob(`+jsStringLit(filepath.Join(dir, "*.txt"))+`)`, defaultEvalTimeout)
	if err != nil {
		t.Fatalf("glob failed: %v", err)
	}
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("glob resolved to %T, want {matches,error} object", v)
	}
	matches, hasM := m["matches"]
	if !hasM {
		t.Errorf("glob result missing 'matches'; got keys %v", mapKeys(m))
	} else {
		// matches may export as []string or []any depending on the binding;
		// assert by content (type-tolerant).
		rendered := fmt.Sprintf("%v", matches)
		if !strings.Contains(rendered, "a.txt") || !strings.Contains(rendered, "b.txt") {
			t.Errorf("glob *.txt matches = %v, want both a.txt and b.txt", matches)
		}
		if strings.Contains(rendered, "c.md") {
			t.Errorf("glob *.txt matched c.md (a .md file): %v", matches)
		}
	}
}

// TestSlow_Tokenizer_LoadFileRejectsInvalid pins osm:tokenizer.loadFile (async)
// error path: loading a file that is NOT valid tokenizer JSON rejects (rather
// than resolving garbage). The valid loader-JSON schema is tokenizer-internal;
// the rejection-on-invalid contract is the user-facing guarantee.
func TestSlow_Tokenizer_LoadFileRejectsInvalid(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte("{ this is not valid tokenizer json }"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	ctx := context.Background()
	engine, _, _ := newComplianceEngine(t, ctx)
	v, err := evalJS(t, engine, `(async function () {
		try { await require('osm:tokenizer').loadFile(`+jsStringLit(bad)+`); return 'RESOLVED'; }
		catch (e) { return 'REJECTED'; }
	})()`, defaultEvalTimeout)
	if err != nil {
		t.Fatalf("loadFile probe failed: %v", err)
	}
	if s, _ := v.(string); s != "REJECTED" {
		t.Errorf("tokenizer.loadFile on invalid JSON should reject; got %v", v)
	}
}
