package scripting

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// ==========================================================================
// RotatingFileWriter.Close — nil file (double close)
// ==========================================================================

func TestRotatingFileWriter_Close_NilFile(t *testing.T) {
	t.Parallel()
	w := &RotatingFileWriter{} // file is nil
	if err := w.Close(); err != nil {
		t.Errorf("Close with nil file should return nil, got: %v", err)
	}
}

// ==========================================================================
// RotatingFileWriter.Close — after successful close
// ==========================================================================

func TestRotatingFileWriter_DoubleClose(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")

	w, err := NewRotatingFileWriter(path, 1, 3)
	if err != nil {
		t.Fatal(err)
	}

	// First close should succeed.
	if err := w.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}

	// Second close with nil file should be a no-op.
	w.file = nil
	if err := w.Close(); err != nil {
		t.Errorf("second Close with nil file should succeed, got: %v", err)
	}
}

// ==========================================================================
// RotatingFileWriter — multiple rotations shifting backup chain
// ==========================================================================

func TestRotatingFileWriter_MultipleRotations_BackupChain(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")

	w := &RotatingFileWriter{
		path:         path,
		maxSizeBytes: 20,
		maxFiles:     3,
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	w.file = f

	// Write 5 chunks that each trigger rotation.
	for i := 1; i <= 5; i++ {
		chunk := strings.Repeat(strconv.Itoa(i), 25) + "\n" // 26 bytes > 20
		if _, err := w.Write([]byte(chunk)); err != nil {
			t.Fatalf("Write %d: %v", i, err)
		}
	}
	w.Close()

	// Backups .1, .2, .3 should exist. .4, .5 should NOT.
	for _, n := range []int{1, 2, 3} {
		bp := path + "." + strconv.Itoa(n)
		if _, err := os.Stat(bp); err != nil {
			t.Errorf("backup .%d should exist: %v", n, err)
		}
	}
	for _, n := range []int{4, 5} {
		bp := path + "." + strconv.Itoa(n)
		if _, err := os.Stat(bp); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("backup .%d should NOT exist, stat returned: %v", n, err)
		}
	}

	// .1 should contain the 4th write (most recent backup).
	data, err := os.ReadFile(path + ".1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), "4444") {
		t.Errorf("backup .1 should contain write #4, got: %q", string(data[:20]))
	}
}

// ==========================================================================
// RotatingFileWriter — oversized single write (larger than maxSizeBytes)
// ==========================================================================

func TestRotatingFileWriter_OversizedSingleWrite(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "big.log")

	w := &RotatingFileWriter{
		path:         path,
		maxSizeBytes: 10,
		maxFiles:     2,
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	w.file = f

	// Write a single chunk that's bigger than maxSizeBytes.
	// currentSize is 0, so rotation condition (currentSize+len > max && currentSize > 0)
	// is false. The write goes directly to the file (best-effort).
	huge := strings.Repeat("X", 100)
	n, err := w.Write([]byte(huge))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if n != 100 {
		t.Errorf("expected 100 bytes written, got %d", n)
	}
	if w.currentSize != 100 {
		t.Errorf("currentSize = %d, want 100", w.currentSize)
	}
	w.Close()

	data, _ := os.ReadFile(path)
	if string(data) != huge {
		t.Error("file should contain the oversized write")
	}
}

// ==========================================================================
// RotatingFileWriter — listBackups with malformed backup files
// ==========================================================================

func TestRotatingFileWriter_ListBackups_MalformedNames(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")

	// Create files that look like backups but with non-numeric suffixes.
	for _, name := range []string{"app.log.abc", "app.log.0", "app.log.-1", "app.log.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("junk"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	// Also create a valid backup.
	if err := os.WriteFile(filepath.Join(dir, "app.log.2"), []byte("valid"), 0644); err != nil {
		t.Fatal(err)
	}

	w := &RotatingFileWriter{path: path}
	backups := w.listBackups()

	// Only backup #2 should be found (abc, 0, -1, txt are invalid).
	if len(backups) != 1 || backups[0] != 2 {
		t.Errorf("expected [2], got %v", backups)
	}
}

// ==========================================================================
// NewRotatingFileWriter — invalid path
// ==========================================================================

func TestNewRotatingFileWriter_InvalidPath(t *testing.T) {
	t.Parallel()
	// Use a null byte to make the path invalid on all platforms.
	_, err := NewRotatingFileWriter("/dev/null\x00/test.log", 1, 3)
	if err == nil {
		t.Fatal("expected error for invalid path")
	}
}
