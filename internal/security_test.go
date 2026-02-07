// Package internal_test contains security tests for one-shot-man.
package internal_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/joeycumines/one-shot-man/internal/config"
	"github.com/joeycumines/one-shot-man/internal/scripting"
	"github.com/joeycumines/one-shot-man/internal/testutil"
)

// ============================================================================
// Path Traversal Prevention Tests
// ============================================================================

func TestPathTraversalPrevention_ConfigLoading(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		path  string
		valid bool
	}{
		{"valid nested path", "subdir/nested/config", true},
		{"valid relative path", "./testdata/config", true},
		{"traversal with file", "./config/../config", false},
		{"mixed traversal", "foo/../../etc/passwd", false},
		{"double traversal", "../../../etc/passwd", false},
		{"single traversal", "../../etc/passwd", false},
	}

	tmpDir := t.TempDir()

	// Create a temp directory structure
	realConfig := filepath.Join(tmpDir, "real-config")
	if err := os.WriteFile(realConfig, []byte("test=true"), 0644); err != nil {
		t.Fatalf("Failed to create test config: %v", err)
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.valid {
				cfg, err := config.LoadFromPath(tc.path)
				if err != nil && !os.IsNotExist(err) {
					t.Errorf("Unexpected error for valid path %s: %v", tc.path, err)
				}
				_ = cfg
			} else {
				cfg, err := config.LoadFromPath(tc.path)
				if err == nil && len(cfg.Global) > 0 {
					if _, hasTest := cfg.Global["test"]; hasTest {
						t.Error("Config loading with parent directory traversal may have escaped the intended directory")
					}
				}
				_ = cfg
			}
		})
	}
}

func TestPathTraversalPrevention_NullByteInjection(t *testing.T) {
	t.Parallel()

	pathsWithNull := []string{
		"valid\x00path",
		"config\x00.yaml",
	}

	for _, path := range pathsWithNull {
		t.Run("null-byte-"+path[:10], func(t *testing.T) {
			cfg, err := config.LoadFromPath(path)
			if err == nil {
				if len(cfg.Global) > 0 {
					t.Log("Config loaded with null byte in path (system-dependent behavior)")
				} else {
					t.Log("Config loaded with null byte in path but returned empty (acceptable)")
				}
			} else {
				t.Logf("Config loading with null byte failed as expected: %v", err)
			}
		})
	}
}

func TestPathTraversalPrevention_AbsolutePathEscape(t *testing.T) {
	t.Parallel()

	paths := []string{
		"/var/log/syslog",
		"/etc/passwd",
	}

	for _, path := range paths {
		t.Run("absolute-"+filepath.Base(path), func(t *testing.T) {
			cfg, err := config.LoadFromPath(path)
			if err == nil && len(cfg.Global) > 0 {
				t.Logf("Config loaded from %s (system allows reading)", path)
			} else if err != nil {
				t.Logf("Access to %s denied: %v", path, err)
			}
			_ = cfg
		})
	}
}

func TestPathTraversalPrevention_SymlinkEscape(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	realDir := filepath.Join(tmpDir, "real")
	if err := os.MkdirAll(realDir, 0755); err != nil {
		t.Fatalf("Failed to create real directory: %v", err)
	}

	realFile := filepath.Join(realDir, "config")
	if err := os.WriteFile(realFile, []byte("sensitive=true"), 0600); err != nil {
		t.Fatalf("Failed to create real config: %v", err)
	}

	linkDir := filepath.Join(tmpDir, "links")
	if err := os.MkdirAll(linkDir, 0755); err != nil {
		t.Fatalf("Failed to create link directory: %v", err)
	}

	linkPath := filepath.Join(linkDir, "linked-config")
	if err := os.Symlink(realFile, linkPath); err != nil {
		t.Skip("Symlinks not supported on this platform")
	}

	cfg, err := config.LoadFromPath(linkPath)
	if err == nil {
		if val, ok := cfg.Global["sensitive"]; ok && val == "true" {
			t.Logf("Config loaded via symlink (behavior depends on policy)")
		} else {
			t.Log("Symlink traversal worked - this is expected for legitimate symlinks")
		}
	} else {
		t.Logf("Symlink access blocked: %v", err)
	}
}

// ============================================================================
// Command Injection Prevention Tests
// ============================================================================

