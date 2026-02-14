package scripting

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
)

func TestRotatingFileWriter_BasicWrite(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")

	w, err := NewRotatingFileWriter(path, 1, 3) // 1 MB max, 3 backups
	if err != nil {
		t.Fatalf("NewRotatingFileWriter: %v", err)
	}
	defer w.Close()

	msg := "hello world\n"
	n, err := w.Write([]byte(msg))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if n != len(msg) {
		t.Fatalf("Write returned %d, want %d", n, len(msg))
	}

	// Verify file contents.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != msg {
		t.Fatalf("file content = %q, want %q", string(data), msg)
	}
}

func TestRotatingFileWriter_RotatesAtSizeLimit(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")

	// Use a tiny max size to trigger rotation easily.
	// We can't go below 1 MB in the public API, so we construct manually.
	w := &RotatingFileWriter{
		path:         path,
		maxSizeBytes: 50, // 50 bytes
		maxFiles:     3,
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	w.file = f

	// Write 40 bytes (under limit).
	line := strings.Repeat("A", 39) + "\n" // 40 bytes
	if _, err := w.Write([]byte(line)); err != nil {
		t.Fatalf("Write 1: %v", err)
	}
	if w.currentSize != 40 {
		t.Fatalf("currentSize = %d, want 40", w.currentSize)
	}

	// Write another 40 bytes (40+40=80 > 50) — should trigger rotation.
	if _, err := w.Write([]byte(line)); err != nil {
		t.Fatalf("Write 2: %v", err)
	}

	// The current file should now contain only the second write (40 bytes).
	if w.currentSize != 40 {
		t.Fatalf("after rotation currentSize = %d, want 40", w.currentSize)
	}

	// Backup file .1 should exist and contain the first write.
	backup1, err := os.ReadFile(path + ".1")
	if err != nil {
		t.Fatalf("ReadFile backup .1: %v", err)
	}
	if string(backup1) != line {
		t.Fatalf("backup .1 content = %q, want %q", string(backup1), line)
	}

	// Current file should contain the second write.
	w.Close()
	current, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile current: %v", err)
	}
	if string(current) != line {
		t.Fatalf("current file content = %q, want %q", string(current), line)
	}
}

func TestRotatingFileWriter_MaxFilesEnforced(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")

	w := &RotatingFileWriter{
		path:         path,
		maxSizeBytes: 20,
		maxFiles:     2, // Keep at most 2 backups (.1 and .2)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	w.file = f

	line := func(id int) string {
		return "line-" + strconv.Itoa(id) + "\n"
	}

	// Write 4 chunks, each triggering rotation after the first.
	for i := 1; i <= 4; i++ {
		if _, err := w.Write([]byte(strings.Repeat(line(i), 3))); err != nil {
			t.Fatalf("Write %d: %v", i, err)
		}
	}
	w.Close()

	// Only .1 and .2 should exist. .3 should NOT exist.
	if _, err := os.Stat(path + ".1"); err != nil {
		t.Errorf("expected backup .1 to exist: %v", err)
	}
	if _, err := os.Stat(path + ".2"); err != nil {
		t.Errorf("expected backup .2 to exist: %v", err)
	}
	if _, err := os.Stat(path + ".3"); !os.IsNotExist(err) {
		t.Errorf("expected backup .3 to NOT exist, but stat returned: %v", err)
	}
}

func TestRotatingFileWriter_ZeroMaxFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")

	w := &RotatingFileWriter{
		path:         path,
		maxSizeBytes: 20,
		maxFiles:     0, // No backups — just truncate.
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	w.file = f

	// Write enough to trigger rotation twice.
	chunk := strings.Repeat("X", 25) + "\n"
	for i := 0; i < 3; i++ {
		if _, err := w.Write([]byte(chunk)); err != nil {
			t.Fatalf("Write %d: %v", i, err)
		}
	}
	w.Close()

	// No backup files should exist.
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "test.log.") {
			t.Errorf("unexpected backup file: %s", e.Name())
		}
	}

	// Current file should contain only the last chunk.
	data, _ := os.ReadFile(path)
	if string(data) != chunk {
		t.Errorf("current file = %q, want %q", string(data), chunk)
	}
}

