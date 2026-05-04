package command

import (
	"bytes"
	"context"
	"flag"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// peekScriptFile tests
// ---------------------------------------------------------------------------

func TestPeekScriptFile_JSExtension(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "test.js")
	if err := os.WriteFile(path, []byte("console.log('hello');"), 0644); err != nil {
		t.Fatal(err)
	}

	peek := peekScriptFile(path)
	if peek.kind != scriptKindJS {
		t.Fatalf("expected scriptKindJS, got %v", peek.kind)
	}
	if peek.interactive {
		t.Error("expected interactive=false for .js without shebang -i")
	}
	if peek.testMode {
		t.Error("expected testMode=false for .js without shebang --test")
	}
}

func TestPeekScriptFile_JSExtensionUpperCase(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "test.JS")
	if err := os.WriteFile(path, []byte("// no shebang"), 0644); err != nil {
		t.Fatal(err)
	}

	peek := peekScriptFile(path)
	if peek.kind != scriptKindJS {
		t.Fatalf("expected scriptKindJS for .JS extension, got %v", peek.kind)
	}
}

func TestPeekScriptFile_JSShebang(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	// File without .js extension but with osm script shebang.
	path := filepath.Join(dir, "myscript")
	if err := os.WriteFile(path, []byte("#!/usr/bin/env osm script\nctx.log('hi');"), 0755); err != nil {
		t.Fatal(err)
	}

	peek := peekScriptFile(path)
	if peek.kind != scriptKindJS {
		t.Fatalf("expected scriptKindJS for osm script shebang, got %v", peek.kind)
	}
}

func TestPeekScriptFile_JSShebangWithInteractiveFlag(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "interactive-app")
	if err := os.WriteFile(path, []byte("#!/usr/bin/env osm script -i\nctx.log('interactive');"), 0755); err != nil {
		t.Fatal(err)
	}

	peek := peekScriptFile(path)
	if peek.kind != scriptKindJS {
		t.Fatalf("expected scriptKindJS, got %v", peek.kind)
	}
	if !peek.interactive {
		t.Error("expected interactive=true from shebang -i")
	}
}

func TestPeekScriptFile_JSShebangWithTestFlag(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "test-script")
	if err := os.WriteFile(path, []byte("#!/usr/bin/env osm script --test\nctx.log('test');"), 0755); err != nil {
		t.Fatal(err)
	}

	peek := peekScriptFile(path)
	if peek.kind != scriptKindJS {
		t.Fatalf("expected scriptKindJS, got %v", peek.kind)
	}
	if !peek.testMode {
		t.Error("expected testMode=true from shebang --test")
	}
}

func TestPeekScriptFile_JSShebangWithInteractiveLongFlag(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "app")
	if err := os.WriteFile(path, []byte("#!/usr/bin/env osm script --interactive\nctx.log('hi');"), 0755); err != nil {
		t.Fatal(err)
	}

	peek := peekScriptFile(path)
	if !peek.interactive {
		t.Error("expected interactive=true from shebang --interactive")
	}
}

func TestPeekScriptFile_ExternalShell(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "shellscript")
	if err := os.WriteFile(path, []byte("#!/bin/sh\necho hello"), 0755); err != nil {
		t.Fatal(err)
	}

	peek := peekScriptFile(path)
	if peek.kind != scriptKindExternal {
		t.Fatalf("expected scriptKindExternal for shell shebang, got %v", peek.kind)
	}
}

func TestPeekScriptFile_NoShebangNoJS(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "plainfile")
	if err := os.WriteFile(path, []byte("just some data"), 0644); err != nil {
		t.Fatal(err)
	}

	peek := peekScriptFile(path)
	if peek.kind != scriptKindExternal {
		t.Fatalf("expected scriptKindExternal for plain file, got %v", peek.kind)
	}
}

func TestPeekScriptFile_JSExtensionWithShebangFlags(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "game.js")
	content := "#!/usr/bin/env osm script -i --test\nctx.log('hello');"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	peek := peekScriptFile(path)
	if peek.kind != scriptKindJS {
		t.Fatalf("expected scriptKindJS, got %v", peek.kind)
	}
	if !peek.interactive {
		t.Error("expected interactive=true from shebang -i")
	}
	if !peek.testMode {
		t.Error("expected testMode=true from shebang --test")
	}
}