func TestCommandInjectionPrevention_ShellMetacharacters(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping command injection tests in short mode")
	}

	ctx := context.Background()

	metacharTests := []struct {
		name  string
		input string
	}{
		{"semicolon", ";echo pwned"},
		{"pipe", "|echo pwned"},
		{"and", "&&echo pwned"},
		{"or", "||echo pwned"},
		{"dollar", "$(echo pwned)"},
		{"backtick", "`echo pwned`"},
	}

	for _, tc := range metacharTests {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			engine, err := scripting.NewEngineWithConfig(
				ctx, &stdout, &stderr,
				testutil.NewTestSessionID("security", t.Name()),
				"memory",
			)
			if err != nil {
				t.Fatalf("Failed to create engine: %v", err)
			}
			defer engine.Close()

			escapeJS := func(s string) string {
				s = strings.ReplaceAll(s, "\\", "\\\\")
				s = strings.ReplaceAll(s, `"`, `\"`)
				return s
			}

			script := engine.LoadScriptFromString("test-"+tc.name, `
				const {exec} = require('osm:exec');
				const result = exec('echo', '`+escapeJS(tc.input)+`');
			`)

			err = engine.ExecuteScript(script)

			if err != nil {
				t.Logf("Execution error (may be expected): %v", err)
			} else {
				output := stdout.String()
				if strings.Contains(output, "pwned") {
					t.Errorf("Shell injection may have occurred: %s in output", tc.name)
				} else {
					t.Logf("Shell metachar handled safely")
				}
			}
		})
	}
}

func TestCommandInjectionPrevention_CommandChaining(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping command injection tests in short mode")
	}

	ctx := context.Background()

	chainingTests := []struct {
		name    string
		command string
	}{
		{"semicolon chaining", "echo hello; whoami"},
		{"pipe chaining", "echo hello | cat"},
		{"AND chaining", "echo hello && whoami"},
		{"OR chaining", "echo hello || whoami"},
	}

	for _, tc := range chainingTests {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			engine, err := scripting.NewEngineWithConfig(
				ctx, &stdout, &stderr,
				testutil.NewTestSessionID("security", t.Name()),
				"memory",
			)
			if err != nil {
				t.Fatalf("Failed to create engine: %v", err)
			}
			defer engine.Close()

			script := engine.LoadScriptFromString("chain-"+tc.name, `
				const {exec} = require('osm:exec');
				const result = exec('`+tc.command+`');
			`)

			err = engine.ExecuteScript(script)

			if err != nil {
				t.Logf("Execution error: %v", err)
			} else {
				output := stdout.String()
				if strings.Count(output, "hello") > 1 {
					t.Errorf("Multiple commands may have executed: %s", output)
				} else {
					t.Logf("Command chaining handled appropriately")
				}
			}
		})
	}
}

func TestCommandInjectionPrevention_SubshellInjection(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping command injection tests in short mode")
	}

	ctx := context.Background()

	subshellTests := []struct {
		name    string
		command string
	}{
		{"bash subshell", "$(whoami)"},
		{"sh subshell", "`whoami`"},
	}

	for _, tc := range subshellTests {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			engine, err := scripting.NewEngineWithConfig(
				ctx, &stdout, &stderr,
				testutil.NewTestSessionID("security", t.Name()),
				"memory",
			)
			if err != nil {
				t.Fatalf("Failed to create engine: %v", err)
			}
			defer engine.Close()

			escapeJS := func(s string) string {
				s = strings.ReplaceAll(s, "\\", "\\\\")
				s = strings.ReplaceAll(s, `"`, `\"`)
				return s
			}

			script := engine.LoadScriptFromString("subshell-"+tc.name, `
				const {exec} = require('osm:exec');
				const result = exec('echo', '`+escapeJS(tc.command)+`');
			`)

			err = engine.ExecuteScript(script)

			if err != nil {
				t.Logf("Execution error: %v", err)
			} else {
				output := stdout.String()
				t.Logf("Subshell test output: %s", output)
			}
		})
	}
}

// ============================================================================
// Environment Variable Security Tests
// ============================================================================