func TestRotatingFileWriter_ConcurrentWrites(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")

	w := &RotatingFileWriter{
		path:         path,
		maxSizeBytes: 200,
		maxFiles:     5,
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	w.file = f

	var wg sync.WaitGroup
	numGoroutines := 10
	writesPerGoroutine := 50

	wg.Add(numGoroutines)
	for g := 0; g < numGoroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < writesPerGoroutine; i++ {
				msg := "goroutine-" + strconv.Itoa(id) + "-write-" + strconv.Itoa(i) + "\n"
				if _, err := w.Write([]byte(msg)); err != nil {
					t.Errorf("goroutine %d write %d: %v", id, i, err)
					return
				}
			}
		}(g)
	}
	wg.Wait()
	w.Close()

	// Verify no panics or corrupted files. Count total lines across all files.
	totalLines := 0
	allFiles := []string{path}
	for n := 1; n <= 5; n++ {
		bp := path + "." + strconv.Itoa(n)
		if _, err := os.Stat(bp); err == nil {
			allFiles = append(allFiles, bp)
		}
	}

	for _, fp := range allFiles {
		data, err := os.ReadFile(fp)
		if err != nil {
			t.Fatalf("ReadFile %s: %v", fp, err)
		}
		lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
		if len(lines) > 0 && lines[0] != "" {
			totalLines += len(lines)
		}
	}

	// We should have some lines (not all 500 because rotation deletes old backups,
	// but at least some from the most recent files).
	if totalLines == 0 {
		t.Fatal("expected at least some lines across all log files, got 0")
	}
}

func TestRotatingFileWriter_CreatesParentDirs(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "deep", "test.log")

	w, err := NewRotatingFileWriter(path, 1, 3)
	if err != nil {
		t.Fatalf("NewRotatingFileWriter: %v", err)
	}
	defer w.Close()

	if _, err := w.Write([]byte("ok\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "ok\n" {
		t.Fatalf("file content = %q, want %q", string(data), "ok\n")
	}
}

func TestRotatingFileWriter_AppendsToExisting(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")

	// Pre-populate the file.
	if err := os.WriteFile(path, []byte("existing\n"), 0644); err != nil {
		t.Fatal(err)
	}

	w, err := NewRotatingFileWriter(path, 1, 3)
	if err != nil {
		t.Fatalf("NewRotatingFileWriter: %v", err)
	}
	defer w.Close()

	if w.currentSize != 9 { // "existing\n" = 9 bytes
		t.Fatalf("initial currentSize = %d, want 9", w.currentSize)
	}

	if _, err := w.Write([]byte("appended\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "existing\nappended\n" {
		t.Fatalf("file content = %q, want %q", string(data), "existing\nappended\n")
	}
}

func TestRotatingFileWriter_MinMaxSizeMB(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")

	// Passing 0 should be clamped to 1.
	w, err := NewRotatingFileWriter(path, 0, 3)
	if err != nil {
		t.Fatalf("NewRotatingFileWriter: %v", err)
	}
	defer w.Close()

	if w.maxSizeBytes != 1*1024*1024 {
		t.Fatalf("maxSizeBytes = %d, want %d", w.maxSizeBytes, 1*1024*1024)
	}
}

func TestRotatingFileWriter_NegativeMaxFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")

	// Passing -1 should be clamped to 0.
	w, err := NewRotatingFileWriter(path, 1, -1)
	if err != nil {
		t.Fatalf("NewRotatingFileWriter: %v", err)
	}
	defer w.Close()

	if w.maxFiles != 0 {
		t.Fatalf("maxFiles = %d, want 0", w.maxFiles)
	}
}