func TestPeekScriptFile_NonexistentFile(t *testing.T) {
	peek := peekScriptFile("/nonexistent/path/file.js")
	// .js extension still returns JS kind even if file doesn't exist for
	// the extension check path; but parseShebangFlags will silently fail.
	if peek.kind != scriptKindJS {
		t.Fatalf("expected scriptKindJS for .js extension, got %v", peek.kind)
	}
}

// ---------------------------------------------------------------------------
// Registry.Get() returns jsScriptCommand for JS files
// ---------------------------------------------------------------------------

func TestRegistry_Get_JSScript(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "app.js")
	if err := os.WriteFile(scriptPath, []byte("// simple script\nctx.log('hello');"), 0644); err != nil {
		t.Fatal(err)
	}

	r := &Registry{
		commands:    make(map[string]Command),
		scriptPaths: []string{dir},
		config:      nil,
	}

	cmd, err := r.Get("app.js")
	if err != nil {
		t.Fatalf("Get app.js: %v", err)
	}

	// Should return a *jsScriptCommand, not *scriptCommand.
	if _, ok := cmd.(*jsScriptCommand); !ok {
		t.Fatalf("expected *jsScriptCommand, got %T", cmd)
	}
	if cmd.Name() != "app.js" {
		t.Fatalf("expected name 'app.js', got %q", cmd.Name())
	}
}

func TestRegistry_Get_JSScriptWithShebang(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires Unix file permissions for executable scripts with shebangs")
	}
	t.Parallel()

	dir := t.TempDir()
	// Non-.js extension but with osm script shebang.
	scriptPath := filepath.Join(dir, "mygame")
	content := "#!/usr/bin/env osm script -i\nctx.log('game');"
	if err := os.WriteFile(scriptPath, []byte(content), 0755); err != nil {
		t.Fatal(err)
	}

	r := &Registry{
		commands:    make(map[string]Command),
		scriptPaths: []string{dir},
		config:      nil,
	}

	cmd, err := r.Get("mygame")
	if err != nil {
		t.Fatalf("Get mygame: %v", err)
	}

	if _, ok := cmd.(*jsScriptCommand); !ok {
		t.Fatalf("expected *jsScriptCommand for shebang file, got %T", cmd)
	}
}

func TestRegistry_Get_NonJSScript(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires Unix file permissions for executable scripts")
	}
	t.Parallel()

	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "shelltool")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\necho hello"), 0755); err != nil {
		t.Fatal(err)
	}

	r := &Registry{
		commands:    make(map[string]Command),
		scriptPaths: []string{dir},
		config:      nil,
	}

	cmd, err := r.Get("shelltool")
	if err != nil {
		t.Fatalf("Get shelltool: %v", err)
	}

	// Shell scripts should still return *scriptCommand (external execution).
	if _, ok := cmd.(*scriptCommand); !ok {
		t.Fatalf("expected *scriptCommand for shell script, got %T", cmd)
	}
}

func TestRegistry_Get_JSFileWithoutExecBits(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "noexec.js")
	// Create .js file WITHOUT execute bits (mode 0644).
	if err := os.WriteFile(scriptPath, []byte("ctx.log('hello');"), 0644); err != nil {
		t.Fatal(err)
	}

	r := &Registry{
		commands:    make(map[string]Command),
		scriptPaths: []string{dir},
		config:      nil,
	}

	cmd, err := r.Get("noexec.js")
	if err != nil {
		t.Fatalf("Get noexec.js: %v", err)
	}
	if _, ok := cmd.(*jsScriptCommand); !ok {
		t.Fatalf("expected *jsScriptCommand for .js without exec bits, got %T", cmd)
	}
}

// ---------------------------------------------------------------------------
// Registry.findScriptCommands() includes .js files
// ---------------------------------------------------------------------------

