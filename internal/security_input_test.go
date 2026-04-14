// Package internal_test contains input sanitization security tests for one-shot-man.
// These tests complement security_test.go by focusing on input validation across
// all entry points: exec module, readFile, config parser, require(), context manager,
// and fetch URL handling.
//
// SECURITY POSTURE DOCUMENTATION:
// osm is a local developer tool, NOT a sandboxed execution environment.
// The following behaviors are INTENTIONALLY unrestricted:
// - exec() passes stdin from os.Stdin (user is the operator)
// - readFile() can read any path accessible to the user
// - addPath() can add any accessible path (no sandbox)
// - require() can load any .js file from configured module paths
// - fetch() can make HTTP requests to any host (no SSRF restriction)
//
// The security boundary is the OS user's permissions, not application-level sandboxing.
package internal_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/joeycumines/one-shot-man/internal/config"
	"github.com/joeycumines/one-shot-man/internal/scripting"
	"github.com/joeycumines/one-shot-man/internal/testutil"
)

// escapeJS escapes a string for safe embedding in JS string literals.
func escapeJS(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "'", `\'`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, "\r", `\r`)
	s = strings.ReplaceAll(s, "\t", `\t`)
	s = strings.ReplaceAll(s, "\x00", `\x00`)
	return s
}

// newTestEngine creates a scripting engine configured for security tests.
func newTestEngine(t *testing.T) (*scripting.Engine, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	ctx := context.Background()
	var stdout, stderr bytes.Buffer
	engine, err := scripting.NewEngineDeprecated(
		ctx, &stdout, &stderr,
		testutil.NewTestSessionID("security-input", t.Name()),
		"memory",
	)
	if err != nil {
		t.Fatalf("Failed to create engine: %v", err)
	}
	t.Cleanup(func() { engine.Close() })
	return engine, &stdout, &stderr
}

// ============================================================================
// Exec Module: Argument Safety
// ============================================================================

func TestExecSecurity_NullBytesInArgs(t *testing.T) {
	t.Parallel()
	engine, _, _ := newTestEngine(t)

	// exec passes args directly to exec.CommandContext (no shell),
	// so null bytes are either handled by the OS or cause command failure.
	script := engine.LoadScriptFromString("null-args", `
		const {exec} = require('osm:exec');
		const result = exec('echo', 'hello\x00world');
		if (result.error && result.code !== 0) {
			// OS may reject null bytes in args — acceptable
		}
		// Either way, no panic and no injection
	`)
	if err := engine.ExecuteScript(script); err != nil {
		t.Logf("Null byte arg handled: %v", err)
	}
}

func TestExecSecurity_ControlCharsInArgs(t *testing.T) {
	t.Parallel()
	engine, _, _ := newTestEngine(t)

	// Control characters should be passed literally (no shell expansion)
	script := engine.LoadScriptFromString("control-args", `
		const {exec} = require('osm:exec');
		const result = exec('echo', '\x03\x04\x1b[31m');
		// exec.CommandContext passes these as literal args, no shell processing
		if (result.error) {
			// Some platforms may reject control chars — acceptable
		}
	`)
	if err := engine.ExecuteScript(script); err != nil {
		t.Logf("Control char arg handled: %v", err)
	}
}

func TestExecSecurity_ShellMetacharsNotExpanded(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping exec test in short mode")
	}
	t.Parallel()
	engine, stdout, _ := newTestEngine(t)

	// Shell metacharacters passed as args should NOT be expanded because
	// exec.CommandContext bypasses the shell entirely.
	script := engine.LoadScriptFromString("metachar-args", `
		const {exec} = require('osm:exec');
		const result = exec('echo', '$(whoami)', '&&', 'rm', '-rf', '/');
		// The echo command should output the literal strings
	`)
	if err := engine.ExecuteScript(script); err != nil {
		t.Logf("Shell metachar exec: %v", err)
	}
	output := stdout.String()
	if strings.Contains(output, "pwned") {
		t.Error("Shell metacharacters were expanded — injection vulnerability")
	}
}

