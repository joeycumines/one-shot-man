package scripting

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// T141: logging.go — WithAttrs/WithGroup fileHandler + groupPrefix combos
// ============================================================================

// TestTUILogHandler_WithAttrs_ForwardsToFileHandler verifies that WithAttrs
// creates a new handler that forwards the attrs to the fileHandler.
func TestTUILogHandler_WithAttrs_ForwardsToFileHandler(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	fileHandler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})

	handler := &tuiLogHandler{
		shared: &tuiLogHandlerShared{
			entries: make([]logEntry, 0),
			maxSize: 100,
			level:   slog.LevelDebug,
		},
		fileHandler: fileHandler,
	}

	newH := handler.WithAttrs([]slog.Attr{slog.String("component", "auth")}).(*tuiLogHandler)
	require.NotNil(t, newH.fileHandler)
	assert.NotSame(t, handler.fileHandler, newH.fileHandler, "fileHandler should be a new handler with attrs forwarded")
	assert.Len(t, newH.preAttrs, 1)
	assert.Equal(t, "component", newH.preAttrs[0].Key)

	// Actually log through the new handler and verify file output has the forwarded attr.
	record := slog.NewRecord(time.Now(), slog.LevelInfo, "test msg", 0)
	err := newH.Handle(context.Background(), record)
	require.NoError(t, err)

	assert.Contains(t, buf.String(), `"component"`)
	assert.Contains(t, buf.String(), `"auth"`)
}

// TestTUILogHandler_WithGroup_ForwardsToFileHandler verifies that WithGroup
// creates a new handler that forwards the group to the fileHandler.
func TestTUILogHandler_WithGroup_ForwardsToFileHandler(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	fileHandler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})

	handler := &tuiLogHandler{
		shared: &tuiLogHandlerShared{
			entries: make([]logEntry, 0),
			maxSize: 100,
			level:   slog.LevelDebug,
		},
		fileHandler: fileHandler,
	}

	newH := handler.WithGroup("mygroup").(*tuiLogHandler)
	require.NotNil(t, newH.fileHandler)
	assert.Equal(t, "mygroup", newH.groupPrefix)

	// Log through it with an attr and verify the group prefix appears in the JSON output.
	record := slog.NewRecord(time.Now(), slog.LevelInfo, "grouped msg", 0)
	record.AddAttrs(slog.String("key", "val"))
	err := newH.Handle(context.Background(), record)
	require.NoError(t, err)

	// In our tuiLogHandler, the group prefix is applied to memory entries.
	require.Len(t, newH.shared.entries, 1)
	assert.Equal(t, "val", newH.shared.entries[0].Attrs["mygroup.key"])

	// The JSON handler should also have the group.
	assert.Contains(t, buf.String(), "mygroup")
}

// TestTUILogHandler_WithGroup_NestedPrefix verifies that calling WithGroup
// twice compounds the prefix: "a" then "b" → "a.b".
func TestTUILogHandler_WithGroup_NestedPrefix(t *testing.T) {
	t.Parallel()
	handler := &tuiLogHandler{
		shared: &tuiLogHandlerShared{
			entries: make([]logEntry, 0, 10),
			maxSize: 100,
			level:   slog.LevelDebug,
		},
	}

	h1 := handler.WithGroup("outer").(*tuiLogHandler)
	assert.Equal(t, "outer", h1.groupPrefix)

	h2 := h1.WithGroup("inner").(*tuiLogHandler)
	assert.Equal(t, "outer.inner", h2.groupPrefix)
	assert.Same(t, handler.shared, h2.shared, "nested handlers share state")

	// Log through the deeply nested handler.
	record := slog.NewRecord(time.Now(), slog.LevelInfo, "nested msg", 0)
	record.AddAttrs(slog.String("x", "1"))
	err := h2.Handle(context.Background(), record)
	require.NoError(t, err)

	require.Len(t, handler.shared.entries, 1)
	assert.Equal(t, "1", handler.shared.entries[0].Attrs["outer.inner.x"])
}

// TestTUILogHandler_Handle_GroupPrefixOnPreAttrs verifies that preAttrs
// from WithAttrs() get the group prefix applied during Handle().
func TestTUILogHandler_Handle_GroupPrefixOnPreAttrs(t *testing.T) {
	t.Parallel()
	handler := &tuiLogHandler{
		shared: &tuiLogHandlerShared{
			entries: make([]logEntry, 0, 10),
			maxSize: 100,
			level:   slog.LevelDebug,
		},
	}

	// First add a group, then add attrs.
	h1 := handler.WithGroup("grp").(*tuiLogHandler)
	h2 := h1.WithAttrs([]slog.Attr{slog.String("pre", "attached")}).(*tuiLogHandler)

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "test", 0)
	record.AddAttrs(slog.String("rec", "attr"))
	err := h2.Handle(context.Background(), record)
	require.NoError(t, err)

	require.Len(t, handler.shared.entries, 1)
	// PreAttr "pre" should have group prefix "grp."
	assert.Equal(t, "attached", handler.shared.entries[0].Attrs["grp.pre"])
	// Record attr "rec" should also have group prefix "grp."
	assert.Equal(t, "attr", handler.shared.entries[0].Attrs["grp.rec"])
}

