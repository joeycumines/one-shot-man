package scripting

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/testutil"
)

// newTestEngineWithOpts creates a test engine with EngineOptions (e.g. WithModulePaths).
func newTestEngineWithOpts(t *testing.T, ctx context.Context, stdout, stderr *bytes.Buffer, opts ...EngineOption) *Engine {
	t.Helper()
	engine, err := NewEngineDetailed(ctx, stdout, stderr,
		testutil.NewTestSessionID("", t.Name()), "memory",
		nil, 0, slog.LevelInfo, opts...)
	if err != nil {
		t.Fatalf("NewEngineDetailed failed: %v", err)
	}
	t.Cleanup(func() { _ = engine.Close() })
	return engine
}

// TestFileScript_RequireRelative verifies that a file-based script can require
// a relative module (e.g. require('./lib/helpers.js')).
func TestFileScript_RequireRelative(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Create a helper module
	libDir := filepath.Join(tmpDir, "lib")
	if err := os.MkdirAll(libDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(libDir, "helpers.js"), []byte(`
		exports.greet = function(name) {
			return "Hello, " + name + "!";
		};
	`), 0644); err != nil {
		t.Fatal(err)
	}

	// Create main script that requires the helper
	mainScript := filepath.Join(tmpDir, "main.js")
	if err := os.WriteFile(mainScript, []byte(`
		var helpers = require('./lib/helpers');
		output.print(helpers.greet("World"));
	`), 0644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	ctx := context.Background()
	engine := newTestEngineWithOpts(t, ctx, &stdout, &stderr)

	script, err := engine.LoadScript("main.js", mainScript)
	if err != nil {
		t.Fatalf("LoadScript failed: %v", err)
	}
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("ExecuteScript failed: %v\nstderr: %s", err, stderr.String())
	}

	got := strings.TrimSpace(stdout.String())
	if got != "Hello, World!" {
		t.Errorf("expected 'Hello, World!', got %q", got)
	}
}

// TestFileScript_RequireRelativeSubdir verifies deeper relative requires work.
func TestFileScript_RequireRelativeSubdir(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Create sub/utils/math.js
	utilsDir := filepath.Join(tmpDir, "sub", "utils")
	if err := os.MkdirAll(utilsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(utilsDir, "math.js"), []byte(`
		exports.add = function(a, b) { return a + b; };
	`), 0644); err != nil {
		t.Fatal(err)
	}

	// Create sub/main.js that requires ../sub/utils/math (relative)
	subDir := filepath.Join(tmpDir, "sub")
	mainScript := filepath.Join(subDir, "main.js")
	if err := os.WriteFile(mainScript, []byte(`
		var math = require('./utils/math');
		output.print("sum=" + math.add(3, 4));
	`), 0644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	ctx := context.Background()
	engine := newTestEngineWithOpts(t, ctx, &stdout, &stderr)

	script, err := engine.LoadScript("main.js", mainScript)
	if err != nil {
		t.Fatalf("LoadScript failed: %v", err)
	}
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("ExecuteScript failed: %v\nstderr: %s", err, stderr.String())
	}

	got := strings.TrimSpace(stdout.String())
	if got != "sum=7" {
		t.Errorf("expected 'sum=7', got %q", got)
	}
}

// TestFileScript_RequireJSON verifies that require() can load JSON files.
func TestFileScript_RequireJSON(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	if err := os.WriteFile(filepath.Join(tmpDir, "data.json"), []byte(`{"name":"osm","version":"1.0"}`), 0644); err != nil {
		t.Fatal(err)
	}

	mainScript := filepath.Join(tmpDir, "main.js")
	if err := os.WriteFile(mainScript, []byte(`
		var data = require('./data.json');
		output.print(data.name + "@" + data.version);
	`), 0644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	ctx := context.Background()
	engine := newTestEngineWithOpts(t, ctx, &stdout, &stderr)

	script, err := engine.LoadScript("main.js", mainScript)
	if err != nil {
		t.Fatalf("LoadScript failed: %v", err)
	}
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("ExecuteScript failed: %v\nstderr: %s", err, stderr.String())
	}

	got := strings.TrimSpace(stdout.String())
	if got != "osm@1.0" {
		t.Errorf("expected 'osm@1.0', got %q", got)
	}
}

