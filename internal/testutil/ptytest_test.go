package testutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRepoRoot(t *testing.T) {
	t.Parallel()

	// Get the real repo root by walking up from this test file's location.
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd: %v", err)
	}
	root := RepoRoot(wd)

	// The repo root must contain cmd/osm/main.go.
	marker := filepath.Join(root, "cmd", "osm", "main.go")
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("RepoRoot returned %q, but marker %q does not exist: %v", root, marker, err)
	}
}

func TestRepoRootFromWD(t *testing.T) {
	t.Parallel()

	root := RepoRootFromWD()
	marker := filepath.Join(root, "cmd", "osm", "main.go")
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("RepoRootFromWD returned %q, but marker %q does not exist: %v", root, marker, err)
	}
}

func TestRepoRoot_Fallback(t *testing.T) {
	t.Parallel()

	// When starting from a path that will never find the marker (e.g., "/"),
	// it should fall back to the current working directory.
	root := RepoRoot("/")
	// Should return something — either "/" itself or the cwd fallback.
	if root == "" {
		t.Fatal("RepoRoot returned empty string")
	}
}