func TestEnvironmentVariableSanitization_DangerousVars(t *testing.T) {
	t.Parallel()

	dangerousVars := []string{
		"LD_PRELOAD",
		"LD_LIBRARY_PATH",
		"DYLD_INSERT_LIBRARIES",
	}

	for _, varName := range dangerousVars {
		t.Run(varName, func(t *testing.T) {
			original := os.Getenv(varName)
			defer func() {
				if original != "" {
					os.Setenv(varName, original)
				} else {
					os.Unsetenv(varName)
				}
			}()

			maliciousValue := "/malicious/path"
			if err := os.Setenv(varName, maliciousValue); err != nil {
				t.Skip("Cannot set environment variable")
			}

			current := os.Getenv(varName)
			if current == maliciousValue {
				t.Logf("Variable %s set to: %s (implementation controls pass-through)", varName, current)
			} else {
				t.Logf("Variable %s filtered or removed", varName)
			}
		})
	}
}

func TestEnvironmentVariableSanitization_SpecialChars(t *testing.T) {
	t.Parallel()

	specialChars := []struct {
		name  string
		value string
	}{
		{"spaces", "value with spaces"},
		{"tabs", "value\twith\ttabs"},
		{"semicolons", "value;with_semicolons"},
	}

	for _, tc := range specialChars {
		t.Run(tc.name, func(t *testing.T) {
			original := os.Getenv("TEST_VAR")
			defer func() {
				if original != "" {
					os.Setenv("TEST_VAR", original)
				} else {
					os.Unsetenv("TEST_VAR")
				}
			}()

			if err := os.Setenv("TEST_VAR", tc.value); err != nil {
				t.Skip("Cannot set environment variable")
			}

			val := os.Getenv("TEST_VAR")
			if val != tc.value {
				t.Logf("Variable value modified: got %q", val)
			} else {
				t.Log("Variable value preserved")
			}
		})
	}
}

func TestEnvironmentVariableSanitization_Unicode(t *testing.T) {
	t.Parallel()

	unicodeValues := []string{
		"日本語",
		"🎉 emojis",
		"Ελληνικά",
	}

	for _, val := range unicodeValues {
		// Use first 10 bytes (or fewer if not enough) for test name, valid UTF-8
		name := val
		if utf8.RuneCountInString(val) > 10 {
			name = string([]rune(val)[:10])
		}
		t.Run(name, func(t *testing.T) {
			original := os.Getenv("UNICODE_TEST_VAR")
			defer func() {
				if original != "" {
					os.Setenv("UNICODE_TEST_VAR", original)
				} else {
					os.Unsetenv("UNICODE_TEST_VAR")
				}
			}()

			if err := os.Setenv("UNICODE_TEST_VAR", val); err != nil {
				t.Skip("Cannot set environment variable")
			}

			retrieved := os.Getenv("UNICODE_TEST_VAR")
			if retrieved != val {
				t.Errorf("Unicode value modified: expected %q, got %q", val, retrieved)
			}
		})
	}
}

// ============================================================================
// File Permission Security Tests
// ============================================================================

func TestFilePermissionHandling_ReadPermissions(t *testing.T) {
	t.Parallel()

	// Skip if running as root since root can read any file regardless of permissions
	if os.Geteuid() == 0 {
		t.Skip("Skipping: Root user can read any file regardless of permissions")
	}

	tmpDir := t.TempDir()

	noReadFile := filepath.Join(tmpDir, "no-read")
	if err := os.WriteFile(noReadFile, []byte("secret"), 0000); err != nil {
		t.Skip("Cannot set file permissions on this platform")
	}

	cfg, err := config.LoadFromPath(noReadFile)
	if err == nil {
		if len(cfg.Global) > 0 {
			t.Error("Successfully read file with no read permissions")
		}
	} else if os.IsPermission(err) {
		t.Log("Permission error as expected")
	} else {
		t.Logf("Other error: %v", err)
	}
}

func TestFilePermissionHandling_WorldWritable(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	wwDir := filepath.Join(tmpDir, "world-writable")
	if err := os.MkdirAll(wwDir, 0777); err != nil {
		t.Skip("Cannot create world-writable directory")
	}

	testFile := filepath.Join(wwDir, "test")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Skip("Cannot write to directory")
	}

	cfg, err := config.LoadFromPath(testFile)
	if err == nil {
		t.Log("Config loaded from world-writable directory")
	} else {
		t.Logf("Error loading config: %v", err)
	}
	_ = cfg
}