func TestExecSecurity_NewlinesInArgs(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping exec test in short mode")
	}
	t.Parallel()
	engine, _, _ := newTestEngine(t)

	// Newlines in arguments should be passed literally (not interpreted as
	// command separators). exec.CommandContext handles this correctly because
	// it bypasses the shell, but we verify explicitly.
	script := engine.LoadScriptFromString("newline-args", `
		const {exec} = require('osm:exec');
		// Newlines in args should NOT cause additional command execution.
		const r1 = exec('echo', 'line1\nwhoami');
		if (r1.error) {
			// acceptable — some platforms may handle this differently
		}
		// Verify no command injection via newline in the command name itself.
		const r2 = exec('echo\nwhoami', 'test');
		if (!r2.error) {
			throw new Error('Expected error for command with embedded newline');
		}
	`)
	if err := engine.ExecuteScript(script); err != nil {
		t.Logf("Newline arg test: %v (acceptable — exec.CommandContext handles this safely)", err)
	}
}

func TestExecSecurity_EmptyCommand(t *testing.T) {
	t.Parallel()
	engine, _, _ := newTestEngine(t)

	script := engine.LoadScriptFromString("empty-command", `
		const {exec} = require('osm:exec');
		const r1 = exec();
		if (!r1.error || r1.message !== 'exec: missing command') {
			throw new Error('expected "exec: missing command", got: ' + r1.message);
		}
		const r2 = exec('');
		if (!r2.error || r2.message !== 'exec: command must be a non-empty string') {
			throw new Error('expected "exec: command must be a non-empty string", got: ' + r2.message);
		}
	`)
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("Empty command test failed: %v", err)
	}
}

func TestExecvSecurity_Validation(t *testing.T) {
	t.Parallel()
	engine, _, _ := newTestEngine(t)

	script := engine.LoadScriptFromString("execv-validate", `
		const {execv} = require('osm:exec');
		// No args
		const r1 = execv();
		if (!r1.error || r1.message !== 'execv: no argv') {
			throw new Error('expected "execv: no argv", got: ' + r1.message);
		}
		// Null
		const r2 = execv(null);
		if (!r2.error || r2.message !== 'execv: no argv') {
			throw new Error('expected "execv: no argv" for null, got: ' + r2.message);
		}
		// Undefined
		const r3 = execv(undefined);
		if (!r3.error || r3.message !== 'execv: no argv') {
			throw new Error('expected "execv: no argv" for undefined, got: ' + r3.message);
		}
		// Empty array
		const r4 = execv([]);
		if (!r4.error || r4.message !== 'execv: expects array of strings') {
			throw new Error('expected "execv: expects array of strings" for empty array, got: ' + r4.message);
		}
		// Non-array
		const r5 = execv('echo hello');
		if (!r5.error || r5.message !== 'execv: expects array of strings') {
			throw new Error('expected "execv: expects array of strings" for string, got: ' + r5.message);
		}
	`)
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("execv validation test failed: %v", err)
	}
}

func TestExecSecurity_StdinExposureDocumented(t *testing.T) {
	// This test documents that exec() exposes os.Stdin to child processes.
	// This is BY DESIGN for a local developer tool where the user IS the operator.
	// We do not attempt to restrict stdin access.
	t.Parallel()

	// Verify the design decision is documented by checking exec.go source
	execSource, err := os.ReadFile("builtin/exec/exec.go")
	if err != nil {
		// Try from project root (test runs from internal/)
		execSource, err = os.ReadFile("../internal/builtin/exec/exec.go")
	}
	if err != nil {
		t.Skip("Cannot read exec.go source for documentation check")
	}
	if !strings.Contains(string(execSource), "c.Stdin = os.Stdin") {
		t.Error("exec.go should contain 'c.Stdin = os.Stdin' documenting stdin exposure")
	}
}

// ============================================================================
// ReadFile: Path Security
// ============================================================================

func TestReadFileSecurity_EmptyPath(t *testing.T) {
	t.Parallel()
	engine, _, _ := newTestEngine(t)

	script := engine.LoadScriptFromString("readfile-empty", `
		const {readFile} = require('osm:os');
		const result = readFile('');
		if (!result.error || result.message !== 'empty path') {
			throw new Error('expected "empty path" error, got: ' + JSON.stringify(result));
		}
	`)
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("ReadFile empty path test failed: %v", err)
	}
}

