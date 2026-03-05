package command

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/joeycumines/one-shot-man/internal/config"
	"github.com/joeycumines/one-shot-man/internal/scripting"
)

// ---------------------------------------------------------------------------
// loadChunkEngine creates a scripting engine that loads ONLY the specified
// pr-split chunk files (not the monolith). This enables independent testing
// of each chunk.
//
// chunkNames should be like: "00_core", "01_analysis", etc.
// Returns an evalJS function that evaluates JS expressions/statements on
// the engine's event loop, properly handling Promises and async/await.
// ---------------------------------------------------------------------------
func loadChunkEngine(t testing.TB, overrides map[string]interface{}, chunkNames ...string) func(string) (interface{}, error) {
	t.Helper()

	var stdout safeBuffer
	var stderr bytes.Buffer

	b := scriptCommandBase{
		config:   config.NewConfig(),
		store:    "memory",
		session:  t.Name(),
		logLevel: "info",
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	engine, cleanup, err := b.PrepareEngine(ctx, &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)

	// Set up globals matching PrSplitCommand.Execute.
	jsConfig := map[string]interface{}{
		"baseBranch":    "main",
		"strategy":      "directory",
		"maxFiles":      10,
		"branchPrefix":  "split/",
		"verifyCommand": "true",
		"dryRun":        false,
		"jsonOutput":    false,
	}
	for k, v := range overrides {
		jsConfig[k] = v
	}
	// B00-safety: ensure a dir is always set so resolveDir never falls back
	// to process CWD which would target the real repository.
	// loadChunkEngine is used by chunk-level tests and loadTUIEngine — none of
	// which os.Chdir to a test repo, so injecting t.TempDir() is safe.
	if _, ok := jsConfig["dir"]; !ok {
		jsConfig["dir"] = t.TempDir()
	}

	engine.SetGlobal("config", map[string]interface{}{"name": "pr-split"})
	engine.SetGlobal("prSplitConfig", jsConfig)

	// Build chunk name→source lookup.
	chunkMap := map[string]*string{}
	for _, c := range prSplitChunks {
		chunkMap[c.name] = c.source
	}

	// Load requested chunks in order.
	for _, name := range chunkNames {
		src, ok := chunkMap[name]
		if !ok {
			t.Fatalf("unknown chunk name %q", name)
		}
		script := engine.LoadScriptFromString("pr-split/"+name, *src)
		if err := engine.ExecuteScript(script); err != nil {
			t.Fatalf("failed to load chunk %s: %v", name, err)
		}
	}

	// Return evalJS (same implementation as the full engine version).
	return makeEvalJS(t, engine, 30*time.Second)
}

// makeEvalJS creates an evalJS function from a scripting.Engine.
// This is a standalone helper reusable by any chunk test.
func makeEvalJS(t testing.TB, engine *scripting.Engine, timeout time.Duration) func(string) (interface{}, error) {
	t.Helper()

	return func(js string) (interface{}, error) {
		done := make(chan struct{})
		var result interface{}
		var resultErr error

		submitErr := engine.Loop().Submit(func() {
			vm := engine.Runtime()

			// Async path: if JS contains 'await', wrap in async IIFE.
			if strings.Contains(js, "await ") {
				_ = vm.Set("__evalResult", func(val interface{}) {
					result = val
					close(done)
				})
				_ = vm.Set("__evalError", func(msg string) {
					resultErr = errors.New(msg)
					close(done)
				})
				wrapped := "(async function() {\n\ttry {\n\t\tvar __res = " + js + ";\n\t\tif (__res && typeof __res.then === 'function') { __res = await __res; }\n\t\t__evalResult(__res);\n\t} catch(e) {\n\t\t__evalError(e.message || String(e));\n\t}\n})();"
				if _, runErr := vm.RunString(wrapped); runErr != nil {
					resultErr = runErr
					close(done)
				}
				return
			}

			// Synchronous: run directly.
			val, err := vm.RunString(js)
			if err != nil {
				resultErr = err
				close(done)
				return
			}

			// Check if result is a Promise.
			if val != nil && !goja.IsUndefined(val) && !goja.IsNull(val) {
				obj := val.ToObject(vm)
				if obj != nil {
					thenProp := obj.Get("then")
					if thenProp != nil && !goja.IsUndefined(thenProp) {
						if thenFn, ok := goja.AssertFunction(thenProp); ok {
							onFulfilled := vm.ToValue(func(call goja.FunctionCall) goja.Value {
								result = call.Argument(0).Export()
								close(done)
								return goja.Undefined()
							})
							onRejected := vm.ToValue(func(call goja.FunctionCall) goja.Value {
								resultErr = fmt.Errorf("promise rejected: %v", call.Argument(0).Export())
								close(done)
								return goja.Undefined()
							})
							thenResult, thenErr := thenFn(val, onFulfilled)
							if thenErr != nil {
								resultErr = thenErr
								close(done)
								return
							}
							thenObj := thenResult.ToObject(vm)
							catchProp := thenObj.Get("catch")
							if catchFn, ok := goja.AssertFunction(catchProp); ok {
								if _, catchErr := catchFn(thenResult, onRejected); catchErr != nil {
									resultErr = catchErr
									close(done)
								}
							}
							return
						}
					}
				}
			}

			if val != nil {
				result = val.Export()
			}
			close(done)
		})
		if submitErr != nil {
			return nil, submitErr
		}

		select {
		case <-done:
			return result, resultErr
		case <-time.After(timeout):
			return nil, fmt.Errorf("evalJS timed out after %s", timeout)
		}
	}
}

// ===========================================================================
//  Chunk 00: Core — Tests
// ===========================================================================

// TestChunk00_Initialization verifies that chunk 00 initializes
// globalThis.prSplit with the expected structure.
func TestChunk00_Initialization(t *testing.T) {
	evalJS := loadChunkEngine(t, nil, "00_core")

	// globalThis.prSplit must exist and be an object.
	val, err := evalJS(`typeof globalThis.prSplit`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "object" {
		t.Fatalf("expected globalThis.prSplit to be 'object', got %v", val)
	}

	// _state must exist.
	val, err = evalJS(`typeof globalThis.prSplit._state`)
	if err != nil {
		t.Fatal(err)
	}
	if val != "object" {
		t.Errorf("expected _state to be 'object', got %v", val)
	}

	// _modules must exist with exec.
	val, err = evalJS(`typeof globalThis.prSplit._modules.exec`)
	if err != nil {
		t.Fatal(err)
	}
	if val != "object" {
		t.Errorf("expected _modules.exec to be 'object', got %v", val)
	}
}

// TestChunk00_ShellQuote tests the shellQuote helper with various inputs.
func TestChunk00_ShellQuote(t *testing.T) {
	evalJS := loadChunkEngine(t, nil, "00_core")

	tests := []struct {
		input    string
		expected string
	}{
		{"hello", "'hello'"},
		{"it's", `'it'\''s'`},
		{"", "''"},
		{"a b", "'a b'"},
		{`"double"`, `'"double"'`},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			// JSON.stringify to get exact string value.
			val, err := evalJS(fmt.Sprintf(`globalThis.prSplit._shellQuote(%q)`, tc.input))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if val != tc.expected {
				t.Errorf("shellQuote(%q) = %v, want %q", tc.input, val, tc.expected)
			}
		})
	}
}