// TestFileScript_RequireIndexJS verifies that require('./dir') loads dir/index.js.
func TestFileScript_RequireIndexJS(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	myLibDir := filepath.Join(tmpDir, "mylib")
	if err := os.MkdirAll(myLibDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(myLibDir, "index.js"), []byte(`
		exports.status = "loaded";
	`), 0644); err != nil {
		t.Fatal(err)
	}

	mainScript := filepath.Join(tmpDir, "main.js")
	if err := os.WriteFile(mainScript, []byte(`
		var mylib = require('./mylib');
		output.print("status=" + mylib.status);
	`), 0644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	ctx := context.Background()
	engine := newTestEngineWithOpts(t, ctx, &stdout, &stderr)

	script, err := engine.LoadScript("main.js", mainScript)
	if err != nil {
		t.Fatalf("LoadScript failed: %v", err)
	}
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("ExecuteScript failed: %v\nstderr: %s", err, stderr.String())
	}

	got := strings.TrimSpace(stdout.String())
	if got != "status=loaded" {
		t.Errorf("expected 'status=loaded', got %q", got)
	}
}

// TestFileScript_RequireCaching verifies that modules are cached (not re-executed).
func TestFileScript_RequireCaching(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// counter.js uses a module-level variable to track load count.
	// If the module is cached, the count stays at 1 across multiple require() calls.
	if err := os.WriteFile(filepath.Join(tmpDir, "counter.js"), []byte(`
		var loadCount = 0;
		loadCount++;
		exports.getCount = function() { return loadCount; };
	`), 0644); err != nil {
		t.Fatal(err)
	}

	mainScript := filepath.Join(tmpDir, "main.js")
	if err := os.WriteFile(mainScript, []byte(`
		var c1 = require('./counter');
		var c2 = require('./counter');
		// Should be 1 because the module is cached (not re-executed)
		output.print("count=" + c1.getCount());
		// Same object reference confirms caching
		output.print("same=" + (c1 === c2));
	`), 0644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	ctx := context.Background()
	engine := newTestEngineWithOpts(t, ctx, &stdout, &stderr)

	script, err := engine.LoadScript("main.js", mainScript)
	if err != nil {
		t.Fatalf("LoadScript failed: %v", err)
	}
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("ExecuteScript failed: %v\nstderr: %s", err, stderr.String())
	}

	out := stdout.String()
	if !strings.Contains(out, "count=1") {
		t.Errorf("expected 'count=1' in output, got %q", out)
	}
	if !strings.Contains(out, "same=true") {
		t.Errorf("expected 'same=true' in output, got %q", out)
	}
}

// TestFileScript_RequireNativeModule verifies require('osm:...') still works from file scripts.
func TestFileScript_RequireNativeModule(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	mainScript := filepath.Join(tmpDir, "main.js")
	if err := os.WriteFile(mainScript, []byte(`
		var flag = require('osm:flag');
		var fs = flag.newFlagSet('test');
		fs.string('name', 'default', 'a name');
		var result = fs.parse(['--name', 'goja']);
		if (result.error !== null) throw new Error('parse failed: ' + result.error);
		output.print("name=" + fs.get('name'));
	`), 0644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	ctx := context.Background()
	engine := newTestEngineWithOpts(t, ctx, &stdout, &stderr)

	script, err := engine.LoadScript("main.js", mainScript)
	if err != nil {
		t.Fatalf("LoadScript failed: %v", err)
	}
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("ExecuteScript failed: %v\nstderr: %s", err, stderr.String())
	}

	got := strings.TrimSpace(stdout.String())
	if got != "name=goja" {
		t.Errorf("expected 'name=goja', got %q", got)
	}
}

// TestFileScript_InlineScriptStillWorks verifies that embedded (string) scripts work as before.
func TestFileScript_InlineScriptStillWorks(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	ctx := context.Background()
	engine := newTestEngineWithOpts(t, ctx, &stdout, &stderr)

	script := engine.LoadScriptFromString("inline", `output.print("inline works");`)
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("ExecuteScript failed: %v", err)
	}

	got := strings.TrimSpace(stdout.String())
	if got != "inline works" {
		t.Errorf("expected 'inline works', got %q", got)
	}
}