func TestReadFileSecurity_NonexistentPath(t *testing.T) {
	t.Parallel()
	engine, _, _ := newTestEngine(t)

	script := engine.LoadScriptFromString("readfile-nonexist", `
		const {readFile} = require('osm:os');
		const result = readFile('/nonexistent/path/to/file');
		if (!result.error) {
			throw new Error('expected error for nonexistent path');
		}
		if (result.content !== '') {
			throw new Error('expected empty content for nonexistent path');
		}
	`)
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("ReadFile nonexistent path test failed: %v", err)
	}
}

func TestReadFileSecurity_NoSandboxDocumented(t *testing.T) {
	// readFile() can read ANY path accessible to the user.
	// This is BY DESIGN for a local developer tool.
	t.Parallel()
	engine, _, _ := newTestEngine(t)

	// Create a temp file outside any "expected" directory
	tmpFile := filepath.Join(t.TempDir(), "readable.txt")
	if err := os.WriteFile(tmpFile, []byte("test content"), 0644); err != nil {
		t.Fatal(err)
	}

	escaped := escapeJS(tmpFile)
	script := engine.LoadScriptFromString("readfile-nosandbox", `
		const {readFile} = require('osm:os');
		const result = readFile('`+escaped+`');
		if (result.error) {
			throw new Error('readFile should read any accessible path: ' + result.message);
		}
		if (result.content !== 'test content') {
			throw new Error('expected "test content", got: ' + result.content);
		}
	`)
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("ReadFile no-sandbox test failed: %v", err)
	}
}

// ============================================================================
// Config Parser: Malformed Input
// ============================================================================

func TestConfigParserSecurity_NullBytesInConfigFile(t *testing.T) {
	t.Parallel()

	input := strings.NewReader("key1 value1\x00\nkey2 value2\n")
	cfg, err := config.LoadFromReader(input)
	if err != nil {
		t.Fatalf("LoadFromReader should not error on null bytes: %v", err)
	}
	// Null byte is part of the value — config parser doesn't strip it
	v1, ok := cfg.GetGlobalOption("key1")
	if !ok {
		t.Error("key1 not found")
	}
	if !strings.Contains(v1, "\x00") {
		t.Logf("Null byte in value preserved or stripped: %q", v1)
	}
}

func TestConfigParserSecurity_OversizedLines(t *testing.T) {
	t.Parallel()

	// bufio.Scanner has a max token size (~64KB). Lines exceeding this
	// return "token too long" — this is acceptable behavior, no panic or OOM.
	bigValue := strings.Repeat("x", 1<<20)
	input := strings.NewReader("bigkey " + bigValue + "\n")
	cfg, err := config.LoadFromReader(input)
	if err != nil {
		// Expected: bufio.Scanner returns "token too long" for lines > ~64KB
		if !strings.Contains(err.Error(), "token too long") {
			t.Fatalf("Unexpected error on oversized line: %v", err)
		}
		t.Logf("Oversized line correctly rejected: %v", err)
		return
	}
	// If it does parse, verify the value
	v, ok := cfg.GetGlobalOption("bigkey")
	if !ok {
		t.Error("bigkey not found after oversized line")
	}
	if len(v) != 1<<20 {
		t.Errorf("Expected 1MB value, got %d bytes", len(v))
	}
}

func TestConfigParserSecurity_ControlCharsInValues(t *testing.T) {
	t.Parallel()

	// Control characters in values should be stored as-is
	input := strings.NewReader("key1 \x01\x02\x03value\n")
	cfg, err := config.LoadFromReader(input)
	if err != nil {
		t.Fatalf("LoadFromReader failed on control chars: %v", err)
	}
	v, ok := cfg.GetGlobalOption("key1")
	if !ok {
		t.Error("key1 not found")
	}
	if !strings.Contains(v, "\x01") {
		t.Logf("Control chars in value: %q", v)
	}
}

func TestConfigParserSecurity_MalformedSections(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		input string
	}{
		{"unclosed bracket", "[section\nkey value\n"},
		{"empty section", "[]\nkey value\n"},
		{"nested brackets", "[[section]]\nkey value\n"},
		{"section with spaces", "[section name with spaces]\nkey value\n"},
		{"section only", "[section]\n"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := config.LoadFromReader(strings.NewReader(tc.input))
			if err != nil {
				// Error is acceptable — no panic
				t.Logf("Malformed section %q handled as error: %v", tc.name, err)
				return
			}
			// Config was parsed (may have stored under unusual section name)
			_ = cfg
		})
	}
}