// TestChunk00_Dirname tests the dirname helper at various depths.
func TestChunk00_Dirname(t *testing.T) {
	evalJS := loadChunkEngine(t, nil, "00_core")

	tests := []struct {
		path     string
		depth    int
		expected string
	}{
		{"internal/command/pr_split.go", 1, "internal"},
		{"internal/command/pr_split.go", 2, "internal/command"},
		{"internal/command/pr_split.go", 3, "internal/command"},
		{"file.go", 1, "."},
	}

	for _, tc := range tests {
		name := fmt.Sprintf("%s_d%d", tc.path, tc.depth)
		t.Run(name, func(t *testing.T) {
			val, err := evalJS(fmt.Sprintf(`globalThis.prSplit._dirname(%q, %d)`, tc.path, tc.depth))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if val != tc.expected {
				t.Errorf("dirname(%q, %d) = %v, want %q", tc.path, tc.depth, val, tc.expected)
			}
		})
	}
}

// TestChunk00_FileExtension tests the fileExtension helper.
func TestChunk00_FileExtension(t *testing.T) {
	evalJS := loadChunkEngine(t, nil, "00_core")

	tests := []struct {
		path     string
		expected string
	}{
		{"file.go", ".go"},
		{"path/to/file.js", ".js"},
		{"Makefile", ""},
		{".gitignore", ""},
		{"archive.tar.gz", ".gz"},
	}

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			val, err := evalJS(fmt.Sprintf(`globalThis.prSplit._fileExtension(%q)`, tc.path))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if val != tc.expected {
				t.Errorf("fileExtension(%q) = %v, want %q", tc.path, val, tc.expected)
			}
		})
	}
}