// TestTUILogHandler_WithGroup_FileHandler_NestedGroup verifies nested
// WithGroup with fileHandler chains correctly.
func TestTUILogHandler_WithGroup_FileHandler_NestedGroup(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	fileHandler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})

	handler := &tuiLogHandler{
		shared: &tuiLogHandlerShared{
			entries: make([]logEntry, 0, 10),
			maxSize: 100,
			level:   slog.LevelDebug,
		},
		fileHandler: fileHandler,
	}

	h1 := handler.WithGroup("a").(*tuiLogHandler)
	h2 := h1.WithGroup("b").(*tuiLogHandler)
	require.NotNil(t, h2.fileHandler)
	assert.Equal(t, "a.b", h2.groupPrefix)

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "deep", 0)
	record.AddAttrs(slog.String("k", "v"))
	err := h2.Handle(context.Background(), record)
	require.NoError(t, err)

	// Memory entries should have "a.b.k"
	require.Len(t, handler.shared.entries, 1)
	assert.Equal(t, "v", handler.shared.entries[0].Attrs["a.b.k"])

	// JSON output should have the nested group structure
	output := buf.String()
	assert.Contains(t, output, "deep")
}

// TestTUILogHandler_WithAttrs_EmptyReturnsOriginal verifies the short-circuit.
func TestTUILogHandler_WithAttrs_EmptyReturnsOriginal(t *testing.T) {
	t.Parallel()
	handler := &tuiLogHandler{
		shared: &tuiLogHandlerShared{level: slog.LevelDebug},
	}
	got := handler.WithAttrs(nil)
	assert.Same(t, handler, got, "empty attrs should return same handler")

	got = handler.WithAttrs([]slog.Attr{})
	assert.Same(t, handler, got, "zero-length attrs should return same handler")
}

// TestTUILogHandler_WithGroup_EmptyNameReturnsOriginal verifies the short-circuit.
func TestTUILogHandler_WithGroup_EmptyNameReturnsOriginal(t *testing.T) {
	t.Parallel()
	handler := &tuiLogHandler{
		shared: &tuiLogHandlerShared{level: slog.LevelDebug},
	}
	got := handler.WithGroup("")
	assert.Same(t, handler, got, "empty group name should return same handler")
}

// TestNewTUILogger_FileLogging_WithGroupAndAttrs exercises the full integration
// path: NewTUILogger with logFile → WithGroup → WithAttrs → log → verify JSON output.
func TestNewTUILogger_FileLogging_WithGroupAndAttrs(t *testing.T) {
	t.Parallel()
	var logBuf bytes.Buffer
	var tuiBuf bytes.Buffer
	logger := NewTUILogger(&tuiBuf, &logBuf, 100, slog.LevelDebug)

	// Use the underlying slog.Logger to exercise the handler chain.
	grouped := logger.Logger().WithGroup("svc").With("version", "1.0")
	grouped.Info("hello", "detail", "test123")

	// Verify the JSON file output contains the group.
	output := logBuf.String()
	assert.NotEmpty(t, output, "JSON log should be written")

	var parsed map[string]any
	err := json.Unmarshal([]byte(output), &parsed)
	require.NoError(t, err)
	assert.Equal(t, "hello", parsed["msg"])

	// slog.JSONHandler nests groups as JSON objects.
	svcMap, ok := parsed["svc"].(map[string]any)
	require.True(t, ok, "svc group should be a JSON object, got: %v", parsed)
	assert.Equal(t, "1.0", svcMap["version"])
	assert.Equal(t, "test123", svcMap["detail"])

	// Verify the in-memory entries also have the group prefix.
	logs := logger.GetLogs()
	require.Len(t, logs, 1)
	assert.Equal(t, "1.0", logs[0].Attrs["svc.version"])
	assert.Equal(t, "test123", logs[0].Attrs["svc.detail"])
}

// ============================================================================
// T142: log_rotate.go — error path coverage
// ============================================================================

// TestNewRotatingFileWriter_MkdirAllError verifies the MkdirAll failure path.
func TestNewRotatingFileWriter_MkdirAllError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission manipulation unreliable on Windows")
	}
	t.Parallel()
	dir := t.TempDir()

	// Create a file where the parent directory should be — MkdirAll will fail.
	blocker := filepath.Join(dir, "blocker")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0644))

	_, err := NewRotatingFileWriter(filepath.Join(blocker, "sub", "test.log"), 1, 3)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "log_rotate: mkdir")
}

