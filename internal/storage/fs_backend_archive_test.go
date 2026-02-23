package storage

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestArchiveSession_DestAlreadyExists_DoesNotOverwrite(t *testing.T) {
	dir := t.TempDir()
	SetTestPaths(dir)
	defer ResetPaths()

	sid := "sid-atomic-check"

	// Create real backend and a session file.
	b, err := NewFileSystemBackend(sid)
	if err != nil {
		t.Fatalf("failed to create backend: %v", err)
	}
	defer b.Close()

	// Save a simple session so there's content to archive.
	sess := &Session{ID: sid, Version: CurrentSchemaVersion, CreateTime: time.Now(), UpdateTime: time.Now()}
	if err := b.SaveSession(sess); err != nil {
		t.Fatalf("failed to save session: %v", err)
	}

	// Create a destination file that already exists with distinct contents.
	archivePath, err := ArchiveSessionFilePath(sid, time.Now(), 0)
	if err != nil {
		t.Fatalf("ArchiveSessionFilePath failed: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(archivePath), 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(archivePath, []byte("PREEXISTING"), 0644); err != nil {
		t.Fatalf("failed to create preexisting dest: %v", err)
	}

	// Attempt archive should return os.ErrExist and must not overwrite the preexisting file.
	if err := b.ArchiveSession(sid, archivePath); err == nil {
		t.Fatalf("expected ArchiveSession to return an error when dest exists")
	} else if !errors.Is(err, os.ErrExist) {
		t.Fatalf("expected os.ErrExist, got: %v", err)
	}

	// Ensure preexisting file retains original contents.
	data, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatalf("failed to read archive file: %v", err)
	}
	if string(data) != "PREEXISTING" {
		t.Fatalf("archive destination was modified; want PREEXISTING, got: %q", string(data))
	}

	// Session file should still exist and be loadable.
	sessionPath, err := SessionFilePath(sid)
	if err != nil {
		t.Fatalf("SessionFilePath failed: %v", err)
	}
	if _, err := os.Stat(sessionPath); err != nil {
		t.Fatalf("expected session file to still exist after failed archive: %v", err)
	}
}

func TestArchiveSession_SessionIDMismatch(t *testing.T) {
	dir := t.TempDir()
	SetTestPaths(dir)
	defer ResetPaths()

	sid := "backend-sid"
	b, err := NewFileSystemBackend(sid)
	if err != nil {
		t.Fatalf("failed to create backend: %v", err)
	}
	defer b.Close()

	// Archive with a mismatched session ID.
	err = b.ArchiveSession("wrong-sid", filepath.Join(dir, "archive.json"))
	if err == nil {
		t.Fatal("expected error for session ID mismatch")
	}
	if got := err.Error(); !contains(got, "session ID mismatch") {
		t.Fatalf("expected session ID mismatch error, got: %v", err)
	}
}

func TestArchiveSession_EmptyDestPath(t *testing.T) {
	dir := t.TempDir()
	SetTestPaths(dir)
	defer ResetPaths()

	sid := "empty-dest"
	b, err := NewFileSystemBackend(sid)
	if err != nil {
		t.Fatalf("failed to create backend: %v", err)
	}
	defer b.Close()

	err = b.ArchiveSession(sid, "")
	if err == nil {
		t.Fatal("expected error for empty destPath")
	}
	if got := err.Error(); !contains(got, "destPath cannot be empty") {
		t.Fatalf("expected 'destPath cannot be empty' error, got: %v", err)
	}
}

func TestArchiveSession_SourceDoesNotExist(t *testing.T) {
	dir := t.TempDir()
	SetTestPaths(dir)
	defer ResetPaths()

	sid := "no-source"
	b, err := NewFileSystemBackend(sid)
	if err != nil {
		t.Fatalf("failed to create backend: %v", err)
	}
	defer b.Close()

	// Don't save any session — source doesn't exist.
	dest := filepath.Join(dir, "archives", "no-source.json")
	err = b.ArchiveSession(sid, dest)
	if err != nil {
		t.Fatalf("expected no-op for missing source, got: %v", err)
	}

	// dest should NOT exist.
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatalf("expected dest to not exist, stat err: %v", err)
	}
}