// TestChunk00_SanitizeBranchName tests special character replacement.
func TestChunk00_SanitizeBranchName(t *testing.T) {
	evalJS := loadChunkEngine(t, nil, "00_core")

	tests := []struct {
		input    string
		expected string
	}{
		{"feature/add-login", "feature/add-login"},
		{"fix bug #123", "fix-bug--123"},
		{"hello world!", "hello-world-"},
		{"a_b/c-d", "a_b/c-d"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			val, err := evalJS(fmt.Sprintf(`globalThis.prSplit._sanitizeBranchName(%q)`, tc.input))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if val != tc.expected {
				t.Errorf("sanitizeBranchName(%q) = %v, want %q", tc.input, val, tc.expected)
			}
		})
	}
}

// TestChunk00_PadIndex tests zero-padding behavior.
func TestChunk00_PadIndex(t *testing.T) {
	evalJS := loadChunkEngine(t, nil, "00_core")

	tests := []struct {
		input    int
		expected string
	}{
		{0, "00"},
		{1, "01"},
		{9, "09"},
		{10, "10"},
		{99, "99"},
		{100, "100"},
	}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("%d", tc.input), func(t *testing.T) {
			val, err := evalJS(fmt.Sprintf(`globalThis.prSplit._padIndex(%d)`, tc.input))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if val != tc.expected {
				t.Errorf("padIndex(%d) = %v, want %q", tc.input, val, tc.expected)
			}
		})
	}
}

// TestChunk00_GitExec tests gitExec with a real temp git repo.
func TestChunk00_GitExec(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses Unix paths; skipping on Windows")
	}

	dir := t.TempDir()
	runGitCmd(t, dir, "init")
	runGitCmd(t, dir, "symbolic-ref", "HEAD", "refs/heads/main")
	runGitCmd(t, dir, "config", "user.email", "test@test.com")
	runGitCmd(t, dir, "config", "user.name", "Test")

	evalJS := loadChunkEngine(t, nil, "00_core")

	// Success case: git rev-parse --git-dir in the temp repo.
	js := fmt.Sprintf(`(function() {
		var r = globalThis.prSplit._gitExec(%q, ['rev-parse', '--git-dir']);
		return JSON.stringify({code: r.code, stdout: r.stdout.trim()});
	})()`, dir)

	val, err := evalJS(js)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s, ok := val.(string)
	if !ok {
		t.Fatalf("expected string, got %T: %v", val, val)
	}
	if !strings.Contains(s, `"code":0`) {
		t.Errorf("expected code 0, got: %s", s)
	}
	if !strings.Contains(s, `"stdout":".git"`) {
		t.Errorf("expected stdout containing .git, got: %s", s)
	}

	// Failure case: git command that should fail.
	js = fmt.Sprintf(`(function() {
		var r = globalThis.prSplit._gitExec(%q, ['log', '--oneline', '-1']);
		return r.code;
	})()`, dir)
	val, err = evalJS(js)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Empty repo has no commits so 'git log' should fail (code != 0).
	code, _ := val.(int64)
	if code == 0 {
		t.Error("expected non-zero exit code for git log in empty repo")
	}
}