// TestRotatingFileWriter_Rotate_OpenFileFailure exercises the final os.OpenFile
// error path in rotate() by removing the parent directory after closing the file.
func TestRotatingFileWriter_Rotate_OpenFileFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("open file handle prevents directory removal on Windows")
	}
	t.Parallel()

	// Create a nested directory so we can remove the leaf.
	base := t.TempDir()
	dir := filepath.Join(base, "logs")
	require.NoError(t, os.Mkdir(dir, 0755))
	path := filepath.Join(dir, "app.log")

	require.NoError(t, os.WriteFile(path, []byte("data"), 0644))
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	require.NoError(t, err)

	w := &RotatingFileWriter{
		path:         path,
		maxSizeBytes: 10,
		maxFiles:     0, // no backups — just remove
		file:         f,
	}

	// Remove the parent directory. Close will succeed on the open fd,
	// but the subsequent Remove and OpenFile will fail (ENOENT).
	require.NoError(t, os.RemoveAll(dir))

	err = w.rotate()
	assert.Error(t, err, "rotate should fail when parent dir is removed")
}

// TestRotatingFileWriter_Rotate_RemoveBeyondRetention exercises the os.Remove
// path for backups beyond maxFiles.
func TestRotatingFileWriter_Rotate_RemoveBeyondRetention(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")

	w := &RotatingFileWriter{
		path:         path,
		maxSizeBytes: 20,
		maxFiles:     1, // only keep .1
	}

	// Create the main file and .1 and .2 backups.
	require.NoError(t, os.WriteFile(path, []byte("current"), 0644))
	require.NoError(t, os.WriteFile(path+".1", []byte("backup1"), 0644))
	require.NoError(t, os.WriteFile(path+".2", []byte("backup2"), 0644))

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	require.NoError(t, err)
	w.file = f

	err = w.rotate()
	require.NoError(t, err)
	defer w.Close()

	// .2 should be deleted (beyond maxFiles=1).
	_, err = os.Stat(path + ".2")
	assert.True(t, errors.Is(err, os.ErrNotExist), "backup .2 should be deleted")

	// .1 should exist (was the old main file).
	_, err = os.Stat(path + ".1")
	assert.NoError(t, err, "backup .1 should exist")
}

// TestRotatingFileWriter_Rotate_MaxFilesZero exercises the maxFiles==0 path
// where the current file is removed instead of renamed.
func TestRotatingFileWriter_Rotate_MaxFilesZero(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")

	w := &RotatingFileWriter{
		path:         path,
		maxSizeBytes: 10,
		maxFiles:     0, // no backups
	}

	require.NoError(t, os.WriteFile(path, []byte("old data"), 0644))
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	require.NoError(t, err)
	w.file = f

	err = w.rotate()
	require.NoError(t, err)
	defer w.Close()

	// No .1 backup should exist.
	_, err = os.Stat(path + ".1")
	assert.True(t, errors.Is(err, os.ErrNotExist), "no backup should exist with maxFiles=0")

	// The main file should be fresh (empty or newly created).
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, int64(0), info.Size(), "rotated file should be empty")
}

// TestRotatingFileWriter_Write_RotateError exercises the rotate error path
// during Write (line 83-85).
func TestRotatingFileWriter_Write_RotateError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("open file handle prevents directory removal on Windows")
	}
	t.Parallel()

	// Create a nested directory so we can remove the leaf.
	base := t.TempDir()
	dir := filepath.Join(base, "logs")
	require.NoError(t, os.Mkdir(dir, 0755))
	path := filepath.Join(dir, "app.log")

	w, err := NewRotatingFileWriter(path, 1, 3)
	require.NoError(t, err)
	defer w.Close()

	w.maxSizeBytes = 10
	w.currentSize = 15 // pretend we already have data

	// Remove the parent directory so rotation's OpenFile will fail.
	require.NoError(t, os.RemoveAll(dir))

	_, err = w.Write([]byte("trigger rotation"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "log_rotate: rotate")
}

// ============================================================================
// T146: module_hardening.go — validateModulePaths additional coverage
// Tests for basic paths (empty, nonexistent, file-not-dir, valid, mixed) are
// already in module_hardening_test.go. These tests cover the symlink-specific
// EvalSymlinks error path.
// ============================================================================

// TestValidateModulePaths_SymlinkToDirectory verifies that a valid symlink
// pointing to a real directory is resolved and accepted.
func TestValidateModulePaths_SymlinkToDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation may require elevated privileges on Windows")
	}
	t.Parallel()
	dir := t.TempDir()
	target := filepath.Join(dir, "real-dir")
	require.NoError(t, os.Mkdir(target, 0755))

	link := filepath.Join(dir, "link-to-dir")
	require.NoError(t, os.Symlink(target, link))

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn + 10}))
	result := validateModulePaths([]string{link}, logger)
	require.Len(t, result, 1)
	// Should resolve to the target (symlink evaluated).
	resolved, _ := filepath.EvalSymlinks(target)
	assert.Equal(t, resolved, result[0])
}