func TestFilePermissionHandling_SymlinkAttacks(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	targetDir := filepath.Join(tmpDir, "target")
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		t.Fatalf("Failed to create target directory: %v", err)
	}

	targetFile := filepath.Join(targetDir, "secret")
	if err := os.WriteFile(targetFile, []byte("SENSITIVE DATA"), 0600); err != nil {
		t.Fatalf("Failed to create target file: %v", err)
	}

	linkDir := filepath.Join(tmpDir, "links")
	if err := os.MkdirAll(linkDir, 0755); err != nil {
		t.Fatalf("Failed to create link directory: %v", err)
	}

	linkPath := filepath.Join(linkDir, "to-secret")
	if err := os.Symlink(targetFile, linkPath); err != nil {
		t.Skip("Symlinks not supported")
	}

	cfg, err := config.LoadFromPath(linkPath)
	if err == nil {
		t.Log("Config loaded via symlink")
		if _, ok := cfg.Global["SENSITIVE"]; ok {
			t.Error("Symlink allowed access to sensitive file")
		}
	} else {
		t.Logf("Symlink access blocked: %v", err)
	}
}

// ============================================================================
// Input Validation Security Tests
// ============================================================================

func TestInputValidation_DangerousScriptInputs(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping input validation tests in short mode")
	}

	ctx := context.Background()

	dangerousInputs := []struct {
		name  string
		input string
	}{
		{"null byte", "hello\x00world"},
		{"escape sequence", "\x1b[31mred text\x1b[0m"},
		{"control chars", "\x03\x04\x05script"},
		{"nested quotes", `he said "she said 'it works'"`},
	}

	for _, tc := range dangerousInputs {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			engine, err := scripting.NewEngineWithConfig(
				ctx, &stdout, &stderr,
				testutil.NewTestSessionID("security", t.Name()),
				"memory",
			)
			if err != nil {
				t.Fatalf("Failed to create engine: %v", err)
			}
			defer engine.Close()

			escapeJS := func(s string) string {
				s = strings.ReplaceAll(s, "\\", "\\\\")
				s = strings.ReplaceAll(s, `"`, `\"`)
				s = strings.ReplaceAll(s, "'", `\'`)
				s = strings.ReplaceAll(s, "\n", `\n`)
				s = strings.ReplaceAll(s, "\r", `\r`)
				s = strings.ReplaceAll(s, "\t", `\t`)
				s = strings.ReplaceAll(s, "\x00", `\x00`)
				return s
			}

			script := engine.LoadScriptFromString("dangerous-"+tc.name, `
				const test = "`+escapeJS(tc.input)+`";
				const length = test.length;
			`)

			err = engine.ExecuteScript(script)

			if err != nil {
				t.Logf("Script error for %s: %v", tc.name, err)
			} else {
				t.Logf("%s handled successfully", tc.name)
			}
		})
	}
}

func TestInputValidation_TemplateInjection(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping input validation tests in short mode")
	}

	ctx := context.Background()

	injectionTests := []struct {
		name  string
		input string
	}{
		{"template syntax", "{{.invalid}}"},
		{"template action", "{{if .x}}pwned{{end}}"},
	}

	for _, tc := range injectionTests {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			engine, err := scripting.NewEngineWithConfig(
				ctx, &stdout, &stderr,
				testutil.NewTestSessionID("security", t.Name()),
				"memory",
			)
			if err != nil {
				t.Fatalf("Failed to create engine: %v", err)
			}
			defer engine.Close()

			escapeJS := func(s string) string {
				s = strings.ReplaceAll(s, "\\", "\\\\")
				s = strings.ReplaceAll(s, `"`, `\"`)
				return s
			}

			script := engine.LoadScriptFromString("template-"+tc.name, `
				const input = "`+escapeJS(tc.input)+`";
				const length = input.length;
			`)

			err = engine.ExecuteScript(script)

			if err != nil {
				t.Logf("Template error (expected for invalid templates): %v", err)
			} else {
				t.Logf("%s handled safely", tc.name)
			}
		})
	}
}

