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

// TestSlow_OutputClipboard_IsAsync pins the DRIFT-4 FIX: output.toClipboard
// and output.fromClipboard must return Promises (async per the JS Binding
// Contract — clipboard is subprocess I/O). Uses OSM_CLIPBOARD[_PASTE] overrides
// so no host clipboard is mutated.
func TestSlow_OutputClipboard_IsAsync(t *testing.T) {
	skipSlow(t)
	// Not parallel: mutates env (OSM_CLIPBOARD*) for isolation.

	// Isolate: redirect clipboard to harmless commands (no host mutation).
	t.Setenv("OSM_CLIPBOARD", "true")                   // copy: discard stdin, exit 0
	t.Setenv("OSM_CLIPBOARD_PASTE", "echo pasted-text") // paste: stdout -> text

	ctx := context.Background()
	engine, _, _ := newComplianceEngine(t, ctx)

	// toClipboard returns a Promise.
	isPromise, err := evalJS(t, engine, `(function(){
		var p = output.toClipboard('jscompliance');
		return (p !== null && p !== undefined && typeof p === 'object' && typeof p.then === 'function');
	})()`, defaultEvalTimeout)
	if err != nil {
		t.Fatalf("toClipboard probe: %v", err)
	}
	if b, _ := isPromise.(bool); !b {
		t.Errorf("DRIFT-4: output.toClipboard must return a Promise (binding contract); got %v", isPromise)
	}

	// fromClipboard returns a Promise<string> that resolves to the pasted text.
	v, err := evalJS(t, engine, `await output.fromClipboard()`, defaultEvalTimeout)
	if err != nil {
		t.Fatalf("fromClipboard await: %v", err)
	}
	if s, _ := v.(string); strings.TrimSpace(s) == "" {
		t.Errorf("DRIFT-4: output.fromClipboard resolved to %q, want a non-empty string from OSM_CLIPBOARD_PASTE", s)
	}
}

// TestSlow_OutputClipboard_GracefulDegradation pins a verified property of the
// DRIFT-4 binding: output.toClipboard DEGRADES GRACEFULLY — it resolves (does
// NOT reject) even when the clipboard override fails. ClipboardCopy (os.go:381)
// falls through a chain (OSM_CLIPBOARD override → platform utility pbcopy/xclip/
// clip → tuiSink fallback) and only errors when EVERY mechanism fails AND no
// tuiSink is set; the output binding always passes a tuiSink, so toClipboard
// effectively never rejects (it prints the content to the TUI as a last
// resort). This test sets the override to a failing command and asserts it
// still resolves — pinning the graceful-degradation contract.
func TestSlow_OutputClipboard_GracefulDegradation(t *testing.T) {
	skipSlow(t)
	// Not parallel: mutates env.
	t.Setenv("OSM_CLIPBOARD", "false") // override exits non-zero → falls through

	ctx := context.Background()
	engine, _, _ := newComplianceEngine(t, ctx)
	v, err := evalJS(t, engine, `(async function () {
		try { await output.toClipboard('jscompliance-degrade-probe'); return 'RESOLVED'; }
		catch (e) { return 'REJECTED'; }
	})()`, defaultEvalTimeout)
	if err != nil {
		t.Fatalf("clipboard degradation probe failed: %v", err)
	}
	if s, _ := v.(string); s != "RESOLVED" {
		t.Errorf("output.toClipboard should degrade gracefully (resolve via the fallback chain), not reject; got %v", v)
	}
}

// TestSlow_Os_WriteAppendValue pins osm:os.writeFile + appendFile (async) with
// VALUE assertions: writeFile creates content, appendFile extends it, readFile
// reads it back. Completes os's async VALUE coverage (was Promise-shape only).
func TestSlow_Os_WriteAppendValue(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	ctx := context.Background()
	engine, _, _ := newComplianceEngine(t, ctx)
	dir := t.TempDir()
	path := filepath.Join(dir, "wa.txt")

	// writeFile creates the file with the given content.
	if _, err := evalJS(t, engine, `await require('osm:os').writeFile(`+jsStringLit(path)+`, 'first')`, defaultEvalTimeout); err != nil {
		t.Fatalf("writeFile: %v", err)
	}
	got, _ := evalJS(t, engine, `await require('osm:os').readFile(`+jsStringLit(path)+`)`, defaultEvalTimeout)
	if m, ok := got.(map[string]any); ok {
		if s, _ := m["content"].(string); s != "first" {
			t.Errorf("after writeFile, readFile content = %q, want 'first'", s)
		}
	} else {
		t.Errorf("readFile after writeFile returned %T", got)
	}

	// appendFile extends the content.
	if _, err := evalJS(t, engine, `await require('osm:os').appendFile(`+jsStringLit(path)+`, '-second')`, defaultEvalTimeout); err != nil {
		t.Fatalf("appendFile: %v", err)
	}
	got2, _ := evalJS(t, engine, `await require('osm:os').readFile(`+jsStringLit(path)+`)`, defaultEvalTimeout)
	if m, ok := got2.(map[string]any); ok {
		if s, _ := m["content"].(string); s != "first-second" {
			t.Errorf("after appendFile, readFile content = %q, want 'first-second'", s)
		}
	}
}
