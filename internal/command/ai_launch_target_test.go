package command

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestTargetPath pins the refusal the reviewers asked to be able to demonstrate:
// two entries whose paths reduce to the same base name must not overwrite one
// another silently.
func TestTargetPath(t *testing.T) {
	seen := map[string]bool{}
	first, err := targetPath("/tmp/launch", "a/config.json", seen)
	if err != nil {
		t.Fatalf("unexpected error for a unique name: %v", err)
	}
	if want := filepath.Join("/tmp/launch", "config.json"); first != want {
		t.Fatalf("got %q, want %q", first, want)
	}

	if _, err := targetPath("/tmp/launch", "b/config.json", seen); err == nil {
		t.Fatal("expected a collision error for two entries sharing a base name, got none")
	} else if !strings.Contains(err.Error(), "two files to the same name") {
		t.Fatalf("expected a collision error, got %v", err)
	}

	other, err := targetPath("/tmp/launch", "a/other.json", seen)
	if err != nil {
		t.Fatalf("a distinct base name must still be accepted: %v", err)
	}
	if want := filepath.Join("/tmp/launch", "other.json"); other != want {
		t.Fatalf("got %q, want %q", other, want)
	}
}