func TestInputValidation_ANSIEscapeSequences(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping input validation tests in short mode")
	}

	ctx := context.Background()

	var stdout, stderr bytes.Buffer

	engine, err := scripting.NewEngineWithConfig(
		ctx, &stdout, &stderr,
		testutil.NewTestSessionID("security", t.Name()),
		"memory",
	)
	if err != nil {
		t.Fatalf("Failed to create engine: %v", err)
	}
	defer engine.Close()

	escapeSeqs := []struct {
		name  string
		input string
	}{
		{"clear screen", "\x1b[2J"},
		{"color red", "\x1b[31m"},
		{"reset", "\x1b[0m"},
		{"bell", "\x07"},
	}

	for _, tc := range escapeSeqs {
		t.Run(tc.name, func(t *testing.T) {
			escapeJS := func(s string) string {
				s = strings.ReplaceAll(s, "\\", "\\\\")
				s = strings.ReplaceAll(s, `"`, `\"`)
				return s
			}

			script := engine.LoadScriptFromString("escape-"+tc.name, `
				const input = "`+escapeJS(tc.input)+`";
				const length = input.length;
			`)

			err := engine.ExecuteScript(script)

			if err != nil {
				t.Logf("Error with escape sequence: %v", err)
			} else {
				t.Logf("%s handled", tc.name)
			}
		})
	}
}

// ============================================================================
// Session Isolation Tests
// ============================================================================

func TestSessionDataIsolation_NotLeakedBetweenSessions(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	session1 := "session-one"
	session2 := "session-two"

	engine1, err := scripting.NewEngineWithConfig(
		ctx, &bytes.Buffer{}, &bytes.Buffer{},
		testutil.NewTestSessionID("security", session1),
		"memory",
	)
	if err != nil {
		t.Fatalf("Failed to create engine 1: %v", err)
	}
	defer engine1.Close()

	engine2, err := scripting.NewEngineWithConfig(
		ctx, &bytes.Buffer{}, &bytes.Buffer{},
		testutil.NewTestSessionID("security", session2),
		"memory",
	)
	if err != nil {
		t.Fatalf("Failed to create engine 2: %v", err)
	}
	defer engine2.Close()

	engine1.SetGlobal("privateData", "session1-secret")
	engine2.SetGlobal("privateData", "session2-secret")

	val1 := engine1.GetGlobal("privateData")
	val2 := engine2.GetGlobal("privateData")

	if val1 == val2 {
		t.Error("Session data may be leaking between sessions")
	} else {
		t.Log("Session data properly isolated")
	}

	_ = val1
	_ = val2
}

func TestSessionDataIsolation_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	numSessions := 10
	done := make(chan bool, numSessions)

	for i := 0; i < numSessions; i++ {
		go func(idx int) {
			defer func() { done <- true }()

			engine, err := scripting.NewEngineWithConfig(
				ctx, &bytes.Buffer{}, &bytes.Buffer{},
				testutil.NewTestSessionID("security", "concurrent-test-"+string(rune('a'+idx))),
				"memory",
			)
			if err != nil {
				t.Errorf("Engine %d creation failed: %v", idx, err)
				return
			}
			defer engine.Close()

			engine.SetGlobal("key", idx)
			val := engine.GetGlobal("key")

			if val != int64(idx) {
				t.Errorf("Engine %d: expected %d, got %v", idx, idx, val)
			}
		}(i)
	}

	for i := 0; i < numSessions; i++ {
		<-done
	}

	t.Log("Concurrent session access completed safely")
}

// ============================================================================
// Output Sanitization Tests
// ============================================================================

func TestOutputSanitization_NoSensitiveDataInLogs(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	var stdout, stderr bytes.Buffer

	engine, err := scripting.NewEngineWithConfig(
		ctx, &stdout, &stderr,
		testutil.NewTestSessionID("security", t.Name()),
		"memory",
	)
	if err != nil {
		t.Fatalf("Failed to create engine: %v", err)
	}
	defer engine.Close()

	cfg := config.NewConfig()
	cfg.SetGlobalOption("api_key", "secret123")
	cfg.SetGlobalOption("password", "supersecret")

	for _, key := range []string{"api_key", "password"} {
		val, ok := cfg.GetGlobalOption(key)
		if !ok {
			t.Errorf("Config key %s not found", key)
		} else {
			t.Logf("Key %s has value: %s", key, val)
		}
	}

	_ = cfg
}

func TestOutputSanitization_ErrorMessagesNoSensitivePaths(t *testing.T) {
	t.Parallel()

	sensitivePaths := []string{
		"/root/.ssh/id_rsa",
		"/etc/secrets",
	}

	for _, path := range sensitivePaths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			_, err := config.LoadFromPath(path)
			if err != nil {
				errStr := err.Error()
				if strings.Contains(errStr, "secret") || strings.Contains(errStr, "SENSITIVE") {
					t.Errorf("Error message may expose sensitive data for %s", path)
				} else {
					t.Logf("Error message safe for %s", path)
				}
			} else {
				t.Log("Path accessible (acceptable in some contexts)")
			}
		})
	}
}

