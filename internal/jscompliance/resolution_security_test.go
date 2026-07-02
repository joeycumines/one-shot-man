package jscompliance

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/scripting"
)

// TestResolution_RelativeAndJSON covers ./../ resolution, the .js/.json
// resolution order, and that requiring the same module twice returns the
// SAME exports object (caching).
func TestResolution_RelativeAndJSON(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "lib", "helpers.js"), `module.exports = { greet: function(){ return 'hi'; } };`)
	writeFile(t, filepath.Join(dir, "data.json"), `{"value": 99}`)
	writeFile(t, filepath.Join(dir, "main.js"), `
		var h = require('./lib/helpers.js');
		var h2 = require('./lib/helpers.js');  // same specifier -> cached identity
		var d = require('./data.json');         // JSON require
		globalThis.__rc = JSON.stringify({
			greet: h.greet(),
			sameObject: (h === h2),
			jsonValue: d.value
		});
	`)
	engine, err := runFileScriptAt(t, ctx, dir, "main.js")
	if err != nil {
		t.Fatalf("script error: %v", err)
	}
	v, _ := evalJS(t, engine, `globalThis.__rc`, defaultEvalTimeout)
	s, _ := v.(string)
	for _, want := range []string{`"greet":"hi"`, `"sameObject":true`, `"jsonValue":99`} {
		if !strings.Contains(s, want) {
			t.Errorf("resolution result %s missing %s", s, want)
		}
	}
}

// TestResolution_ExportsAliasTrap pins the classic CommonJS footgun:
// reassigning `exports = {...}` BREAKS the module.exports alias, so the
// caller receives the ORIGINAL (empty) module.exports. require'ers must use
// module.exports = {...}.
func TestResolution_ExportsAliasTrap(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "broken.js"), `exports = { x: 1 };`) // reassigns local, alias broken
	writeFile(t, filepath.Join(dir, "good.js"), `module.exports = { x: 2 };`)
	writeFile(t, filepath.Join(dir, "main.js"), `
		var b = require('./broken');
		var g = require('./good');
		globalThis.__rc = JSON.stringify({ bKeys: Object.keys(b), gX: g.x });
	`)
	engine, err := runFileScriptAt(t, ctx, dir, "main.js")
	if err != nil {
		t.Fatalf("script error: %v", err)
	}
	v, _ := evalJS(t, engine, `globalThis.__rc`, defaultEvalTimeout)
	s, _ := v.(string)
	if !strings.Contains(s, `"bKeys":[]`) {
		t.Errorf("exports-alias trap: expected broken module to have NO keys (alias broken); got %s", s)
	}
	if !strings.Contains(s, `"gX":2`) {
		t.Errorf("module.exports path should work; got %s", s)
	}
}

// TestResolution_FilenameDirname asserts file scripts get __filename/__dirname
// matching their real path (the absPath compile, engine_core.go:534-549).
// Documents RISK-D (inline -e lacks them — covered by the harness using evalJS,
// which has no __filename).
func TestResolution_FilenameDirname(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	dir := t.TempDir()
	wantFile := filepath.Join(dir, "self.js")
	writeFile(t, wantFile, `module.exports = { fn: __filename, dn: __dirname };`)
	writeFile(t, filepath.Join(dir, "main.js"), `
		var s = require('./self');
		globalThis.__fn = s.fn;
		globalThis.__dn = s.dn;
	`)
	engine, err := runFileScriptAt(t, ctx, dir, "main.js")
	if err != nil {
		t.Fatalf("script error: %v", err)
	}
	// Compare with forward-slash normalization so the test is robust to the
	// platform path separator (Windows uses '\', Unix '/').
	gotFn, _ := evalJS(t, engine, `globalThis.__fn`, defaultEvalTimeout)
	gotDn, _ := evalJS(t, engine, `globalThis.__dn`, defaultEvalTimeout)
	fnStr, _ := gotFn.(string)
	dnStr, _ := gotDn.(string)
	if filepath.ToSlash(fnStr) != filepath.ToSlash(wantFile) {
		t.Errorf("__filename drift: got %q, want %q", fnStr, wantFile)
	}
	if filepath.ToSlash(dnStr) != filepath.ToSlash(dir) {
		t.Errorf("__dirname drift: got %q, want %q", dnStr, dir)
	}
}

// TestSecurity_BareNameTraversalBlocked asserts that a BARE module name (no ./
// ../ prefix) containing `..` components that would escape the configured
// module-paths is BLOCKED by the path-traversal hardening
// (module_hardening.go). Relative requires (./ ../) are NOT hardened (the
// calling script's dir is trusted) — that's the documented exception.
func TestSecurity_BareNameTraversalBlocked(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	tmp := t.TempDir()
	libDir := filepath.Join(tmp, "libs")
	writeFile(t, filepath.Join(libDir, "x", ".gitkeep"), "")     // x subdir exists (for the bare-name resolver)
	writeFile(t, filepath.Join(libDir, "allowed.js"), `module.exports = { ok: true };`)
	writeFile(t, filepath.Join(tmp, "secret.js"), `module.exports = { data: "stolen" };`) // OUTSIDE libDir

	engine, _, _ := newComplianceEngineOpts(t, ctx, scripting.WithModulePaths(libDir))

	// "x/../../secret" is a BARE name (starts with "x") resolved through
	// libDir; join(libDir,"x/../../secret") escapes to tmp/secret.js. The
	// hardened resolver must block it.
	v, err := evalJS(t, engine, `(function(){ try { require('x/../../secret'); return 'LEAKED'; } catch(e){ return 'BLOCKED'; } })()`, defaultEvalTimeout)
	if err != nil {
		t.Fatalf("traversal probe failed: %v", err)
	}
	if s, _ := v.(string); s != "BLOCKED" {
		t.Errorf("bare-name traversal x/../../secret was NOT blocked (path-traversal hardening failed); got %s", s)
	}

	// Benign bare name within libDir loads normally.
	if _, err := evalJS(t, engine, `require('allowed').ok`, defaultEvalTimeout); err != nil {
		t.Errorf("benign bare-name require('allowed') failed: %v", err)
	}
}

// --- helpers ---

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// runFileScriptAt executes mainFile (by name) within baseDir on a fresh engine.
func runFileScriptAt(t *testing.T, ctx context.Context, baseDir, mainName string) (*scripting.Engine, error) {
	t.Helper()
	engine, _, _ := newComplianceEngine(t, ctx)
	return engine, execMain(t, engine, filepath.Join(baseDir, mainName))
}

// execMain loads+executes a script file on an existing engine.
func execMain(t *testing.T, engine *scripting.Engine, path string) error {
	t.Helper()
	script, err := engine.LoadScript(filepath.Base(path), path)
	if err != nil {
		return err
	}
	return engine.ExecuteScript(script)
}