// TestFileScript_RequireNotFoundError verifies error message for missing modules.
func TestFileScript_RequireNotFoundError(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	mainScript := filepath.Join(tmpDir, "main.js")
	if err := os.WriteFile(mainScript, []byte(`
		var missing = require('./nonexistent');
	`), 0644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	ctx := context.Background()
	engine := newTestEngineWithOpts(t, ctx, &stdout, &stderr)

	script, err := engine.LoadScript("main.js", mainScript)
	if err != nil {
		t.Fatalf("LoadScript failed: %v", err)
	}
	err = engine.ExecuteScript(script)
	if err == nil {
		t.Fatal("expected error for missing module, got nil")
	}
	if !strings.Contains(err.Error(), "script execution failed") {
		t.Errorf("expected 'script execution failed' in error, got %q", err.Error())
	}
}

// TestFileScript_ExecutionContextInModule verifies ctx.log works from file-based scripts.
func TestFileScript_ExecutionContextInModule(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	mainScript := filepath.Join(tmpDir, "main.js")
	if err := os.WriteFile(mainScript, []byte(`
		ctx.log("hello from module");
	`), 0644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	ctx := context.Background()
	engine := newTestEngineWithOpts(t, ctx, &stdout, &stderr)
	engine.SetTestMode(true)

	script, err := engine.LoadScript("main.js", mainScript)
	if err != nil {
		t.Fatalf("LoadScript failed: %v", err)
	}
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("ExecuteScript failed: %v\nstderr: %s", err, stderr.String())
	}

	if !strings.Contains(stdout.String(), "hello from module") {
		t.Errorf("expected 'hello from module' in output, got %q", stdout.String())
	}
}

// TestModulePaths_BareModuleName verifies that WithModulePaths enables bare module resolution.
func TestModulePaths_BareModuleName(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Create a module in a shared library directory
	sharedLibDir := filepath.Join(tmpDir, "shared-libs")
	if err := os.MkdirAll(sharedLibDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sharedLibDir, "myutil.js"), []byte(`
		exports.version = "2.0";
	`), 0644); err != nil {
		t.Fatal(err)
	}

	// Create script in a different directory
	scriptDir := filepath.Join(tmpDir, "scripts")
	if err := os.MkdirAll(scriptDir, 0755); err != nil {
		t.Fatal(err)
	}
	mainScript := filepath.Join(scriptDir, "main.js")
	if err := os.WriteFile(mainScript, []byte(`
		var myutil = require('myutil');
		output.print("version=" + myutil.version);
	`), 0644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	ctx := context.Background()
	engine := newTestEngineWithOpts(t, ctx, &stdout, &stderr,
		WithModulePaths(sharedLibDir))

	script, err := engine.LoadScript("main.js", mainScript)
	if err != nil {
		t.Fatalf("LoadScript failed: %v", err)
	}
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("ExecuteScript failed: %v\nstderr: %s", err, stderr.String())
	}

	got := strings.TrimSpace(stdout.String())
	if got != "version=2.0" {
		t.Errorf("expected 'version=2.0', got %q", got)
	}
}

// TestModulePaths_MultipleSearchPaths verifies multiple search paths work.
func TestModulePaths_MultipleSearchPaths(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Create modules in two different directories
	dir1 := filepath.Join(tmpDir, "libs1")
	dir2 := filepath.Join(tmpDir, "libs2")
	if err := os.MkdirAll(dir1, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir2, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir1, "alpha.js"), []byte(`exports.name = "alpha";`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir2, "beta.js"), []byte(`exports.name = "beta";`), 0644); err != nil {
		t.Fatal(err)
	}

	mainScript := filepath.Join(tmpDir, "main.js")
	if err := os.WriteFile(mainScript, []byte(`
		var alpha = require('alpha');
		var beta = require('beta');
		output.print(alpha.name + "+" + beta.name);
	`), 0644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	ctx := context.Background()
	engine := newTestEngineWithOpts(t, ctx, &stdout, &stderr,
		WithModulePaths(dir1, dir2))

	script, err := engine.LoadScript("main.js", mainScript)
	if err != nil {
		t.Fatalf("LoadScript failed: %v", err)
	}
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("ExecuteScript failed: %v\nstderr: %s", err, stderr.String())
	}

	got := strings.TrimSpace(stdout.String())
	if got != "alpha+beta" {
		t.Errorf("expected 'alpha+beta', got %q", got)
	}
}