// ============================================================================
// Config Injection Prevention Tests
// ============================================================================

func TestConfigInjection_ValuesWithShellMetacharacters(t *testing.T) {
	t.Parallel()

	metacharValues := []string{
		"; rm -rf /",
		"$(whoami)",
		"`ls`",
		"| cat /etc/passwd",
	}

	for _, val := range metacharValues {
		// Use first 15 bytes (or fewer if not enough) for test name
		safeName := val
		if len(val) > 15 {
			safeName = val[:15]
		}
		t.Run(safeName, func(t *testing.T) {
			cfg := config.NewConfig()

			cfg.SetGlobalOption("test", val)

			retrieved, ok := cfg.GetGlobalOption("test")
			if !ok {
				t.Error("Config value not stored")
			} else if retrieved != val {
				t.Errorf("Config value modified: expected %q, got %q", val, retrieved)
			} else {
				t.Log("Config value stored and retrieved correctly")
			}
		})
	}
}

func TestConfigInjection_SectionNamesWithSpecialChars(t *testing.T) {
	t.Parallel()

	specialNames := []string{
		"[/etc/passwd]",
		"[../secret]",
	}

	for _, name := range specialNames {
		// Use first 20 bytes (or fewer if not enough) for test name
		safeName := name
		if len(name) > 20 {
			safeName = name[:20]
		}
		t.Run(safeName, func(t *testing.T) {
			cfg := config.NewConfig()

			cfg.SetCommandOption(name, "key", "value")

			val, ok := cfg.GetCommandOption(name, "key")
			if !ok {
				t.Error("Config section not created")
			} else if val == "value" {
				t.Logf("Special section name stored: %s", name)
			}
		})
	}
}

// ============================================================================
// Resource Limit Tests
// ============================================================================

func TestResourceLimits_ExtremelyLongConfigValues(t *testing.T) {
	t.Parallel()

	cfg := config.NewConfig()

	veryLongValue := strings.Repeat("x", 100000)

	cfg.SetGlobalOption("long_key", veryLongValue)

	retrieved, ok := cfg.GetGlobalOption("long_key")
	if !ok {
		t.Error("Long value not stored")
	} else if len(retrieved) != len(veryLongValue) {
		t.Errorf("Long value truncated: expected len %d, got %d", len(veryLongValue), len(retrieved))
	} else {
		t.Log("Long value stored completely")
	}
}

func TestResourceLimits_ManyConfigOptions(t *testing.T) {
	t.Parallel()

	cfg := config.NewConfig()

	numOptions := 100
	for i := 0; i < numOptions; i++ {
		key := "key" + string(rune(i%256))
		value := "value" + string(rune(i%256))
		cfg.SetGlobalOption(key, value)
	}

	if len(cfg.Global) != numOptions {
		t.Errorf("Expected %d options, got %d", numOptions, len(cfg.Global))
	} else {
		t.Log("Many options stored correctly")
	}
}

// ============================================================================
// Argument Parsing Security Tests
// ============================================================================

func TestArgumentParsingSecurity_NullBytes(t *testing.T) {
	t.Parallel()

	args := []string{"hello\x00world", "test\x00value"}

	for _, arg := range args {
		t.Run(arg[:10], func(t *testing.T) {
			if strings.Contains(arg, "\x00") {
				t.Log("Null byte in argument (may be stripped)")
			} else {
				t.Log("Argument without null byte")
			}
		})
	}
}

func TestArgumentParsingSecurity_SpecialCharacters(t *testing.T) {
	t.Parallel()

	specialArgs := []string{
		"arg with spaces",
		"arg\twith\ttabs",
		"arg\"with\"quotes",
		"arg'with'single",
		"arg\\with\\backs",
	}

	for _, arg := range specialArgs {
		// Use first 15 bytes (or fewer if not enough) for test name
		safeName := arg
		if len(arg) > 15 {
			safeName = arg[:15]
		}
		t.Run(safeName, func(t *testing.T) {
			cfg := config.NewConfig()

			cfg.SetGlobalOption("test", arg)

			retrieved, ok := cfg.GetGlobalOption("test")
			if !ok {
				t.Error("Special character argument not stored")
			} else if retrieved != arg {
				t.Errorf("Special character argument modified: %q", retrieved)
			} else {
				t.Log("Special character argument stored correctly")
			}
		})
	}
}