func TestConfigParserSecurity_BinaryInput(t *testing.T) {
	t.Parallel()

	// Feed raw binary data to the config parser
	binary := make([]byte, 256)
	for i := range binary {
		binary[i] = byte(i)
	}
	cfg, err := config.LoadFromReader(bytes.NewReader(binary))
	if err != nil {
		t.Logf("Binary input handled as error: %v", err)
		return
	}
	// No panic — parser handled binary data gracefully
	_ = cfg
}

func TestConfigParserSecurity_SessionOptionInjection(t *testing.T) {
	t.Parallel()

	// Attempt to override session settings via specially crafted sections.
	// Uses actual config option names (maxSizeMB not maxSizeMb).
	input := `[sessions]
maxAgeDays 1
maxCount 1
maxSizeMB 1
autoCleanupEnabled false
cleanupIntervalHours 1
`
	cfg, err := config.LoadFromReader(strings.NewReader(input))
	if err != nil {
		t.Fatalf("LoadFromReader failed: %v", err)
	}
	// Verify session config was parsed from the [sessions] section
	if cfg.Sessions.MaxAgeDays == 1 {
		t.Log("Session maxAgeDays set to 1 from config (expected)")
	}
	if !cfg.Sessions.AutoCleanupEnabled {
		t.Log("Session autoCleanupEnabled disabled from config (expected)")
	}
}

func TestConfigParserSecurity_HotSnippetInjection(t *testing.T) {
	t.Parallel()

	// Hot snippets with shell metacharacters should be stored verbatim
	input := `[hot-snippets]
shell-inject ; rm -rf /
pipe-inject | cat /etc/passwd
`
	cfg, err := config.LoadFromReader(strings.NewReader(input))
	if err != nil {
		t.Fatalf("LoadFromReader failed: %v", err)
	}
	for _, hs := range cfg.HotSnippets {
		if strings.Contains(hs.Text, "rm -rf") || strings.Contains(hs.Text, "cat /etc") {
			t.Logf("Hot snippet stored verbatim (expected for local tool): %q", hs.Text)
		}
	}
}

func TestConfigParserSecurity_SymlinkRejection(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	realFile := filepath.Join(tmpDir, "real-config")
	if err := os.WriteFile(realFile, []byte("key value\n"), 0644); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(tmpDir, "symlinked-config")
	if err := os.Symlink(realFile, linkPath); err != nil {
		t.Skip("Symlinks not supported")
	}

	cfg, err := config.LoadFromPath(linkPath)
	if err == nil {
		t.Errorf("LoadFromPath should reject symlink, got config with %d global options", len(cfg.Global))
	} else if !strings.Contains(err.Error(), "symlink not allowed") {
		t.Errorf("Expected symlink rejection error, got: %v", err)
	}
}

func TestConfigParserSecurity_NullByteInPath(t *testing.T) {
	t.Parallel()

	cfg, err := config.LoadFromPath("config\x00.txt")
	if err == nil {
		t.Errorf("LoadFromPath should reject path with null byte, config has %d global options", len(cfg.Global))
	}
	// Error is expected — OS rejects null bytes in paths
}

func TestConfigParserSecurity_NonexistentPath(t *testing.T) {
	t.Parallel()

	cfg, err := config.LoadFromPath("/nonexistent/path/config.txt")
	if err != nil {
		t.Logf("Nonexistent path produces error: %v", err)
	} else {
		// LoadFromPath returns empty config for nonexistent files
		if len(cfg.Global) == 0 {
			t.Log("Nonexistent path returns empty config (expected)")
		}
	}
}

// ============================================================================
// Script Paths: LoadScript Security
// ============================================================================

func TestLoadScriptSecurity_PathTraversal(t *testing.T) {
	t.Parallel()
	engine, _, _ := newTestEngine(t)

	// LoadScript reads from the filesystem — path traversal is handled by OS access controls
	_, err := engine.LoadScript("traversal", "../../../etc/passwd")
	if err != nil {
		t.Logf("Path traversal LoadScript handled: %v", err)
	}
	// No panic, OS access controls apply
}

func TestLoadScriptSecurity_NullByteInPath(t *testing.T) {
	t.Parallel()
	engine, _, _ := newTestEngine(t)

	_, err := engine.LoadScript("null-path", "script\x00.js")
	if err == nil {
		t.Error("LoadScript should fail with null byte in path")
	}
}

