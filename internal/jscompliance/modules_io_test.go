package jscompliance

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// jsStringLit returns a JS string literal for s (single-quoted, escaped).
func jsStringLit(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `'`, `\'`, "\n", `\n`, "\r", `\r`)
	return "'" + r.Replace(s) + "'"
}

// TestSlow_Os_ReadFileShape is the osm:os.readFile gut-check: it must return a
// Promise resolving to the DOCUMENTED {content, error, message} shape (not just
// any Promise). A regression returning {data: ...} or a bare string would
// fail. SLOW because it does real file I/O (in t.TempDir, no host mutation).
func TestSlow_Os_ReadFileShape(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.txt")
	const want = "jscompliance-readfile-probe"
	if err := os.WriteFile(path, []byte(want), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	engine, _, _ := newComplianceEngine(t, ctx)
	got, err := evalJS(t, engine, `await require('osm:os').readFile(`+jsStringLit(path)+`)`, defaultEvalTimeout)
	if err != nil {
		t.Fatalf("readFile failed: %v", err)
	}
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("readFile resolved to %T, want an object with {content,error,message}", got)
	}
	content, hasContent := m["content"]
	if !hasContent {
		t.Errorf("readFile result missing 'content' key; got keys %v — response-shape drift (gut-check)", mapKeys(m))
	} else if s, _ := content.(string); s != want {
		t.Errorf("readFile content = %q, want %q", s, want)
	}
	if _, hasErr := m["error"]; !hasErr {
		t.Errorf("readFile result missing 'error' key (documented shape {content,error,message})")
	}
	if _, hasMsg := m["message"]; !hasMsg {
		t.Errorf("readFile result missing 'message' key (documented shape {content,error,message})")
	}
}

// TestSlow_Os_ReadFileMissingIsNotCrash asserts readFile on a missing path
// resolves (does not reject unhandled) with a populated error/message — the
// documented "no throw, error in result" contract.
func TestSlow_Os_ReadFileMissingIsNotCrash(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	ctx := context.Background()
	engine, _, _ := newComplianceEngine(t, ctx)
	got, err := evalJS(t, engine, `await require('osm:os').readFile(`+jsStringLit(filepath.Join(t.TempDir(), "nope.txt"))+`)`, defaultEvalTimeout)
	if err != nil {
		t.Fatalf("readFile(missing) rejected/errored: %v (should resolve with in-result error)", err)
	}
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("readFile(missing) resolved to %T, want object", got)
	}
	if e := m["error"]; e == nil || e == "" {
		t.Errorf("readFile(missing) should populate 'error'; got %v", e)
	}
}

func mapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
