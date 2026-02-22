package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// ============================================================================
// LoadFromPath — symlink and error-path coverage
// ============================================================================

func TestLoadFromPath_DirectFileSymlink_Rejected(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require elevated privileges on Windows")
	}
	dir := t.TempDir()

	// Create a real config file.
	realFile := filepath.Join(dir, "real-config")
	if err := os.WriteFile(realFile, []byte("test real"), 0600); err != nil {
		t.Fatalf("write real config: %v", err)
	}

	// Create a symlink pointing to the real file.
	linkFile := filepath.Join(dir, "link-config")
	if err := os.Symlink(realFile, linkFile); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	// LoadFromPath should reject the direct-file symlink.
	_, err := LoadFromPath(linkFile)
	if err == nil {
		t.Fatal("expected error for direct-file symlink, got nil")
	}
	if !strings.Contains(err.Error(), "symlink not allowed") {
		t.Errorf("error = %q, want 'symlink not allowed'", err.Error())
	}
}

func TestLoadFromPath_StatError_NotENOENT(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("permission-based stat errors differ on Windows")
	}
	dir := t.TempDir()

	// Create an unreadable directory to trigger a non-ENOENT stat error.
	restricted := filepath.Join(dir, "restricted")
	if err := os.MkdirAll(restricted, 0000); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	t.Cleanup(func() { os.Chmod(restricted, 0755) })

	// Path inside restricted directory — Lstat will fail with permission denied.
	badPath := filepath.Join(restricted, "config")
	_, err := LoadFromPath(badPath)
	if err == nil {
		t.Fatal("expected error for stat failure, got nil")
	}
	if !strings.Contains(err.Error(), "failed to stat config file") {
		t.Errorf("error = %q, want 'failed to stat config file'", err.Error())
	}
}