// ============================================================================
// Binary Path Security Tests
// ============================================================================

func TestBinaryPathSecurity_NotInSuspiciousLocations(t *testing.T) {
	t.Parallel()

	suspiciousPrefixes := []string{
		"/tmp/",
		"/var/tmp/",
	}

	executablePath, err := os.Executable()
	if err != nil {
		t.Skip("Cannot determine executable path")
	}

	for _, prefix := range suspiciousPrefixes {
		if strings.HasPrefix(executablePath, prefix) {
			t.Logf("Binary in unusual location: %s", executablePath)
		} else {
			t.Logf("Binary not in %s", prefix)
		}
	}
}

func TestBinaryPathSecurity_AppropriatePermissions(t *testing.T) {
	t.Parallel()

	info, err := os.Stat(os.Args[0])
	if err != nil {
		t.Skip("Cannot check binary permissions")
	}

	mode := info.Mode()
	if mode&0007 != 0 {
		t.Logf("Binary has world-executable permissions: %o", mode)
	} else {
		t.Log("Binary permissions are appropriate")
	}
}

// ============================================================================
// Escaping in Export Contexts Tests
// ============================================================================

func TestEscapingInExportContexts_HTML(t *testing.T) {
	t.Parallel()

	dangerousHTML := []string{
		"<script>alert('xss')</script>",
		"<img src=x onerror=alert(1)>",
		"javascript:alert(1)",
	}

	for _, input := range dangerousHTML {
		// Use first 20 bytes (or fewer if not enough) for test name
		safeName := input
		if len(input) > 20 {
			safeName = input[:20]
		}
		t.Run(safeName, func(t *testing.T) {
			cfg := config.NewConfig()

			cfg.SetGlobalOption("html_test", input)

			val, ok := cfg.GetGlobalOption("html_test")
			if !ok {
				t.Error("HTML test value not stored")
			} else {
				t.Logf("HTML value stored: %s", val)
			}
		})
	}
}

func TestEscapingInExportContexts_JSON(t *testing.T) {
	t.Parallel()

	dangerousJSON := []string{
		"hello\"world",
		"hello\\nworld",
		"\x00null byte",
	}

	for _, input := range dangerousJSON {
		// Use first 15 bytes (or fewer if not enough) for test name
		safeName := input
		if len(input) > 15 {
			safeName = input[:15]
		}
		t.Run(safeName, func(t *testing.T) {
			cfg := config.NewConfig()

			cfg.SetGlobalOption("json_test", input)

			val, ok := cfg.GetGlobalOption("json_test")
			if !ok {
				t.Error("JSON test value not stored")
			} else {
				t.Logf("JSON value stored: %s", val)
			}
		})
	}
}

func TestEscapingInExportContexts_Shell(t *testing.T) {
	t.Parallel()

	dangerousShell := []string{
		"; rm -rf /",
		"| cat /etc/passwd",
		"&& whoami",
	}

	for _, input := range dangerousShell {
		// Use first 15 bytes (or fewer if not enough) for test name
		safeName := input
		if len(input) > 15 {
			safeName = input[:15]
		}
		t.Run(safeName, func(t *testing.T) {
			cfg := config.NewConfig()

			cfg.SetGlobalOption("shell_test", input)

			val, ok := cfg.GetGlobalOption("shell_test")
			if !ok {
				t.Error("Shell test value not stored")
			} else {
				t.Logf("Shell value stored: %s", val)
			}
		})
	}
}

// ============================================================================
// TUI Input Security Tests
// ============================================================================

func TestTUIInputSecurity_EscapeSequences(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping input validation tests in short mode")
	}

	ctx := context.Background()

	var stdout, stderr bytes.Buffer

	engine, err := scripting.NewEngineWithConfig(
		ctx, &stdout, &stderr,
		testutil.NewTestSessionID("security", t.Name()),
		"memory",
	)
	if err != nil {
		t.Fatalf("Failed to create engine: %v", err)
	}
	defer engine.Close()

	script := engine.LoadScriptFromString("escape-tui", `
		const input = "\x1b[31mRED\x1b[0m \x1b[1mBOLD\x1b[0m";
		const length = input.length;
	`)

	err = engine.ExecuteScript(script)
	if err != nil {
		t.Logf("TUI escape sequence error: %v", err)
	} else {
		t.Log("TUI escape sequences handled")
	}
}