func TestRegistry_FindScriptCommands_IncludesJS(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	// Create .js file WITHOUT exec bits.
	if err := os.WriteFile(filepath.Join(dir, "app.js"), []byte("// script"), 0644); err != nil {
		t.Fatal(err)
	}
	// Create non-executable, non-.js file.
	if err := os.WriteFile(filepath.Join(dir, "data.txt"), []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	r := &Registry{
		commands:    make(map[string]Command),
		scriptPaths: []string{dir},
	}

	scripts := r.listScript()
	found := false
	for _, n := range scripts {
		if n == "app.js" {
			found = true
		}
		if n == "data.txt" {
			t.Fatal("non-executable non-.js file should not appear in script list")
		}
	}
	if !found {
		t.Fatal("expected app.js in script list")
	}
}

// ---------------------------------------------------------------------------
// jsScriptCommand execution tests
// ---------------------------------------------------------------------------

func TestJSScriptCommand_Execute_BasicScript(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns JS runtime")
	}
	t.Parallel()

	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "hello.js")
	// ctx.log() writes to the engine's logger. In test mode, output goes to stdout.
	content := "ctx.log('hello from auto-detected script');"
	if err := os.WriteFile(scriptPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cmd := newJSScriptCommand("hello.js", scriptPath, nil, scriptPeekInfo{kind: scriptKindJS})
	cmd.store = "memory"
	cmd.session = t.Name()
	cmd.testMode = true

	var stdout, stderr bytes.Buffer
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd.ctxFactory = func() (context.Context, context.CancelFunc) { return ctx, cancel }

	if err := cmd.Execute(nil, &stdout, &stderr); err != nil {
		t.Fatalf("Execute: %v\nstdout=%q\nstderr=%q", err, stdout.String(), stderr.String())
	}

	output := stdout.String()
	if !strings.Contains(output, "hello from auto-detected script") {
		t.Fatalf("expected output containing script message, got stdout=%q stderr=%q", output, stderr.String())
	}
}

func TestJSScriptCommand_Execute_WithArgs(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns JS runtime")
	}
	t.Parallel()

	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "args.js")
	// 'args' is set as a global by jsScriptCommand.Execute.
	content := "ctx.log('args: ' + JSON.stringify(args));"
	if err := os.WriteFile(scriptPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cmd := newJSScriptCommand("args.js", scriptPath, nil, scriptPeekInfo{kind: scriptKindJS})
	cmd.store = "memory"
	cmd.session = t.Name()
	cmd.testMode = true

	var stdout, stderr bytes.Buffer
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd.ctxFactory = func() (context.Context, context.CancelFunc) { return ctx, cancel }

	if err := cmd.Execute([]string{"foo", "bar"}, &stdout, &stderr); err != nil {
		t.Fatalf("Execute: %v\nstdout=%q\nstderr=%q", err, stdout.String(), stderr.String())
	}

	output := stdout.String()
	if !strings.Contains(output, `args: ["foo","bar"]`) {
		t.Fatalf("expected args in output, got stdout=%q stderr=%q", output, stderr.String())
	}
}

func TestJSScriptCommand_Execute_ScriptError(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns JS runtime")
	}
	t.Parallel()

	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "bad.js")
	content := "throw new Error('intentional error');"
	if err := os.WriteFile(scriptPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cmd := newJSScriptCommand("bad.js", scriptPath, nil, scriptPeekInfo{kind: scriptKindJS})
	cmd.store = "memory"
	cmd.session = t.Name()

	var stdout, stderr bytes.Buffer
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd.ctxFactory = func() (context.Context, context.CancelFunc) { return ctx, cancel }

	err := cmd.Execute(nil, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error from script that throws")
	}
	if !strings.Contains(err.Error(), "intentional error") {
		t.Fatalf("expected error to mention 'intentional error', got: %v", err)
	}
}

func TestJSScriptCommand_Execute_NonexistentScript(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns JS runtime")
	}
	t.Parallel()

	cmd := newJSScriptCommand("missing.js", "/nonexistent/path/missing.js", nil, scriptPeekInfo{kind: scriptKindJS})
	cmd.store = "memory"
	cmd.session = t.Name()

	var stdout, stderr bytes.Buffer
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd.ctxFactory = func() (context.Context, context.CancelFunc) { return ctx, cancel }

	err := cmd.Execute(nil, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for nonexistent script")
	}
}