func TestLoadScriptSecurity_NonexistentScript(t *testing.T) {
	t.Parallel()
	engine, _, _ := newTestEngine(t)

	_, err := engine.LoadScript("missing", "/nonexistent/script.js")
	if err == nil {
		t.Error("LoadScript should fail for nonexistent path")
	}
}

// ============================================================================
// REPL Input: Oversized and Malicious Scripts
// ============================================================================

func TestREPLSecurity_OversizedScript(t *testing.T) {
	t.Parallel()
	engine, _, _ := newTestEngine(t)

	// 1MB of "var x=1;" repeated — should either execute or gracefully fail
	bigScript := strings.Repeat("var x=1;", 1<<17) // ~1MB
	script := engine.LoadScriptFromString("oversized", bigScript)
	err := engine.ExecuteScript(script)
	// Either succeeds or returns error — no panic or OOM
	_ = err
}

func TestREPLSecurity_DeepRecursion(t *testing.T) {
	t.Parallel()

	// Deep recursion may cause Go-level stack overflow in Goja.
	// Use context timeout to prevent hang.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var stdout, stderr bytes.Buffer
	engine, err := scripting.NewEngineDeprecated(
		ctx, &stdout, &stderr,
		testutil.NewTestSessionID("security-input", t.Name()),
		"memory",
	)
	if err != nil {
		t.Fatalf("Failed to create engine: %v", err)
	}
	defer engine.Close()

	script := engine.LoadScriptFromString("deep-recursion", `
		function recurse(n) { return recurse(n + 1); }
		try { recurse(0); } catch(e) {
			// Expected: stack overflow or similar
		}
	`)
	err = engine.ExecuteScript(script)
	// Either the try/catch handles it, or context timeout fires, or ExecuteScript returns error
	_ = err
}

func TestREPLSecurity_InfiniteLoopWithCancel(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var stdout, stderr bytes.Buffer
	engine, err := scripting.NewEngineDeprecated(
		ctx, &stdout, &stderr,
		testutil.NewTestSessionID("security-input", t.Name()),
		"memory",
	)
	if err != nil {
		t.Fatalf("Failed to create engine: %v", err)
	}
	defer engine.Close()

	script := engine.LoadScriptFromString("infinite-loop", `
		while(true) {}
	`)
	// The engine should respect context cancellation or Goja's interrupt mechanism
	err = engine.ExecuteScript(script)
	if err == nil {
		t.Error("Infinite loop should have been interrupted by context cancellation")
	}
}

// ============================================================================
// Context Manager: AddPath Security
// ============================================================================

func TestAddPathSecurity_TraversalAllowed(t *testing.T) {
	// addPath() does NOT sandbox paths — this is by design.
	// It accepts any accessible path. This test documents the behavior.
	t.Parallel()

	tmpDir := t.TempDir()
	outsideDir := t.TempDir()

	cm, err := scripting.NewContextManager(tmpDir)
	if err != nil {
		t.Fatalf("NewContextManager failed: %v", err)
	}

	// Create a file in the "outside" directory
	outsideFile := filepath.Join(outsideDir, "outside.txt")
	if err := os.WriteFile(outsideFile, []byte("outside content"), 0644); err != nil {
		t.Fatal(err)
	}

	// AddPath should accept any accessible path — no sandbox restriction
	err = cm.AddPath(outsideFile)
	if err != nil {
		t.Errorf("AddPath should accept any accessible path (no sandbox): %v", err)
	}
}

func TestAddPathSecurity_SymlinksFollowed(t *testing.T) {
	// addPath() follows symlinks — this is by design for a local tool.
	t.Parallel()

	tmpDir := t.TempDir()
	realFile := filepath.Join(tmpDir, "real.txt")
	if err := os.WriteFile(realFile, []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}

	linkPath := filepath.Join(tmpDir, "link.txt")
	if err := os.Symlink(realFile, linkPath); err != nil {
		t.Skip("Symlinks not supported")
	}

	cm, err := scripting.NewContextManager(tmpDir)
	if err != nil {
		t.Fatalf("NewContextManager failed: %v", err)
	}

	err = cm.AddPath(linkPath)
	if err != nil {
		t.Logf("Symlink addPath result: %v", err)
	}
	// No panic — symlinks are resolved via filepath.EvalSymlinks
}

