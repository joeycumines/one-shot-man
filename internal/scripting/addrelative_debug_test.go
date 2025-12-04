package scripting

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDebug_AddRelativePath_BackslashBehavior(t *testing.T) {
	tmpDir := t.TempDir()

	nested := filepath.Join(tmpDir, "dir", "file.txt")
	if err := os.MkdirAll(filepath.Dir(nested), 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(nested, []byte("hello"), 0644); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	cm, err := NewContextManager(tmpDir)
	if err != nil {
		t.Fatalf("failed to create cm: %v", err)
	}

	// On Windows we expect Windows-style labels (with backslashes) to work
	// as separators. On POSIX we *do not* expect AddRelativePath to mutate
	// the input label and therefore skip this assertion there.
	if runtime.GOOS == "windows" {
		owner, err := cm.AddRelativePath("dir\\file.txt")
		if err != nil {
			t.Fatalf("AddRelativePath failed resolving backslash label: %v", err)
		}
		if owner == "" {
			t.Fatalf("AddRelativePath returned empty owner for dir\\file.txt")
		}
	} else {
		t.Skip("Windows-specific path-separator behavior; skipping on non-windows hosts")
	}
}
