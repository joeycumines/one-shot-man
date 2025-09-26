package config

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
)

func TestGetConfigPathEnvOverride(t *testing.T) {
	t.Setenv("ONESHOTMAN_CONFIG", "/tmp/custom-config")

	got, err := GetConfigPath()
	if err != nil {
		t.Fatalf("GetConfigPath returned error: %v", err)
	}

	if got != "/tmp/custom-config" {
		t.Fatalf("expected override path, got %q", got)
	}
}

func TestGetConfigPathDefault(t *testing.T) {
	dir := t.TempDir()

	// Derive home env var key depending on platform.
	homeVar := "HOME"
	if runtime.GOOS == "windows" {
		homeVar = "USERPROFILE"
	}
	t.Setenv(homeVar, dir)
	t.Setenv("ONESHOTMAN_CONFIG", "")

	got, err := GetConfigPath()
	if err != nil {
		t.Fatalf("GetConfigPath returned error: %v", err)
	}

	expected := filepath.Join(dir, ".one-shot-man", "config")
	if got != expected {
		t.Fatalf("expected default path %q, got %q", expected, got)
	}
}

func TestEnsureConfigDirCreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "nested", "config")
	t.Setenv("ONESHOTMAN_CONFIG", configPath)

	if err := EnsureConfigDir(); err != nil {
		t.Fatalf("EnsureConfigDir failed: %v", err)
	}

	info, err := os.Stat(filepath.Dir(configPath))
	if err != nil {
		t.Fatalf("expected config directory to exist: %v", err)
	}

	if !info.IsDir() {
		t.Fatalf("expected config directory, got %v", info.Mode())
	}
}

func TestEnsureConfigDirFailsWhenParentIsFile(t *testing.T) {
	dir := t.TempDir()
	parent := filepath.Join(dir, "file")
	if err := os.WriteFile(parent, []byte("ignore"), 0600); err != nil {
		t.Fatalf("failed to create parent file: %v", err)
	}

	configPath := filepath.Join(parent, "config")
	t.Setenv("ONESHOTMAN_CONFIG", configPath)

	err := EnsureConfigDir()
	if err == nil {
		t.Fatalf("expected error when parent is file")
	}

	var pathErr *fs.PathError
	if !errors.As(err, &pathErr) {
		t.Fatalf("expected PathError, got %v", err)
	}

	if pathErr.Path != parent {
		t.Fatalf("expected error path %q, got %q", parent, pathErr.Path)
	}

	if !errors.Is(pathErr.Err, fs.ErrExist) && !errors.Is(pathErr.Err, syscall.ENOTDIR) && !os.IsPermission(pathErr.Err) {
		t.Fatalf("unexpected underlying error: %v", pathErr.Err)
	}
}