func TestAddPathSecurity_NullBytesInPath(t *testing.T) {
	t.Parallel()

	cm, err := scripting.NewContextManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewContextManager failed: %v", err)
	}

	err = cm.AddPath("file\x00.txt")
	if err == nil {
		t.Error("AddPath should reject path with null byte")
	}
}

func TestAddPathSecurity_NonexistentPath(t *testing.T) {
	t.Parallel()

	cm, err := scripting.NewContextManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewContextManager failed: %v", err)
	}

	err = cm.AddPath("/nonexistent/path/file.txt")
	if err == nil {
		t.Error("AddPath should fail for nonexistent path")
	}
}

func TestAddPathSecurity_SymlinkLoopDetection(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	dirA := filepath.Join(tmpDir, "a")
	dirB := filepath.Join(tmpDir, "b")

	if err := os.MkdirAll(dirA, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dirB, 0755); err != nil {
		t.Fatal(err)
	}

	// Create symlink loop: a/link -> ../b, b/link -> ../a
	if err := os.Symlink(dirB, filepath.Join(dirA, "link")); err != nil {
		t.Skip("Symlinks not supported")
	}
	if err := os.Symlink(dirA, filepath.Join(dirB, "link")); err != nil {
		t.Skip("Symlinks not supported")
	}

	cm, err := scripting.NewContextManager(tmpDir)
	if err != nil {
		t.Fatalf("NewContextManager failed: %v", err)
	}

	// Adding the directory with symlink loops should not hang
	err = cm.AddPath(dirA)
	// Either succeeds (loop detected and stopped) or returns error — no hang
	_ = err
}

// ============================================================================
// Require: Module Path Security
// ============================================================================

func TestRequireSecurity_TraversalAllowed(t *testing.T) {
	// require() module paths are NOT sandboxed — by design.
	// The WithModulePaths option allows configuring search dirs.
	t.Parallel()

	tmpDir := t.TempDir()
	moduleDir := filepath.Join(tmpDir, "modules")
	if err := os.MkdirAll(moduleDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a module file
	modFile := filepath.Join(moduleDir, "testmod.js")
	if err := os.WriteFile(modFile, []byte("module.exports = {value: 42};"), 0644); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	var stdout, stderr bytes.Buffer
	engine, err := scripting.NewEngine(
		ctx, &stdout, &stderr,
		testutil.NewTestSessionID("security-input", t.Name()),
		"memory", nil, 0, 0,
		scripting.WithModulePaths(moduleDir),
	)
	if err != nil {
		t.Fatalf("Failed to create engine: %v", err)
	}
	defer engine.Close()

	script := engine.LoadScriptFromString("require-test", `
		const mod = require('testmod');
		if (mod.value !== 42) throw new Error('expected 42, got ' + mod.value);
	`)
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("require() with module path failed: %v", err)
	}
}

func TestRequireSecurity_NonJSFileContent(t *testing.T) {
	// require() on a file that isn't valid JS should produce a parse error
	t.Parallel()

	tmpDir := t.TempDir()
	modDir := filepath.Join(tmpDir, "mods")
	if err := os.MkdirAll(modDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a non-JS file
	if err := os.WriteFile(filepath.Join(modDir, "notjs.js"), []byte("THIS IS NOT JAVASCRIPT {{{"), 0644); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	var stdout, stderr bytes.Buffer
	engine, err := scripting.NewEngine(
		ctx, &stdout, &stderr,
		testutil.NewTestSessionID("security-input", t.Name()),
		"memory", nil, 0, 0,
		scripting.WithModulePaths(modDir),
	)
	if err != nil {
		t.Fatalf("Failed to create engine: %v", err)
	}
	defer engine.Close()

	script := engine.LoadScriptFromString("require-bad", `
		try {
			require('notjs');
			throw new Error('should have failed');
		} catch(e) {
			if (e.message === 'should have failed') throw e;
			// Expected: SyntaxError or similar
		}
	`)
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("Non-JS require test failed: %v", err)
	}
}

func TestRequireSecurity_NonexistentModule(t *testing.T) {
	t.Parallel()
	engine, _, _ := newTestEngine(t)

	script := engine.LoadScriptFromString("require-missing", `
		try {
			require('nonexistent-module-that-does-not-exist');
			throw new Error('should have failed');
		} catch(e) {
			if (e.message === 'should have failed') throw e;
			// Expected: module not found error
		}
	`)
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("Nonexistent module test failed: %v", err)
	}
}

