package scripting

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestSaveHistory tests the new saveHistory function for persisting
// prompt history to disk with deduplication and size limits.
func TestSaveHistory(t *testing.T) {
	t.Parallel()

	t.Run("empty filename is no-op", func(t *testing.T) {
		err := saveHistory("", []string{"a", "b"}, 100)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("writes entries to file", func(t *testing.T) {
		dir := t.TempDir()
		f := filepath.Join(dir, "history.txt")

		err := saveHistory(f, []string{"ls", "pwd", "exit"}, 100)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		content, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		expected := "ls\npwd\nexit\n"
		if string(content) != expected {
			t.Errorf("expected %q, got %q", expected, string(content))
		}
	})

	t.Run("deduplicates consecutive identical entries", func(t *testing.T) {
		dir := t.TempDir()
		f := filepath.Join(dir, "history.txt")

		err := saveHistory(f, []string{"ls", "ls", "ls", "pwd", "pwd", "ls"}, 100)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		content, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		expected := "ls\npwd\nls\n"
		if string(content) != expected {
			t.Errorf("expected %q, got %q", expected, string(content))
		}
	})

	t.Run("trims to maxEntries keeping most recent", func(t *testing.T) {
		dir := t.TempDir()
		f := filepath.Join(dir, "history.txt")

		err := saveHistory(f, []string{"a", "b", "c", "d", "e"}, 3)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		content, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		expected := "c\nd\ne\n"
		if string(content) != expected {
			t.Errorf("expected %q, got %q", expected, string(content))
		}
	})

	t.Run("creates parent directories", func(t *testing.T) {
		dir := t.TempDir()
		f := filepath.Join(dir, "sub", "dir", "history.txt")

		err := saveHistory(f, []string{"hello"}, 100)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		content, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(content), "hello") {
			t.Errorf("expected 'hello' in file, got %q", string(content))
		}
	})

	t.Run("skips empty and whitespace-only entries", func(t *testing.T) {
		dir := t.TempDir()
		f := filepath.Join(dir, "history.txt")

		err := saveHistory(f, []string{"a", "", "  ", "b"}, 100)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		content, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		expected := "a\nb\n"
		if string(content) != expected {
			t.Errorf("expected %q, got %q", expected, string(content))
		}
	})

	t.Run("zero maxEntries means no limit", func(t *testing.T) {
		dir := t.TempDir()
		f := filepath.Join(dir, "history.txt")

		entries := []string{"a", "b", "c", "d", "e"}
		err := saveHistory(f, entries, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		content, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		expected := "a\nb\nc\nd\ne\n"
		if string(content) != expected {
			t.Errorf("expected %q, got %q", expected, string(content))
		}
	})

	t.Run("round-trip with loadHistory", func(t *testing.T) {
		dir := t.TempDir()
		f := filepath.Join(dir, "history.txt")

		original := []string{"cmd1", "cmd2", "cmd3"}
		if err := saveHistory(f, original, 100); err != nil {
			t.Fatalf("save failed: %v", err)
		}

		loaded := loadHistory(f)
		if len(loaded) != len(original) {
			t.Fatalf("expected %d entries, got %d", len(original), len(loaded))
		}
		for i, v := range original {
			if loaded[i] != v {
				t.Errorf("entry %d: expected %q, got %q", i, v, loaded[i])
			}
		}
	})

	t.Run("handles all empty entries", func(t *testing.T) {
		dir := t.TempDir()
		f := filepath.Join(dir, "history.txt")

		err := saveHistory(f, []string{"", "", ""}, 100)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		content, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		// Only a trailing newline from empty content
		if string(content) != "\n" {
			t.Errorf("expected just newline, got %q", string(content))
		}
	})

	t.Run("nil entries slice", func(t *testing.T) {
		dir := t.TempDir()
		f := filepath.Join(dir, "history.txt")

		err := saveHistory(f, nil, 100)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		content, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if string(content) != "\n" {
			t.Errorf("expected just newline, got %q", string(content))
		}
	})

	t.Run("first entry empty does not panic", func(t *testing.T) {
		dir := t.TempDir()
		f := filepath.Join(dir, "history.txt")

		// Regression: when first entry is empty and skipped, dedup must not
		// index into an empty slice.
		err := saveHistory(f, []string{"", "a", "b"}, 100)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		content, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		expected := "a\nb\n"
		if string(content) != expected {
			t.Errorf("expected %q, got %q", expected, string(content))
		}
	})

	t.Run("dedup then trim interaction", func(t *testing.T) {
		dir := t.TempDir()
		f := filepath.Join(dir, "history.txt")

		// 10 entries, pairs of duplicates → 5 unique → trim to 3 keeps last 3
		err := saveHistory(f, []string{"a", "a", "b", "b", "c", "c", "d", "d", "e", "e"}, 3)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		content, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		expected := "c\nd\ne\n"
		if string(content) != expected {
			t.Errorf("expected %q, got %q", expected, string(content))
		}
	})

	t.Run("file permissions are 0600", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("Windows does not support POSIX file permissions")
		}
		dir := t.TempDir()
		f := filepath.Join(dir, "history.txt")

		if err := saveHistory(f, []string{"secret"}, 100); err != nil {
			t.Fatal(err)
		}

		info, err := os.Stat(f)
		if err != nil {
			t.Fatal(err)
		}
		perm := info.Mode().Perm()
		if perm != 0o600 {
			t.Errorf("expected file permissions 0600, got %04o", perm)
		}
	})

	t.Run("overwrites existing file", func(t *testing.T) {
		dir := t.TempDir()
		f := filepath.Join(dir, "history.txt")

		// Write initial content
		if err := saveHistory(f, []string{"old1", "old2"}, 100); err != nil {
			t.Fatalf("first save: %v", err)
		}

		// Overwrite with new content
		if err := saveHistory(f, []string{"new1"}, 100); err != nil {
			t.Fatalf("second save: %v", err)
		}

		content, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		expected := "new1\n"
		if string(content) != expected {
			t.Errorf("expected %q, got %q", expected, string(content))
		}
	})
}
