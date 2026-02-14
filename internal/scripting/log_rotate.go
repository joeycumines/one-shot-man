package scripting

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// RotatingFileWriter is an io.WriteCloser that implements size-based log
// rotation. When the current file exceeds maxSizeBytes, it is rotated:
// the current file becomes <path>.1, the previous .1 becomes .2, and so on.
// At most maxFiles backup files are retained; older backups are deleted.
//
// All operations are safe for concurrent use.
type RotatingFileWriter struct {
	mu           sync.Mutex
	path         string
	maxSizeBytes int64
	maxFiles     int
	currentSize  int64
	file         *os.File
}

// NewRotatingFileWriter creates a new RotatingFileWriter that writes to the
// given path. maxSizeMB is the maximum file size in megabytes before rotation
// (minimum 1). maxFiles is the maximum number of backup files to keep
// (minimum 0; 0 means no backups, just truncate).
//
// The file is opened in append mode (created if it does not exist).
// The initial file size is determined by stat'ing the file.
func NewRotatingFileWriter(path string, maxSizeMB, maxFiles int) (*RotatingFileWriter, error) {
	if maxSizeMB < 1 {
		maxSizeMB = 1
	}
	if maxFiles < 0 {
		maxFiles = 0
	}

	// Ensure parent directory exists.
	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("log_rotate: mkdir %s: %w", dir, err)
		}
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("log_rotate: open %s: %w", path, err)
	}

	// Determine current file size.
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("log_rotate: stat %s: %w", path, err)
	}

	return &RotatingFileWriter{
		path:         path,
		maxSizeBytes: int64(maxSizeMB) * 1024 * 1024,
		maxFiles:     maxFiles,
		currentSize:  info.Size(),
		file:         f,
	}, nil
}

// Write writes p to the underlying file. If the write would cause the file
// to exceed maxSizeBytes, the file is rotated BEFORE writing p. This means
// individual writes are never split across files. If a single write is larger
// than maxSizeBytes, it is still written to a fresh file (best-effort).
func (w *RotatingFileWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Check if rotation is needed before this write.
	if w.currentSize+int64(len(p)) > w.maxSizeBytes && w.currentSize > 0 {
		if err := w.rotate(); err != nil {
			return 0, fmt.Errorf("log_rotate: rotate: %w", err)
		}
	}

	n, err := w.file.Write(p)
	w.currentSize += int64(n)
	return n, err
}

// Close closes the underlying file.
func (w *RotatingFileWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file != nil {
		return w.file.Close()
	}
	return nil
}

// rotate performs the actual file rotation. Must be called with w.mu held.
func (w *RotatingFileWriter) rotate() error {
	// Close current file.
	if err := w.file.Close(); err != nil {
		return err
	}

	// Shift existing backups: .N → .N+1 (in reverse order to avoid overwriting).
	// Then rename the current file to .1.
	// Finally, delete any backup beyond maxFiles.

	// Collect existing backup numbers.
	backups := w.listBackups()

	// Sort descending so we rename highest first.
	sort.Sort(sort.Reverse(sort.IntSlice(backups)))

	for _, num := range backups {
		src := w.backupPath(num)
		if num+1 > w.maxFiles {
			// Beyond retention limit — delete.
			_ = os.Remove(src)
		} else {
			dst := w.backupPath(num + 1)
			_ = os.Rename(src, dst)
		}
	}

	// Rename current file to .1 (unless maxFiles is 0, in which case just remove).
	if w.maxFiles > 0 {
		_ = os.Rename(w.path, w.backupPath(1))
	} else {
		_ = os.Remove(w.path)
	}

	// Open a fresh file.
	f, err := os.OpenFile(w.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	w.file = f
	w.currentSize = 0
	return nil
}

// backupPath returns the path for backup number n (e.g., "app.log.1").
func (w *RotatingFileWriter) backupPath(n int) string {
	return w.path + "." + strconv.Itoa(n)
}

// listBackups returns the sorted list of existing backup numbers.
func (w *RotatingFileWriter) listBackups() []int {
	dir := filepath.Dir(w.path)
	base := filepath.Base(w.path)
	prefix := base + "."

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var nums []int
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		suffix := name[len(prefix):]
		n, err := strconv.Atoi(suffix)
		if err != nil || n < 1 {
			continue
		}
		nums = append(nums, n)
	}

	sort.Ints(nums)
	return nums
}

// Compile-time check that RotatingFileWriter implements io.WriteCloser.
var _ io.WriteCloser = (*RotatingFileWriter)(nil)