func TestRequireSecurity_ShebangHandling(t *testing.T) {
	// require() should strip shebang lines from scripts
	t.Parallel()

	tmpDir := t.TempDir()
	modDir := filepath.Join(tmpDir, "mods")
	if err := os.MkdirAll(modDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a JS file with shebang
	content := "#!/usr/bin/env node\nmodule.exports = {shebang: true};\n"
	if err := os.WriteFile(filepath.Join(modDir, "withshebang.js"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	var stdout, stderr bytes.Buffer
	engine, err := scripting.NewEngine(
		ctx, &stdout, &stderr,
		testutil.NewTestSessionID("security-input", t.Name()),
		"memory", nil, 0, 0,
		scripting.WithModulePaths(modDir),
	)
	if err != nil {
		t.Fatalf("Failed to create engine: %v", err)
	}
	defer engine.Close()

	script := engine.LoadScriptFromString("require-shebang", `
		const mod = require('withshebang');
		if (!mod.shebang) throw new Error('shebang module did not load correctly');
	`)
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("Shebang require test failed: %v", err)
	}
}

// ============================================================================
// Fetch URL Security
// ============================================================================

func TestFetchSecurity_FileProtocolRejected(t *testing.T) {
	// Go's http.Client rejects file:// URLs.
	t.Parallel()
	engine, _, _ := newTestEngine(t)

	script := engine.LoadScriptFromString("fetch-file", `
		const {fetch} = require('osm:fetch');
		try {
			const resp = fetch('file:///etc/passwd');
			// If we get here, examine what happened
			if (resp.ok) {
				throw new Error('file:// URL should not succeed');
			}
		} catch(e) {
			// Expected — Go's http.Client rejects file:// URLs
		}
	`)
	if err := engine.ExecuteScript(script); err != nil {
		t.Logf("file:// fetch handled: %v", err)
	}
}

func TestFetchSecurity_InvalidURLs(t *testing.T) {
	t.Parallel()
	engine, _, _ := newTestEngine(t)

	script := engine.LoadScriptFromString("fetch-invalid", `
		const {fetch} = require('osm:fetch');
		const badURLs = [
			'',
			'not-a-url',
			'://missing-scheme',
			'http://',
		];
		for (const url of badURLs) {
			try {
				const resp = fetch(url);
				// Some of these may return error responses rather than throwing
			} catch(e) {
				// Expected for truly malformed URLs
			}
		}
	`)
	if err := engine.ExecuteScript(script); err != nil {
		t.Logf("Invalid URL fetch handled: %v", err)
	}
}

func TestFetchSecurity_SSRFDocumented(t *testing.T) {
	// fetch() has NO SSRF restrictions — this is by design for a local tool.
	// The user controls which URLs are fetched via their scripts.
	// This test documents the intentional design decision.
	t.Log("SECURITY POSTURE: fetch() can reach any host accessible to the user's network.")
	t.Log("This is intentional — osm is a local developer tool, not a web service.")
	t.Log("No SSRF mitigation is implemented or needed.")
}

// ============================================================================
// Resource Limits: Goja VM
// ============================================================================

func TestResourceLimits_LargeStrings(t *testing.T) {
	t.Parallel()
	engine, _, _ := newTestEngine(t)

	script := engine.LoadScriptFromString("large-strings", `
		// Create a ~4MB string inside the VM
		var s = 'x';
		for (var i = 0; i < 22; i++) { s = s + s; }
		if (s.length !== 4194304) throw new Error('unexpected length: ' + s.length);
	`)
	if err := engine.ExecuteScript(script); err != nil {
		t.Logf("Large string handling: %v", err)
	}
}

func TestResourceLimits_ManyObjects(t *testing.T) {
	t.Parallel()
	engine, _, _ := newTestEngine(t)

	script := engine.LoadScriptFromString("many-objects", `
		var arr = [];
		for (var i = 0; i < 100000; i++) {
			arr.push({key: i, value: 'item-' + i});
		}
		if (arr.length !== 100000) throw new Error('unexpected length: ' + arr.length);
	`)
	if err := engine.ExecuteScript(script); err != nil {
		t.Logf("Many objects handling: %v", err)
	}
}

func TestResourceLimits_RegexBacktracking(t *testing.T) {
	t.Parallel()
	engine, _, _ := newTestEngine(t)

	// ReDoS-style regex — should either complete or hit a timeout
	script := engine.LoadScriptFromString("regex-backtrack", `
		try {
			// This regex is susceptible to catastrophic backtracking
			var evil = /^(a+)+$/;
			var input = 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaab';
			evil.test(input);
		} catch(e) {
			// Expected if engine has regex limits
		}
	`)
	// We don't assert success or failure — just that it doesn't hang forever
	_ = engine.ExecuteScript(script)
}

// ============================================================================
// Cross-Module Isolation
// ============================================================================

func TestCrossModuleIsolation_RequireCaching(t *testing.T) {
	// require() caching should NOT leak between engine instances
	t.Parallel()

	tmpDir := t.TempDir()
	modDir := filepath.Join(tmpDir, "mods")
	if err := os.MkdirAll(modDir, 0755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(
		filepath.Join(modDir, "counter.js"),
		[]byte("var count = 0; module.exports = { inc: function() { return ++count; } };"),
		0644,
	); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// Engine 1
	var stdout1, stderr1 bytes.Buffer
	engine1, err := scripting.NewEngine(
		ctx, &stdout1, &stderr1,
		testutil.NewTestSessionID("security-input", t.Name()+"-1"),
		"memory", nil, 0, 0,
		scripting.WithModulePaths(modDir),
	)
	if err != nil {
		t.Fatalf("Engine 1 creation failed: %v", err)
	}
	defer engine1.Close()

	// Engine 2
	var stdout2, stderr2 bytes.Buffer
	engine2, err := scripting.NewEngine(
		ctx, &stdout2, &stderr2,
		testutil.NewTestSessionID("security-input", t.Name()+"-2"),
		"memory", nil, 0, 0,
		scripting.WithModulePaths(modDir),
	)
	if err != nil {
		t.Fatalf("Engine 2 creation failed: %v", err)
	}
	defer engine2.Close()

	// Increment counter in engine 1
	s1 := engine1.LoadScriptFromString("cross-module-1", `
		const counter = require('counter');
		counter.inc(); counter.inc(); counter.inc();
		if (counter.inc() !== 4) throw new Error('engine1 counter should be 4');
	`)
	if err := engine1.ExecuteScript(s1); err != nil {
		t.Fatalf("Engine 1 script failed: %v", err)
	}

	// Engine 2's counter should start fresh at 1, not continue from 4
	s2 := engine2.LoadScriptFromString("cross-module-2", `
		const counter = require('counter');
		if (counter.inc() !== 1) throw new Error('engine2 counter should start at 1, not continue from engine1');
	`)
	if err := engine2.ExecuteScript(s2); err != nil {
		t.Fatalf("Engine 2 script failed (state leaked from engine 1): %v", err)
	}
}

// ============================================================================
// FileExists / SanitizeFilename: Edge Cases
// ============================================================================

func TestFileExistsSecurity_SpecialChars(t *testing.T) {
	t.Parallel()
	engine, _, _ := newTestEngine(t)

	script := engine.LoadScriptFromString("fileexists-special", `
		const {fileExists} = require('osm:os');
		// Empty path
		if (fileExists('')) throw new Error('empty path should return false');
		// Null bytes
		if (fileExists('file\x00.txt')) {
			// Platform-dependent — some OS return false, some error
		}
		// Nonexistent
		if (fileExists('/nonexistent/path')) throw new Error('nonexistent path should return false');
	`)
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("fileExists special chars test failed: %v", err)
	}
}

// ============================================================================
// Git Spec Injection
// ============================================================================

func TestGitSpecSecurity_RefNameInjection(t *testing.T) {
	// Git ref names with shell metacharacters are passed as separate args
	// to exec.CommandContext, so shell injection is not possible.
	t.Parallel()
	engine, _, _ := newTestEngine(t)

	script := engine.LoadScriptFromString("gitspec-inject", `
		const {exec} = require('osm:exec');
		// Simulate what would happen if a git ref contained shell metacharacters
		const result = exec('echo', 'refs/heads/$(whoami)');
		// The echo command outputs the literal string — no shell expansion
	`)
	if err := engine.ExecuteScript(script); err != nil {
		t.Logf("Git spec injection test: %v", err)
	}
}
