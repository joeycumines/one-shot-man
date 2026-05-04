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
	t.Setenv("OSM_CONFIG", "/tmp/custom-config")

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
	t.Setenv("OSM_CONFIG", "")

	got, err := GetConfigPath()
	if err != nil {
		t.Fatalf("GetConfigPath returned error: %v", err)
	}

	// Neither ~/.osm nor ~/.one-shot-man exist, so the new default is used.
	expected := filepath.Join(dir, DefaultConfigDir, "config")
	if got != expected {
		t.Fatalf("expected default path %q, got %q", expected, got)
	}
}

func TestGetConfigPathNewDirExists(t *testing.T) {
	dir := t.TempDir()

	homeVar := "HOME"
	if runtime.GOOS == "windows" {
		homeVar = "USERPROFILE"
	}
	t.Setenv(homeVar, dir)
	t.Setenv("OSM_CONFIG", "")

	// Create ~/.osm/
	if err := os.MkdirAll(filepath.Join(dir, DefaultConfigDir), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	got, err := GetConfigPath()
	if err != nil {
		t.Fatalf("GetConfigPath returned error: %v", err)
	}

	expected := filepath.Join(dir, DefaultConfigDir, "config")
	if got != expected {
		t.Fatalf("expected new path %q, got %q", expected, got)
	}
}

func TestGetConfigPathLegacyFallback(t *testing.T) {
	dir := t.TempDir()

	homeVar := "HOME"
	if runtime.GOOS == "windows" {
		homeVar = "USERPROFILE"
	}
	t.Setenv(homeVar, dir)
	t.Setenv("OSM_CONFIG", "")

	// Create only ~/.one-shot-man/config (legacy)
	legacyDir := filepath.Join(dir, LegacyConfigDir)
	if err := os.MkdirAll(legacyDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "config"), []byte("# legacy"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := GetConfigPath()
	if err != nil {
		t.Fatalf("GetConfigPath returned error: %v", err)
	}

	expected := filepath.Join(dir, LegacyConfigDir, "config")
	if got != expected {
		t.Fatalf("expected legacy path %q, got %q", expected, got)
	}
}

func TestGetConfigPathNewOverridesLegacy(t *testing.T) {
	dir := t.TempDir()

	homeVar := "HOME"
	if runtime.GOOS == "windows" {
		homeVar = "USERPROFILE"
	}
	t.Setenv(homeVar, dir)
	t.Setenv("OSM_CONFIG", "")

	// Create both ~/.osm/ and ~/.one-shot-man/config
	if err := os.MkdirAll(filepath.Join(dir, DefaultConfigDir), 0755); err != nil {
		t.Fatalf("mkdir .osm: %v", err)
	}
	legacyDir := filepath.Join(dir, LegacyConfigDir)
	if err := os.MkdirAll(legacyDir, 0755); err != nil {
		t.Fatalf("mkdir legacy: %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "config"), []byte("# legacy"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := GetConfigPath()
	if err != nil {
		t.Fatalf("GetConfigPath returned error: %v", err)
	}

	// New directory takes precedence
	expected := filepath.Join(dir, DefaultConfigDir, "config")
	if got != expected {
		t.Fatalf("expected new path %q (should override legacy), got %q", expected, got)
	}
}

func TestGetConfigPathNeitherExists(t *testing.T) {
	dir := t.TempDir()

	homeVar := "HOME"
	if runtime.GOOS == "windows" {
		homeVar = "USERPROFILE"
	}
	t.Setenv(homeVar, dir)
	t.Setenv("OSM_CONFIG", "")

	got, err := GetConfigPath()
	if err != nil {
		t.Fatalf("GetConfigPath returned error: %v", err)
	}

	// Neither exists — should default to new path
	expected := filepath.Join(dir, DefaultConfigDir, "config")
	if got != expected {
		t.Fatalf("expected new default path %q, got %q", expected, got)
	}
}

func TestConfigDirConstants(t *testing.T) {
	if DefaultConfigDir != ".osm" {
		t.Fatalf("DefaultConfigDir = %q, want %q", DefaultConfigDir, ".osm")
	}
	if LegacyConfigDir != ".one-shot-man" {
		t.Fatalf("LegacyConfigDir = %q, want %q", LegacyConfigDir, ".one-shot-man")
	}
}

func TestEnsureConfigDirCreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "nested", "config")
	t.Setenv("OSM_CONFIG", configPath)

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
	t.Setenv("OSM_CONFIG", configPath)

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

	if !errors.Is(pathErr.Err, fs.ErrExist) && !errors.Is(pathErr.Err, syscall.ENOTDIR) && !errors.Is(pathErr.Err, os.ErrPermission) {
		t.Fatalf("unexpected underlying error: %v", pathErr.Err)
	}
}