// TestChunk00_GitAddChangedFiles tests staging behavior with a real git repo.
func TestChunk00_GitAddChangedFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses Unix paths; skipping on Windows")
	}

	dir := t.TempDir()
	runGitCmd(t, dir, "init")
	runGitCmd(t, dir, "symbolic-ref", "HEAD", "refs/heads/main")
	runGitCmd(t, dir, "config", "user.email", "test@test.com")
	runGitCmd(t, dir, "config", "user.name", "Test")

	// Create initial commit.
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitCmd(t, dir, "add", "-A")
	runGitCmd(t, dir, "commit", "-m", "initial")

	// Create a new untracked file and modify existing one.
	if err := os.WriteFile(filepath.Join(dir, "new.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Updated\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Also create a tool artifact that should be excluded.
	if err := os.WriteFile(filepath.Join(dir, ".pr-split-plan.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	evalJS := loadChunkEngine(t, nil, "00_core")

	// Call gitAddChangedFiles.
	_, err := evalJS(fmt.Sprintf(`globalThis.prSplit._gitAddChangedFiles(%q)`, dir))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check what was staged.
	staged := runGitCmd(t, dir, "diff", "--cached", "--name-only")

	if !strings.Contains(staged, "new.go") {
		t.Error("expected new.go to be staged")
	}
	if !strings.Contains(staged, "README.md") {
		t.Error("expected README.md to be staged")
	}
	if strings.Contains(staged, ".pr-split-plan.json") {
		t.Error("expected .pr-split-plan.json to be excluded from staging")
	}
}

// TestChunk00_RuntimeConfig verifies the runtime config object picks up
// injected prSplitConfig values and applies defaults.
func TestChunk00_RuntimeConfig(t *testing.T) {
	overrides := map[string]interface{}{
		"baseBranch":   "develop",
		"strategy":     "extension",
		"maxFiles":     20,
		"branchPrefix": "pr/",
		"dryRun":       true,
		"retryBudget":  5,
	}
	evalJS := loadChunkEngine(t, overrides, "00_core")

	tests := []struct {
		field    string
		expected interface{}
	}{
		{"runtime.baseBranch", "develop"},
		{"runtime.strategy", "extension"},
		{"runtime.maxFiles", int64(20)},
		{"runtime.branchPrefix", "pr/"},
		{"runtime.dryRun", true},
		{"runtime.retryBudget", int64(5)},
		// Defaults for non-overridden fields.
		{"runtime.jsonOutput", false},
		{"runtime.mode", "heuristic"},
		{"runtime.view", "toggle"},
		{"runtime.autoMerge", false},
		{"runtime.mergeMethod", "squash"},
	}

	for _, tc := range tests {
		t.Run(tc.field, func(t *testing.T) {
			val, err := evalJS(fmt.Sprintf(`globalThis.prSplit.%s`, tc.field))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if val != tc.expected {
				t.Errorf("runtime.%s = %v (%T), want %v (%T)", tc.field, val, val, tc.expected, tc.expected)
			}
		})
	}
}

// TestChunk00_IsCancelled verifies the cooperative cancellation functions.
func TestChunk00_IsCancelled(t *testing.T) {
	evalJS := loadChunkEngine(t, nil, "00_core")

	// Without autoSplitTUI, all should return false.
	for _, fn := range []string{"isCancelled", "_isPaused", "_isForceCancelled"} {
		val, err := evalJS(fmt.Sprintf(`globalThis.prSplit.%s()`, fn))
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", fn, err)
		}
		if val != false {
			t.Errorf("%s() = %v, want false (no autoSplitTUI)", fn, val)
		}
	}
}

// TestChunk00_ScopedVerifyCommand tests the scoped verification logic.
func TestChunk00_ScopedVerifyCommand(t *testing.T) {
	evalJS := loadChunkEngine(t, nil, "00_core")

	// All Go files → scoped go test command.
	val, err := evalJS(`globalThis.prSplit.scopedVerifyCommand(
		['internal/cmd/foo.go', 'internal/cmd/bar.go', 'internal/pkg/baz.go'],
		'make'
	)`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s, ok := val.(string)
	if !ok {
		t.Fatalf("expected string, got %T: %v", val, val)
	}
	// Should be scoped to the two package dirs.
	if !strings.HasPrefix(s, "go test -race") {
		t.Errorf("expected 'go test -race ...', got: %s", s)
	}
	if !strings.Contains(s, "./internal/cmd/...") {
		t.Errorf("expected ./internal/cmd/... in result, got: %s", s)
	}
	if !strings.Contains(s, "./internal/pkg/...") {
		t.Errorf("expected ./internal/pkg/... in result, got: %s", s)
	}

	// Mixed files (Go + non-Go) → fallback.
	val, err = evalJS(`globalThis.prSplit.scopedVerifyCommand(
		['internal/cmd/foo.go', 'README.md'],
		'make'
	)`)
	if err != nil {
		t.Fatal(err)
	}
	if val != "make" {
		t.Errorf("expected fallback 'make' for mixed files, got: %v", val)
	}

	// Custom verify command → not scopable.
	val, err = evalJS(`globalThis.prSplit.scopedVerifyCommand(
		['internal/cmd/foo.go'],
		'echo ok'
	)`)
	if err != nil {
		t.Fatal(err)
	}
	if val != "echo ok" {
		t.Errorf("expected custom command preserved, got: %v", val)
	}

	// Empty files → fallback.
	val, err = evalJS(`globalThis.prSplit.scopedVerifyCommand([], 'make')`)
	if err != nil {
		t.Fatal(err)
	}
	if val != "make" {
		t.Errorf("expected fallback for empty files, got: %v", val)
	}
}

// TestChunk00_StyleDegracesGracefully tests that style helpers work
// even when lipgloss is not available (returns plain text).
func TestChunk00_StyleDegracesGracefully(t *testing.T) {
	evalJS := loadChunkEngine(t, nil, "00_core")

	// Style helpers should at minimum return the input string.
	for _, fn := range []string{"success", "error", "warning", "info", "header", "dim", "bold"} {
		val, err := evalJS(fmt.Sprintf(`globalThis.prSplit._style.%s("test")`, fn))
		if err != nil {
			t.Fatalf("_style.%s: unexpected error: %v", fn, err)
		}
		s, ok := val.(string)
		if !ok {
			t.Fatalf("_style.%s: expected string, got %T", fn, val)
		}
		// The rendered value must contain "test" (even if wrapped in ANSI).
		if !strings.Contains(s, "test") {
			t.Errorf("_style.%s(\"test\") = %q, does not contain 'test'", fn, s)
		}
	}

	// Progress bar should produce something meaningful.
	val, err := evalJS(`globalThis.prSplit._style.progressBar(3, 10, 10)`)
	if err != nil {
		t.Fatal(err)
	}
	s, _ := val.(string)
	if !strings.Contains(s, "3/10") {
		t.Errorf("progressBar(3,10,10) = %q, expected '3/10' in output", s)
	}
}

// TestChunk00_CommandNameFromConfig verifies COMMAND_NAME is derived from
// the Go-injected config global.
func TestChunk00_CommandNameFromConfig(t *testing.T) {
	evalJS := loadChunkEngine(t, nil, "00_core")

	val, err := evalJS(`globalThis.prSplit._COMMAND_NAME`)
	if err != nil {
		t.Fatal(err)
	}
	if val != "pr-split" {
		t.Errorf("expected 'pr-split', got %v", val)
	}

	val, err = evalJS(`globalThis.prSplit._MODE_NAME`)
	if err != nil {
		t.Fatal(err)
	}
	if val != "pr-split" {
		t.Errorf("expected 'pr-split', got %v", val)
	}
}