// TestModulePaths_InlineScriptWithModulePaths verifies that inline scripts can also use module paths.
func TestModulePaths_InlineScriptWithModulePaths(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Create a module in the search path
	if err := os.WriteFile(filepath.Join(tmpDir, "greeting.js"), []byte(`
		exports.hello = function() { return "hi there"; };
	`), 0644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	ctx := context.Background()
	engine := newTestEngineWithOpts(t, ctx, &stdout, &stderr,
		WithModulePaths(tmpDir))

	// Inline scripts can use bare module names via global folders
	script := engine.LoadScriptFromString("inline", `
		var greeting = require('greeting');
		output.print(greeting.hello());
	`)
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("ExecuteScript failed: %v\nstderr: %s", err, stderr.String())
	}

	got := strings.TrimSpace(stdout.String())
	if got != "hi there" {
		t.Errorf("expected 'hi there', got %q", got)
	}
}

// TestFileScript_DirnameFilename verifies __dirname and __filename are set in required modules.
// Note: the main script is NOT loaded through require() (it uses Compile+RunProgram for
// backward compatibility), so __dirname/__filename are not available in the main script.
// They ARE available in modules loaded via require().
func TestFileScript_DirnameFilename(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Create a helper module that exposes __dirname and __filename
	if err := os.WriteFile(filepath.Join(tmpDir, "info.js"), []byte(`
		exports.dirname = __dirname;
		exports.filename = __filename;
	`), 0644); err != nil {
		t.Fatal(err)
	}

	mainScript := filepath.Join(tmpDir, "main.js")
	if err := os.WriteFile(mainScript, []byte(`
		var info = require('./info');
		output.print("dirname=" + info.dirname);
		output.print("filename=" + info.filename);
	`), 0644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	ctx := context.Background()
	engine := newTestEngineWithOpts(t, ctx, &stdout, &stderr)

	script, err := engine.LoadScript("main.js", mainScript)
	if err != nil {
		t.Fatalf("LoadScript failed: %v", err)
	}
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("ExecuteScript failed: %v\nstderr: %s", err, stderr.String())
	}

	out := stdout.String()
	// __dirname should be the directory containing info.js.
	// goja_nodejs stores the resolved path without following OS symlinks,
	// so we use filepath.Abs (not EvalSymlinks) for comparison.
	absPath, _ := filepath.Abs(tmpDir)
	if !strings.Contains(out, "dirname="+absPath) {
		t.Errorf("expected dirname=%s in output, got %q", absPath, out)
	}
	expectedFilename := filepath.Join(absPath, "info.js")
	if !strings.Contains(out, "filename="+expectedFilename) {
		t.Errorf("expected filename=%s in output, got %q", expectedFilename, out)
	}
}

// TestFileScript_Shebang verifies that scripts with shebangs are handled correctly.
func TestFileScript_Shebang(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	mainScript := filepath.Join(tmpDir, "main.js")
	if err := os.WriteFile(mainScript, []byte("#!/usr/bin/env osm script --test\noutput.print(\"shebang ok\");\n"), 0644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	ctx := context.Background()
	engine := newTestEngineWithOpts(t, ctx, &stdout, &stderr)

	script, err := engine.LoadScript("main.js", mainScript)
	if err != nil {
		t.Fatalf("LoadScript failed: %v", err)
	}
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("ExecuteScript failed: %v\nstderr: %s", err, stderr.String())
	}

	got := strings.TrimSpace(stdout.String())
	if got != "shebang ok" {
		t.Errorf("expected 'shebang ok', got %q", got)
	}
}

// TestFileScript_ShebangWithRequire verifies shebang scripts can require modules.
func TestFileScript_ShebangWithRequire(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Create a helper module
	if err := os.WriteFile(filepath.Join(tmpDir, "helper.js"), []byte(`
		exports.name = "helper";
	`), 0644); err != nil {
		t.Fatal(err)
	}

	mainScript := filepath.Join(tmpDir, "main.js")
	if err := os.WriteFile(mainScript, []byte("#!/usr/bin/env osm script\nvar h = require('./helper');\noutput.print(h.name);\n"), 0644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	ctx := context.Background()
	engine := newTestEngineWithOpts(t, ctx, &stdout, &stderr)

	script, err := engine.LoadScript("main.js", mainScript)
	if err != nil {
		t.Fatalf("LoadScript failed: %v", err)
	}
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("ExecuteScript failed: %v\nstderr: %s", err, stderr.String())
	}

	got := strings.TrimSpace(stdout.String())
	if got != "helper" {
		t.Errorf("expected 'helper', got %q", got)
	}
}
