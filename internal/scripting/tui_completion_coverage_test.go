package scripting

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/dop251/goja"
)

// ============================================================================
// tui_completion.go — getFilepathSuggestions, getExecutableSuggestions
// ============================================================================

func TestGetFilepathSuggestions_Tilde(t *testing.T) {
	result := getFilepathSuggestions("~")
	if len(result) != 1 || result[0].Text != "~/" {
		t.Errorf("expected single suggestion '~/', got %v", result)
	}
}

func TestGetFilepathSuggestions_EmptyPath(t *testing.T) {
	// Change to a temp dir with known contents
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "afile.txt"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(tmpDir, "adir"), 0755); err != nil {
		t.Fatal(err)
	}

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	result := getFilepathSuggestions("")
	if len(result) < 2 {
		t.Fatalf("expected at least 2 suggestions, got %d: %v", len(result), result)
	}

	foundFile := false
	foundDir := false
	for _, s := range result {
		if s.Text == "afile.txt" {
			foundFile = true
		}
		if s.Text == "adir/" {
			foundDir = true
		}
	}
	if !foundFile {
		t.Error("expected 'afile.txt' in suggestions")
	}
	if !foundDir {
		t.Error("expected 'adir/' in suggestions (directory should have trailing /)")
	}
}

func TestGetFilepathSuggestions_ExplicitDir(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "inner.go"), []byte("package x"), 0644); err != nil {
		t.Fatal(err)
	}

	// Request contents of the tmpDir by providing a trailing slash
	result := getFilepathSuggestions(tmpDir + "/")
	if len(result) == 0 {
		t.Fatal("expected at least 1 suggestion for explicit directory")
	}
	found := false
	for _, s := range result {
		if filepath.Base(s.Text) == "inner.go" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected inner.go in suggestions, got %v", result)
	}
}

func TestGetFilepathSuggestions_NonexistentDir(t *testing.T) {
	result := getFilepathSuggestions("/nonexistent-path-abc123xyz/")
	if result != nil {
		t.Errorf("expected nil for nonexistent path, got %v", result)
	}
}

func TestGetFilepathSuggestions_PrefixFilter(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "alpha.txt"), []byte("a"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "beta.txt"), []byte("b"), 0644); err != nil {
		t.Fatal(err)
	}

	result := getFilepathSuggestions(tmpDir + "/al")
	foundAlpha := false
	for _, s := range result {
		if filepath.Base(s.Text) == "alpha.txt" {
			foundAlpha = true
		}
		if filepath.Base(s.Text) == "beta.txt" {
			t.Error("prefix 'al' should not match 'beta.txt'")
		}
	}
	if !foundAlpha {
		t.Errorf("expected alpha.txt in suggestions, got %v", result)
	}
}

func TestGetExecutableSuggestions_EmptyPrefix(t *testing.T) {
	result := getExecutableSuggestions("")
	if len(result) == 0 {
		t.Fatal("expected common command suggestions for empty prefix")
	}

	// Should include well-known commands
	found := make(map[string]bool)
	for _, s := range result {
		found[s.Text] = true
	}
	for _, expected := range []string{"cat", "echo", "grep", "ls"} {
		if !found[expected] {
			t.Errorf("expected %q in common suggestions", expected)
		}
	}
}

func TestGetExecutableSuggestions_WithPrefix(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("executable permission bits not supported on Windows")
	}

	// Create a temp directory with a fake executable and set it as PATH
	tmpDir := t.TempDir()
	fakeBin := filepath.Join(tmpDir, "mytestcmd")
	if err := os.WriteFile(fakeBin, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", tmpDir)

	result := getExecutableSuggestions("mytest")
	foundCmd := false
	for _, s := range result {
		if s.Text == "mytestcmd" {
			foundCmd = true
		}
	}
	if !foundCmd {
		t.Errorf("expected 'mytestcmd' in suggestions, got %v", result)
	}
}

func TestGetExecutableSuggestions_EmptyPATH(t *testing.T) {
	t.Setenv("PATH", "")
	result := getExecutableSuggestions("something")
	if result != nil {
		t.Errorf("expected nil for empty PATH, got %v", result)
	}
}

func TestGetExecutableSuggestions_PathSeparator(t *testing.T) {
	// If prefix contains a path separator, should delegate to filepath suggestions
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "script.sh"), []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}

	result := getExecutableSuggestions(tmpDir + "/scr")
	foundScript := false
	for _, s := range result {
		if filepath.Base(s.Text) == "script.sh" {
			foundScript = true
		}
	}
	if !foundScript {
		t.Errorf("expected 'script.sh' when prefix contains path separator, got %v", result)
	}
}

// ============================================================================
// tui_parsing.go — isUndefined, currentWord, tokenizeCommandLine
// ============================================================================

func TestIsUndefined_NilValue(t *testing.T) {
	if !isUndefined(nil) {
		t.Error("expected nil to be undefined")
	}
}

func TestIsUndefined_GojaUndefined(t *testing.T) {
	if !isUndefined(goja.Undefined()) {
		t.Error("expected goja.Undefined() to be undefined")
	}
}

func TestIsUndefined_GojaNull(t *testing.T) {
	if isUndefined(goja.Null()) {
		t.Error("goja.Null() should NOT be reported as undefined")
	}
}

func TestIsUndefined_GojaStringValue(t *testing.T) {
	vm := goja.New()
	val := vm.ToValue("hello")
	if isUndefined(val) {
		t.Error("a goja string value should NOT be undefined")
	}
}

func TestIsUndefined_GoString(t *testing.T) {
	if isUndefined("hello") {
		t.Error("a Go string should NOT be undefined")
	}
}

func TestIsUndefined_GoInt(t *testing.T) {
	if isUndefined(42) {
		t.Error("a Go int should NOT be undefined")
	}
}

func TestCurrentWord_Simple(t *testing.T) {
	got := currentWord("git commit -m ")
	if got != "" {
		t.Errorf("expected empty current word after trailing space, got %q", got)
	}
}

func TestCurrentWord_MidWord(t *testing.T) {
	got := currentWord("git com")
	if got != "com" {
		t.Errorf("expected 'com', got %q", got)
	}
}

func TestCurrentWord_Empty(t *testing.T) {
	got := currentWord("")
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestTokenizeCommandLine_Simple(t *testing.T) {
	tokens := tokenizeCommandLine("ls -la /tmp")
	if len(tokens) != 3 {
		t.Fatalf("expected 3 tokens, got %d: %v", len(tokens), tokens)
	}
	if tokens[0] != "ls" || tokens[1] != "-la" || tokens[2] != "/tmp" {
		t.Errorf("unexpected tokens: %v", tokens)
	}
}

func TestTokenizeCommandLine_Quoted(t *testing.T) {
	tokens := tokenizeCommandLine(`echo "hello world" foo`)
	if len(tokens) != 3 {
		t.Fatalf("expected 3 tokens, got %d: %v", len(tokens), tokens)
	}
	if tokens[1] != "hello world" {
		t.Errorf("expected 'hello world', got %q", tokens[1])
	}
}

func TestTokenizeCommandLine_Empty(t *testing.T) {
	tokens := tokenizeCommandLine("")
	if len(tokens) != 0 {
		t.Errorf("expected 0 tokens for empty input, got %d: %v", len(tokens), tokens)
	}
}