func TestArchiveSession_SourceGoneButDestExists(t *testing.T) {
	dir := t.TempDir()
	SetTestPaths(dir)
	defer ResetPaths()

	sid := "source-gone-dest-exists"
	b, err := NewFileSystemBackend(sid)
	if err != nil {
		t.Fatalf("failed to create backend: %v", err)
	}
	defer b.Close()

	// Source doesn't exist, but dest already does — conflict.
	dest := filepath.Join(dir, "archives", "conflicting.json")
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(dest, []byte("existing"), 0644); err != nil {
		t.Fatalf("failed to write dest: %v", err)
	}

	err = b.ArchiveSession(sid, dest)
	if !errors.Is(err, os.ErrExist) {
		t.Fatalf("expected os.ErrExist when source missing but dest exists, got: %v", err)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestArchiveSession_StatSourceNonENOENT exercises the "failed to stat session file"
// error path. This happens when os.Stat returns an error that is NOT os.ErrNotExist
// (e.g., ENOTDIR when an intermediate path component is a regular file).
func TestArchiveSession_StatSourceNonENOENT(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("ENOTDIR path trick behaves differently on Windows")
	}
	dir := t.TempDir()
	SetTestPaths(dir)
	defer ResetPaths()

	sid := "stat-src-err"
	b, err := NewFileSystemBackend(sid)
	if err != nil {
		t.Fatalf("failed to create backend: %v", err)
	}
	defer b.Close()

	// Save a session so the backend is initialized.
	sess := &Session{ID: sid, Version: CurrentSchemaVersion, CreateTime: time.Now(), UpdateTime: time.Now()}
	if err := b.SaveSession(sess); err != nil {
		t.Fatalf("failed to save: %v", err)
	}

	// Override sessionDirectory to point to a regular file (triggers ENOTDIR on stat).
	blocker := filepath.Join(t.TempDir(), "blocker-file")
	if err := os.WriteFile(blocker, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	pathsMu.Lock()
	origDir := sessionDirectory
	sessionDirectory = func() (string, error) { return blocker, nil }
	pathsMu.Unlock()
	defer func() {
		pathsMu.Lock()
		sessionDirectory = origDir
		pathsMu.Unlock()
	}()

	err = b.ArchiveSession(sid, filepath.Join(t.TempDir(), "dest.json"))
	if err == nil {
		t.Fatal("expected error for stat source non-ENOENT")
	}
	if !contains(err.Error(), "failed to stat session file") {
		t.Fatalf("expected 'failed to stat session file' error, got: %v", err)
	}
}

// TestArchiveSession_ReadFileError exercises the "failed to read session for archive"
// path. This happens when os.Stat succeeds but os.ReadFile fails (e.g., the session
// path is a directory instead of a regular file).
func TestArchiveSession_ReadFileError(t *testing.T) {
	dir := t.TempDir()
	SetTestPaths(dir)
	defer ResetPaths()

	sid := "readfile-err"
	b, err := NewFileSystemBackend(sid)
	if err != nil {
		t.Fatalf("failed to create backend: %v", err)
	}
	defer b.Close()

	// Save a session, then replace the session file with a directory.
	sess := &Session{ID: sid, Version: CurrentSchemaVersion, CreateTime: time.Now(), UpdateTime: time.Now()}
	if err := b.SaveSession(sess); err != nil {
		t.Fatalf("failed to save: %v", err)
	}

	sessionPath, err := SessionFilePath(sid)
	if err != nil {
		t.Fatalf("SessionFilePath: %v", err)
	}

	// Remove the session file and replace with a directory.
	if err := os.Remove(sessionPath); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if err := os.Mkdir(sessionPath, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	dest := filepath.Join(dir, "dest-readfile.json")
	err = b.ArchiveSession(sid, dest)
	if err == nil {
		t.Fatal("expected error when session path is a directory")
	}
	if !contains(err.Error(), "failed to read session for archive") {
		t.Fatalf("expected 'failed to read session for archive' error, got: %v", err)
	}
}

// TestArchiveSession_StatDestNonENOENT exercises the "failed to stat destination"
// path. This happens when os.Stat on destPath returns an error other than os.ErrNotExist
// (e.g., ENOTDIR when a path component is a regular file).
func TestArchiveSession_StatDestNonENOENT(t *testing.T) {
	dir := t.TempDir()
	SetTestPaths(dir)
	defer ResetPaths()

	sid := "stat-dest-err"
	b, err := NewFileSystemBackend(sid)
	if err != nil {
		t.Fatalf("failed to create backend: %v", err)
	}
	defer b.Close()

	// Save a session so source exists.
	sess := &Session{ID: sid, Version: CurrentSchemaVersion, CreateTime: time.Now(), UpdateTime: time.Now()}
	if err := b.SaveSession(sess); err != nil {
		t.Fatalf("failed to save: %v", err)
	}

	// Use a dest path where an intermediate component is a regular file.
	blocker := filepath.Join(dir, "blocker-dest")
	if err := os.WriteFile(blocker, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(blocker, "subdir", "archive.json")

	err = b.ArchiveSession(sid, dest)
	if err == nil {
		t.Fatal("expected error for stat dest non-ENOENT")
	}
	// Could be either "failed to create archive directory" (from MkdirAll) or
	// "failed to stat destination" depending on which operation fails first.
	// MkdirAll fails first because it tries to create the directory path
	// containing the blocker file.
	errStr := err.Error()
	if !contains(errStr, "failed to stat destination") && !contains(errStr, "failed to create archive directory") {
		t.Fatalf("expected stat/mkdir error, got: %v", err)
	}
}