func TestJSScriptCommand_Execute_ShebangStripped(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns JS runtime")
	}
	t.Parallel()

	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "shebang.js")
	// Shebang lines are stripped by the engine's source loader.
	content := "#!/usr/bin/env osm script\nctx.log('shebang works');"
	if err := os.WriteFile(scriptPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cmd := newJSScriptCommand("shebang.js", scriptPath, nil, scriptPeekInfo{kind: scriptKindJS})
	cmd.store = "memory"
	cmd.session = t.Name()
	cmd.testMode = true

	var stdout, stderr bytes.Buffer
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd.ctxFactory = func() (context.Context, context.CancelFunc) { return ctx, cancel }

	if err := cmd.Execute(nil, &stdout, &stderr); err != nil {
		t.Fatalf("Execute: %v\nstdout=%q\nstderr=%q", err, stdout.String(), stderr.String())
	}

	output := stdout.String()
	if !strings.Contains(output, "shebang works") {
		t.Fatalf("expected output from shebang script, got stdout=%q stderr=%q", output, stderr.String())
	}
}

// ---------------------------------------------------------------------------
// jsScriptCommand.SetupFlags tests
// ---------------------------------------------------------------------------

func TestJSScriptCommand_SetupFlags_Defaults(t *testing.T) {
	t.Parallel()

	cmd := newJSScriptCommand("test.js", "/path/test.js", nil, scriptPeekInfo{kind: scriptKindJS})

	fs := flag.NewFlagSet("test.js", flag.ContinueOnError)
	cmd.SetupFlags(fs)

	// Verify flags exist.
	if f := fs.Lookup("i"); f == nil {
		t.Error("expected -i flag")
	}
	if f := fs.Lookup("test"); f == nil {
		t.Error("expected --test flag")
	}
	if f := fs.Lookup("session"); f == nil {
		t.Error("expected --session flag")
	}
}

func TestJSScriptCommand_SetupFlags_ShebangDefaults(t *testing.T) {
	t.Parallel()

	peek := scriptPeekInfo{
		kind:        scriptKindJS,
		interactive: true,
		testMode:    true,
	}
	cmd := newJSScriptCommand("game.js", "/path/game.js", nil, peek)

	fs := flag.NewFlagSet("game.js", flag.ContinueOnError)
	cmd.SetupFlags(fs)

	// Shebang -i should make interactive default to true.
	if !cmd.interactive {
		t.Error("expected interactive=true from shebang")
	}
	// Shebang --test should make testMode default to true.
	if !cmd.testMode {
		t.Error("expected testMode=true from shebang")
	}
	if f := fs.Lookup("test"); f != nil && f.DefValue != "true" {
		t.Errorf("expected --test DefValue='true', got %q", f.DefValue)
	}
}

// ---------------------------------------------------------------------------
// End-to-end: registry.Get → jsScriptCommand.Execute
// ---------------------------------------------------------------------------

func TestRegistry_Get_ThenExecute_JSScript(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns JS runtime")
	}
	t.Parallel()

	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "e2e.js")
	content := "ctx.log('e2e ok');"
	if err := os.WriteFile(scriptPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	r := &Registry{
		commands:    make(map[string]Command),
		scriptPaths: []string{dir},
		config:      nil,
	}

	cmd, err := r.Get("e2e.js")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	jsCmd, ok := cmd.(*jsScriptCommand)
	if !ok {
		t.Fatalf("expected *jsScriptCommand, got %T", cmd)
	}

	jsCmd.store = "memory"
	jsCmd.session = t.Name()
	jsCmd.testMode = true

	var stdout, stderr bytes.Buffer
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	jsCmd.ctxFactory = func() (context.Context, context.CancelFunc) { return ctx, cancel }

	if err := jsCmd.Execute(nil, &stdout, &stderr); err != nil {
		t.Fatalf("Execute: %v\nstdout=%q\nstderr=%q", err, stdout.String(), stderr.String())
	}

	output := stdout.String()
	if !strings.Contains(output, "e2e ok") {
		t.Fatalf("expected 'e2e ok' in output, got stdout=%q stderr=%q", output, stderr.String())
	}
}
