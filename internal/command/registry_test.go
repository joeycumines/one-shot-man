package command

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/config"
)

// TestCommand implements Command interface for testing
type TestCommand struct {
	*BaseCommand
}

func NewTestCommand(name, description, usage string) *TestCommand {
	return &TestCommand{
		BaseCommand: NewBaseCommand(name, description, usage),
	}
}

func (c *TestCommand) Execute(args []string, stdout, stderr io.Writer) error {
	return nil
}

func TestRegistry(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	registry := NewRegistryWithConfig(cfg)

	// Test registering built-in command
	testCmd := NewTestCommand("test", "Test command", "test [options]")
	registry.Register(testCmd)

	// Test getting built-in command
	cmd, err := registry.Get("test")
	if err != nil {
		t.Fatalf("Failed to get registered command: %v", err)
	}

	if cmd.Name() != "test" {
		t.Errorf("Expected command name 'test', got '%s'", cmd.Name())
	}

	// Test getting non-existent command
	_, err = registry.Get("nonexistent")
	if err == nil {
		t.Error("Expected error for non-existent command, got nil")
	}

	// Test listing commands
	commands := registry.ListBuiltin()
	found := false
	for _, name := range commands {
		if name == "test" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected 'test' command in builtin list")
	}
}

func TestScriptPathDuplication(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, "custom-config")
	scriptsDir := filepath.Join(configDir, "scripts")

	if err := os.MkdirAll(scriptsDir, 0755); err != nil {
		t.Fatalf("Failed to create scripts directory: %v", err)
	}

	configPath := filepath.Join(configDir, "config")
	t.Setenv("OSM_CONFIG", configPath)

	cfg := config.NewConfig()
	cfg.SetGlobalOption("script.paths", strings.Join([]string{scriptsDir, "../custom-config/scripts"}, ","))

	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to determine original working directory: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(originalDir)
	})

	if err := os.Chdir(configDir); err != nil {
		t.Fatalf("Failed to change working directory: %v", err)
	}

	registry := NewRegistryWithConfig(cfg)

	counts := make(map[string]int)
	for _, path := range registry.scriptPaths {
		counts[path]++
	}

	if count := counts[scriptsDir]; count != 1 {
		t.Errorf("Expected scripts directory %s to be deduplicated, found %d entries", scriptsDir, count)
	}

	for path, count := range counts {
		if count > 1 {
			t.Errorf("Expected no duplicate script paths, but %s appeared %d times", path, count)
		}
	}
}

func TestScriptCommand(t *testing.T) {
	t.Parallel()
	// Create temporary script file
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "testscript")

	scriptContent := `#!/bin/bash
echo "Test script output"
`

	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0755); err != nil {
		t.Fatalf("Failed to create test script: %v", err)
	}
	if err := os.Chmod(scriptPath, 0755); err != nil {
		t.Fatalf("Failed to chmod test script: %v", err)
	}

	// Test script command creation
	scriptCmd := NewScriptCommand("testscript", scriptPath)

	if scriptCmd.Name() != "testscript" {
		t.Errorf("Expected script name 'testscript', got '%s'", scriptCmd.Name())
	}

	if !strings.Contains(scriptCmd.Description(), "Script command") {
		t.Errorf("Expected description to contain 'Script command', got '%s'", scriptCmd.Description())
	}
}

func TestIsExecutable_UnixAndWindows(t *testing.T) {
	t.Parallel()

	// Create temp dir
	tmp := t.TempDir()

	// Create two files and inspect their executability depending on platform
	exe := filepath.Join(tmp, "a.exe")
	txt := filepath.Join(tmp, "b.txt")

	if err := os.WriteFile(exe, []byte("x"), 0644); err != nil {
		t.Fatalf("failed to create %s: %v", exe, err)
	}
	if err := os.WriteFile(txt, []byte("x"), 0644); err != nil {
		t.Fatalf("failed to create %s: %v", txt, err)
	}

	infoExe, err := os.Stat(exe)
	if err != nil {
		t.Fatalf("stat exe: %v", err)
	}
	infoTxt, err := os.Stat(txt)
	if err != nil {
		t.Fatalf("stat txt: %v", err)
	}

	if runtime.GOOS == "windows" {
		// On Windows our conservative policy treats .exe as executable.
		if !isExecutable(infoExe) {
			t.Errorf("expected %s to be executable on Windows", exe)
		}
		// .txt should not be executable
		if isExecutable(infoTxt) {
			t.Errorf("did not expect %s to be executable on Windows", txt)
		}
		// Ensure scripts that require an interpreter are not considered directly executable
		ps1 := filepath.Join(tmp, "c.ps1")
		sh := filepath.Join(tmp, "d.sh")
		_ = os.WriteFile(ps1, []byte("echo"), 0644)
		_ = os.WriteFile(sh, []byte("echo"), 0644)
		infoPs1, _ := os.Stat(ps1)
		infoSh, _ := os.Stat(sh)
		if isExecutable(infoPs1) {
			t.Errorf("did not expect %s to be considered executable on Windows", ps1)
		}
		if isExecutable(infoSh) {
			t.Errorf("did not expect %s to be considered executable on Windows", sh)
		}
	} else {
		// On Unix-like systems executability depends on file mode bits.
		// Make exe executable and re-stat.
		if err := os.Chmod(exe, 0755); err != nil {
			t.Fatalf("chmod exe: %v", err)
		}
		infoExe2, _ := os.Stat(exe)
		if !isExecutable(infoExe2) {
			t.Errorf("expected %s to be executable on Unix-like", exe)
		}
		// ensure txt without exec bit is not executable
		if isExecutable(infoTxt) {
			t.Errorf("did not expect %s to be executable on Unix-like", txt)
		}
	}
}
